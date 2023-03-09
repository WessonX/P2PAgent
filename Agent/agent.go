package agent

import (
	"P2PAgent/utils"
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

type Agent struct {
	// 与中继服务器的连接
	ServerConn net.Conn
	// p2p 连接
	P2PConn net.Conn

	// 本机的uuid
	UUID string
	// 本地的局域网地址
	PrivAddr string

	// 本机的公网地址
	PubAddr string

	// 本机的ipv6地址
	Ipv6Addr string

	// 本地使用的端口
	LocalPort int

	// 需要读取的报文长度
	Remain_cnt int

	// P2PRead读取到的数据存入到通道中
	ChannelData chan string
}

// agent的初始化方法
func (agent *Agent) InitAgent(port int) {
	// 设置本地端口
	agent.LocalPort = port

	// 设置数据通道
	ch := make(chan string)
	agent.ChannelData = ch

	// 设置剩余读取的数据长度
	agent.Remain_cnt = 0

	// 获取局域网地址
	agent.PrivAddr = utils.GetPrivAddr()

	// 获取本机的ipv6地址
	agent.Ipv6Addr = utils.GetIPV6Addr()

	//读取uuid文件
	filePath := "../uuid.txt"
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		panic("文件打开失败")
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	// 设置uuid
	agent.UUID, _ = reader.ReadString('\n')
}

func (agent *Agent) Close() {
	agent.P2PConn.Close()
	agent.ServerConn.Close()
}

// 连接到中继服务器
/*
relayAddr:中继服务器的地址
*/
func (agent *Agent) ConnectToRelay(relayAddr string) (err error) {
	var serverConn net.Conn
	d := net.Dialer{
		Timeout: 10 * time.Second,
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: agent.LocalPort,
		},
		Control: Control,
	}
	serverConn, err = d.Dial("tcp", relayAddr)
	if err != nil {
		fmt.Println("连接失败:" + err.Error())
		return err
	}
	fmt.Println("请求远程服务器成功...")
	agent.ServerConn = serverConn

	// 发送ipv6地址、局域网地址和本机的uuid给中继服务器
	err = agent.SendPrivAddrAndUUID(agent.Ipv6Addr, agent.PrivAddr, agent.UUID)
	if err != nil {
		fmt.Println("发送本机信息给中继服务器失败" + err.Error())
		return err
	}

	// 获取uuid和本机的公网地址
	id, localPubAddr := agent.GetUidAndPubAddr()
	fmt.Println("uuid:", id, " pubAddr:", localPubAddr)
	// 将uuid保存到本地
	if agent.UUID == "" {
		agent.UUID = id
		utils.SaveUUID(id)
	}
	return nil
}

// 等待服务器回传我们的uuid和公网地址
func (s *Agent) GetUidAndPubAddr() (uuid string, pubAddr string) {
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

// 将ipv6地址、局域网地址和uuid发送给中继服务器
func (s *Agent) SendPrivAddrAndUUID(ipv6Addr string, privAddr string, uuid string) error {
	var data = make(map[string]string)
	data["method"] = "recvUUIDAndPrivAddr"
	data["privAddr"] = privAddr + fmt.Sprintf(":%d", s.LocalPort)
	data["ipv6Addr"] = fmt.Sprintf("[%s]:%d", ipv6Addr, s.LocalPort)
	data["uuid"] = uuid
	body, _ := json.Marshal(data)
	_, err := s.ServerConn.Write(body)
	if err != nil {
		return err
	}
	return nil
}

// 向中继服务器请求目标uuid对应的公网地址
func (s *Agent) RequestForAddr(uuid string) error {
	var data = make(map[string]string)
	data["method"] = "exchangeInfo"
	data["targetUUID"] = uuid // 目标uuid
	body, _ := json.Marshal(data)
	_, err := s.ServerConn.Write(body)
	if err != nil {
		return err
	}
	return nil
}

// WaitNotify 等待远程服务器发送通知告知我们另一个用户的ipv6地址，公网IP和局域网IP
func (s *Agent) WaitNotify() (pubAddr string, privAddr string, ipv6Addr string) {
	buffer := make([]byte, 1024)
	n, err := s.ServerConn.Read(buffer)
	if err != nil {
		panic("从服务器获取用户地址失败" + err.Error())
	}
	data := make(map[string]string)
	if err := json.Unmarshal(buffer[:n], &data); err != nil {
		panic("获取用户信息失败" + err.Error())
	}

	return data["address"], data["privAddr"], data["ipv6Addr"]
}

// DailP2P 连接对方临时的公网地址,并且不停的发送数据
func (s *Agent) DailP2P(address string) bool {
	var errCount = 1
	var conn net.Conn
	var err error
	for {
		// 重试三次
		if errCount > 3 {
			break
		}

		d := net.Dialer{
			Timeout: 10 * time.Second,
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
	s.P2PConn = conn
	go s.P2PRead()
	return true
}

// P2PRead 读取 P2P 节点的数据
func (s *Agent) P2PRead() {
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
					s.P2PConn.Close()
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
