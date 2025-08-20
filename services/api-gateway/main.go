package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	chat "kubechat/proto/chat"
	presence "kubechat/proto/presence"
	users "kubechat/proto/users"
)

type Gateway struct {
	usersClient    users.UsersServiceClient
	chatClient     chat.ChatServiceClient
	presenceClient presence.PresenceServiceClient
	natsConn       *nats.Conn
	clients        map[string]*Client
	clientsMutex   sync.RWMutex
}

type Client struct {
	UserID       string
	Conn         *websocket.Conn
	Send         chan []byte
	Subscription *nats.Subscription
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

	// Subscribe to user's messages before adding to clients map
	subscription := g.subscribeToUserMessages(userID, client)
	client.Subscription = subscription

	g.clientsMutex.Lock()
	// Check if user already has a connection and clean it up
	if existingClient, exists := g.clients[userID]; exists {
		existingClient.Conn.Close()
		if existingClient.Subscription != nil {
			existingClient.Subscription.Unsubscribe()
		}
	}
	g.clients[userID] = client
	g.clientsMutex.Unlock()

	// Set user online
	g.presenceClient.SetUserOnline(context.Background(), &presence.SetUserOnlineRequest{
		UserId: userID,
	})

	go g.writePump(client)
	go g.readPump(client)
}

func (g *Gateway) readPump(client *Client) {
	defer func() {
		g.clientsMutex.Lock()
		delete(g.clients, client.UserID)
		g.clientsMutex.Unlock()

		// Unsubscribe from NATS messages
		if client.Subscription != nil {
			client.Subscription.Unsubscribe()
		}

		// Set user offline
		g.presenceClient.SetUserOffline(context.Background(), &presence.SetUserOfflineRequest{
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
					resp, err := g.chatClient.SendMessage(context.Background(), &chat.SendMessageRequest{
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
		resp, err := g.presenceClient.GetOnlineUsers(context.Background(), &presence.GetOnlineUsersRequest{})
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

func (g *Gateway) subscribeToUserMessages(userID string, client *Client) *nats.Subscription {
	subject := "chat.messages." + userID
	sub, err := g.natsConn.Subscribe(subject, func(msg *nats.Msg) {
		g.clientsMutex.RLock()
		currentClient, exists := g.clients[userID]
		g.clientsMutex.RUnlock()

		// Only process if this is still the current client for this user
		if exists && currentClient == client {
			var chatMessage chat.Message
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

	if err != nil {
		log.Printf("Failed to subscribe to user messages: %v", err)
		return nil
	}

	return sub
}

func (g *Gateway) subscribeToUserStatus() {
	g.natsConn.Subscribe("users.status", func(msg *nats.Msg) {
		var statusEvent presence.UserStatusEvent
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
				// Avoid map write under read lock; removal is handled on client tear-down
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

	var loginReq users.LoginUserRequest
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

	var createReq users.CreateUserRequest
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

func (g *Gateway) handleGetUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from URL path
	path := r.URL.Path
	if len(path) <= 6 { // "/user/" is 6 characters
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}
	userID := path[6:] // Remove "/user/" prefix

	resp, err := g.usersClient.GetUser(context.Background(), &users.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) handleGetChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract parameters from query string
	userID1 := r.URL.Query().Get("user1")
	userID2 := r.URL.Query().Get("user2")
	
	if userID1 == "" || userID2 == "" {
		http.Error(w, "Both user1 and user2 parameters required", http.StatusBadRequest)
		return
	}

	// Get chat history from chat service
	resp, err := g.chatClient.GetMessageHistory(context.Background(), &chat.GetMessageHistoryRequest{
		UserId1: userID1,
		UserId2: userID2,
		Limit:   50, // Default limit
		Offset:  0,
	})
	if err != nil {
		log.Printf("Failed to get chat history: %v", err)
		http.Error(w, "Failed to get chat history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	// Connect to gRPC services
	usersURL := os.Getenv("USERS_SERVICE_URL")
	if usersURL == "" {
		usersURL = "localhost:50051"
	}
	usersConn, err := grpc.NewClient(usersURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to users service: %v", err)
	}
	defer usersConn.Close()

	chatURL := os.Getenv("CHAT_SERVICE_URL")
	if chatURL == "" {
		chatURL = "localhost:50053"
	}
	chatConn, err := grpc.NewClient(chatURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to chat service: %v", err)
	}
	defer chatConn.Close()

	presenceURL := os.Getenv("PRESENCE_SERVICE_URL")
	if presenceURL == "" {
		presenceURL = "localhost:50052"
	}
	presenceConn, err := grpc.NewClient(presenceURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to presence service: %v", err)
	}
	defer presenceConn.Close()

	// Connect to NATS
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	gateway := &Gateway{
		usersClient:    users.NewUsersServiceClient(usersConn),
		chatClient:     chat.NewChatServiceClient(chatConn),
		presenceClient: presence.NewPresenceServiceClient(presenceConn),
		natsConn:       nc,
		clients:        make(map[string]*Client),
	}

	// Subscribe to user status updates
	gateway.subscribeToUserStatus()

	// HTTP routes
	http.HandleFunc("/ws", gateway.handleWebSocket)
	http.HandleFunc("/login", gateway.handleLogin)
	http.HandleFunc("/register", gateway.handleRegister)
	http.HandleFunc("/user/", gateway.handleGetUser)
	http.HandleFunc("/chat/history", gateway.handleGetChatHistory)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "/app/demo/index.html")
	})

	log.Println("API Gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
