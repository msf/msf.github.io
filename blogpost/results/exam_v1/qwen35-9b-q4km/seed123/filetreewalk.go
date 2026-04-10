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
	root := os.Args[1]
	info, err := os.Stat(root)
	if err != nil {
		return
	}
	if !info.IsDir() {
		return
	}

	var files []struct {
		name string
		size int64
	}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, struct {
			name string
			size int64
		}{info.Name(), info.Size()})
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		if files[i].size != files[j].size {
			return files[i].size > files[j].size
		}
		return files[i].name < files[j].name
	})

	for _, f := range files {
		fmt.Printf("%d %s\n", f.size, filepath.Join(root, f.name))
	}
}
