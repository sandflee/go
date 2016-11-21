package main

import "fmt"

func modifySlice(num int , slice []int)  {
	slice[0] = 10
	slice = append(slice, num)
	fmt.Printf("%+v", slice)
}

type Person struct {
	name string
	age int
}

func main() {
	a := []int{1,2,3,4}
	modifySlice(8, a)
	fmt.Printf("%+v", a)

	s := make([]Person, 3)
	s = append(s, Person{"a",1})
	s = append(s, Person{"b",2})

	for index, v := range s {
		v.age = 10
		fmt.Printf("%+v,%+v\n",index, v)
	}

	fmt.Printf("%+v\n", s)

	var b []int
	b = append(b, 1)
	fmt.Printf("%+v\n", b)
}
