package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"time"

	agent "P2PAgent/Agent"

	"github.com/gorilla/websocket"
)

/*
**自定义的tcp报文包头格式：length:{num}
**length后，是数据的总长度，后面填充空格，使得填充到18个字节
 */

// 本地的代理对象
var localAgent agent.Agent

// 与浏览器建立的数据连接
var dataConn *websocket.Conn

// 与浏览器建立的控制连接
var controlConn *websocket.Conn

// 记录p2p连接是否成功
var isSuccess bool

// 记录ros_agent的uuid的通道
var rosUuid_chan chan string

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024 * 1024,
	WriteBufferSize: 1024 * 1024 * 1024,
	// 解决跨域问题
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 浏览器和agent之间的控制连接
func controlHandler(w http.ResponseWriter, r *http.Request) {
	conn, error := upgrader.Upgrade(w, r, nil)
	if error != nil {
		panic("websocket请求建立失败:" + error.Error())
	}
	controlConn = conn
	fmt.Println("websocket控制连接建立成功")
	for {
		// Read message from browser
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		readCnt := len(msg)
		// Print the message to the console
		fmt.Printf(">读取到%d个字节,浏览器发来内容:%s\n", readCnt, string(msg))

		rosUuid_chan <- string(msg)

	}
}

// 浏览器和agent之间的数据连接
func dataHandler(w http.ResponseWriter, r *http.Request) {
	conn, error := upgrader.Upgrade(w, r, nil)
	if error != nil {
		panic("websocket请求建立失败:" + error.Error())
	}
	dataConn = conn

	fmt.Println("websocket数据连接建立成功")
	for {
		// Read message from browser
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		readCnt := len(msg)
		// Print the message to the console
		fmt.Printf(">读取到%d个字节,浏览器发来内容:%s\n", readCnt, string(msg))

		// 如果建立好了p2p连接，就将浏览器传来的内容转发给对端节点
		if localAgent.P2PConn != nil {
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
}

// 发消息给浏览器，告知p2p连接的状态
func NotifyStatus(status string) {
	var data = make(map[string]string)
	data["status"] = status
	body, _ := json.Marshal(data)
	err := controlConn.WriteMessage(websocket.TextMessage, body)
	if err != nil {
		panic("fail:" + err.Error())
	}
	fmt.Println("成功通知浏览器，p2p连接状态:", status)
}

func init() {
	localPort := randPort(10000, 50000)
	localAgent.InitAgent(int(localPort))

	// 初始化存储对端uuid的通道
	ch_uuid := make(chan string)
	rosUuid_chan = ch_uuid

}

func main() {
	/*
		与浏览器建立webSocket连接
	*/
	go func() {
		// 与浏览器的控制连接
		http.HandleFunc("/control", controlHandler)

		// 与浏览器的数据连接
		http.HandleFunc("/data", dataHandler)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "go.html")
		})
		http.ListenAndServe(":3000", nil)

		fmt.Println("与浏览器成功建立websocket连接")
	}()
	defer localAgent.Close()
	/*
		与对端节点建立p2p连接
	*/

	// 连接服务器
	var err error
	for {
		err = localAgent.ConnectToRelay("47.112.96.50:3001")
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		fmt.Println("connected to relayServer")
		break
	}

	for {

		// 等待浏览器发来对端节点的uuid
		peer_id := <-rosUuid_chan

		// 请求目标uuid的节点的信息
		err = localAgent.RequestForAddr(peer_id)
		if err != nil {
			fmt.Println("请求对端节点信息失败" + err.Error())
		}

		// 等待服务器回传对端节点的信息
		remotePubAddr, remotePrivAddr, remoteIpv6Addr := localAgent.WaitNotify()
		fmt.Println("对端的公网地址:", remotePubAddr, " 对端的局域网地址:", remotePrivAddr, " 对端的ipv6地址:", remoteIpv6Addr)

		// 在尝试连接之前，先关掉可能的已有连接，防止端口占用
		if localAgent.P2PConn != nil {
			localAgent.P2PConn.Close()
		}

		// 分别尝试连接对端的局域网地址、ipv6地址、公网地址
		isSuccess = localAgent.DailP2P(remotePrivAddr) || localAgent.DailP2P(remoteIpv6Addr) || localAgent.DailP2P(remotePubAddr)

		// 通知浏览器，是否成功建立p2p连接
		if !isSuccess {
			fmt.Println("p2p连接失败")
			NotifyStatus("fail")
			continue
		} else {
			NotifyStatus("success")
		}

		// 如果p2p连接成功,则尝试从agent的通道中读取数据，并发送给浏览器
		if isSuccess {
			fmt.Println("P2P直连成功")
			go func() {
				for {
					content := <-localAgent.ChannelData
					// 如果连接已经中断，通知浏览器并中断循环
					if content == "EOF" {
						NotifyStatus("disconnected")
						break
					}
					err := dataConn.WriteMessage(websocket.TextMessage, []byte(content))
					if err != nil {
						fmt.Println("消息转发给浏览器失败:", err.Error())
					} else {
						fmt.Println("消息转发给浏览器成功")
					}
				}
			}()
		}
	}
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
