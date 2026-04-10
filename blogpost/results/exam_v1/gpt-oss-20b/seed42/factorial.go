package main

import (
	"fmt"
)

func main() {
	var f uint64 = 1
	for i := 2; i <= 10; i++ {
		f *= uint64(i)
	}
	fmt.Println(f)
}
