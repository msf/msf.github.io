package main

import (
	"fmt"
)

func main() {
	var result uint64 = 1
	for i := uint64(2); i <= 10; i++ {
		result *= i
	}
	fmt.Println(result)
}
