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
	
	var files []struct {
		path string
		size int64
	}
	
	filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, struct {
				path string
				size int64
			}{path, info.Size()})
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
