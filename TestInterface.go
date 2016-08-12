package main

import "fmt"

func main() {
	var i interface{}
	i = 1
	decribe(i)
	i = "hello"
	decribe(i)
}

func decribe(i interface{}) {
	fmt.Printf("%v,%T\n",i,i)
}
