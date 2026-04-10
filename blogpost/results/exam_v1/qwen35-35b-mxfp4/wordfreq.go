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
		line := strings.ToLower(scanner.Text())
		words := strings.Fields(line)
		for _, word := range words {
			if word != "" {
				counts[word]++
			}
		}
	}

	type wordCount struct {
		word  string
		count int
	}

	var results []wordCount
	for word, count := range counts {
		results = append(results, wordCount{word, count})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].count == results[j].count {
			return results[i].word < results[j].word
		}
		return results[i].count > results[j].count
	})

	if len(results) > 10 {
		results = results[:10]
	}

	for _, wc := range results {
		fmt.Printf("%s: %d\n", wc.word, wc.count)
	}
}
