package main

import "fmt"

type Person struct {
	height int;
	male bool;
	label map[string]int;
}

func (person *Person) Add(num int)  {
	person.height += num
}

func AddPersonHeight(p Person) {
	p.Add(1)
	fmt.Println(p.height)
}

func main() {
	p := &Person{height:160,label : map[string]int{"a":1}}
	fmt.Println("%v",p)
	fmt.Println(p.height)
	p.Add(10)
	fmt.Println(p.height)

	AddPersonHeight(*p)
	fmt.Println(p.height)

	m := *p
	m.label["a"] = 2
	m.height = 0

	fmt.Println(p)
}
