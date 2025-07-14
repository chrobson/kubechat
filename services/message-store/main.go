package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/nats-io/nats.go"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "kubechat/proto"
)

type server struct {
	pb.UnimplementedMessageStoreServiceServer
	db       *sql.DB
	natsConn *nats.Conn
}

func (s *server) StoreMessage(ctx context.Context, req *pb.StoreMessageRequest) (*pb.StoreMessageResponse, error) {
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
		return &pb.StoreMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return &pb.StoreMessageResponse{
		Success: true,
	}, nil
}

func (s *server) GetMessageHistory(ctx context.Context, req *pb.GetMessageHistoryRequest) (*pb.GetMessageHistoryResponse, error) {
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

	rows, err := s.db.QueryContext(ctx, query, req.UserId1, req.UserId2, limit, req.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*pb.StoredMessage
	for rows.Next() {
		var msg pb.StoredMessage
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

	return &pb.GetMessageHistoryResponse{
		Messages: messages,
	}, nil
}

func (s *server) DeleteMessage(ctx context.Context, req *pb.DeleteMessageRequest) (*pb.DeleteMessageResponse, error) {
	query := `DELETE FROM messages WHERE message_id = $1 AND sender_id = $2`

	result, err := s.db.ExecContext(ctx, query, req.MessageId, req.UserId)
	if err != nil {
		return &pb.DeleteMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return &pb.DeleteMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	if rowsAffected == 0 {
		return &pb.DeleteMessageResponse{
			Success: false,
			Error:   "Message not found or not authorized to delete",
		}, nil
	}

	return &pb.DeleteMessageResponse{
		Success: true,
	}, nil
}

func (s *server) subscribeToMessages() {
	sub, err := s.natsConn.Subscribe("chat.messages.*", func(msg *nats.Msg) {
		var message pb.Message
		err := json.Unmarshal(msg.Data, &message)
		if err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			return
		}

		// Store the message
		storeReq := &pb.StoreMessageRequest{
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
		defer sub.Unsubscribe()
	}
}

func initDB() (*sql.DB, error) {
	// For demo purposes, using SQLite. In production, use PostgreSQL
	db, err := sql.Open("postgres", "postgres://user:password@localhost/kubechat?sslmode=disable")
	if err != nil {
		// Fallback to in-memory SQLite for demo
		log.Println("PostgreSQL not available, this would normally connect to a real database")
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
		return nil, err
	}

	return db, nil
}

func main() {
	// Connect to database
	db, err := initDB()
	if err != nil {
		log.Printf("Database connection failed, running without persistence: %v", err)
	}

	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
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

	pb.RegisterMessageStoreServiceServer(s, messageStoreServer)

	log.Println("Message store service listening on :50054")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}