package utils

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

/*
该package用于提供整个程序所有一些工具方法
*/

// 网络地址类型枚举
type IPType string

const (
	IPV4    IPType = "IPv4"
	IPV6    IPType = "IPv6"
	ILLEGAL IPType = "Neither"
)

// 解析tcp切片的包头，获取到整个数据的长度
func ResolveDataHead(data_head string) int {
	len, err := strconv.Atoi(strings.TrimSpace(data_head[7:18]))
	if err != nil {
		panic("解析包头失败" + err.Error())
	}
	return len
}

// 判断是否为合法的ipv4地址
func Judgev4(ips string) bool {
	if len(ips) > 3 {
		return false
	}
	nums, err := strconv.Atoi(ips)
	if err != nil {
		return false
	}

	if nums < 0 || nums > 255 {
		return false
	}
	if len(ips) > 1 && ips[0] == '0' {
		return false
	}
	return true
}

// 判断是否为合法的ipv6地址
func Judgev6(ips string) bool {
	if ips == "" {
		return true
	}
	if len(ips) > 4 {
		return false
	}
	for _, val := range ips {
		if !((val >= '0' && val <= '9') || (val >= 'a' && val <= 'f') || (val >= 'A' && val <= 'F')) {
			return false
		}
	}
	return true
}

// 判断地址是ipv4还是ipv6
func IsIpv4OrIpv6(ip string) IPType {
	if len(ip) < 7 {
		return ILLEGAL
	}
	if ip[0] == '[' && ip[len(ip)-1] == ']' {
		ip = ip[1 : len(ip)-1]
	}
	arrIpv4 := strings.Split(ip, ".")
	if len(arrIpv4) == 4 {
		// 判断IPv4
		for _, val := range arrIpv4 {
			if !Judgev4(val) {
				return ILLEGAL
			}
		}
		return IPV4
	}
	arrIpv6 := strings.Split(ip, ":")
	if len(arrIpv6) == 8 {
		// 判断Ipv6
		for _, val := range arrIpv6 {
			if !Judgev6(val) {
				return ILLEGAL
			}
		}
		return IPV6
	}
	return ILLEGAL
}

// 将uuid存储到本地
func SaveUUID(uuid string) {
	filePath := GetAppPath() + "/uuid.txt"
	file, err := os.OpenFile(filePath, os.O_WRONLY, 0666)
	if err != nil {
		panic("文件打开失败")
	}
	defer file.Close()

	write := bufio.NewWriter(file)
	write.WriteString(uuid)
	write.Flush()
}

// 获取本机的ipv6地址
func GetIPV6Addr() (ip string, err error) {
	conn, err := net.Dial("udp6", "[2001:4860:4860::8888]:53") //2001:4860:4860::8888是Google提供的免费DNS服务器的IPV6地址
	if err != nil {
		fmt.Println(err)
		ip = ""
		return
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip = strings.Split(localAddr.String(), "]")[0][1:]
	return
}

// 获取本机的局域网地址
func GetPrivAddr() (ip string, err error) {
	conn, err := net.Dial("udp", "8.8.8.8:53") //8.8.8.8是Google提供的免费DNS服务器的IP地址
	if err != nil {
		fmt.Println(err)
		return
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip = strings.Split(localAddr.String(), ":")[0]
	return
}

// 获取当前所在目录的路径
// 在go中"./"指的并不是文件所在的目录，而是工程目录。所以需要避免使用相对路径，而是使用绝对路径
func GetAppPath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	index := strings.LastIndex(path, string(os.PathSeparator))
	return path[:index]
}
