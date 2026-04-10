package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

type WordFreq struct {
	Word  string
	Count int
}

type ByCountDesc []WordFreq

func (a ByCountDesc) Len() int           { return len(a) }
func (a ByCountDesc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByCountDesc) Less(i, j int) bool { return a[i].Count > a[j].Count }

func main() {
	counts := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		for _, field := range fields {
			word := strings.ToLower(field)
			counts[word]++
		}
	}

	var sortedWords []WordFreq
	for word, count := range counts {
		sortedWords = append(sortedWords, WordFreq{Word: word, Count: count})
	}

	sort.Sort(ByCountDesc(sortedWords))

	limit := 10
	if len(sortedWords) < limit {
		limit = len(sortedWords)
	}

	for i := 0; i < limit; i++ {
		fmt.Printf("%s: %d\n", sortedWords[i].Word, sortedWords[i].Count)
	}
}
