<template>
  <div class="chat-container">
    <div class="chat-header">
      <h2>Go Chatting</h2>
      <div class="user-controls">
        <input v-model="username" placeholder="Your name" />
        <button @click="rename">Set Name</button>
        <span class="status-indicator" :class="status"></span>
      </div>
    </div>

    <div class="chat-main">
      <div class="user-list">
        <h3>Online Users ({{ users.length }})</h3>
        <button @click="getUserList">Refresh</button>
        <ul>
          <li v-for="user in users" :key="user.addr" :class="[user.addr === currentUserAddr ? 'self' : '', user.state]">
            <span class="user-name">{{ user.name }}</span>
            <span class="user-addr">{{ user.addr }}</span>
            <span class="status-dot"></span>
            <button v-if="user.addr !== currentUserAddr" @click="startPrivateChat(user)">私信</button>
          </li>
        </ul>
      </div>

      <div ref="scrollContainer" class="chat-messages">
        <div v-for="(msg, index) in messages" :key="index" :class="msg.type">
          <template v-if="msg.type === 'system'">
            <span class="system-indicator">[系统]</span> {{ msg.content }}
          </template>
          <template v-else-if="msg.type === 'private'">
            <div>
              <span class="sender">[私信]{{ msg.sender.name }}:</span>
              <p :style="{color: msg.content.slice(0,4)==='[警告]'?'red':'black'}" class="inline_p_and_span">
                {{ msg.content }}
              </p>
            </div>
          </template>
          <template v-else-if="msg.type === 'status'">
            <span class="status-indicator">[状态]</span>
            <span class="user-name">{{ msg.status.name }}</span>
            <span v-if="msg.status.state === 'online'">上线了</span>
            <span v-else-if="msg.status.state === 'offline'">下线了</span>
            <span v-else-if="msg.status.state === 'rename'">改名为 {{ msg.status.name }}</span>
          </template>
          <template v-else>
            <div>
              <span class="sender">{{ msg.sender.name }}:</span>
              <p :style="{color: msg.content.slice(0,4)==='[警告]'?'red':'black'}" class="inline_p_and_span">
                {{ msg.content }}
              </p>
            </div>
          </template>
        </div>
      </div>
    </div>

    <div class="chat-input">
      <div v-if="privateTarget" class="private-indicator">
        私信给: {{ privateTarget.name }}
        <button @click="cancelPrivate">取消</button>
      </div>
      <div class="input_place">
        <input 
          v-model="message" 
          @keyup.enter="sendMessage" 
          :placeholder="privateTarget ? '私信给 ' + privateTarget.name : '输入消息...'" 
        />
      </div>
      <button @click="sendMessage">发送</button>
    </div>
  </div>
</template>

<script>
export default {
  data() {
    return {
      ws: null,
      message: "",
      username: "",
      messages: [],
      users: [],
      status: "disconnected",
      currentUserAddr: "",
      privateTarget: null,
      reconnectAttempts: 0
    };
  },
  mounted() {
    this.connectWebSocket();
    this.username = `User_${Math.floor(Math.random() * 1000)}`;
  },
  methods: {
    connectWebSocket() {
      const host = window.location.hostname;
      this.ws = new WebSocket(`ws://${host}:8081/ws`);
      console.log("已连接到聊天服务器")
      this.ws.onopen = () => {
        this.status = "connected";
        this.reconnectAttempts = 0;
        this.messages.push({
          type: "system",
          content: "已连接到聊天服务器"
        });
        // 设置用户名
        if (this.username) {
          this.$nextTick(() => {
            this.rename();
          });
        }
      };

      this.ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        switch (msg.type) {
          case "userlist":
            console.log("收到消息userlist");
            console.log(msg)
            this.users = msg.users.map(user => ({
              ...user,
              state: "online"
            }));
            break;
          
          case "status":
            if (msg.status) {
              this.updateUserStatus(msg.status);
            }
            break;
          
          case "private":
            this.messages.push({
              type: "private",
              content: msg.content,
              sender: msg.sender
            });
            // 收到私信时自动聚焦到私信发送者
            if ((!this.privateTarget || this.privateTarget.addr !== msg.sender.addr) && msg.sender.name!=this.username) {
              this.privateTarget = {
                name: msg.sender.name,
                addr: msg.sender.addr
              };
            }
            break;
          
          default:
            this.messages.push(msg);
        }
        
        // 自动滚动到底部
        this.$nextTick(() => {
          const container = this.$el.querySelector(".chat-messages");
          if (container) {
            container.scrollTop = container.scrollHeight;
          }
        });
      };

      this.ws.onclose = () => {
        this.status = "disconnected";
        this.messages.push({
          type: "system",
          content: "与服务器断开连接，正在尝试重新连接..."
        });
        
        this.reconnectAttempts++;
        const delay = Math.min(5000, this.reconnectAttempts * 1000);
        setTimeout(() => this.connectWebSocket(), delay);
      };
    },
    
    updateUserStatus(status) {
      const index = this.users.findIndex(u => u.addr === status.addr);
      
      if (status.state === "offline") {
        if (index !== -1) {
          this.users.splice(index, 1);
        }
      } else {
        if (index === -1) {
          this.users.push({
            name: status.name,
            addr: status.addr,
            state: status.state
          });
        } else {
          this.users[index] = {
            ...this.users[index],
            name: status.name,
            state: status.state
          };
        }
      }
      
      // 如果是当前用户的状态更新
      if (status.addr === this.currentUserAddr) {
        this.currentUserAddr = status.addr;
      }
    },
    
    sendMessage() {
      if (!this.message.trim()) return;
      
      const msg = {
        type: "message",
        content: this.message
      };
      
      if (this.privateTarget) {
        msg.type = "private";
        msg.target = this.privateTarget.addr;
      }
      
      this.ws.send(JSON.stringify(msg));
      this.message = "";
    },
    
    rename() {
      if (!this.username.trim()) return;
      
      this.ws.send(JSON.stringify({
        type: "rename",
        content: this.username
      }));
    },
    
    getUserList() {
      this.ws.send(JSON.stringify({
        type: "list"
      }));
    },
    
    startPrivateChat(user) {
      this.privateTarget = {
        name: user.name,
        addr: user.addr
      };
    },
    
    cancelPrivate() {
      this.privateTarget = null;
    }
  }
};
</script>

<style>
.chat-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  max-width: 1200px;
  margin: 0 auto;
  font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
  background-color: #f9f9f9;
  box-shadow: 0 0 10px rgba(0,0,0,0.1);
}

.chat-header {
  background: #2c3e50;
  color: white;
  padding: 1rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
}

.user-controls {
  display: flex;
  gap: 10px;
  align-items: center;
}

.user-controls input {
  padding: 8px;
  border: 1px solid #ddd;
  border-radius: 4px;
}

.user-controls button {
  padding: 8px 15px;
  background: #3498db;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background 0.3s;
}

.user-controls button:hover {
  background: #2980b9;
}

.status-indicator {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  margin-left: 10px;
}

.status-indicator.connected {
  background: #2ecc71;
}

.status-indicator.disconnected {
  background: #e74c3c;
}

.chat-main {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.user-list {
  width: 280px;
  background: white;
  padding: 1rem;
  border-right: 1px solid #eee;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.user-list h3 {
  margin-top: 0;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.user-list button {
  padding: 5px 10px;
  font-size: 0.9rem;
}

.user-list ul {
  list-style: none;
  padding: 0;
  margin: 10px 0 0 0;
  flex: 1;
  overflow-y: auto;
}
.user-list li {
  padding: 10px;
  margin-bottom: 8px;
  border-radius: 4px;
  background: #f5f7fa;
  display: flex;
  flex-direction: column;
  position: relative;
  transition: all 0.3s ease;
}

.user-list li.self {
  background: #e3f2fd;
  border-left: 3px solid #2196f3;
}

.user-list li.rename {
  animation: highlight 2s;
}

.user-name {
  font-weight: bold;
  margin-bottom: 4px;
}

.user-addr {
  font-size: 0.8rem;
  color: #777;
}

.status-dot {
  position: absolute;
  top: 10px;
  right: 10px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #2ecc71;
}

.user-list li button {
  margin-top: 8px;
  padding: 4px 8px;
  font-size: 0.8rem;
  background: #9b59b6;
  color: white;
  border: none;
  border-radius: 3px;
  cursor: pointer;
}

.user-list li button:hover {
  background: #8e44ad;
}

@keyframes highlight {
  0% { background-color: rgba(255, 255, 0, 0.3); }
  100% { background-color: transparent; }
}

.chat-messages{
  height: auto;
  width: 800px;
  overflow-y: auto;
}

.chat-messages .sender {
  font-weight: bold;
  color: #2c3e50;
  margin-right: 8px;
}

.system-indicator,
.private-indicator,
.status-indicator {
  font-weight: bold;
  margin-right: 5px;
}

.private-indicator {
  color: #9b59b6;
}

.chat-input{
  margin: 0 auto;
  background: #f5e6ff;
  padding: 8px;
  border-radius: 4px;
  margin-bottom: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.private-indicator {
  margin: 0 auto;
  background: #f5e6ff;
  padding: 8px;
  border-radius: 4px;

  display: flex;
  justify-content: space-between;
  align-items: center;
}
.input_place{
  padding: 9px;
}
.chat-input button{
  padding: 3px 8px;
  font-size: 0.8rem;
}
.private-indicator button {
  padding: 3px 8px;
  font-size: 0.8rem;
}
.inline_p_and_span{
  display: inline-block;
  margin: 0;
}
</style>

