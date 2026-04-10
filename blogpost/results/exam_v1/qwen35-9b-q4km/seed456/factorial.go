package main

import "fmt"

func main() {
	fact := 1
	for i := 2; i <= 10; i++ {
		fact *= i
	}
	fmt.Println(fact)
}
