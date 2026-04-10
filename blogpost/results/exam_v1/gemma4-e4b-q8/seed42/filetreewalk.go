package main

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"path/filepath"
)

type fileInfo struct {
	size int64
	path string
}

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}
	rootDir := os.Args[1]

	var files []fileInfo

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
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
		fmt.Fprintln(os.Stderr, "Error walking directory:", err)
		os.Exit(1)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, f.path)
	}
}
