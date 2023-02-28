package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {

	//获取本机的uuid
	filePath := "../uuid.txt"
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		panic("文件打开失败")
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	uuid, err := reader.ReadString('\n')
	fmt.Println("uuid:", uuid)

}
