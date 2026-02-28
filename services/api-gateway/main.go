package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
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
	jwtSecret      []byte
}

var (
	wsConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gateway_ws_connections",
		Help: "Current number of active WebSocket connections",
	})
	wsMessages = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gateway_ws_messages_total",
		Help: "Total number of WS messages sent to clients",
	})
)

type Client struct {
	UserID        string
	Conn          *websocket.Conn
	Send          chan []byte
	Subscriptions []*nats.Subscription
	Limiter       *rate.Limiter
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

func (g *Gateway) verifyToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return g.jwtSecret, nil
	})

	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if userID, ok := claims["user_id"].(string); ok {
			return userID, nil
		}
	}

	return "", fmt.Errorf("invalid token claims")
}

func (g *Gateway) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header or query param
		authHeader := r.Header.Get("Authorization")
		tokenString := ""

		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		} else {
			tokenString = r.URL.Query().Get("token")
		}

		if tokenString == "" {
			http.Error(w, "Unauthorized: Token required", http.StatusUnauthorized)
			return
		}

		userID, err := g.verifyToken(tokenString)
		if err != nil {
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		// Add userID to context
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (g *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token required", http.StatusUnauthorized)
		return
	}

	userID, err := g.verifyToken(token)
	if err != nil {
		log.Printf("Token verification failed: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		UserID:  userID,
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 5), // 5 msg/s burst
	}
	wsConnections.Inc()

	// Subscribe to user's messages and events before adding to clients map
	msgSub := g.subscribeToUserMessages(userID, client)
	eventSub := g.subscribeToUserEvents(userID, client)
	client.Subscriptions = []*nats.Subscription{msgSub, eventSub}

	g.clientsMutex.Lock()
	// Check if user already has a connection and clean it up
	if existingClient, exists := g.clients[userID]; exists {
		existingClient.Conn.Close()
		for _, sub := range existingClient.Subscriptions {
			if sub != nil {
				sub.Unsubscribe()
			}
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
		for _, sub := range client.Subscriptions {
			if sub != nil {
				sub.Unsubscribe()
			}
		}

		// Set user offline
		g.presenceClient.SetUserOffline(context.Background(), &presence.SetUserOfflineRequest{
			UserId: client.UserID,
		})

		client.Conn.Close()
		wsConnections.Dec()
	}()

	for {
		var msg Message
		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		// Rate limiting
		if !client.Limiter.Allow() {
			log.Printf("Rate limit exceeded for user %s", client.UserID)
			continue
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

	case "typing_event":
		if content, ok := msg.Content.(map[string]interface{}); ok {
			recipientID, _ := content["recipient_id"].(string)
			isTyping, _ := content["is_typing"].(bool)
			
			event := chat.TypingEvent{
				SenderId:    client.UserID,
				RecipientId: recipientID,
				IsTyping:    isTyping,
			}
			
			data, _ := json.Marshal(event)
			g.natsConn.Publish("chat.events."+recipientID, data)
		}

	case "read_receipt":
		if content, ok := msg.Content.(map[string]interface{}); ok {
			recipientID, _ := content["recipient_id"].(string)
			messageID, _ := content["message_id"].(string)
			
			receipt := chat.ReadReceipt{
				SenderId:    client.UserID,
				RecipientId: recipientID,
				MessageId:   messageID,
			}
			
			data, _ := json.Marshal(receipt)
			g.natsConn.Publish("chat.events."+recipientID, data)
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
		wsMessages.Inc()
	}
}

func (g *Gateway) subscribeToUserEvents(userID string, client *Client) *nats.Subscription {
	subject := "chat.events." + userID
	sub, err := g.natsConn.Subscribe(subject, func(msg *nats.Msg) {
		g.clientsMutex.RLock()
		currentClient, exists := g.clients[userID]
		g.clientsMutex.RUnlock()

		if exists && currentClient == client {
			// Determine event type from JSON content
			var raw map[string]interface{}
			json.Unmarshal(msg.Data, &raw)
			
			var response Message
			if _, ok := raw["is_typing"]; ok {
				var event chat.TypingEvent
				json.Unmarshal(msg.Data, &event)
				response = Message{
					Type:    "typing_event",
					Content: event,
				}
			} else if _, ok := raw["message_id"]; ok {
				var receipt chat.ReadReceipt
				json.Unmarshal(msg.Data, &receipt)
				response = Message{
					Type:    "read_receipt",
					Content: receipt,
				}
			}
			
			data, _ := json.Marshal(response)
			select {
			case client.Send <- data:
			default:
				// Channel probably closed
			}
		}
	})

	if err != nil {
		log.Printf("Failed to subscribe to user events: %v", err)
		return nil
	}

	return sub
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

	// Verify that the authenticated user is one of the participants
	authUserID, _ := r.Context().Value("user_id").(string)
	if userID1 != authUserID && userID2 != authUserID {
		http.Error(w, "Forbidden: You can only access your own chat history", http.StatusForbidden)
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
	// Metrics
	prometheus.MustRegister(wsConnections, wsMessages)
	metricsAddr := os.Getenv("METRICS_ADDR_GATEWAY")
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Printf("Gateway metrics on %s", metricsAddr)
		_ = http.ListenAndServe(metricsAddr, nil)
	}()
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

	// JWT secret
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Println("WARNING: JWT_SECRET not set; using default for demo")
		secret = "default_secret_change_me"
	}

	gateway := &Gateway{
		usersClient:    users.NewUsersServiceClient(usersConn),
		chatClient:     chat.NewChatServiceClient(chatConn),
		presenceClient: presence.NewPresenceServiceClient(presenceConn),
		natsConn:       nc,
		clients:        make(map[string]*Client),
		jwtSecret:      []byte(secret),
	}

	// Subscribe to user status updates
	gateway.subscribeToUserStatus()

	// HTTP routes
	http.HandleFunc("/ws", gateway.handleWebSocket)
	http.HandleFunc("/login", gateway.handleLogin)
	http.HandleFunc("/register", gateway.handleRegister)
	http.HandleFunc("/user/", gateway.authMiddleware(gateway.handleGetUser))
	http.HandleFunc("/chat/history", gateway.authMiddleware(gateway.handleGetChatHistory))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "/app/demo/index.html")
	})

	log.Println("API Gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
