package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	conn, error := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity
	if error != nil {
		panic("websocket请求建立失败:" + error.Error())
	}

	fmt.Println("websocket连接建立成功，对端地址：", conn.RemoteAddr())
	for {
		// Read message from browser
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// Print the message to the console
		fmt.Printf("%s sent: %s\n", conn.RemoteAddr(), string(msg))

		// Write message back to browser
		if err = conn.WriteMessage(msgType, msg); err != nil {
			return
		}
	}
}

func main() {
	fmt.Println("localAgent is listening on 3001")
	http.HandleFunc("/echo", httpHandler)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "websocket.html")
	})
	http.ListenAndServe(":3001", nil)
}
