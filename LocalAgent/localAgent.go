package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	agent "P2PAgent/Agent"
	"P2PAgent/utils"

	"github.com/gorilla/websocket"
)

/*
**自定义的tcp报文包头格式：length:{num}
**length后，是数据的总长度，后面填充空格，使得填充到18个字节
 */

// 与对端节点建立p2p连接的handler
var localAgent *agent.Agent

// 与浏览器建立的websocket连接
var browserConn *websocket.Conn

// 记录局域网地址
var privAddr string

// 记录uuid
var uuid string

// 记录p2p连接是否成功
var isSuccess bool

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024 * 1024,
	WriteBufferSize: 1024 * 1024 * 1024,
	// 解决跨域问题
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 接受来自浏览器的请求，并返回rsp
func httpHandler(w http.ResponseWriter, r *http.Request) {
	conn, error := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if error != nil {
		panic("websocket请求建立失败:" + error.Error())
	}
	browserConn = conn

	// 通知浏览器，是否成功建立p2p连接
	if !isSuccess {
		fmt.Println("p2p连接失败")
		NotifyIfSuccess("fail")
		browserConn.Close()
	} else {
		NotifyIfSuccess("success")
	}
	fmt.Println("websocket连接建立成功，对端地址：", conn.RemoteAddr())
	for {
		// Read message from browser
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		readCnt := len(msg)
		// Print the message to the console
		fmt.Printf(">读取到%d个字节,浏览器发来内容:%s\n", readCnt, string(msg))

		// 自定义的包头
		data_head := fmt.Sprintf("length:%-11d", readCnt)

		// 将包头和数据主体拼接
		body := string(msg)
		content := data_head + body

		fmt.Println(content)
		// 将消息转发给对端节点
		writeCnt, err := localAgent.P2PConn.Write([]byte(content))
		if err != nil {
			panic("消息转发给对端节点失败" + err.Error())
		}
		fmt.Println("消息转发给对端节点,大小:", writeCnt)

	}
}

// 发消息给浏览器，告知p2p连接是否成功
func NotifyIfSuccess(isSuccess string) {
	var data = make(map[string]string)
	data["isSuccess"] = isSuccess
	body, _ := json.Marshal(data)
	err := browserConn.WriteMessage(websocket.TextMessage, body)
	if err != nil {
		panic("通知浏览器p2p连接是否成功，fail:" + err.Error())
	}
	fmt.Println("成功通知浏览器，p2p连接是否成功:", isSuccess)
}

/*
relayAddr: 中继服务器的地址。如果是ipv4地址，则逻辑为tcp打洞；如果是ipv6地址，则逻辑为ipv6直连。
bool:连接是否成功
*/
func CreateP2pConn(relayAddr string) bool {
	var serverConn net.Conn
	var err error

	// 随机生成本地端口
	localPort := randPort(10000, 50000)

	// 向 P2P 转发服务器注册自己的公网 IP (请注意,Dial 这里拨号指定了自己临时生成的本地端口。如果用net.Dial方法，使用的端口是随机分配的，就无法穿透了)
	d := net.Dialer{
		Timeout: 1 * time.Second,
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
	localAgent = &agent.Agent{ServerConn: serverConn, LocalPort: int(localPort), Remain_cnt: 0, ChannelData: ch}

	// 发送局域网地址和本机的uuid给中继服务器
	err = agent.SendPrivAddrAndUUID(localAgent, privAddr, uuid)
	if err != nil {
		panic("发送局域网地址和uuid给中继服务器失败" + err.Error())
	}

	// 获取uuid和本机的公网地址
	id, localPubAddr := agent.GetUidAndPubAddr(localAgent)
	fmt.Println("uuid:", id, " pubAddr:", localPubAddr)
	// 将uuid保存到本地
	if uuid == "" {
		uuid = id
		utils.SaveUUID(uuid)
	}

	var uid string
	fmt.Scanln(&uid)

	// 请求获取指定uuid的地址
	err = agent.RequestForAddr(localAgent, uid)
	if err != nil {
		panic("请求获取指定uuid的地址失败" + err.Error())
	}

	// 等待服务器回传对端节点的地址，并发起连接
	rosPubAddr, rosPrivAddr, shouldDownGrade := agent.WaitNotify(localAgent)
	// 如果需要降级
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
		return agent.DailP2P(localAgent, rosPrivAddr) || agent.DailP2P(localAgent, rosPubAddr)
	}
	fmt.Println("trying hole_punching")
	return agent.DailP2P(localAgent, rosPubAddr)
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
		与对端节点建立p2p连接
	*/

	// 先尝试ipv6连接
	isSuccess = CreateP2pConn("[2408:4003:1093:d933:908d:411d:fc28:d28f]:3001")

	// 若失败，尝试打洞
	if !isSuccess {
		fmt.Println("ipv6直连失败")
		isSuccess = CreateP2pConn("47.112.96.50:3001")
	}

	// 如果p2p连接成功,则尝试从agent的通道中读取数据，并发送给浏览器
	if isSuccess {
		fmt.Println("P2P直连成功")
		go func() {
			for {
				content := <-localAgent.ChannelData
				err := browserConn.WriteMessage(websocket.TextMessage, []byte(content))
				if err != nil {
					fmt.Println("消息转发给浏览器失败:", err.Error())
				} else {
					fmt.Println("消息转发给浏览器成功")
				}
			}
		}()
	}
	/*
		与浏览器建立webSocket连接
	*/
	http.HandleFunc("/echo", httpHandler)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "go.html")
	})
	http.ListenAndServe(":3001", nil)

	fmt.Println("与浏览器成功建立websocket连接")
	time.Sleep(time.Hour)
}

// RandPort 生成区间范围内的随机端口
func randPort(min, max int64) int64 {
	if min > max {
		panic("the min is greater than max!")
	}
	if min < 0 {
		f64Min := math.Abs(float64(min))
		i64Min := int64(f64Min)
		result, _ := rand.Int(rand.Reader, big.NewInt(max+1+i64Min))
		return result.Int64() - i64Min
	}
	result, _ := rand.Int(rand.Reader, big.NewInt(max-min+1))
	return min + result.Int64()
}
