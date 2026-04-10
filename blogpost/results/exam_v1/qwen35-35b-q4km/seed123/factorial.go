package main

import "fmt"

func main() {
	var f int64 = 1
	for i := int64(1); i <= 10; i++ {
		f *= i
	}
	fmt.Println(f)
}
