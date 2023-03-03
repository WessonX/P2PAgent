package main

import (
	agent "P2PAgent/Agent"
	"P2PAgent/utils"
	"bufio"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// 与对端节点建立连接的对象
var rosAgent *agent.Agent

// 与ros_server建立的连接对象
var roshandler *RosHandler

// 记录局域网地址
var privAddr string

// 记录uuid
var uuid string

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
	d := net.Dialer{
		Timeout: 10 * time.Second,
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: int(localPort),
		},
		Control: agent.Control,
	}
	serverConn, err = d.Dial("tcp", relayAddr)
	if err != nil {
		fmt.Println("连接失败:" + err.Error())
		return false
	}
	fmt.Println("请求远程服务器成功...")

	ch := make(chan string)
	rosAgent = &agent.Agent{ServerConn: serverConn, LocalPort: int(localPort), Remain_cnt: 0, ChannelData: ch}

	// 发送局域网地址和本机的uuid给中继服务器
	err = agent.SendPrivAddrAndUUID(rosAgent, privAddr, uuid)
	if err != nil {
		panic("发送局域网地址和uuid给中继服务器失败" + err.Error())
	}

	// 获取uuid和本机的公网地址
	id, localPubAddr := agent.GetUidAndPubAddr(rosAgent)
	fmt.Println("uuid:", id, " pubAddr:", localPubAddr)
	// 将uuid保存到本地
	if uuid == "" {
		uuid = id
		utils.SaveUUID(uuid)
	}

	// 等待服务器回传对端节点的地址，并发起连接
	rosPubAddr, rosPrivAddr, shouldDownGrade := agent.WaitNotify(rosAgent)
	if shouldDownGrade == "true" {
		return false
	}
	fmt.Println("对端的公网地址:", rosPubAddr, " 对端的局域网地址:", rosPrivAddr)

	// 如果对端的公网地址和本机的相同，说明二者位于同一个局域网下,则局域网直连
	localIP, _ := net.ResolveTCPAddr("tcp", localPubAddr)
	rosIP, _ := net.ResolveTCPAddr("tcp", rosPubAddr)
	if string(localIP.IP) == string(rosIP.IP) {
		fmt.Println("connecting within LAN")
		// 如果局域网直连失败，再尝试打洞
		return agent.DailP2P(rosAgent, rosPrivAddr) || agent.DailP2P(rosAgent, rosPubAddr)
	}
	fmt.Println("trying hole_punching")
	return agent.DailP2P(rosAgent, rosPubAddr)
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

	//读取uuid文件
	filePath := "../uuid.txt"
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		panic("文件打开失败")
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	uuid, _ = reader.ReadString('\n')
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
