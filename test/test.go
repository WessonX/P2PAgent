package main

import (
	"fmt"
)

func main() {
	str := "240e:47d:a20:2ce1:db:aa5a:88e4:f7e4"
	s := fmt.Sprintf("[%s]:%d", str, 50)
	fmt.Println(s)
}
