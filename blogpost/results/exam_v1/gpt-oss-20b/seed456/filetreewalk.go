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
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Mode().IsRegular() {
			files = append(files, fileInfo{info.Size(), path})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})
	for _, fi := range files {
		fmt.Printf("%d %s\n", fi.size, fi.path)
	}
}
