package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	wordCounts := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		words := strings.Fields(line)
		for _, word := range words {
			cleanWord := strings.ToLower(strings.TrimFunc(word, func(r rune) bool {
				return r == ',' || r == '.' || r == ';' || r == ':'
			}))
			if cleanWord != "" {
				wordCounts[cleanWord]++
			}
		}
	}

	type wordCount struct {
		word  string
		count int
	}

	var counts []wordCount
	for w, c := range wordCounts {
		counts = append(counts, wordCount{w, c})
	}

	// Simple bubble sort implementation for demonstration brevity
	for i := 0; i < len(counts); i++ {
		for j := i + 1; j < len(counts); j++ {
			if counts[i].count < counts[j].count {
				counts[i], counts[j] = counts[j], counts[i]
			}
		}
	}

	limit := 10
	if len(counts) < limit {
		limit = len(counts)
	}

	for i := 0; i < limit; i++ {
		fmt.Printf("%s: %d\n", counts[i].word, counts[i].count)
	}
}
