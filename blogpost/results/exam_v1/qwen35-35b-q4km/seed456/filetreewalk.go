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

	sort.Slice(files, func(i, j int) bool {
		info1, _ := os.Stat(files[i])
		info2, _ := os.Stat(files[j])
		return info1.Size() > info2.Size()
	})

	for _, f := range files {
		info, _ := os.Stat(f)
		fmt.Printf("%d %s\n", info.Size(), f)
	}
}
