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
	freq := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		for _, field := range strings.Fields(line) {
			word := strings.ToLower(field)
			freq[word]++
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	pairs := make([]pair, 0, len(freq))
	for w, c := range freq {
		pairs = append(pairs, pair{w, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].word < pairs[j].word
		}
		return pairs[i].count > pairs[j].count
	})
	top := 10
	if len(pairs) < top {
		top = len(pairs)
	}
	for i := 0; i < top; i++ {
		fmt.Printf("%s: %d\n", pairs[i].word, pairs[i].count)
	}
}
