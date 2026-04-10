package main

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
)

type fileInfo struct {
    size int64
    path string
}

func main() {
    if len(os.Args) < 2 {
        return
    }
    
    dir := os.Args[1]
    var files []fileInfo
    
    err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if info.Mode().IsRegular() {
            files = append(files, fileInfo{size: info.Size(), path: path})
        }
        return nil
    })
    
    if err != nil {
        return
    }
    
    sort.Slice(files, func(i, j int) bool {
        return files[i].size > files[j].size
    })
    
    for _, f := range files {
        fmt.Printf("%d %s\n", f.size, f.path)
    }
}
