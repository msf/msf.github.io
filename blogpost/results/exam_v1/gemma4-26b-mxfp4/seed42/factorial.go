package main

import "fmt"

func main() {
	var res uint64 = 1
	for i := 1; i <= 10; i++ {
		res *= uint64(i)
	}
	fmt.Println(res)
}
