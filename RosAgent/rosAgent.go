package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// 与ros_server的连接句柄
var roshandler *RosHandler

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
	n, err := sconn.udpConn.Read(buffer)
	if err != nil {
		fmt.Println("error while reading data")
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

	// 从对端节点收数据
	ListenConn *net.UDPConn

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

	// 收消息连接
	var listenConn *net.UDPConn

	var err error

	// 在连接对方之前，应该先监听自己这边的端口
	listenConn, err = net.ListenUDP("udp", pconn.LocalAddr)
	if err != nil {
		panic("监听p2p连接失败:" + err.Error())
	}
	pconn.ListenConn = listenConn
	for {
		if retryCount > 3 {
			break
		}
		time.Sleep(time.Second)
		dialConn, err = net.DialUDP("udp", pconn.LocalAddr, pconn.RemoteAddr)
		_, e := dialConn.Write([]byte("HandShake"))
		if e != nil {
			fmt.Println("发送握手请求失败:", e.Error())
		}
		if err != nil {
			fmt.Println("请求第", retryCount, "次地址失败", "error:", err.Error())
			retryCount++
			continue
		}
		break
	}
	if retryCount > 3 {
		panic("客户端连接失败")
	}
	pconn.DialConn = dialConn
	fmt.Println("p2p直连成功")
	go pconn.P2PRead()

}

// 从对端节点读取数据
func (pconn *P2PConn) P2PRead() {
	for {
		buffer := make([]byte, 1024*1024)
		cnt, _, err := pconn.ListenConn.ReadFromUDP(buffer)
		if err != nil {
			panic("从对端节点读取数据失败" + err.Error())
		}
		msg := string(buffer)
		fmt.Printf(">读取到%d个字节,对端节点发来内容:%s\n", cnt, msg)

		if msg == "HandShake" {
			continue
		}
		//将内容转发给ros_server
		err = roshandler.RosConn.WriteMessage(websocket.TextMessage, []byte(msg))
		if err != nil {
			panic("转发内容给ros_server失败:" + err.Error())
		}
		fmt.Println("消息转发给ros_server")
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

		// 将读取到的内容，回传给p2p节点
		writeCnt, error := p2pConn.DialConn.Write([]byte(msg))
		if err != nil {
			panic("消息转发给对端节点失败" + error.Error())
		}
		fmt.Println(">消息转发给对端节点成功,大小:", writeCnt)
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
	// 生成随机端口
	localPort := randPort(10000, 50000)

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
		panic("dial error:" + err.Error())
	}
	fmt.Println("成功连接到服务器...")

	sc := &ServerConn{udpConn: conn, LocalAddr: laddr}
	sc.sayHelloToServer()
	sc.HolePucnh()

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
