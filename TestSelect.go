package main

import (
	"fmt"
	"time"
)

func main() {
	a := make(chan string, 1)
	stop := make(chan string, 1)

	go func() {
		time.Sleep(1e9)
		fmt.Println("send message to a")
		a<-"test"
		fmt.Println("send message to stop")
		stop<-"stop"
		time.Sleep(1e9)
	}()

	for {
		select {
		case <-a:
			fmt.Println("get message from a")
		case <-stop:
			fmt.Println("get message from stop")
			break
		}
	}
	fmt.Println("finished")

}
