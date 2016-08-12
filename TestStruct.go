package main

import "fmt"

type Person struct {
	height int;
}

func (person *Person) Add(num int)  {
	person.height += num
}

func AddPersonHeight(p Person) {
	p.Add(1)
	fmt.Println(p.height)
}

func main() {
	p := &Person{height:160}
	fmt.Println(p.height)
	p.Add(10)
	fmt.Println(p.height)

	AddPersonHeight(*p)
	fmt.Println(p.height)
}
