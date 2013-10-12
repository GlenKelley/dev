package main

import (
   "fmt"
   "io"
   "os"
   "time"
   "flag"
   "path"
   "os/exec"
   "math/rand"
   "path/filepath"
   "encoding/json"
   "github.com/GlenKelley/dev/git"
)

func main() {
   flags()
   
   groot, err := git.GitRoot()
   panicOnError(err)

   buildDir, err := mkdirRandom()
   panicOnError(err)
   defer os.RemoveAll(buildDir)

   deployDir := path.Join(os.Getenv("HOME"), "Sites")
   c, err := walkDir(groot, buildDir)
   panicOnError(err)
   err = <- c
   panicOnError(err)

   err = os.RemoveAll(deployDir)
   panicOnError(err)
   err = exec.Command("mv", buildDir, deployDir).Run()
   panicOnError(err)
}

func flags() string {
   envPtr := flag.String("env", "local", "environment")
   flag.Parse()
   fmt.Printf("building for environment [%s]\n", *envPtr)
   return *envPtr
}

func mkdirRandom() (string, error) {
    randomName := fmt.Sprintf("%v", rand.Int())
    dirPath := path.Join(os.TempDir(), randomName)
    err := os.Mkdir(dirPath, os.ModeDir | os.ModeTemporary)
    err = os.Chmod(dirPath, 0755)
    return dirPath, err
}

func walkDir(srcDir string, buildDir string) (chan error, error) {
    onFinish := make(chan error)
    run := func (f ProcessFile, path string, info os.FileInfo) {
        err := f(srcDir, buildDir, path, info)
        if err != nil { fmt.Printf("%s : %s\n", path, err) }
        onFinish <- err
    }

    handlers := map[string]ProcessFile {
        ".less": compileLess,
        ".html": copyAndZip,
        ".css": copyAndZip,
        ".js": copyAndZip,
        ".jpg": copyToBuild,
        ".jpeg": copyToBuild,
        ".png": copyToBuild,
        ".gif": copyToBuild,
        ".woff": copyToBuild,
        ".ttf": copyToBuild,
        ".eot": copyToBuild,
        ".otf": copyToBuild,
        ".coffee": compileCoffeeScript,
        ".json": copyAndZip,
        ".svg": copyAndZip,
        ".go": compileGo,
        ".coffeejson": compileCoffeeJson,
        ".fs": copyToBuild,
        ".vs": copyToBuild,
        ".dae": copyToBuild,
        ".DS_Store": ignore,
    }
    ignore := map[string]bool {
       ".git":true,
    }
    
    visited := 0
    err := filepath.Walk(srcDir, func (path string, info os.FileInfo, err error) error {
        if err != nil { 
            fmt.Println(err)
            return nil  
        }
        if ignore[info.Name()] { return filepath.SkipDir }
        if info.IsDir() { return nil }
        ext := filepath.Ext(info.Name())
        handler := handlers[ext]
        if handler != nil {
            visited++;
            go run(handler, path, info)
        } else {
            fmt.Printf("unknown ext %s\n", ext)
        }
        return nil
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

type ProcessFile func(srcDir string, buildDir string, path string, info os.FileInfo) error

func compileGo(srcDir string, buildDir string, path string, info os.FileInfo) error {
    dest, err := replacePathAndExtention(srcDir, buildDir, path, "")
    if err != nil { return err }
    
    dir := filepath.Dir(dest)
    err = MkdirAll(dir)
    if err != nil { return err }
        
    cmd := exec.Command("go", "build", "-o", dest, path)
	stdout, err := cmd.StdoutPipe()
    if err != nil { return err }
	stderr, err := cmd.StderrPipe()
    if err != nil { return err }
    
    err = cmd.Start()
    if err != nil { return err }
    
    go io.Copy(os.Stdout, stdout)
    go io.Copy(os.Stdout, stderr)
    
	err = cmd.Wait()
    if err != nil { return err }
    
    err = setFileTimestamp(dest, info.ModTime())
    if err != nil { return err }

    err = os.Chmod(dest, 0755)
    if err != nil { return err }

    return nil
}

func ignore(srcDir string, buildDir string, path string, info os.FileInfo) error {
    return nil
}

func compileLess(srcDir string, buildDir string, path string, info os.FileInfo) error {
    base := filepath.Base(path)
    isChild, err := filepath.Match("_*", base)
    if err != nil { return err }
    if (!isChild) {
        dest, err := replacePathAndExtention(srcDir, buildDir, path, ".css")
        if err != nil { return err }
        
        dir := filepath.Dir(dest)
        err = MkdirAll(dir)
        if err != nil { return err }
        
        cmd := exec.Command("lessc", path)
        err = pipeCommandToFile(cmd, dest)
        if err != nil { return err }
    
        err = gZipFile(dest)
        if err != nil { return err }

        err = setFileTimestamp(dest, info.ModTime())
        if err != nil { return err }

        err = os.Chmod(dest, 0755)
        if err != nil { return err }
    }
    return nil
}

func writeJson(content map[string]interface{}, buildDir string, path string) error {
    dest := filepath.Join(buildDir, path)
    
    file, err := os.Create(dest)
    if err != nil { return err }
    defer file.Close()
    
    encoder := json.NewEncoder(file)
    err = encoder.Encode(content)
    if err != nil { return err }
    file.Close()

    err = gZipFile(dest)
    if err != nil { return err }

    err = os.Chmod(dest, 0755)
    if err != nil { return err }
    
    return nil
}

func compileCoffeeScript(srcDir string, buildDir string, path string, info os.FileInfo) error {
    dest, err := replacePathAndExtention(srcDir, buildDir, path, ".js")
    if err != nil { return err }
    
    dir := filepath.Dir(dest)
    err = MkdirAll(dir)
    if err != nil { return err }

    cmd := exec.Command("coffee", "-p", path)
    err = pipeCommandToFile(cmd, dest)
    if err != nil { return err }

    err = gZipFile(dest)
    if err != nil { return err }

    err = setFileTimestamp(dest, info.ModTime())
    if err != nil { return err }

    err = os.Chmod(dest, 0755)
    if err != nil { return err }
    
    return nil
}

func compileCoffeeJson(srcDir string, buildDir string, path string, info os.FileInfo) error {
    srcDir = filepath.Join(srcDir, "coffee")
    buildDir = filepath.Join(buildDir, "js")
    
    dest, err := replacePathAndExtention(srcDir, buildDir, path, ".json")
    if err != nil { return err }
    
    dir := filepath.Dir(dest)
    err = MkdirAll(dir)
    if err != nil { return err }

    cmd := exec.Command("coffee", "-p", "-b", path)
    err = pipeCommandToFile(cmd, dest)
    if err != nil { return err }

    cmd = exec.Command("perl", "-pi", "-e", "s/;|\\(|\\)//g", dest)
    err = cmd.Run()
    if err != nil { return err }
    
    cmd = exec.Command("perl", "-pi", "-e", "s/(\\w+):/\"$1\":/g", dest)
    err = cmd.Run()
    if err != nil { return err }

    err = gZipFile(dest)
    if err != nil { return err }

    err = setFileTimestamp(dest, info.ModTime())
    if err != nil { return err }

    err = os.Chmod(dest, 0755)
    if err != nil { return err }
    
    return nil
}

func copyToBuild(srcDir string, buildDir string, path string, info os.FileInfo) error {
    dest, err := copy(srcDir, buildDir, path, info)
    if err != nil { return err }
    
    err = setFileTimestamp(dest, info.ModTime())
    if err != nil { return err }

    err = os.Chmod(dest, 0755)
    if err != nil { return err }
    
    return nil
}

func copyAndZip(srcDir string, buildDir string, path string, info os.FileInfo) error {
    dest, err:= copy(srcDir, buildDir, path, info)
    if err != nil { return err }
    err = gZipFile(dest)
    if err != nil { return err }
    
    err = setFileTimestamp(dest, info.ModTime())
    if err != nil { return err }

    err = os.Chmod(dest, 0755)
    if err != nil { return err }
    
    return nil
}

func pipeCommandToFile(cmd *exec.Cmd, path string) error {
    file, err := os.Create(path)
    if err != nil { return err }
    defer file.Close()
    stdout, err := cmd.StdoutPipe()
    if err != nil { return err }
	stderr, err := cmd.StderrPipe()
    if err != nil { return err }
    
    err = cmd.Start()
    if err != nil { return err }
    
    go io.Copy(os.Stdout, stderr)
    
    io.Copy(file, stdout)
    return cmd.Wait()
}

func replaceBasePath(srcDir string, buildDir string, path string) (string, error) {
    relativePath, err := filepath.Rel(srcDir, path)
    if err != nil { return "", err }
    return filepath.Join(buildDir, relativePath), nil
}

func replaceExtention(path string, ext string) string {
    dir := filepath.Dir(path)
    name := nameWithoutExt(path)
    return filepath.Join(dir, name + ext)
}

func replacePathAndExtention(srcDir string, buildDir string, path string, ext string) (string, error) {
    path, err := replaceBasePath(srcDir, buildDir, path)
    if err != nil { return "", err }
    
    return replaceExtention(path, ext), nil
}

func copy(srcDir string, buildDir string, path string, info os.FileInfo) (string, error) {
    relativePath, err := filepath.Rel(srcDir, path)
    if err != nil { return "", err }
    
    dest := filepath.Join(buildDir, relativePath)
    dir := filepath.Dir(dest)
    err = MkdirAll(dir)
    if err != nil { return "", err }
    
    cmd := exec.Command("cp", path, dest)
    return dest, cmd.Run()
}

func MkdirAll(path string) error {
    return os.MkdirAll(path, 0755)
}

func nameWithoutExt(path string) string {
    filename := filepath.Base(path)
    ext := filepath.Ext(path)
    return filename[0:len(filename)-len(ext)]
}

func minifyJs(jarDir, src, dest string) error {
    jarPath := filepath.Join(jarDir, "compiler.jar")
    cmd := exec.Command("java", "-jar", jarPath, "--js", src, "--js_output_file", dest, "--compilation_level", "SIMPLE_OPTIMIZATIONS")
    return cmd.Run()
}

func gZipFile(path string) error {
    err := exec.Command("gzip", "-n", path).Run()
    if err != nil { return err }
    return os.Rename(path + ".gz", path)
}

func setFileTimestamp(path string, time time.Time) error {
    return os.Chtimes(path, time, time)
}

func panicOnError (err error) {
    if err != nil { panic(err) }
}
