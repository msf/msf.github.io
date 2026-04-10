package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

type pair struct {
	word  string
	count int
}

func main() {
	counts := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		words := strings.Fields(scanner.Text())
		for _, w := range words {
			w = strings.ToLower(w)
			w = strings.Trim(w, ".,!?;:()\"'")
			if w != "" {
				counts[w]++
			}
		}
	}

	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, pair{k, v})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].word < pairs[j].word
		}
		return pairs[i].count > pairs[j].count
	})

	for i := 0; i < len(pairs) && i < 10; i++ {
		fmt.Printf("%s: %d\n", pairs[i].word, pairs[i].count)
	}
}
