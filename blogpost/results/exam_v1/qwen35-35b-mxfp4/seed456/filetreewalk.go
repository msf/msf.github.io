package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type fileInfo struct {
	name string
	size int64
}

func main() {
	if len(os.Args) < 2 {
		return
	}

	root := os.Args[1]
	var files []fileInfo

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		if info.Mode().IsRegular() {
			files = append(files, fileInfo{path, info.Size()})
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, f.name)
	}
}
