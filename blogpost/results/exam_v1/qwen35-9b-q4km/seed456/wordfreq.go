package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

type freq struct {
	word  string
	count int
}

func main() {
	m := make(map[string]int)
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.ToLower(sc.Text())
		for _, w := range strings.Fields(line) {
			m[w]++
		}
	}
	s := make([]freq, 0, len(m))
	for w, c := range m {
		s = append(s, freq{w, c})
	}
	sort.Slice(s, func(i, j int) bool {
		if s[i].count != s[j].count {
			return s[i].count > s[j].count
		}
		return s[i].word < s[j].word
	})
	for i := 0; i < 10 && i < len(s); i++ {
		fmt.Println(s[i].word, ":", s[i].count)
	}
}
