package main

import (
	"P2PAgent/utils"
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
	P2PConn net.Conn
	// 端口复用
	LocalPort int

	// 需要读取的报文长度
	remain_cnt int
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
		conn, err = reuseport.Dial("tcp6", fmt.Sprintf("[::]:%d", s.LocalPort), address)
		if err != nil {
			fmt.Println("请求第", errCount, "次地址失败,用户地址:", address, "error:", err.Error())
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
	// 用于拼接分片，组成完整的报文
	var content string

	// 用于存储读到的流数据
	var buffer []byte

	for {
		// 先获取到流数据
		temp_buffer := make([]byte, 1024*1024)
		_, err := s.P2PConn.Read(temp_buffer)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("连接中断")
				break
			}
			fmt.Println("读取失败", err.Error())
			continue
		}
		buffer = append(buffer, temp_buffer...)

		// 根据remain_cnt,判断目前将要读到的内容是包头，还是包的内容
		//等于0，说明之前的包已经读完，将要读的是一个新的包
		if s.remain_cnt == 0 {
			// 读取前18个字节，获取包头
			data_head := string(buffer[:18])

			// 解析出长度
			s.remain_cnt = utils.ResolveDataHead(data_head)

			// drop掉buffer的前18个字节，因为已读
			buffer = buffer[18:]
		} else {
			// 大于0.说明接下来要读的都是包的分片

			// 获取当前缓冲区的长度
			buffer_len := len(buffer)

			// 如果缓冲区长度小于需要读的.那就不读，继续接受流数据
			if buffer_len < s.remain_cnt {
				continue
			} else {
				// 如果缓冲区长度大于等于需要读的,按需读取
				content = string(buffer[:s.remain_cnt])

				// 将已读的部分清除掉
				buffer = buffer[s.remain_cnt:]

				fmt.Printf(">读取到%d个字节,对端节点发来内容:%s\n", s.remain_cnt, content)

				//将内容转发给ros_server
				err = roshandler.RosConn.WriteMessage(websocket.TextMessage, []byte(content))
				if err != nil {
					panic("转发内容给ros_server失败:" + err.Error())
				}
				fmt.Println("消息转发给ros_server")

				// 将remain_cnt 归零
				s.remain_cnt = 0
			}
		}
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

		// 自定义的包头
		data_head := fmt.Sprintf("length:%-11d", cnt)

		// 将包头和数据主体拼接
		body := string(msg)
		content := data_head + body

		// 将读取到的内容，回传给p2p节点
		writeCnt, error := p2phandler.P2PConn.Write([]byte(content))
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
	serverConn, err := reuseport.Dial("tcp6", fmt.Sprintf("[::]:%d", localPort), "[2408:4003:1093:d933:908d:411d:fc28:d28f]:3001")
	if err != nil {
		panic("请求远程服务器失败:" + err.Error())
	}
	fmt.Println("请求远程服务器成功...")
	p2phandler = &P2PHandler{ServerConn: serverConn, LocalPort: int(localPort), remain_cnt: 0}
	p2phandler.WaitNotify()

	time.Sleep(time.Hour)

}
