package main

import (
	"P2PAgent/utils"
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
		c := &Client{
			Conn:    conn,
			Address: conn.RemoteAddr().String(),
		}
		fmt.Println("一个客户端连接进去了,他的公网IP是", conn.RemoteAddr().String())

		// 响应来自客户端的请求
		go s.HandleReq(c)
	}
}

// 等待接收客户端传来的uuid和privAddr
func (s *Handler) recvUUIDAndPrivAddr(c *Client, data map[string]string) {

	if data["privAddr"] != "" {
		c.PrivAddr = data["privAddr"]
	}
	if data["uuid"] != "" {
		c.UID = data["uuid"]
		s.ClientPool[c.UID] = c
	} else {
		uuid := uuid.New()
		c.UID = uuid
		s.ClientPool[uuid] = c
	}

	// 将uuid和pubAddr回传给客户端
	WriteBackUidAndPubAddr(c.Conn, c.UID)
}

// 交换连接双方的信息
func (s *Handler) exchangeInfo(c *Client, data map[string]string) {
	if data["targetUUID"] != "" {
		// 读取目标uuid
		uuid := data["targetUUID"]

		// 回传给localAgent的数据
		var dataForLocalAgent = make(map[string]string)

		// 回传给rosAGent的数据
		var dataForRosAgent = make(map[string]string)

		// 判断二者的网络类型是否一致
		ros_type := utils.IsIpv4OrIpv6(s.ClientPool[uuid].Address)
		local_type := utils.IsIpv4OrIpv6(c.Conn.RemoteAddr().String())

		// 如果二者的类型不一致
		if ros_type != local_type {
			// ros_agent需要降级
			if ros_type == utils.IPV6 {
				dataForRosAgent["shouldDownGrade"] = "true"
				dataForLocalAgent["shouldDownGrade"] = "false"
			}

			// local_agent需要降级
			if local_type == utils.IPV6 {
				dataForLocalAgent["shouldDownGrade"] = "true"
				dataForRosAgent["shouldDownGrade"] = "false"
			}
		}

		// 写回给localAgent
		dataForLocalAgent["address"] = s.ClientPool[uuid].Address   // rosAgent的公网地址
		dataForLocalAgent["privAddr"] = s.ClientPool[uuid].PrivAddr // rosAgent的局域网地址
		body, _ := json.Marshal(dataForLocalAgent)
		_, err := c.Conn.Write(body)
		if err != nil {
			fmt.Println("回传地址给localAgent失败:", err.Error())
		}

		// 写回给rosAgent
		dataForRosAgent["address"] = c.Conn.RemoteAddr().String() // localAgent的公网地址
		dataForRosAgent["privAddr"] = c.PrivAddr                  // localAgent的局域网地址
		body, _ = json.Marshal(dataForRosAgent)
		_, err = s.ClientPool[uuid].Conn.Write(body)
		if err != nil {
			fmt.Println("回传地址给rosAgent失败:", err.Error())
		}
	}
}

// 处理来自Agent的请求
func (s *Handler) HandleReq(c *Client) {
	for {
		// 解析出数据
		conn := c.Conn
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("读取失败" + err.Error())
			return
		}
		data := make(map[string]string)
		if err = json.Unmarshal(buffer[:n], &data); err != nil {
			fmt.Println("获取数据失败" + err.Error())
			return
		}

		// 根据请求的方法名，进行请求的分发

		// 接收客户端传来的uuid和局域网地址
		if data["method"] == "recvUUIDAndPrivAddr" {
			s.recvUUIDAndPrivAddr(c, data)
		} else if data["method"] == "exchangeInfo" {
			// 收到localAgent的连接请求，交换双方的信息
			s.exchangeInfo(c, data)
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
