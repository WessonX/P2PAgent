package agent

import (
	"P2PAgent/utils"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type Agent struct {
	// 与中继服务器的连接
	ServerConn net.Conn
	// p2p 连接
	P2PConn net.Conn

	// 本地使用的端口
	LocalPort int

	// 需要读取的报文长度
	Remain_cnt int

	// P2PRead读取到的数据存入到通道中
	ChannelData chan string
}

// 等待服务器回传我们的uuid和公网地址
func GetUidAndPubAddr(s *Agent) (uuid string, pubAddr string) {
	buffer := make([]byte, 1024)
	n, err := s.ServerConn.Read(buffer)
	if err != nil {
		panic("读取失败" + err.Error())
	}
	data := make(map[string]string)
	if err := json.Unmarshal(buffer[:n], &data); err != nil {
		panic("获取uuid失败" + err.Error())
	}
	return data["uuid"], data["pubAddr"]
}

// 将局域网地址发送给中继服务器
func SendPrivAddr(s *Agent, privAddr string) {
	var data = make(map[string]string)
	data["privAddr"] = privAddr + fmt.Sprintf(":%d", s.LocalPort)
	body, _ := json.Marshal(data)
	_, err := s.ServerConn.Write(body)
	if err != nil {
		panic("发送局域网地址失败" + err.Error())
	}
	fmt.Println("向中继服务器发送局域网地址成功:", data["privAddr"])
}

// 向中继服务器请求目标uuid对应的公网地址
func RequestForAddr(s *Agent, uuid string) {
	var data = make(map[string]string)
	data["targetUUID"] = uuid // 目标uuid
	body, _ := json.Marshal(data)
	_, err := s.ServerConn.Write(body)
	if err != nil {
		panic("发送requestForAddr请求失败" + err.Error())
	}
	fmt.Println("向服务器发送请求，获取uuid为:", uuid, "的地址")
}

// WaitNotify 等待远程服务器发送通知告知我们另一个用户的公网IP和局域网IP
func WaitNotify(s *Agent) (pubAddr string, privAddr string) {
	buffer := make([]byte, 1024)
	n, err := s.ServerConn.Read(buffer)
	if err != nil {
		panic("从服务器获取用户地址失败" + err.Error())
	}
	data := make(map[string]string)
	if err := json.Unmarshal(buffer[:n], &data); err != nil {
		panic("获取用户信息失败" + err.Error())
	}
	// 断开服务器连接
	defer s.ServerConn.Close()

	return data["address"], data["privAddr"]
}

// DailP2P 连接对方临时的公网地址,并且不停的发送数据
func DailP2P(s *Agent, address string) bool {
	var errCount = 1
	var conn net.Conn
	var err error
	for {
		// 重试三次
		if errCount > 3 {
			break
		}

		d := net.Dialer{
			Timeout: 100 * time.Millisecond,
			LocalAddr: &net.TCPAddr{
				IP:   net.ParseIP("0.0.0.0"),
				Port: s.LocalPort,
			},
			Control: Control,
		}
		conn, err = d.Dial("tcp", address)
		if err != nil {
			fmt.Println("请求第", errCount, "次地址失败,用户地址:", address, "error:", err.Error())
			errCount++
			continue
		}
		break
	}
	if errCount > 3 {
		fmt.Println("客户端连接失败")
		return false
	}
	fmt.Println("P2P连接成功")
	s.P2PConn = conn
	go P2PRead(s)
	return true
}

// P2PRead 读取 P2P 节点的数据
func P2PRead(s *Agent) {
	// 用于拼接分片，组成完整的报文
	var content string

	// 用于存储读到的流数据
	var buffer []byte

	// 判断是否需要读更多数据
	var needReadMore bool

	for {
		// 如果buffer等于空，说明没有待处理的数据，则先获取到流数据；否则，就先处理buffer中的数据，不再额外获取
		if len(buffer) == 0 || needReadMore {
			temp_buffer := make([]byte, 1024*1024)
			cnt, err := s.P2PConn.Read(temp_buffer)
			if err != nil {
				if err.Error() == "EOF" {
					fmt.Println("连接中断")
					break
				}
				fmt.Println("读取失败", err.Error())
				continue
			}
			buffer = append(buffer, temp_buffer[:cnt]...)
			needReadMore = false
		}

		// 如果没读到数据，就不往下执行，直到read到数据为止
		if len(buffer) == 0 {
			continue
		}

		// 根据remain_cnt,判断目前将要读到的内容是包头，还是包的内容
		//等于0，说明之前的包已经读完，将要读的是一个新的包
		if s.Remain_cnt == 0 {
			// 读取前18个字节，获取包头
			data_head := string(buffer[:18])

			// 解析出长度
			s.Remain_cnt = utils.ResolveDataHead(data_head)

			// drop掉buffer的前18个字节，因为已读
			buffer = buffer[18:]
		} else {
			// 大于0.说明接下来要读的都是包的分片

			// 获取当前缓冲区的长度
			buffer_len := len(buffer)

			// 如果缓冲区长度小于需要读的.那就不读，继续接受流数据
			if buffer_len < s.Remain_cnt {
				needReadMore = true
				continue
			} else {
				// 如果缓冲区长度大于等于需要读的,按需读取
				content = string(buffer[:s.Remain_cnt])

				// 将已读的部分清除掉
				buffer = buffer[s.Remain_cnt:]

				fmt.Printf(">读取到%d个字节,对端节点发来内容:%s\n", s.Remain_cnt, content)

				// 将读取到的内容，存入管道中
				s.ChannelData <- content

				// 将remain_cnt 归零
				s.Remain_cnt = 0
			}
		}
	}
}
