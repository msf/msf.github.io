package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type pair struct {
	word  string
	count int
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	counts := make(map[string]int)
	re := regexp.MustCompile(`[a-zA-Z0-9]+`)

	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		words := re.FindAllString(line, -1)
		for _, w := range words {
			counts[w]++
		}
	}

	var pairs []pair
	for w, c := range counts {
		pairs = append(pairs, pair{w, c})
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
