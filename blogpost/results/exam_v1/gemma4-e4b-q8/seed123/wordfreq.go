package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	wordCounts := make(map[string]int)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		words := strings.Fields(line)
		for _, word := range words {
			cleanedWord := strings.ToLower(strings.TrimFunc(word, func(r rune) bool {
				return !('a' <= r && r <= 'z' || '0' <= r && r <= '9')
			}))
			if cleanedWord != "" {
				wordCounts[cleanedWord]++
			}
		}
	}

	type wordCount struct {
		word  string
		count int
	}

	var counts []wordCount
	for word, count := range wordCounts {
		counts = append(counts, wordCount{word, count})
	}

	quickSort(counts, 0, len(counts)-1)

	for i := 0; i < 10 && i < len(counts); i++ {
		fmt.Printf("%s: %d\n", counts[i].word, counts[i].count)
	}
}

func quickSort(arr []wordCount, low, high int) {
	if low < high {
		pi := partition(arr, low, high)
		quickSort(arr, low, pi-1)
		quickSort(arr, pi+1, high)
	}
}

func partition(arr []wordCount, low, high int) int {
	pivot := arr[high].count
	i := low - 1
	for j := low; j < high; j++ {
		if arr[j].count > pivot {
			i++
			arr[i], arr[j] = arr[j], arr[i]
		}
	}
	arr[i+1], arr[high] = arr[high], arr[i+1]
	return i + 1
}
