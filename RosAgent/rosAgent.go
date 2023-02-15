package main

import (
	"P2PAgent/utils"
	"encoding/json"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/libp2p/go-reuseport"
	"golang.org/x/sys/unix"
)

var p2phandler *P2PHandler
var roshandler *RosHandler

// 记录局域网地址
var privAddr string

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

// 等待服务器回传我们的uuid和公网地址
func (s *P2PHandler) getUidAndPubAddr() (uuid string, pubAddr string) {
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
func (s *P2PHandler) sendPrivAddr(privAddr string) {
	var data = make(map[string]string)
	data["privAddr"] = privAddr
	body, _ := json.Marshal(data)
	_, err := s.ServerConn.Write(body)
	if err != nil {
		panic("发送局域网地址失败" + err.Error())
	}
	fmt.Println("向中继服务器发送局域网地址成功")
}

// WaitNotify 等待远程服务器发送通知告知我们另一个用户的公网IP
func (s *P2PHandler) WaitNotify() (pubAddr string, privAddr string) {
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
	return data["address"], data["privAddr"]
}

func MyControl(network, address string, c syscall.RawConn) error {
	var err error
	c.Control(func(fd uintptr) {
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if err != nil {
			return
		}

		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if err != nil {
			return
		}
	})
	return err
}

// DailP2PAndSayHello 连接对方临时的公网地址,并且不停的发送数据
func (s *P2PHandler) DailP2P(address string) bool {
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
			Control: MyControl,
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
	go s.P2PRead()
	return true
}

// P2PRead 读取 P2P 节点的数据
func (s *P2PHandler) P2PRead() {
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
				needReadMore = true
				continue
			} else {
				// 如果缓冲区长度大于等于需要读的,按需读取
				content = string(buffer[:s.remain_cnt])

				// 将已读的部分清除掉
				buffer = buffer[s.remain_cnt:]

				fmt.Printf(">读取到%d个字节,对端节点发来内容:%s\n", s.remain_cnt, content)

				//将内容转发给ros_server
				err := roshandler.RosConn.WriteMessage(websocket.TextMessage, []byte(content))
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

/*
relayAddr: 中继服务器的地址。如果是ipv4地址，则逻辑为tcp打洞；如果是ipv6地址，则逻辑为ipv6直连。
bool:连接是否成功
*/
func CreateP2pConn(relayAddr string) bool {
	var serverConn net.Conn
	var err error

	// 随机生成本地端口
	localPort := 3002

	// 向 P2P 转发服务器注册自己的公网 IP (请注意,Dial 这里拨号指定了自己临时生成的本地端口。如果用net.Dial方法，使用的端口是随机分配的，就无法穿透了)
	serverConn, err = reuseport.Dial("tcp", fmt.Sprintf("[::]:%d", localPort), relayAddr)
	if err != nil {
		fmt.Println("连接失败:" + err.Error())
		return false
	}
	fmt.Println("请求远程服务器成功...")
	p2phandler = &P2PHandler{ServerConn: serverConn, LocalPort: int(localPort), remain_cnt: 0}

	// 发送局域网地址给中继服务器
	p2phandler.sendPrivAddr(privAddr)
	// 获取uuid
	uuid, pubAddr := p2phandler.getUidAndPubAddr()
	fmt.Println("uuid:", uuid, " pubAddr:", pubAddr)

	// 等待服务器回传对端节点的地址，并发起连接
	pubAddr, privAddr := p2phandler.WaitNotify()
	fmt.Println("对端的公网地址:", pubAddr, " 对端的局域网地址:", privAddr)
	return p2phandler.DailP2P(pubAddr)
}

func init() {
	//获取局域网地址
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		// 检查ip地址判断是否回环地址
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				privAddr = ipnet.IP.String()
				fmt.Println("privAddr:", privAddr)
			}
		}
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

	// 先尝试ipv6连接
	isSuccess := CreateP2pConn("[2408:4003:1093:d933:908d:411d:fc28:d28f]:3001")

	// 若失败，尝试打洞
	if !isSuccess {
		fmt.Println("ipv6直连失败")
		isSuccess = CreateP2pConn("47.112.96.50:3001")
	}

	// 若失败，则断开与ros_server的连接；浏览器会直接通过frp连接ros_server
	if !isSuccess {
		rosConn.Close()
	}

	time.Sleep(time.Hour)
}
