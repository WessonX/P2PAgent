package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GetAppPath() string {
	file, _ := exec.LookPath(os.Args[0])
	fmt.Println(file)
	path, _ := filepath.Abs(file)
	fmt.Println(path)
	index := strings.LastIndex(path, string(os.PathSeparator))
	return path[:index]
}

func main() {
	fmt.Println(GetAppPath())
}
