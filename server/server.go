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
		go s.HandleReq(conn)
	}
}

// 处理来自localAgent的请求
func (s *Handler) HandleReq(conn net.Conn) {
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

		// 查找到uuid的地址
		addr := s.ClientPool[uuid].Address

		// 写回给localAgent
		var dataForLocalAgent = make(map[string]string)
		dataForLocalAgent["address"] = addr // rosAgent的公网地址
		body, _ := json.Marshal(dataForLocalAgent)
		_, err := conn.Write(body)
		if err != nil {
			fmt.Println("回传地址给localAgent失败:", err.Error())
		}

		// 写回给rosAgent
		var dataForRosAgent = make(map[string]string)
		dataForRosAgent["address"] = conn.RemoteAddr().String() // localAgent的公网地址
		body, _ = json.Marshal(dataForRosAgent)
		_, err = s.ClientPool[uuid].Conn.Write(body)
		if err != nil {
			fmt.Println("回传地址给rosAgent失败:", err.Error())
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

func main() {
	address := ":3001"
	listener, err := reuseport.Listen("tcp", address)
	if err != nil {
		panic("服务端监听失败" + err.Error())
	}
	fmt.Println("服务器开始监听...")
	h := &Handler{Listener: listener, ClientPool: make(map[string]*Client)}
	// 监听内网节点连接,交换彼此的公网 IP 和端口
	h.Handle()
	time.Sleep(time.Hour) // 防止主线程退出
}
