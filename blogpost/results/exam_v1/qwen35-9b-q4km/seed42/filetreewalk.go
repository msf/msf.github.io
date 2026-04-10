package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		return
	}
	dir := os.Args[1]

	var files []struct {
		name string
		size int64
	}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		files = append(files, struct {
			name string
			size int64
		}{info.Name(), info.Size()})
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, filepath.Join(dir, f.name))
	}
}
