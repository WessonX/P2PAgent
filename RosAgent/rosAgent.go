package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	"github.com/libp2p/go-reuseport"
)

var p2phandler *P2PHandler
var roshandler *RosHandler

// p2p连接的结构体
type P2PHandler struct {
	// 中继服务器的连接句柄
	ServerConn net.Conn
	// p2p 连接
	P2PConn net.Conn
	// 端口复用
	LocalPort int
}

// WaitNotify 等待远程服务器发送通知告知我们另一个用户的公网IP
func (s *P2PHandler) WaitNotify() {
	buffer := make([]byte, 1024)
	n, err := s.ServerConn.Read(buffer)
	if err != nil {
		panic("从服务器获取用户地址失败" + err.Error())
	}
	data := make(map[string]string)
	if err := json.Unmarshal(buffer[:n], &data); err != nil {
		panic("获取用户信息失败" + err.Error())
	}
	fmt.Println("客户端获取到了对方的地址:", data["address"])
	// 断开服务器连接
	defer s.ServerConn.Close()
	// 请求用户的临时公网IP 以及uid
	go s.DailP2PAndSayHello(data["address"], data["dst_uid"])
}

// DailP2PAndSayHello 连接对方临时的公网地址,并且不停的发送数据
func (s *P2PHandler) DailP2PAndSayHello(address, uid string) {
	var errCount = 1
	var conn net.Conn
	var err error
	for {
		// 重试三次
		if errCount > 3 {
			break
		}
		time.Sleep(time.Second)
		conn, err = reuseport.Dial("tcp", fmt.Sprintf(":%d", s.LocalPort), address)
		if err != nil {
			fmt.Println("请求第", errCount, "次地址失败,用户地址:", address)
			errCount++
			continue
		}
		break
	}
	if errCount > 3 {
		panic("客户端连接失败")
	}
	fmt.Println("P2P连接成功")
	s.P2PConn = conn
	go s.P2PRead()
}

// P2PRead 读取 P2P 节点的数据
func (s *P2PHandler) P2PRead() {
	for {
		buffer := make([]byte, 1024*1024)
		n, err := s.P2PConn.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("连接中断")
				break
			}
			fmt.Println("读取失败", err.Error())
			continue
		}
		body := string(buffer[:n])
		fmt.Println("读取到的内容是:", body)

		//将内容转发给ros_server
		_, err = roshandler.RosConn.Write([]byte(body))
		if err != nil {
			panic("转发内容给ros_server失败:" + err.Error())
		}

	}
}

type RosHandler struct {
	// 与ros_server的连接
	RosConn net.Conn
}

// 从ros_server读取数据
func (s *RosHandler) rosRead() {
	for {
		buffer := make([]byte, 1024*1024)
		n, err := s.RosConn.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("连接中断")
				break
			}
			fmt.Println("读取失败", err.Error())
			continue
		}
		body := string(buffer[:n])
		fmt.Println("读取到的内容是:", body)

		// 将读取到的内容，回传给p2p节点
		_, err = p2phandler.P2PConn.Write([]byte(body))
		if err != nil {
			panic("消息转发给p2p节点失败" + err.Error())
		}
		fmt.Println("消息转发给p2p节点成功")
	}
}

func main() {
	/*
		与9090端口的ros_server建立连接
	*/
	rosConn, err := net.Dial("tcp", "127.0.0.1:9090")
	if err != nil {
		panic("连接ros_server失败" + err.Error())
	}
	fmt.Println("连接ros_server成功")
	roshandler = &RosHandler{RosConn: rosConn}
	go roshandler.rosRead()

	/*
		与对端节点建立p2p连接
	*/

	// 指定本地端口
	localPort := randPort(10000, 50000)
	// 向 P2P 转发服务器注册自己的临时生成的公网 IP (请注意,Dial 这里拨号指定了自己临时生成的本地端口)
	serverConn, err := reuseport.Dial("tcp", fmt.Sprintf(":%d", localPort), "47.112.96.50:3001")
	if err != nil {
		panic("请求远程服务器失败:" + err.Error())
	}
	fmt.Println("请求远程服务器成功...")
	p2phandler = &P2PHandler{ServerConn: serverConn, LocalPort: int(localPort)}
	p2phandler.WaitNotify()

	time.Sleep(time.Hour)

}

// RandPort 生成区间范围内的随机端口
func randPort(min, max int64) int64 {
	if min > max {
		panic("the min is greater than max!")
	}
	if min < 0 {
		f64Min := math.Abs(float64(min))
		i64Min := int64(f64Min)
		result, _ := rand.Int(rand.Reader, big.NewInt(max+1+i64Min))
		return result.Int64() - i64Min
	}
	result, _ := rand.Int(rand.Reader, big.NewInt(max-min+1))
	return min + result.Int64()
}