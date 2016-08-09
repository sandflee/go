package main

import "fmt"

type Person struct {
	height int;
}

func (person *Person) Add(num int)  {
	person.height += num
}

func main() {
	p := &Person{height:160}
	fmt.Println(p.height)
	p.Add(10)
	fmt.Println(p.height)
}
