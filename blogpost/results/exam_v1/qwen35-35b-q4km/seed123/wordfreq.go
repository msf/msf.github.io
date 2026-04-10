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
		for _, w := range words {
			w = strings.Trim(w, ".,!?;:\"'()[]{}")
			if w != "" {
				counts[w]++
			}
		}
	}
	type pair struct {
		word  string
		count int
	}
	var pairs []pair
	for w, c := range counts {
		pairs = append(pairs, pair{w, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].word < pairs[j].word
	})
	if len(pairs) > 10 {
		pairs = pairs[:10]
	}
	for _, p := range pairs {
		fmt.Printf("%s: %d\n", p.word, p.count)
	}
}
