package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: filetreewalk <dir>")
		os.Exit(1)
	}
	startPath := os.Args[1]
	var files []struct {
		name string
		size int64
	}
	err := filepath.Walk(startPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, struct {
				name string
				size int64
			}{path, info.Size()})
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].size != files[j].size {
			return files[i].size > files[j].size
		}
		return files[i].name < files[j].name
	})
	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, f.name)
	}
}
