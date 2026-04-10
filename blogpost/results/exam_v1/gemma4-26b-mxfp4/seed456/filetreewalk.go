package main

import (
	"fmt"
	"io/fs"
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
	root := os.Args[1]
	var files []fileInfo

	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil && info.Mode().IsRegular() {
				files = append(files, fileInfo{info.Size(), path})
			}
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, f.path)
	}
}
