package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

type wordCount struct {
	word  string
	count int
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	counts := make(map[string]int)
	for scanner.Scan() {
		words := strings.Fields(scanner.Text())
		for _, w := range words {
			low := strings.ToLower(w)
			counts[low]++
		}
	}
	items := make([]wordCount, 0, len(counts))
	for w, c := range counts {
		items = append(items, wordCount{w, c})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].word < items[j].word
		}
		return items[i].count > items[j].count
	})
	top := 10
	if len(items) < top {
		top = len(items)
	}
	for i := 0; i < top; i++ {
		fmt.Printf("%s: %d\n", items[i].word, items[i].count)
	}
}
