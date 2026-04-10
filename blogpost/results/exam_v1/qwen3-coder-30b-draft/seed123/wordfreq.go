package main

import (
    "bufio"
    "fmt"
    "os"
    "sort"
    "strings"
)

func main() {
    wordCount := make(map[string]int)
    scanner := bufio.NewScanner(os.Stdin)
    
    for scanner.Scan() {
        line := scanner.Text()
        words := strings.Fields(line)
        for _, word := range words {
            cleanWord := strings.ToLower(word)
            wordCount[cleanWord]++
        }
    }
    
    type wordFreq struct {
        word  string
        count int
    }
    
    var freqList []wordFreq
    for word, count := range wordCount {
        freqList = append(freqList, wordFreq{word, count})
    }
    
    sort.Slice(freqList, func(i, j int) bool {
        return freqList[i].count > freqList[j].count
    })
    
    for i := 0; i < len(freqList) && i < 10; i++ {
        fmt.Printf("%s: %d\n", freqList[i].word, freqList[i].count)
    }
}
