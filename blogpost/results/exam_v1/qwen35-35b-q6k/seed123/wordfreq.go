package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	words := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		for _, w := range strings.Fields(line) {
			w = strings.Trim(w, ".,!?;:\"'()[]{}")
			if w != "" {
				words[w]++
			}
		}
	}

	type pair struct {
		word  string
		count int
	}
	var pairs []pair
	for w, c := range words {
		pairs = append(pairs, pair{w, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].word < pairs[j].word
	})

	for i := 0; i < len(pairs) && i < 10; i++ {
		fmt.Printf("%s: %d\n", pairs[i].word, pairs[i].count)
	}
}
