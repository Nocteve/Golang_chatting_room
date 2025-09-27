package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	//"strings"
	"sync"
	"bytes"
	"github.com/gorilla/websocket"
)

type Request struct {
    Text string `json:"text"`
}

type Response struct {
    Embeddings string `json:"embeddings"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type (
	ClientMessage struct {
		Type    string `json:"type"` // "message", "rename", "list", "private"
		Content string `json:"content"`
		Target  string `json:"target,omitempty"` // For private messages
	}

	ServerMessage struct {
		Type    string      `json:"type"` // "system", "chat", "list", "error", "status", "private"
		Content string      `json:"content,omitempty"`
		Users   []User      `json:"users,omitempty"`
		Status  *UserStatus `json:"status,omitempty"`
		Sender  *UserStatus `json:"sender,omitempty"`
	}

	User struct {
		Name string `json:"name"`
		Addr string `json:"addr"`
	}

	UserStatus struct {
		Name  string `json:"name"`
		Addr  string `json:"addr"`
		State string `json:"state"` // online, offline, rename
	}

	client struct {
		conn     *websocket.Conn
		name     string
		//addr     string
		server   net.Conn
		serverIP string
	}
)

var (
	clients   = make(map[*client]bool)
	users     = make(map[string]UserStatus) // 全局用户状态跟踪
	userMutex sync.Mutex
	mutex     sync.Mutex
)
func checker(msg string) string{
	// 准备请求数据
    //reqData := Request{Text: "你好"}
	fmt.Println(msg)
	reqData := Request{Text: msg}
    jsonData, err := json.Marshal(reqData)
    if err != nil {
        panic(err)
    }

    // 发送POST请求
    resp, err := http.Post("http://localhost:8000/predict", 
                          "application/json", 
                          bytes.NewBuffer(jsonData))
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    // 解析响应
    var result Response
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        panic(err)
    }

    fmt.Printf("BERT Embeddings: %v\n", result.Embeddings)

	return result.Embeddings
}
func main() {
	checker("你好")
	if len(os.Args) < 2 {
		fmt.Println("Usage: client <server-ip:port>")
		return
	}

	//serverAddr := os.Args[1]
	http.HandleFunc("/ws", handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./dist")))

	go func() {
		log.Println("HTTP server started on :8081")
		err := http.ListenAndServe(":8081", nil)
		if err != nil {
			log.Fatal("HTTP server error: ", err)
		}
	}()

	select {}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	serverConn, err := net.Dial("tcp", os.Args[1])
	if err != nil {
		log.Println("Server connection error:", err)
		ws.Close()
		return
	}

	client := &client{
		conn:     ws,
		server:   serverConn,
		serverIP: os.Args[1],
	}

	mutex.Lock()
	clients[client] = true
	mutex.Unlock()

	go client.readFromServer()
	go client.readFromWebSocket()

	// 发送欢迎消息
	welcomeMsg := ServerMessage{
		Type:    "system",
		Content: "Connected to chat server at " + os.Args[1],
	}
	client.sendToWebSocket(welcomeMsg)
}

func (c *client) readFromServer() {
	defer c.cleanup()

	scanner := bufio.NewScanner(c.server)
	for scanner.Scan() {
		text := scanner.Text()
		log.Printf("Received from server: %s", text)

		var msgObj map[string]interface{}
		if err := json.Unmarshal([]byte(text), &msgObj); err != nil {
			log.Println("JSON unmarshal error:", err)
			continue
		}

		msgType, ok := msgObj["type"].(string)
		if !ok {
			log.Println("Missing or invalid message type")
			continue
		}

		msg := ServerMessage{Type: msgType}

		switch msgType {
		case "userlist":
			users, ok := msgObj["users"].([]interface{})
			fmt.Println(msgObj["users"])
			if ok {
				fmt.Println("yes")
				for _, u := range users {
					userMap, ok := u.(map[string]interface{})
					if !ok {
						log.Println("Invalid user data format")
						continue
					}

					// 安全处理 name 字段
					name, nameOk := "", false
					if nameVal, ok := userMap["Name"]; ok {
						name, nameOk = nameVal.(string)
					}
					if !nameOk {
						name = "Unknown"
					}

					// 安全处理 addr 字段
					addr, addrOk := "", false
					if addrVal, ok := userMap["Addr"]; ok {
						addr, addrOk = addrVal.(string)
					}
					if !addrOk {
						addr = "Unknown"
					}

					user := User{
						Name: name,
						Addr: addr,
					}
					msg.Users = append(msg.Users, user)
					updateUserStatus(UserStatus{
						Name:  name,
						Addr:  addr,
						State: "online",
					})
				}
			}
		case "status":
			if status, ok := msgObj["status"].(map[string]interface{}); ok {
				// 安全处理所有状态字段
				name, _ := status["name"].(string)
				addr, _ := status["addr"].(string)
				state, _ := status["state"].(string)
				
				userStatus := UserStatus{
					Name:  name,
					Addr:  addr,
					State: state,
				}
				msg.Status = &userStatus
				updateUserStatus(userStatus)
			}
		case "message", "private":
			if content, ok := msgObj["content"].(string); ok {
				check_msg:=checker(content)
				msg.Content = content
				if check_msg!="正常"{
					msg.Content="[警告]-"+check_msg+"-"+msg.Content
				}
			}
			if sender, ok := msgObj["status"].(map[string]interface{}); ok {
				// 安全处理发送者信息
				name, _ := sender["name"].(string)
				addr, _ := sender["addr"].(string)
				
				msg.Sender = &UserStatus{
					Name:  name,
					Addr:  addr,
					State: "online",
				}
			}
		default:
			if content, ok := msgObj["content"].(string); ok {
				msg.Content = content
			}
		}
		c.sendToWebSocket(msg)
	}
}

func updateUserStatus(status UserStatus) {
	userMutex.Lock()
	defer userMutex.Unlock()

	switch status.State {
	case "online", "rename":
		users[status.Addr] = status
	case "offline":
		delete(users, status.Addr)
	}
}

func (c *client) readFromWebSocket() {
	defer c.cleanup()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var clientMsg ClientMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			log.Println("JSON unmarshal error:", err)
			continue
		}

		switch clientMsg.Type {
		case "message":
			c.server.Write([]byte(clientMsg.Content + "\n"))
		case "rename":
			c.server.Write([]byte("rename#" + clientMsg.Content + "\n"))
			c.name = clientMsg.Content
		case "list":
			c.server.Write([]byte("online\n"))
		case "private":
			cmd := "to#" + clientMsg.Target + "#" + clientMsg.Content
			c.server.Write([]byte(cmd + "\n"))
		}
	}
}

func (c *client) sendToWebSocket(msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("JSON marshal error:", err)
		return
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Println("WebSocket write error:", err)
		c.cleanup()
	}
}

func (c *client) cleanup() {
	c.conn.Close()
	c.server.Close()
	mutex.Lock()
	delete(clients, c)
	mutex.Unlock()
}