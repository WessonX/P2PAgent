package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// 与浏览器建立的websocket连接
var browserConn *websocket.Conn

// 与对端节点建立的p2p连接句柄
var p2pConn *P2PConn

// 与中继服务器建立的连接
type ServerConn struct {
	udpConn *net.UDPConn

	// 本地节点的地址
	LocalAddr *net.UDPAddr
}

// 向中继服务器发送hello报文
func (sc *ServerConn) sayHelloToServer() {
	msg := "echo from client"
	_, err := sc.udpConn.Write([]byte(msg))
	if err != nil {
		panic("向服务器发送echo报文失败" + err.Error())
	}
	fmt.Println("成功向服务器发送echo报文...")
}

// 读取服务器回传的对端节点的地址
func (sconn *ServerConn) HolePucnh() {
	buffer := make([]byte, 1024)
	n, addr, err := sconn.udpConn.ReadFromUDP(buffer)
	fmt.Println("addr:", addr.String())
	if err != nil {
		panic("error while reading data:" + err.Error())
	}
	data := make(map[string]string)
	if err = json.Unmarshal(buffer[:n], &data); err != nil {
		panic("获取对端节点地址失败" + err.Error())
	}
	// 断开与中继服务器的连接
	sconn.udpConn.Close()

	fmt.Println("对端节点的地址:", data["address"])

	// 解析对端节点的地址
	address := data["address"]
	idx := strings.Index(address, ":")
	ip_part := address[:idx]
	port_part, err := strconv.Atoi(address[idx+1:])
	if err != nil {
		panic("地址解析错误" + err.Error())
	}

	// 构建对端节点的地址
	remote_Addr := &net.UDPAddr{
		IP:   net.ParseIP(ip_part),
		Port: port_part,
	}

	// 构建p2p连接结构体
	p2pConn = &P2PConn{
		RemoteAddr: remote_Addr,
		LocalAddr:  sconn.LocalAddr,
	}
	p2pConn.dialP2P()
}

// 与对端节点建立的连接
type P2PConn struct {
	// 给对端节点传数据
	DialConn *net.UDPConn

	// 对端节点的地址
	RemoteAddr *net.UDPAddr

	// 本地节点的地址
	LocalAddr *net.UDPAddr
}

// 建立p2p直连
func (pconn *P2PConn) dialP2P() {
	// 重试次数
	var retryCount = 1

	// 发消息连接
	var dialConn *net.UDPConn

	var err error

	for {
		if retryCount > 3 {
			break
		}
		time.Sleep(time.Second)
		dialConn, err = net.DialUDP("udp", pconn.LocalAddr, pconn.RemoteAddr)
		if err != nil {
			fmt.Println("请求第", retryCount, "次地址失败", "error:", err.Error())
			retryCount++
			continue
		}
		_, e := dialConn.Write([]byte("HandShake"))
		if e != nil {
			fmt.Println("发送握手请求失败:", e.Error())
			retryCount++
			continue
		}
		// 将打洞阶段的连接关闭
		dialConn.Close()
		break
	}
	if retryCount > 3 {
		panic("客户端连接失败")
	}
	// 等待3s，确保rosagent端已经开始监听
	time.Sleep(time.Second * 3)
	pconn.DialConn, err = net.DialUDP("udp", pconn.LocalAddr, pconn.RemoteAddr)
	if err != nil {
		panic("建立p2p连接失败:" + err.Error())
	}
	fmt.Println("p2p直连成功")
	go pconn.P2PRead()
	go pconn.P2PWrite()

}

// 从对端节点读取数据
func (pconn *P2PConn) P2PRead() {
	for {
		buffer := make([]byte, 1024*1024)
		cnt, addr, err := pconn.DialConn.ReadFromUDP(buffer)
		fmt.Println("对端地址是:", addr.String())
		if err != nil {
			fmt.Println("从对端节点读取数据失败:", err.Error())
			continue
		}
		msg := string(buffer)
		fmt.Printf(">读取到%d个字节,对端节点发来内容:%s\n", cnt, msg)

		if msg == "HandShake" {
			continue
		}
		// 将读取到的内容，写回给浏览器
		err = browserConn.WriteMessage(websocket.TextMessage, []byte(msg))
		if err != nil {
			panic("消息转发给浏览器失败:" + err.Error())
		}
		fmt.Println(">消息转发给浏览器成功")
	}
}

func (pconn *P2PConn) P2PWrite() {
	for {
		var msg string
		fmt.Scanln(&msg)
		_, err := pconn.DialConn.Write([]byte(msg))
		if err != nil {
			panic("消息转发给对端节点失败:" + err.Error())
		}
		fmt.Println("消息转发给对端节点成功")
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
		fmt.Printf(">读取到%d个字节,浏览器发来内容:%s\n", readCnt, string(msg))

		// 将消息转发给对端节点
		writeCnt, err := p2pConn.DialConn.Write([]byte(msg))
		if err != nil {
			panic("消息转发给对端节点失败" + err.Error())
		}
		fmt.Println(">消息转发给对端节点,大小:", writeCnt)

	}
}

func main() {
	/*
		与对端节点建立p2p连接
	*/
	// 生成随机端口
	// localPort := randPort(10000, 50000)
	localPort := 3002

	// 本地地址
	laddr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: int(localPort),
	}

	// 中继服务器地址
	raddr := &net.UDPAddr{
		IP:   net.ParseIP("47.112.96.50"),
		Port: 3001,
	}

	conn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		panic("dial error" + err.Error())
	}
	fmt.Println("成功连接到服务器...")

	sc := &ServerConn{udpConn: conn, LocalAddr: laddr}
	sc.sayHelloToServer()
	sc.HolePucnh()

	/*
		与浏览器建立websocket连接
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
