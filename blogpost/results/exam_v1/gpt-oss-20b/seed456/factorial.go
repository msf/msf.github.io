package main

import (
	"fmt"
)

func main() {
	var result int64 = 1
	for i := int64(2); i <= 10; i++ {
		result *= i
	}
	fmt.Println(result)
}
