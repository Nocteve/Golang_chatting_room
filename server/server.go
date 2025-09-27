package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type (
	client struct {
		c    chan string
		name string
		addr string
	}
)

var (
	onlineUser  = make(map[string]client) // 在线用户
	messageList = make(chan string)       // 消息列表
	mu          sync.Mutex                // 保护onlineUser的互斥锁
)

// 新增：用户状态结构
type UserStatus struct {
	Name  string `json:"name"`
	Addr  string `json:"addr"`
	State string `json:"state"` // online, offline
}
type User struct{
	Name string 
	Addr string 
}

// 新增：统一消息格式
type Message struct {
	Type    string      `json:"type"` // "message", "userlist", "status"
	Content string      `json:"content,omitempty"`
	Users   []User      `json:"users,omitempty"`
	Status  *UserStatus `json:"status,omitempty"`
}

func whiteMsgToUser(user client, conn net.Conn) {
	for msg := range user.c {
		_, err := conn.Write([]byte(msg + "\n"))
		if err != nil {
			fmt.Printf("发送消息给用户 %s 失败: %v\n", user.name, err)
			return
		}
	}
}

// func makeMsg(user client, msg string) string {
// 	return fmt.Sprintf("[%s]{%s}:%s", user.addr, user.name, msg)
// }

// 新增：生成结构化消息
func makeStructuredMsg(msgType, content string, users []User, status *UserStatus) string {
	msg := Message{
		Type:    msgType,
		Content: content,
		Users:   users,
		Status:  status,
	}
	jsonData, _ := json.Marshal(msg)
	return string(jsonData)
}

func contentHandler(conn net.Conn) {
	defer conn.Close()
	addr := conn.RemoteAddr().String()
	
	// 新增：用户上线状态通知
	notifyUserStatus := func(name, addr, state string) {
		status := &UserStatus{
			Name:  name,
			Addr:  addr,
			State: state,
		}
		msg := makeStructuredMsg("status", "", nil, status)
		messageList <- msg
	}

	user := client{
		c:    make(chan string, 10),
		name: addr,
		addr: addr,
	}

	go whiteMsgToUser(user, conn)

	mu.Lock()
	onlineUser[addr] = user
	mu.Unlock()

	// 发送用户上线通知
	notifyUserStatus(user.name, user.addr, "online")

	quitList := make(chan bool)
	activeStatus := make(chan bool)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if n == 0 {
				quitList <- true
				fmt.Printf("用户 {%s} 主动退出\n", user.name)
				return
			}
			if err != nil {
				fmt.Println("conn.Read err:", err)
				quitList <- true
				return
			}

			msg := strings.TrimSpace(string(buf[:n]))
			if msg == "online" {
				// 构建用户列表
				userList := make([]User, 0)
				mu.Lock()
				for _, v := range onlineUser {
					userList = append(userList, User{
						Name: v.name,
						Addr: v.addr,
					})
				}
				mu.Unlock()
				
				// 发送结构化用户列表
				userMsg := makeStructuredMsg("userlist", "", userList, nil)
				conn.Write([]byte(userMsg + "\n"))
			} else if len(msg) > 6 && msg[:6] == "rename" {
				if len(msg) < 8 || msg[6] != '#' {
					conn.Write([]byte("改名格式错误，正确格式: rename#新名字\n"))
					activeStatus <- true
					continue
				}
				newName := msg[7:]
				oldName := user.name
				user.name = newName

				mu.Lock()
				onlineUser[user.addr] = user
				mu.Unlock()

				// 发送改名通知
				notifyUserStatus(oldName, user.addr, "rename")
				notifyUserStatus(newName, user.addr, "online")
			} else if len(msg) > 3 && msg[:3] == "to#" {
				content := strings.Split(msg, "#")
				if len(content) < 3 {
					conn.Write([]byte("私信格式错误，正确格式: to#目标地址#消息\n"))
					activeStatus <- true
					continue
				}
				targetAddr := content[1]
				privateMsg := makeStructuredMsg("private", content[2], nil, &UserStatus{
					Name: user.name,
					Addr: user.addr,
				})

				mu.Lock()
				targetUser, exists := onlineUser[targetAddr]
				mu.Unlock()

				if exists {
					targetUser.c <- privateMsg
					user.c <- privateMsg
				} else {
					conn.Write([]byte("目标用户不存在或已离线\n"))
				}
			} else {
				// 发送普通消息
				msg := makeStructuredMsg("message", msg, nil, &UserStatus{
					Name: user.name,
					Addr: user.addr,
				})
				messageList <- msg
			}
			activeStatus <- true
		}
	}()

	ticker := time.NewTicker(600 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-quitList:
			mu.Lock()
			delete(onlineUser, user.addr)
			mu.Unlock()
			notifyUserStatus(user.name, user.addr, "offline")
			return
		case <-activeStatus:
			ticker.Reset(600 * time.Second)
		case <-ticker.C:
			mu.Lock()
			delete(onlineUser, user.addr)
			mu.Unlock()
			notifyUserStatus(user.name, user.addr, "offline")
			return
		}
	}
}

func manager() {
	for {
		msg := <-messageList
		mu.Lock()
		for _, v := range onlineUser {
			select {
			case v.c <- msg:
			default:
				fmt.Printf("用户 %s 的消息通道已满，丢弃消息: %s\n", v.name, msg)
			}
		}
		mu.Unlock()
	}
}

func main() {
	listen, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		fmt.Println("Listen Err:", err)
		return
	}
	defer listen.Close()
	fmt.Println("服务端启动成功，监听地址：127.0.0.1:8080")

	go manager()

	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println("Accept Err:", err)
			continue
		}
		go contentHandler(conn)
	}
}