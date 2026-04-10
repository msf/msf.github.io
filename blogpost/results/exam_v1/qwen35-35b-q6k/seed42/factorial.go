package main

import "fmt"

func main() {
	f := 1
	for i := 2; i <= 10; i++ {
		f *= i
	}
	fmt.Println(f)
}
