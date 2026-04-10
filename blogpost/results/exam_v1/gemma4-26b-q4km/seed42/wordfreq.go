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
	scanner := bufio.NewScanner(os.Stdin)
	re := regexp.MustCompile(`[a-zA-Z0-9]+`)

	for scanner.Scan() {
		words := re.FindAllString(strings.ToLower(scanner.Text()), -1)
		for _, w := range words {
			counts[w]++
		}
	}

	var pairs []pair
	for w, c := range counts {
		pairs = append(pairs, pair{w, c})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].c == pairs[j].c {
			return pairs[i].w < pairs[j].w
		}
		return pairs[i].c > pairs[j].c
	})

	for i := 0; i < len(pairs) && i < 10; i++ {
		fmt.Printf("%s: %d\n", pairs[i].w, pairs[i].c)
	}
}
