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
	// uuid
	UID string

	// 连接句柄
	Conn net.Conn

	// 公网地址
	Address string

	// 局域网地址
	PrivAddr string
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
		go WriteBackUidAndPubAddr(conn, id)
		go s.HandleReq(s.ClientPool[id])
	}
}

// 处理来自Agent的请求
func (s *Handler) HandleReq(c *Client) {
	for {
		conn := c.Conn
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("读取失败" + err.Error())
			return
		}
		data := make(map[string]string)
		if err = json.Unmarshal(buffer[:n], &data); err != nil {
			fmt.Println("获取uuid失败" + err.Error())
			return
		}

		if data["targetUUID"] != "" {
			// 读取目标uuid
			uuid := data["targetUUID"]

			// 写回给localAgent
			var dataForLocalAgent = make(map[string]string)
			dataForLocalAgent["address"] = s.ClientPool[uuid].Address   // rosAgent的公网地址
			dataForLocalAgent["privAddr"] = s.ClientPool[uuid].PrivAddr // rosAgent的局域网地址
			body, _ := json.Marshal(dataForLocalAgent)
			_, err := conn.Write(body)
			if err != nil {
				fmt.Println("回传地址给localAgent失败:", err.Error())
			}

			// 写回给rosAgent
			var dataForRosAgent = make(map[string]string)
			dataForRosAgent["address"] = conn.RemoteAddr().String() // localAgent的公网地址
			dataForRosAgent["privAddr"] = c.PrivAddr                // localAgent的局域网地址
			body, _ = json.Marshal(dataForRosAgent)
			_, err = s.ClientPool[uuid].Conn.Write(body)
			if err != nil {
				fmt.Println("回传地址给rosAgent失败:", err.Error())
			}
			// 回传地址后，断开连接
			conn.Close()
			s.ClientPool[uuid].Conn.Close()
		}
		if data["privAddr"] != "" {
			c.PrivAddr = data["privAddr"]
		}
	}
}

// 将分配的uuid以及客户端的公网地址回传给客户端
func WriteBackUidAndPubAddr(conn net.Conn, uuid string) {
	var data = make(map[string]string)
	data["uuid"] = uuid
	data["pubAddr"] = conn.RemoteAddr().String()
	body, _ := json.Marshal(data)
	_, err := conn.Write(body)
	if err != nil {
		fmt.Println("回传信息时出现了错误", err.Error())
	}
	fmt.Println("回传uuid和公网地址给客户端:", conn.RemoteAddr().String())
}

func main() {
	address := ":3001"
	listener, err := reuseport.Listen("tcp", address)
	if err != nil {
		panic("服务端监听失败" + err.Error())
	}
	fmt.Println("服务器开始监听...")
	h := &Handler{Listener: listener, ClientPool: make(map[string]*Client)}
	// 监听内网节点连接
	h.Handle()
	time.Sleep(time.Hour) // 防止主线程退出
}
