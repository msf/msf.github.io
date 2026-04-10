package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	counts := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		words := strings.Fields(strings.ToLower(scanner.Text()))
		for _, w := range words {
			counts[w]++
		}
	}
	type entry struct {
		word  string
		count int
	}
	var entries []entry
	for word, count := range counts {
		entries = append(entries, entry{word, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].word < entries[j].word
	})
	for i := 0; i < 10 && i < len(entries); i++ {
		fmt.Printf("%s: %d\n", entries[i].word, entries[i].count)
	}
}
