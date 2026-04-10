package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: filetreewalk <directory>")
		os.Exit(1)
	}

	var files []struct {
		size int64
		path string
	}

	err := filepath.WalkDir(os.Args[1], func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, _ := d.Info()
			files = append(files, struct {
				size int64
				path string
			}{info.Size(), path})
		}
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
