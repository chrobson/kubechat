package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	
	chat "kubechat/proto/chat"
	messagestore "kubechat/proto/messagestore"
)

type server struct {
	chat.UnimplementedChatServiceServer
	natsConn         *nats.Conn
	messageStoreConn messagestore.MessageStoreServiceClient
}

func (s *server) SendMessage(ctx context.Context, req *chat.SendMessageRequest) (*chat.SendMessageResponse, error) {
	messageID := generateMessageID()
	now := time.Now()

	message := &chat.Message{
		MessageId:   messageID,
		SenderId:    req.SenderId,
		RecipientId: req.RecipientId,
		Content:     req.Message,
		Timestamp:   timestamppb.New(now),
	}

	// Publish message to NATS for real-time delivery
	messageData, err := json.Marshal(message)
	if err != nil {
		return &chat.SendMessageResponse{
			Success: false,
			Error:   "Failed to marshal message",
		}, err
	}

	// Publish to recipient's channel
	recipientSubject := "chat.messages." + req.RecipientId
	err = s.natsConn.Publish(recipientSubject, messageData)
	if err != nil {
		return &chat.SendMessageResponse{
			Success: false,
			Error:   "Failed to publish message to recipient",
		}, err
	}

	// Also publish to sender's channel so they can see their own message
	senderSubject := "chat.messages." + req.SenderId
	err = s.natsConn.Publish(senderSubject, messageData)
	if err != nil {
		log.Printf("Failed to publish message to sender: %v", err)
		// Don't return error for sender notification failure, message was delivered to recipient
	}

	// Store message in message store service
	if s.messageStoreConn != nil {
		storeReq := &messagestore.StoreMessageRequest{
			MessageId:   messageID,
			SenderId:    req.SenderId,
			RecipientId: req.RecipientId,
			Content:     req.Message,
			Timestamp:   timestamppb.New(now),
		}
		_, err := s.messageStoreConn.StoreMessage(ctx, storeReq)
		if err != nil {
			log.Printf("Failed to store message: %v", err)
		}
	}

	return &chat.SendMessageResponse{
		MessageId: messageID,
		Success:   true,
	}, nil
}

func (s *server) GetMessageHistory(ctx context.Context, req *chat.GetMessageHistoryRequest) (*chat.GetMessageHistoryResponse, error) {
	if s.messageStoreConn == nil {
		return &chat.GetMessageHistoryResponse{
			Messages: []*chat.Message{},
		}, nil
	}

	storeReq := &messagestore.GetMessageHistoryRequest{
		UserId1: req.UserId1,
		UserId2: req.UserId2,
		Limit:   req.Limit,
		Offset:  req.Offset,
	}

	resp, err := s.messageStoreConn.GetMessageHistory(ctx, storeReq)
	if err != nil {
		return nil, err
	}

	var messages []*chat.Message
	for _, stored := range resp.Messages {
		messages = append(messages, &chat.Message{
			MessageId:   stored.MessageId,
			SenderId:    stored.SenderId,
			RecipientId: stored.RecipientId,
			Content:     stored.Content,
			Timestamp:   stored.Timestamp,
		})
	}

	return &chat.GetMessageHistoryResponse{
		Messages: messages,
	}, nil
}

func generateMessageID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func main() {
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

	// Connect to message store service (optional)
	var messageStoreConn messagestore.MessageStoreServiceClient
	messageStoreURL := os.Getenv("MESSAGE_STORE_URL")
	if messageStoreURL == "" {
		messageStoreURL = "localhost:50054"
	}
	conn, err := grpc.Dial(messageStoreURL, grpc.WithInsecure())
	if err != nil {
		log.Printf("Failed to connect to message store service: %v", err)
	} else {
		messageStoreConn = messagestore.NewMessageStoreServiceClient(conn)
		defer conn.Close()
	}

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	chatServer := &server{
		natsConn:         nc,
		messageStoreConn: messageStoreConn,
	}

	chat.RegisterChatServiceServer(s, chatServer)

	log.Println("Chat service listening on :50053")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}