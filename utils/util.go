package utils

import (
	"strconv"
	"strings"
)

/*
该package用于提供整个程序所有一些工具方法
*/

// 解析tcp切片的包头，获取到整个数据的长度
func ResolveDataHead(data_head string) int {
	len, err := strconv.Atoi(strings.TrimSpace(data_head[7:18]))
	if err != nil {
		panic("解析包头失败" + err.Error())
	}
	return len
}
