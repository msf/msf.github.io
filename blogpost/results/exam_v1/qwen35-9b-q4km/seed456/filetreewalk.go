package main

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"path/filepath"
)

func walkDir(path string, files []string, sizes map[string]uint64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	infos, err := f.Readdir(-1)
	if err != nil {
		return err
	}
	for _, info := range infos {
		p := filepath.Join(path, info.Name())
		if info.IsDir() {
			if err := walkDir(p, files, sizes); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			files = append(files, p)
			s, err := os.Stat(p)
			if err == nil {
				sizes[p] = s.Size()
			}
		}
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	var files []string
	var sizes map[string]uint64
	if err := walkDir(os.Args[1], files, sizes); err != nil {
		fmt.Println(err)
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return sizes[files[i]] > sizes[files[j]]
	})
	for _, f := range files {
		s := sizes[f]
		fmt.Printf("%d %s\n", s, f)
	}
}
