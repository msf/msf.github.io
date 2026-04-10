package main

import "fmt"

func main() {
	n := 10
	result := 1
	for i := 1; i <= n; i++ {
		result *= i
	}
	fmt.Println(result)
}
