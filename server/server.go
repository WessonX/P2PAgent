package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-basic/uuid"
)

type Server struct {
	// 相当于udp的Socket描述符
	udpConn *net.UDPConn
	Addr    *net.UDPAddr
	// 客户端地址池
	ClientPool map[string]*net.UDPAddr
}

func (server *Server) ExchangeAddress() {
	for uid, address := range server.ClientPool {
		for id, addr := range server.ClientPool {
			// 自己不交换
			if uid == id {
				continue
			}
			var data = make(map[string]string)
			data["address"] = address.String() // 对方的公网地址
			body, _ := json.Marshal(data)
			_, err := server.udpConn.WriteToUDP([]byte(body), addr)
			if err != nil {
				panic("交换地址失败:" + err.Error())
			}
		}
	}
}

func (server *Server) readUdp() {
	for {
		data := make([]byte, 1024)
		_, remoteAddr, err := server.udpConn.ReadFromUDP(data)
		if err != nil {
			fmt.Println("error while reading data")
		}
		server.Addr = remoteAddr
		id := uuid.New()
		server.ClientPool[id] = remoteAddr
		fmt.Println("一个客户端连接进来了,它的公网ip是:", remoteAddr.String())
		// 暂时只接受两个客户端,多余的不处理
		if len(server.ClientPool) == 2 {
			// 交换双方的公网地址
			server.ExchangeAddress()
			break
		}
	}

}

func main() {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 3001,
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("connection error")
		os.Exit(1)
	}
	fmt.Println("服务端开始监听...")
	h := &Server{udpConn: conn, ClientPool: make(map[string]*net.UDPAddr)}

	go h.readUdp()
	defer conn.Close()
	time.Sleep(time.Hour) //防止主线程退出

}
