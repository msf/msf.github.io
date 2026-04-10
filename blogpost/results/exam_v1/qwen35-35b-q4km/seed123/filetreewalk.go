package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: filetreewalk <directory>")
		os.Exit(1)
	}
	dir := os.Args[1]
	var files []struct {
		path string
		size int64
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, struct {
			path string
			size int64
		}{path, info.Size()})
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})
	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, f.path)
	}
}
