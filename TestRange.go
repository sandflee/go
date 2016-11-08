package main

import "fmt"

type Person struct {
	name string
	height int
}

func main()  {
	persons := []Person{}

	a := Person{"a", 1}
	b := Person{"b", 2}
	persons = append(persons, a)
	persons = append(persons, b)

	for _, person := range(persons) {
		person.height = 100
	}

	fmt.Printf("%+v", persons)
}
