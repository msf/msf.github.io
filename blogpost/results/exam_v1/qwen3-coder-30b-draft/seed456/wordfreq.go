package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	words := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		for _, word := range strings.Fields(line) {
			words[strings.ToLower(word)]++
		}
	}
	
	type kv struct {
		Key   string
		Value int
	}
	var ss []kv
	for k, v := range words {
		ss = append(ss, kv{k, v})
	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})
	
	for i := 0; i < len(ss) && i < 10; i++ {
		fmt.Printf("%s: %d\n", ss[i].Key, ss[i].Value)
	}
}
