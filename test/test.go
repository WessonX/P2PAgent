package main

import (
	"fmt"
)

func main() {

	length := 20
	data_head := fmt.Sprintf("length:%-11d", length)
	fmt.Println(data_head)
	fmt.Println(len(data_head))

}
