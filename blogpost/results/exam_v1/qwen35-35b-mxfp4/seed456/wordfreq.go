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
		line := scanner.Text()
		words := strings.Fields(line)
		for _, w := range words {
			w = strings.ToLower(w)
			if w != "" {
				counts[w]++
			}
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
		if sorted[i].Value == sorted[j].Value {
			return sorted[i].Key < sorted[j].Key
		}
		return sorted[i].Value > sorted[j].Value
	})

	if len(sorted) > 10 {
		sorted = sorted[:10]
	}

	for _, item := range sorted {
		fmt.Printf("%s: %d\n", item.Key, item.Value)
	}
}
