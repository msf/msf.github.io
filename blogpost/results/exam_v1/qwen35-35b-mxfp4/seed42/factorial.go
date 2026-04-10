package main

import "fmt"

func main() {
	factorial := 1
	for i := 1; i <= 10; i++ {
		factorial *= i
	}
	fmt.Println(factorial)
}
