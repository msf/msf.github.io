package main

import "fmt"

func main() {
	n := 10
	f := 1
	for i := 2; i <= n; i++ {
		f *= i
	}
	fmt.Println(f)
}
