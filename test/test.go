package main

import "fmt"

func main() {
	str := "length:12"
	buffer := []byte(str)
	fmt.Println(len(buffer))
}
