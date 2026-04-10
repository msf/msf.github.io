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
		fmt.Fprintln(os.Stderr, "Usage: filetreewalk <directory>")
		return
	}
	root := os.Args[1]
	var files []fileInfo
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
		}
		files = append(files, fileInfo{size: info.Size(), path: path})
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})
	for _, fi := range files {
		fmt.Printf("%d %s\n", fi.size, fi.path)
	}
}
