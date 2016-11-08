package main

import "fmt"

func modifySlice(num int , slice []int)  {
	slice[0] = 10
	slice = append(slice, num)
	fmt.Printf("%+v", slice)
}

func main()  {
	a := []int{1,2,3,4}
	modifySlice(8, a)
	fmt.Printf("%+v", a)
}
