package main

import (
	"fmt"
	"net"
	"strings"
)

func GetOutBoundIP() (ip string, err error) {
	conn, err := net.Dial("udp", "8.8.8.8:53") //8.8.8.8是Google提供的免费DNS服务器的IP地址
	if err != nil {
		fmt.Println(err)
		return
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	fmt.Println(localAddr.String())
	ip = strings.Split(localAddr.String(), ":")[0]
	return
}
func main() {
	ip, err := GetOutBoundIP()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(ip)
}
