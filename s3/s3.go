package s3

import "fmt"
import "io/ioutil"
import "net/http"
import "path/filepath"
import "os"
import "regexp"
import "strings"
import "strconv"
import "time"
import "errors"
import "crypto/sha1"
import "crypto/hmac"
import "encoding/base64"
import "bytes"
import "sort"

type S3Info struct {
    Size int64
    ModTime time.Time 
    ContentType string
    MD5 string
    URL string
    RedirectURL string
}

type S3UploadInfo struct {
    Encoding string
    ContentType string
    Public bool
    ContentLength int64
    ModTime time.Time
    MD5 string
    ItemPath string
}

type S3Credentials struct {
    Key string
    Secret []byte
}

var (
    fileSizePattern = regexp.MustCompile("File size:\\s*(\\d+)")
    modTimePattern = regexp.MustCompile("Last mod:\\s*(.*)")
    mimeTypePattern = regexp.MustCompile("MIME type:\\s*(\\S+)")
    md5Pattern = regexp.MustCompile("MD5 sum:\\s*(\\w+)")
    aclPattern = regexp.MustCompile("ACL:\\s*(\\S+):(\\w+)")
    urlPattern = regexp.MustCompile("URL:\\s*(\\S+)")
    awsKeyPattern = regexp.MustCompile("aws_access_key_id\\s*=\\s*(\\S+)")
    awsSecretPattern = regexp.MustCompile("aws_secret_access_key\\s*=\\s*(\\S+)")
    s3TimeFormat = "Mon, 02 Jan 2006 15:04:05 -0700"
    awsHost = ".s3.amazonaws.com"
)

func GetCredentials() (S3Credentials, error) {
    awsConfigFile := os.Getenv("AWS_CONFIG_FILE")
    bytes, err := ioutil.ReadFile(awsConfigFile)
    if err != nil { return S3Credentials{}, err }
    
    keyMatch := awsKeyPattern.FindSubmatch(bytes)
    secretMatch := awsSecretPattern.FindSubmatch(bytes)
    if keyMatch != nil && secretMatch != nil {
        key := string(keyMatch[1])
        secret := secretMatch[1]
        return S3Credentials{key, secret}, nil
    }
    return S3Credentials{}, errors.New("invalid aws credentials file")
}

func GetS3Info(bucket string, s3ItemPath string) (S3Info, error) {
    s3Path := "http://" + filepath.Join(bucket + awsHost, s3ItemPath)
    s3Info := S3Info{}
    
    request, err := http.NewRequest("HEAD", s3Path, nil)
    if err != nil { return s3Info, err }
    
    client := &http.Client{}
    response, err := client.Do(request)
    if err != nil { return s3Info, err }
    if response.StatusCode == 200 {
        length := response.Header.Get("Content-Length")
        if length != "" {
            size, err := strconv.Atoi(length)
            if err != nil { return s3Info, err }
            s3Info.Size = int64(size)
        }

        dateString := response.Header.Get("x-amz-meta-modifiedtime")
        if dateString != "" {
            s3Info.ModTime, err = time.Parse(s3TimeFormat, dateString)
            if err != nil { return s3Info, err }
        }
        s3Info.ContentType = response.Header.Get("Content-Type")
        s3Info.MD5 = response.Header.Get("x-amz-meta-md5")
        s3Info.URL = s3Path
    
        s3Info.RedirectURL = response.Header.Get("x-amz-website-redirect-location")
    }
    
    return s3Info, nil
}

func signRequest(request *http.Request, credentials S3Credentials) error {
    var buffer bytes.Buffer
    buffer.WriteString(request.Method)
    buffer.WriteString("\n")
    buffer.WriteString(request.Header.Get("Content-Md5"))
    buffer.WriteString("\n")
    buffer.WriteString(request.Header.Get("Content-Type"))
    buffer.WriteString("\n")
    buffer.WriteString(request.Header.Get("Date"))
    buffer.WriteString("\n")

    var awsKeys = make([]string, 0, len(request.Header))
    for key, _ := range request.Header {
        lkey := strings.ToLower(key)
        if strings.HasPrefix(lkey, "x-amz-") {
             awsKeys = append(awsKeys, key)
        }
    }
    sort.Strings(awsKeys)
    for _, key := range awsKeys {
        lkey := strings.ToLower(key)
        buffer.WriteString(lkey)
        buffer.WriteString(":")
        buffer.WriteString(strings.Join(request.Header[key], ","))
        buffer.WriteString("\n")
    }
    
    host := request.URL.Host
    if strings.HasSuffix(host, awsHost) {
        i := strings.Index(host,awsHost)
        canonicalizedResource := "/" + host[:i] + request.URL.Path
        buffer.WriteString(canonicalizedResource)
    }
    
    hash := hmac.New(sha1.New, credentials.Secret)
    hash.Write(buffer.Bytes())
    signature := make([]byte, 0, hash.Size())
    signature = hash.Sum(signature)
    encodedSignature := base64.StdEncoding.EncodeToString(signature)

    request.Header.Add("Authorization", "AWS " + credentials.Key + ":" + encodedSignature)
    return nil
}

func UploadToS3(path string, bucket string, info S3UploadInfo) error {
    s3Path := "http://" + filepath.Join(bucket + awsHost, info.ItemPath)
        
    bs, err := ioutil.ReadFile(path)
    if err != nil { return err }
    body := strings.NewReader(string(bs))
    
    t := time.Now()
    fmt.Printf("Uploading %s\n", s3Path)
    request, err := http.NewRequest("PUT", s3Path, body)
    if err != nil { return err }
    d := time.Since(t)
    fmt.Printf("Upload %s took %s\n", s3Path, d)

    // request.Header.Add("Cache-Control", "public;max-age=60")
    
    length := strconv.FormatInt(info.ContentLength, 10)
    request.Header.Add("Content-Length", length)
    date := time.Now().Format(s3TimeFormat)
    request.Header.Add("Date", date)

    modifiedTime := info.ModTime.Format(s3TimeFormat)
    request.Header.Add("x-amz-meta-modifiedtime", modifiedTime)
    
    // request.Header.Add("Content-Md5", info.MD5)
    request.Header.Add("x-amz-meta-md5", info.MD5)
    
    if info.ContentType != "" {
        request.Header.Add("Content-Type", info.ContentType)
    }
    
    if (info.Encoding != "") {
        request.Header.Add("Content-Encoding", info.Encoding)
    }
    
    if info.Public {
        request.Header.Add("x-amz-acl", "public-read")
    } else {
        request.Header.Add("x-amz-acl", "bucket-owner-full-control")
    }
    
    credentials, err := GetCredentials()
    if err != nil { return err }
    err = signRequest(request, credentials)
    if err != nil { return err }
    client := &http.Client{}
    response, err := client.Do(request)
    if err != nil { return err }
    defer response.Body.Close()
    
    return nil
}
