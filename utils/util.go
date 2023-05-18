package utils

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

// 获取当前的goroutine的id
func GetGoid() int64 {
	var (
		buf [64]byte
		n   = runtime.Stack(buf[:], false)
		stk = strings.TrimPrefix(string(buf[:n]), "goroutine")
	)

	idField := strings.Fields(stk)[0]
	id, err := strconv.Atoi(idField)
	if err != nil {
		panic(fmt.Errorf("can not get goroutine id: %v", err))
	}

	return int64(id)
}

// 获取本机的ipv6地址
func GetIPV6Addr() string {
	s, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range s {
		i := regexp.MustCompile(`(\w+:){7}\w+`).FindString(a.String())
		if strings.Count(i, ":") == 7 {
			return i
		}
	}
	return ""
}

// 获取本机的局域网地址
func GetPrivAddr() string {
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		// 检查ip地址判断是否回环地址
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				privAddr := ipnet.IP.String()
				return privAddr
			}
		}
	}
	return ""
}

// 获取当前所在目录的路径
// 在go中"./"指的并不是文件所在的目录，而是工程目录。所以需要避免使用相对路径，而是使用绝对路径
func GetAppPath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	index := strings.LastIndex(path, string(os.PathSeparator))
	return path[:index]
}
