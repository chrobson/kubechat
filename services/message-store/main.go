package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	chat "kubechat/proto/chat"
	messagestore "kubechat/proto/messagestore"
)

type server struct {
	messagestore.UnimplementedMessageStoreServiceServer
	db       *sql.DB
	natsConn *nats.Conn
}

var (
	dbQueryLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "messagestore_db_query_seconds",
		Help:    "Latency of DB queries",
		Buckets: prometheus.DefBuckets,
	})
)

func (s *server) StoreMessage(ctx context.Context, req *messagestore.StoreMessageRequest) (*messagestore.StoreMessageResponse, error) {
	// If database is not available, just log and return success
	if s.db == nil {
		log.Printf("Database not available, message not persisted: %s from %s to %s",
			req.Content, req.SenderId, req.RecipientId)
		return &messagestore.StoreMessageResponse{
			Success: true,
		}, nil
	}

	query := `
		INSERT INTO messages (message_id, sender_id, recipient_id, content, timestamp, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := s.db.ExecContext(ctx, query,
		req.MessageId,
		req.SenderId,
		req.RecipientId,
		req.Content,
		req.Timestamp.AsTime(),
		time.Now(),
	)

	if err != nil {
		log.Printf("Failed to store message in database: %v", err)
		return &messagestore.StoreMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return &messagestore.StoreMessageResponse{
		Success: true,
	}, nil
}

func (s *server) GetMessageHistory(ctx context.Context, req *messagestore.GetMessageHistoryRequest) (*messagestore.GetMessageHistoryResponse, error) {
	// If database is not available, return empty history
	if s.db == nil {
		log.Printf("Database not available, returning empty message history")
		return &messagestore.GetMessageHistoryResponse{
			Messages: []*messagestore.StoredMessage{},
		}, nil
	}

	query := `
		SELECT message_id, sender_id, recipient_id, content, timestamp, created_at
		FROM messages
		WHERE (sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1)
		ORDER BY timestamp DESC
		LIMIT $3 OFFSET $4`

	limit := req.Limit
	if limit == 0 {
		limit = 50 // Default limit
	}

	qStart := time.Now()
	rows, err := s.db.QueryContext(ctx, query, req.UserId1, req.UserId2, limit, req.Offset)
	if err != nil {
		log.Printf("Failed to query message history: %v", err)
		return &messagestore.GetMessageHistoryResponse{
			Messages: []*messagestore.StoredMessage{},
		}, nil
	}
	defer rows.Close()
	dbQueryLatency.Observe(time.Since(qStart).Seconds())

	var messages []*messagestore.StoredMessage
	for rows.Next() {
		var msg messagestore.StoredMessage
		var timestamp, createdAt time.Time

		err := rows.Scan(
			&msg.MessageId,
			&msg.SenderId,
			&msg.RecipientId,
			&msg.Content,
			&timestamp,
			&createdAt,
		)
		if err != nil {
			log.Printf("Error scanning message: %v", err)
			continue
		}

		msg.Timestamp = timestamppb.New(timestamp)
		msg.CreatedAt = timestamppb.New(createdAt)
		messages = append(messages, &msg)
	}

	return &messagestore.GetMessageHistoryResponse{
		Messages: messages,
	}, nil
}

func (s *server) DeleteMessage(ctx context.Context, req *messagestore.DeleteMessageRequest) (*messagestore.DeleteMessageResponse, error) {
	query := `DELETE FROM messages WHERE message_id = $1 AND sender_id = $2`

	result, err := s.db.ExecContext(ctx, query, req.MessageId, req.UserId)
	if err != nil {
		return &messagestore.DeleteMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return &messagestore.DeleteMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	if rowsAffected == 0 {
		return &messagestore.DeleteMessageResponse{
			Success: false,
			Error:   "Message not found or not authorized to delete",
		}, nil
	}

	return &messagestore.DeleteMessageResponse{
		Success: true,
	}, nil
}

func (s *server) subscribeToMessages() {
	_, err := s.natsConn.Subscribe("chat.messages.*", func(msg *nats.Msg) {
		var message chat.Message
		err := json.Unmarshal(msg.Data, &message)
		if err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			return
		}

		// Store the message
		storeReq := &messagestore.StoreMessageRequest{
			MessageId:   message.MessageId,
			SenderId:    message.SenderId,
			RecipientId: message.RecipientId,
			Content:     message.Content,
			Timestamp:   message.Timestamp,
		}

		_, err = s.StoreMessage(context.Background(), storeReq)
		if err != nil {
			log.Printf("Failed to store message from NATS: %v", err)
		}
	})

	if err != nil {
		log.Printf("Failed to subscribe to messages: %v", err)
	} else {
		log.Println("Subscribed to chat messages from NATS")
	}
}

func initDB() (*sql.DB, error) {
	// Get database connection string from environment
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "postgres" // Docker service name
	}

	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}

	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "user"
	}

	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		dbPassword = "password"
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "kubechat"
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	log.Printf("Attempting to connect to database: %s", connStr)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Failed to open database connection: %v", err)
		return nil, err
	}

	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Printf("Failed to ping database: %v", err)
		db.Close()
		return nil, err
	}

	// Create messages table
	createTableQuery := `
		CREATE TABLE IF NOT EXISTS messages (
			id SERIAL PRIMARY KEY,
			message_id VARCHAR(255) UNIQUE NOT NULL,
			sender_id VARCHAR(255) NOT NULL,
			recipient_id VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			timestamp TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`

	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Printf("Failed to create table: %v", err)
		db.Close()
		return nil, err
	}

	log.Println("Successfully connected to database and created tables")
	return db, nil
}

func main() {
	// Metrics
	prometheus.MustRegister(dbQueryLatency)
	metricsAddr := os.Getenv("METRICS_ADDR_STORE")
	if metricsAddr == "" {
		metricsAddr = ":9093"
	}
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Printf("Message-store metrics on %s", metricsAddr)
		_ = http.ListenAndServe(metricsAddr, nil)
	}()

	// Connect to database
	db, err := initDB()
	if err != nil {
		log.Printf("Database connection failed, running without persistence: %v", err)
	}

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

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	messageStoreServer := &server{
		db:       db,
		natsConn: nc,
	}

	// Subscribe to incoming messages from NATS
	if nc != nil {
		go messageStoreServer.subscribeToMessages()
	}

	messagestore.RegisterMessageStoreServiceServer(s, messageStoreServer)

	log.Println("Message store service listening on :50054")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
