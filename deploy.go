package main

import (
   "os"
   "fmt"
   "flag"
   "strings"
   "os/exec"
   "path"
   "path/filepath"
   "github.com/GlenKelley/dev/s3"
)

func main() {
    concurrent := flag.Bool("c", true, "run file uploads concurrently")
    bucket := flag.String("bucket", "akusete.com", "s3 destination bucket")
    flag.Parse()
    
    buildDir := path.Join(os.Getenv("HOME"), "Sites")
    c, err := walkDir(buildDir, *bucket, *concurrent)
    panicOnError(err)
    err = <- c
    panicOnError(err)
}

func walkDir(buildDir string, bucket string, concurrent bool) (chan error, error) {
    onFinish := make(chan error)
    run := func (path string, info os.FileInfo) error {
        err := error(nil)
        relativePath, err := filepath.Rel(buildDir, path)
        if err != nil { return err }
        uploadInfo, err := GetUploadInfo(path, relativePath, info)
        if err != nil { return err }
        s3info, err := s3.GetS3Info(bucket, uploadInfo.ItemPath)
        if err != nil { return err }

        if NeedsUpdate(s3info, uploadInfo)  {
            fmt.Println(path)
            err = s3.UploadToS3(path, bucket, uploadInfo)
        }
        return err
    }
    visited := 0
    err := filepath.Walk(buildDir, func (path string, info os.FileInfo, err error) error {
        if err != nil { return err }
        if info.IsDir() { 
            return nil 
        }
        if (concurrent) {
            visited++
            go func() { onFinish <- run(path, info) }()
            err = nil
        } else {
            err = run(path, info)
        }
        return err
    })
    if err != nil {
        return nil, err
    }
    finished := make(chan error)
    go func(v int) {
        fail := error(nil)
        for ; v > 0; v-- {
            e := <- onFinish
            if e != nil {
               fail = e 
            }
        }
        finished <- fail
    }(visited)
    return finished, nil
}

func panicOnError (err error) {
    if err != nil { panic(err) }
}

func NeedsUpdate(s3info s3.S3Info, uploadInfo s3.S3UploadInfo) bool {
    hashDiff := uploadInfo.MD5 != s3info.MD5 
    if hashDiff {
        fmt.Printf("Hash Diff local[%s] != s3[%s]\n", uploadInfo.MD5, s3info.MD5)
    }
    sizeDiff := uploadInfo.ContentLength != s3info.Size
    if sizeDiff {
        fmt.Printf("Size Diff local[%d] != s3[%d]\n", uploadInfo.ContentLength, s3info.Size)
    }
    // dateDiff := uploadInfo.ModTime.After(s3info.ModTime)
    // if dateDiff {
    //     fmt.Printf("Date Diff local[%s] != s3[%s]\n", uploadInfo.ModTime, s3info.ModTime)
    // }
    typeDiff := (uploadInfo.ContentType != "") && (uploadInfo.ContentType != s3info.ContentType)
    if typeDiff {
        fmt.Printf("Type Diff local[%s] != s3[%s]\n", uploadInfo.ContentType, s3info.ContentType)
    }
    return hashDiff || sizeDiff || typeDiff
}

func GetUploadInfo(path string, relativePath string, info os.FileInfo) (s3.S3UploadInfo, error) {
    
    encodings := map[string]string {
        ".html": "gzip",
        ".css": "gzip",
        ".js": "gzip",
        ".json": "gzip",
        ".svg": "gzip",
        ".jpg": "",
        ".png": "",
        ".gif": "",
    }
    contentTypes := map[string]string {
        ".html": "text/html; charset=UTF-8",
        ".css": "text/css; charset=UTF-8",
        ".js": "application/x-javascript; charset=UTF-8",
        ".jpg": "image/jpeg",
        ".png": "image/png",
        ".svg": "image/svg+xml",
        ".json": "application/json",
        ".go": "binary/octet-stream",
    }
    
    ext := filepath.Ext(path)
    uploadInfo := s3.S3UploadInfo{}    
    uploadInfo.Encoding = encodings[ext]
    uploadInfo.ContentType = contentTypes[ext]
    uploadInfo.Public = true
    uploadInfo.ContentLength = info.Size()
    uploadInfo.ModTime = info.ModTime()
    md5, err := GetFileMD5(path)
    uploadInfo.MD5 = md5

    if (ext == ".html") {
        relativePath = relativePath[0:len(relativePath)-len(ext)]
    }
    uploadInfo.ItemPath = relativePath
    
    return uploadInfo, err
}


func GetFileMD5(path string) (string, error) {
    // file, err := os.Open(path)
    // if err != nil { return "", err }
    // defer file .Close()
    // hash := md5.New()
    // io.Copy(hash, file)
    // signature := make([]byte, 0, hash.Size())
    // md51 := hex.EncodeToString(hash.Sum(signature))
    bytes, err := exec.Command("md5", "-q", path).Output()
    if err != nil { return "", nil } 
    md5 := strings.TrimSpace(string(bytes))
    return md5, nil
}
