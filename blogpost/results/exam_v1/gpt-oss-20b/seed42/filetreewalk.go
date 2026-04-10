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
	type entry struct{ sz int64; path string }
	var entries []entry
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			entries = append(entries, entry{info.Size(), p})
		}
		return nil
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sz > entries[j].sz
	})
	for _, e := range entries {
		fmt.Printf("%d %s\n", e.sz, e.path)
	}
}
