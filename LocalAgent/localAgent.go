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

	"P2PAgent/utils"

	"github.com/gorilla/websocket"
	"github.com/libp2p/go-reuseport"
)

/*
**自定义的tcp报文包头格式：length:{num}
**length后，是数据的总长度，后面填充空格，使得填充到18个字节
 */

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

	// 需要读取的报文长度
	remain_cnt int
}

// 等待服务器回传我们的uuid
func (s *Handler) getUUID() string {
	buffer := make([]byte, 1024)
	n, err := s.ServerConn.Read(buffer)
	if err != nil {
		panic("读取失败" + err.Error())
	}
	data := make(map[string]string)
	if err := json.Unmarshal(buffer[:n], &data); err != nil {
		panic("获取uuid失败" + err.Error())
	}
	return data["uuid"]
}

func (s *Handler) requestForAddr(uuid string) {
	var data = make(map[string]string)
	data["targetUUID"] = uuid // 目标uuid
	body, _ := json.Marshal(data)
	_, err := s.ServerConn.Write(body)
	if err != nil {
		panic("发送requestForAddr请求失败" + err.Error())
	}
	fmt.Println("向服务器发送请求，获取uuid为:", uuid, "的地址")
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
	go s.DailP2PAndSayHello(data["address"])
}

// DailP2PAndSayHello 连接对方临时的公网地址,并且不停的发送数据
func (s *Handler) DailP2PAndSayHello(address string) {
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

				// 将读取到的内容，写回给浏览器
				err := browserConn.WriteMessage(websocket.TextMessage, []byte(content))
				if err != nil {
					panic("消息转发给浏览器失败:" + err.Error())
				}
				fmt.Println("消息转发给浏览器成功")
				// 将remain_cnt 归零
				s.remain_cnt = 0
			}
		}
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

		// 自定义的包头
		data_head := fmt.Sprintf("length:%-11d", readCnt)

		// 将包头和数据主体拼接
		body := string(msg)
		content := data_head + body

		fmt.Println(content)
		// 将消息转发给对端节点
		writeCnt, err := handler.P2PConn.Write([]byte(content))
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
	handler = &Handler{ServerConn: serverConn, LocalPort: int(localPort), remain_cnt: 0}

	// 获取uuid
	uuid := handler.getUUID()
	fmt.Println("uuid:", uuid)

	var uid string
	fmt.Scanln(&uid)
	// 请求获取指定uuid的地址
	handler.requestForAddr(uid)

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
