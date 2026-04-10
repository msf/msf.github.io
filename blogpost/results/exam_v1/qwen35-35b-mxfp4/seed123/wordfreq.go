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
		line := strings.ToLower(scanner.Text())
		words := strings.Fields(line)
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

	for i := 0; i < len(sorted) && i < 10; i++ {
		fmt.Printf("%s: %d\n", sorted[i].Key, sorted[i].Value)
	}
}
