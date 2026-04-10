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
		line := scanner.Text()
		for _, w := range strings.Fields(strings.ToLower(line)) {
			counts[w]++
		}
	}

	type pair struct {
		count int
		word  string
	}
	var top10 []pair
	for word, count := range counts {
		if len(top10) < 10 {
			top10 = append(top10, pair{count, word})
		} else {
			for i, p := range top10 {
				if p.count < count {
					top10[i] = pair{count, word}
					break
				}
			}
		}
	}

	sort.Slice(top10, func(i, j int) bool {
		return top10[i].count > top10[j].count
	})

	for _, p := range top10 {
		fmt.Printf("%s: %d\n", p.word, p.count)
	}
}
