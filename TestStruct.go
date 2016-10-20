package main

import "fmt"

type Score struct {
	name string
	date string
}

type Person struct {
	Score `json:",inline"`
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
	p.name = "score"
	fmt.Printf("%v\n",p)
	fmt.Printf("%+v\n",p)
}
