package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	counts := make(map[string]int)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		words := strings.Fields(strings.ToLower(line))
		for _, word := range words {
			counts[word]++
		}
	}

	type kv struct {
		Key   string
		Value int
	}

	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	limit := len(sorted)
	if limit > 10 {
		limit = 10
	}

	for i := 0; i < limit; i++ {
		fmt.Printf("%s: %d\n", sorted[i].Key, sorted[i].Value)
	}
}
