package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "kubechat/proto"
)

type server struct {
	pb.UnimplementedChatServiceServer
	natsConn         *nats.Conn
	messageStoreConn pb.MessageStoreServiceClient
}

func (s *server) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	messageID := generateMessageID()
	now := time.Now()

	message := &pb.Message{
		MessageId:   messageID,
		SenderId:    req.SenderId,
		RecipientId: req.RecipientId,
		Content:     req.Message,
		Timestamp:   timestamppb.New(now),
	}

	// Publish message to NATS for real-time delivery
	messageData, err := json.Marshal(message)
	if err != nil {
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "Failed to marshal message",
		}, err
	}

	// Publish to recipient's channel
	subject := "chat.messages." + req.RecipientId
	err = s.natsConn.Publish(subject, messageData)
	if err != nil {
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "Failed to publish message",
		}, err
	}

	// Also publish to sender's channel for confirmation
	senderSubject := "chat.messages." + req.SenderId
	s.natsConn.Publish(senderSubject, messageData)

	// Store message in message store service
	if s.messageStoreConn != nil {
		storeReq := &pb.StoreMessageRequest{
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

	return &pb.SendMessageResponse{
		MessageId: messageID,
		Success:   true,
	}, nil
}

func (s *server) GetMessageHistory(ctx context.Context, req *pb.GetMessageHistoryRequest) (*pb.GetMessageHistoryResponse, error) {
	if s.messageStoreConn == nil {
		return &pb.GetMessageHistoryResponse{
			Messages: []*pb.Message{},
		}, nil
	}

	storeReq := &pb.GetMessageHistoryRequest{
		UserId1: req.UserId1,
		UserId2: req.UserId2,
		Limit:   req.Limit,
		Offset:  req.Offset,
	}

	resp, err := s.messageStoreConn.GetMessageHistory(ctx, storeReq)
	if err != nil {
		return nil, err
	}

	var messages []*pb.Message
	for _, stored := range resp.Messages {
		messages = append(messages, &pb.Message{
			MessageId:   stored.MessageId,
			SenderId:    stored.SenderId,
			RecipientId: stored.RecipientId,
			Content:     stored.Content,
			Timestamp:   stored.Timestamp,
		})
	}

	return &pb.GetMessageHistoryResponse{
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
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Connect to message store service (optional)
	var messageStoreConn pb.MessageStoreServiceClient
	conn, err := grpc.Dial("localhost:50054", grpc.WithInsecure())
	if err != nil {
		log.Printf("Failed to connect to message store service: %v", err)
	} else {
		messageStoreConn = pb.NewMessageStoreServiceClient(conn)
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

	pb.RegisterChatServiceServer(s, chatServer)

	log.Println("Chat service listening on :50053")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}