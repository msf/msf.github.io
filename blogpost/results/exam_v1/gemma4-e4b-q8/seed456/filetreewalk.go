package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type FileInfo struct {
	Size int64
	Path string
}

type BySizeDesc []FileInfo

func (a BySizeDesc) Len() int           { return len(a) }
func (a BySizeDesc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a BySizeDesc) Less(i, j int) bool { return a[i].Size > a[j].Size }

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}
	rootDir := os.Args[1]

	var files []FileInfo

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, FileInfo{Size: info.Size(), Path: path})
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	sort.Sort(BySizeDesc(files))

	for _, f := range files {
		fmt.Printf("%d %s\n", f.Size, f.Path)
	}
}
