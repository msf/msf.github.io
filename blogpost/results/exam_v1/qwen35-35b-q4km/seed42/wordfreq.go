package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	wordCounts := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		words := strings.Fields(line)
		for _, word := range words {
			wordsOnly := strings.Trim(word, ".,!?;:\"'()-")
			if wordsOnly != "" {
				wordCounts[wordsOnly]++
			}
		}
	}
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range wordCounts {
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
	for _, kv := range sorted {
		fmt.Printf("%s: %d\n", kv.Key, kv.Value)
	}
}
