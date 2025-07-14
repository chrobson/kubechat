package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	pb "kubechat/proto"
)

type Gateway struct {
	usersClient    pb.UsersServiceClient
	chatClient     pb.ChatServiceClient
	presenceClient pb.PresenceServiceClient
	natsConn       *nats.Conn
	clients        map[string]*Client
	clientsMutex   sync.RWMutex
}

type Client struct {
	UserID string
	Conn   *websocket.Conn
	Send   chan []byte
}

type Message struct {
	Type      string      `json:"type"`
	UserID    string      `json:"user_id,omitempty"`
	Content   interface{} `json:"content"`
	Timestamp string      `json:"timestamp,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for demo
	},
}

func (g *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		conn.Close()
		return
	}

	client := &Client{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}

	g.clientsMutex.Lock()
	g.clients[userID] = client
	g.clientsMutex.Unlock()

	// Set user online
	g.presenceClient.SetUserOnline(context.Background(), &pb.SetUserOnlineRequest{
		UserId: userID,
	})

	// Subscribe to user's messages
	g.subscribeToUserMessages(userID)

	go g.writePump(client)
	go g.readPump(client)
}

func (g *Gateway) readPump(client *Client) {
	defer func() {
		g.clientsMutex.Lock()
		delete(g.clients, client.UserID)
		g.clientsMutex.Unlock()

		// Set user offline
		g.presenceClient.SetUserOffline(context.Background(), &pb.SetUserOfflineRequest{
			UserId: client.UserID,
		})

		client.Conn.Close()
	}()

	for {
		var msg Message
		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		g.handleMessage(client, &msg)
	}
}

func (g *Gateway) writePump(client *Client) {
	defer client.Conn.Close()

	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		}
	}
}

func (g *Gateway) handleMessage(client *Client, msg *Message) {
	switch msg.Type {
	case "send_message":
		if content, ok := msg.Content.(map[string]interface{}); ok {
			if recipientID, ok := content["recipient_id"].(string); ok {
				if messageText, ok := content["message"].(string); ok {
					// Send message via chat service
					resp, err := g.chatClient.SendMessage(context.Background(), &pb.SendMessageRequest{
						SenderId:    client.UserID,
						RecipientId: recipientID,
						Message:     messageText,
					})
					if err != nil {
						log.Printf("Failed to send message: %v", err)
					} else {
						log.Printf("Message sent: %s", resp.MessageId)
					}
				}
			}
		}

	case "get_online_users":
		resp, err := g.presenceClient.GetOnlineUsers(context.Background(), &pb.GetOnlineUsersRequest{})
		if err != nil {
			log.Printf("Failed to get online users: %v", err)
			return
		}

		response := Message{
			Type:    "online_users",
			Content: resp.UserIds,
		}

		data, _ := json.Marshal(response)
		client.Send <- data
	}
}

func (g *Gateway) subscribeToUserMessages(userID string) {
	subject := "chat.messages." + userID
	g.natsConn.Subscribe(subject, func(msg *nats.Msg) {
		g.clientsMutex.RLock()
		client, exists := g.clients[userID]
		g.clientsMutex.RUnlock()

		if exists {
			var chatMessage pb.Message
			err := json.Unmarshal(msg.Data, &chatMessage)
			if err != nil {
				log.Printf("Failed to unmarshal chat message: %v", err)
				return
			}

			response := Message{
				Type:    "new_message",
				Content: chatMessage,
			}

			data, _ := json.Marshal(response)
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				g.clientsMutex.Lock()
				delete(g.clients, userID)
				g.clientsMutex.Unlock()
			}
		}
	})
}

func (g *Gateway) subscribeToUserStatus() {
	g.natsConn.Subscribe("users.status", func(msg *nats.Msg) {
		var statusEvent pb.UserStatusEvent
		err := json.Unmarshal(msg.Data, &statusEvent)
		if err != nil {
			log.Printf("Failed to unmarshal status event: %v", err)
			return
		}

		response := Message{
			Type:    "user_status",
			Content: statusEvent,
		}

		data, _ := json.Marshal(response)

		// Broadcast to all connected clients
		g.clientsMutex.RLock()
		for _, client := range g.clients {
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				delete(g.clients, client.UserID)
			}
		}
		g.clientsMutex.RUnlock()
	})
}

func (g *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var loginReq pb.LoginUserRequest
	err := json.NewDecoder(r.Body).Decode(&loginReq)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := g.usersClient.LoginUser(context.Background(), &loginReq)
	if err != nil {
		http.Error(w, "Login failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var createReq pb.CreateUserRequest
	err := json.NewDecoder(r.Body).Decode(&createReq)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := g.usersClient.CreateUser(context.Background(), &createReq)
	if err != nil {
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	// Connect to gRPC services
	usersConn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to users service: %v", err)
	}
	defer usersConn.Close()

	chatConn, err := grpc.Dial("localhost:50053", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to chat service: %v", err)
	}
	defer chatConn.Close()

	presenceConn, err := grpc.Dial("localhost:50052", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to presence service: %v", err)
	}
	defer presenceConn.Close()

	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	gateway := &Gateway{
		usersClient:    pb.NewUsersServiceClient(usersConn),
		chatClient:     pb.NewChatServiceClient(chatConn),
		presenceClient: pb.NewPresenceServiceClient(presenceConn),
		natsConn:       nc,
		clients:        make(map[string]*Client),
	}

	// Subscribe to user status updates
	gateway.subscribeToUserStatus()

	// HTTP routes
	http.HandleFunc("/ws", gateway.handleWebSocket)
	http.HandleFunc("/login", gateway.handleLogin)
	http.HandleFunc("/register", gateway.handleRegister)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "/app/demo/index.html")
	})

	log.Println("API Gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}