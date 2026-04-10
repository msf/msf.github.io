package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type WordCount struct {
	Word  string
	Count int
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	freqs := make(map[string]int)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		words := strings.Fields(line)
		for _, w := range words {
			freqs[w]++
		}
	}

	type countPair struct {
		word  string
		count int
	}
	var items []countPair
	for w, c := range freqs {
		items = append(items, countPair{w, c})
	}

	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].count > items[i].count || (items[j].count == items[i].count && items[j].word < items[i].word) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	if len(items) > 10 {
		items = items[:10]
	}
	for _, item := range items {
		fmt.Printf("%s: %d\n", item.word, item.count)
	}
}
