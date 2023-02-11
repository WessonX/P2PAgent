package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/libp2p/go-reuseport"
)

// 与对端节点建立p2p连接的handler
var handler *Handler

// 与浏览器建立的websocket连接
var browserConn *websocket.Conn

type Handler struct {
	// 中继服务器的连接句柄
	ServerConn net.Conn
	// p2p 连接
	P2PConn net.Conn
	// 端口复用
	LocalPort int
}

// WaitNotify 等待远程服务器发送通知告知我们另一个用户的公网IP
func (s *Handler) WaitNotify() {
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
func (s *Handler) DailP2PAndSayHello(address, uid string) {
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
func (s *Handler) P2PRead() {
	for {
		// 1024 * 1024，1M的缓存上限。后续这个值可能要根据实际数据量提高调整
		buffer := make([]byte, 1024*1024*1024)
		n, err := s.P2PConn.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("连接中断")
				break
			}
			fmt.Println("读取失败", err.Error())
			continue
		}
		body := string(buffer[:n])
		fmt.Printf(">读取到%d个字节,对端节点发来内容:%s", n, body)

		// 将读取到的内容，写回给浏览器

		err = browserConn.WriteMessage(websocket.TextMessage, []byte(body))
		if err != nil {
			panic("消息转发给浏览器失败:" + err.Error())
		}
		fmt.Println("消息转发给浏览器成功")
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024 * 1024,
	WriteBufferSize: 1024 * 1024 * 1024,
}

// 接受来自浏览器的请求，并返回rsp
func httpHandler(w http.ResponseWriter, r *http.Request) {
	conn, error := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if error != nil {
		panic("websocket请求建立失败:" + error.Error())
	}
	browserConn = conn

	fmt.Println("websocket连接建立成功，对端地址：", conn.RemoteAddr())
	for {
		// Read message from browser
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		readCnt := len(msg)
		// Print the message to the console
		fmt.Printf(">读取到%d个字节,浏览器发来内容:%s", readCnt, string(msg))

		// 将消息转发给对端节点
		writeCnt, err := handler.P2PConn.Write([]byte(msg))
		if err != nil {
			panic("消息转发给对端节点失败" + err.Error())
		}
		fmt.Println("消息转发给对端节点,大小:", writeCnt)

	}
}

func main() {
	/*
		与对端节点建立p2p连接
	*/

	// 随机生成本地端口
	localPort := randPort(10000, 50000)

	// 向 P2P 转发服务器注册自己的临时生成的公网 IP (请注意,Dial 这里拨号指定了自己临时生成的本地端口。如果用net.Dial方法，使用的端口是随机分配的，就无法穿透了)
	serverConn, err := reuseport.Dial("tcp6", fmt.Sprintf("[::]:%d", localPort), "[2408:4003:1093:d933:908d:411d:fc28:d28f]:3001")
	if err != nil {
		panic("请求远程服务器失败:" + err.Error())
	}
	fmt.Println("请求远程服务器成功...")
	handler = &Handler{ServerConn: serverConn, LocalPort: int(localPort)}
	// 等待服务器回传对端节点的地址，并发起连接
	handler.WaitNotify()

	/*
		与浏览器建立webSocket连接
	*/
	fmt.Println("localAgent is listening on 3001")
	http.HandleFunc("/echo", httpHandler)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "go.html")
	})
	http.ListenAndServe(":3001", nil)

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
