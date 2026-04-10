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
	w string
	c int
}

func main() {
	counts := make(map[string]int)
	re := regexp.MustCompile(`[a-zA-Z0-9]+`)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		words := re.FindAllString(strings.ToLower(scanner.Text()), -1)
		for _, w := range words {
			counts[w]++
		}
	}

	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, pair{k, v})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].c == pairs[j].c {
			return pairs[i].w < pairs[j].w
		}
		return pairs[i].c > pairs[j].c
	})

	limit := 10
	if len(pairs) < 10 {
		limit = len(pairs)
	}

	for i := 0; i < limit; i++ {
		fmt.Printf("%s: %d\n", pairs[i].w, pairs[i].c)
	}
}
