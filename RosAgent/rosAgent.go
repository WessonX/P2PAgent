package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/gorilla/websocket"
	"github.com/libp2p/go-reuseport"
)

var p2phandler *P2PHandler
var roshandler *RosHandler

// p2p连接的结构体
type P2PHandler struct {
	// 中继服务器的连接句柄
	ServerConn net.Conn
	// p2p 连接
	P2PConn net.PacketConn
	// 端口复用
	LocalPort int

	// 远端地址
	Addr *net.UDPAddr
}

// 向中继服务器发送报文失败
func (s *P2PHandler) SayHelloToServer() {
	_, err := s.ServerConn.Write([]byte("hello"))
	if err != nil {
		panic("发送hello消息给服务器失败" + err.Error())
	}
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
	// 监听公网IP 以及uid
	go s.ListenP2P(data["address"], data["dst_uid"])
}

// ListenP2P 监听p2p连接
func (s *P2PHandler) ListenP2P(address, uid string) {
	var conn net.PacketConn
	var err error
	conn, err = reuseport.ListenPacket("udp6", "[::]:3002")
	addr, _ := net.ResolveUDPAddr("udp6", address)
	s.Addr = addr
	if err != nil {
		panic("监听对端节点失败" + err.Error())
	}
	s.P2PConn = conn
	go s.P2PRead()
}

// P2PRead 读取 P2P 节点的数据
func (s *P2PHandler) P2PRead() {
	for {
		buffer := make([]byte, 1024*1024*1024)
		n, _, err := s.P2PConn.ReadFrom(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("连接中断")
				break
			}
			fmt.Println("读取失败", err.Error())
			continue
		}
		body := string(buffer[:n])

		fmt.Printf(">读取到%d个字节,对端节点发来内容：%s\n", n, body)

		// 过滤掉握手消息
		if body == "hello" {
			continue
		}

		//将内容转发给ros_server
		err = roshandler.RosConn.WriteMessage(websocket.TextMessage, []byte(body))
		if err != nil {
			panic("转发内容给ros_server失败:" + err.Error())
		}
		fmt.Println("消息转发给ros_server")

	}
}

type RosHandler struct {
	// 与ros_server的连接
	RosConn *websocket.Conn
}

// 从ros_server读取数据
func (s *RosHandler) rosRead() {
	for {
		_, msg, err := s.RosConn.ReadMessage()
		if err != nil {
			return
		}
		cnt := len(msg)

		fmt.Printf(">读取到%d个字节,ros_server发来内容:%s\n", cnt, string(msg))

		// 将读取到的内容，回传给p2p节点
		writeCnt, error := p2phandler.P2PConn.WriteTo([]byte(msg), p2phandler.Addr)
		if err != nil {
			panic("消息转发给对端节点失败" + error.Error())
		}
		fmt.Println("消息转发给对端节点成功,大小:", writeCnt)
	}
}

func main() {
	/*
		与9090端口的ros_server建立websocket连接
	*/
	dialer := websocket.Dialer{}
	rosConn, _, err := dialer.Dial("ws://127.0.0.1:9090", nil)
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
	localPort := 3002
	// 向 P2P 转发服务器注册自己的临时生成的公网 IP (请注意,Dial 这里拨号指定了自己临时生成的本地端口)
	serverConn, err := reuseport.Dial("udp6", fmt.Sprintf("[::]:%d", localPort), "[2408:4003:1093:d933:908d:411d:fc28:d28f]:3001")
	if err != nil {
		panic("请求远程服务器失败:" + err.Error())
	}
	fmt.Println("请求远程服务器成功...")
	p2phandler = &P2PHandler{ServerConn: serverConn, LocalPort: int(localPort)}
	p2phandler.SayHelloToServer()
	p2phandler.WaitNotify()

	time.Sleep(time.Hour)

}
