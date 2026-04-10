package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: filetreewalk <directory>")
		os.Exit(1)
	}
	dir := os.Args[1]

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	type fileInfo struct {
		path string
		size int64
	}
	var data []fileInfo
	for _, p := range files {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		data = append(data, fileInfo{p, info.Size()})
	}

	sort.Slice(data, func(i, j int) bool {
		return data[i].size > data[j].size
	})

	for _, f := range data {
		fmt.Printf("%d %s\n", f.size, f.path)
	}
}
