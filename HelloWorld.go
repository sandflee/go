package main

import (
	"fmt"
)

func main() {
	fmt.Println("hello world")
	a := []string {}
	a = append(a, "hello")
	a = append(a, "hello")
	a = append(a, "hello")
	for _, b := range(a) {
		fmt.Println(b)
	}
}

