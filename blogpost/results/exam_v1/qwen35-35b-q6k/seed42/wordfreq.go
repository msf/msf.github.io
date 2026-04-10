package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	counts := make(map[string]int)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		words := strings.Fields(line)
		for _, w := range words {
			if w == "" {
				continue
			}
			counts[w]++
		}
	}
	if err := scanner.Err(); err != nil {
		return
	}

	type pair struct {
		word  string
		count int
	}
	var pairs []pair
	for word, count := range counts {
		pairs = append(pairs, pair{word, count})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].word < pairs[j].word
	})

	limit := len(pairs)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("%s: %d\n", pairs[i].word, pairs[i].count)
	}
}
