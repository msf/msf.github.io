package main

import "fmt"

func main() {
	result := uint64(1)
	for i := 1; i <= 10; i++ {
		result *= uint64(i)
	}
	fmt.Println(result)
}
