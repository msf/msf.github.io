package main

import (
	"fmt"
	"os"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: filetreewalk <directory>")
		os.Exit(1)
	}
	root := os.Args[1]
	var files []struct {
		Path  string
		Size  int64
	}
	err := walkDir(root, func(path string, info os.FileInfo) error {
		if !info.IsDir() && info.Mode().IsRegular() {
			files = append(files, struct {
				Path string
				Size int64
			}{path, info.Size()})
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Size > files[j].Size
	})
	for _, f := range files {
		fmt.Printf("%d %s\n", f.Size, f.Path)
	}
}

func walkDir(root string, fn func(string, os.FileInfo) error) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := root
		if path != "" && path != "." {
			path = path + "/" + entry.Name()
		} else {
			path = entry.Name()
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if err := fn(path, info); err != nil {
			if !entry.IsDir() {
				continue
			}
			if err := walkDir(path, fn); err != nil {
				return err
			}
		}
	}
	return nil
}
