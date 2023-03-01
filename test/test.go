package main

import (
	"fmt"
	"time"
)

var done chan bool

// https://gobyexample.com/channel-synchronization
func worker() {
	fmt.Println("working...")
	time.Sleep(time.Second)
	fmt.Println("done")

	done <- true
}

func main() {
	done = make(chan bool)
	go worker()

	<-done
}
