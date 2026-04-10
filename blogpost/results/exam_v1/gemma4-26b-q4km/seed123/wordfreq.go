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
		words := strings.Fields(scanner.Text())
		for _, w := range words {
			w = strings.ToLower(strings.Trim(w, ".,!?;:\"()"))
			if w != "" {
				counts[w]++
			}
		}
	}

	type pair struct {
		w string
		c int
	}
	var sorted []pair
	for k, v := range counts {
		sorted = append(sorted, pair{k, v})
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].c == sorted[j].c {
			return sorted[i].w < sorted[j].w
		}
		return sorted[i].c > sorted[j].c
	})

	for i := 0; i < len(sorted) && i < 10; i++ {
		fmt.Printf("%s: %d\n", sorted[i].w, sorted[i].c)
	}
}
