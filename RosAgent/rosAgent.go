package main

import (
	agent "P2PAgent/Agent"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// ros的代理对象
var rosAgent agent.Agent

// 与ros_server建立的连接对象
var roshandler *RosHandler

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

		// 自定义的包头
		data_head := fmt.Sprintf("length:%-11d", cnt)

		// 将包头和数据主体拼接
		body := string(msg)
		content := data_head + body

		// 将读取到的内容，回传给p2p节点
		writeCnt, error := rosAgent.P2PConn.Write([]byte(content))
		if err != nil {
			panic("消息转发给对端节点失败" + error.Error())
		}
		fmt.Println("消息转发给对端节点成功,大小:", writeCnt)
	}
}

func init() {
	localPort := 3002
	rosAgent.InitAgent(localPort)
}

func main() {
	var err error
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
	// 先连接服务器
	err = rosAgent.ConnectToRelay("47.112.96.50:3001")
	if err != nil {
		fmt.Println("连接服务器失败:" + err.Error())
	}

	// 等待服务器回传对端节点的信息
	remotePubAddr, remotePrivAddr, remoteIpv6Addr := rosAgent.WaitNotify()
	fmt.Println("对端的公网地址:", remotePubAddr, " 对端的局域网地址:", remotePrivAddr, " 对端的ipv6地址:", remoteIpv6Addr)

	// 分别尝试连接对端的局域网地址、ipv6地址、公网地址
	isSuccess := rosAgent.DailP2P(remotePrivAddr) || rosAgent.DailP2P(remoteIpv6Addr) || rosAgent.DailP2P(remotePubAddr)

	// 若失败，则断开与ros_server的连接；浏览器会直接通过frp连接ros_server
	if !isSuccess {
		// rosConn.Close()
	} else {
		fmt.Println("p2p直连成功")
		// 若成功，则从rosAgent的channelDate中读取数据，发送给ros_server
		go func() {
			for {
				content := <-rosAgent.ChannelData
				err := roshandler.RosConn.WriteMessage(websocket.TextMessage, []byte(content))
				if err != nil {
					fmt.Println("发送数据给ros_server失败:", err.Error())
				} else {
					fmt.Println("发送数据给ros_server成功")
				}
			}
		}()
	}
	time.Sleep(time.Hour)
}
