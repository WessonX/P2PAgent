package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/go-basic/uuid"
	"github.com/libp2p/go-reuseport"
)

type Client struct {
	UID     string
	Conn    net.Conn
	Address string
}

type Handler struct {
	// 服务端句柄
	Listener net.Listener
	// 客户端句柄池
	ClientPool map[string]*Client
}

func (s *Handler) Handle() {
	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			fmt.Println("获取连接句柄失败", err.Error())
			continue
		}
		id := uuid.New()
		s.ClientPool[id] = &Client{
			UID:     id,
			Conn:    conn,
			Address: conn.RemoteAddr().String(),
		}
		fmt.Println("一个客户端连接进去了,他的公网IP是", conn.RemoteAddr().String())
		go WriteBackUuid(conn, id)
		// 暂时只接受两个客户端,多余的不处理
		if len(s.ClientPool) == 2 {
			// 交换双方的公网地址
			s.ExchangeAddress()
			break
		}
	}
}

// ExchangeAddress 交换地址
func (s *Handler) ExchangeAddress() {
	for uid, client := range s.ClientPool {
		for id, c := range s.ClientPool {
			// 自己不交换
			if uid == id {
				continue
			}
			var data = make(map[string]string)
			data["dst_uid"] = client.UID     // 对方的 UID
			data["address"] = client.Address // 对方的公网地址
			body, _ := json.Marshal(data)
			if _, err := c.Conn.Write(body); err != nil {
				fmt.Println("交换地址时出现了错误", err.Error())
			}
		}
	}
}

// 将分配的uuid回传给客户端
func WriteBackUuid(conn net.Conn, uuid string) {
	var data = make(map[string]string)
	data["uuid"] = uuid
	body, _ := json.Marshal(data)
	_, err := conn.Write(body)
	if err != nil {
		fmt.Println("回传uuid时出现了错误", err.Error())
	}
	fmt.Println("回传uuid给客户端:", conn.RemoteAddr().String())
}

// // 处理来自localAgent的p2p连接请求
// func RecvReq(conn net.Conn) {
// 	buffer := make([]byte, 1024)
// 	_, err := conn.Read(buffer)
// 	if err != nil {
// 		panic("读取失败" + err.Error())
// 	}
// 	if string(buffer[:14]) == "requestForAddr" {
// 		uuid := string(buffer[15:])
// 		fmt.Println()
// 	}

// }

func main() {
	address := "[::]:3001"
	listener, err := reuseport.Listen("tcp6", address)
	if err != nil {
		panic("服务端监听失败" + err.Error())
	}
	fmt.Println("服务器开始监听...")
	h := &Handler{Listener: listener, ClientPool: make(map[string]*Client)}
	// 监听内网节点连接,交换彼此的公网 IP 和端口
	h.Handle()
	time.Sleep(time.Hour) // 防止主线程退出
}
