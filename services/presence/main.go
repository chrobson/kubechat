package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "kubechat/proto"
)

type UserStatus struct {
	UserID   string
	Online   bool
	LastSeen time.Time
}

type server struct {
	pb.UnimplementedPresenceServiceServer
	userStatuses map[string]*UserStatus
	mutex        sync.RWMutex
	natsConn     *nats.Conn
}

func (s *server) SetUserOnline(ctx context.Context, req *pb.SetUserOnlineRequest) (*pb.SetUserOnlineResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	s.userStatuses[req.UserId] = &UserStatus{
		UserID:   req.UserId,
		Online:   true,
		LastSeen: now,
	}

	// Publish status update to NATS
	event := &pb.UserStatusEvent{
		UserId:    req.UserId,
		Online:    true,
		Timestamp: timestamppb.New(now),
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal status event: %v", err)
	} else {
		s.natsConn.Publish("users.status", eventData)
	}

	return &pb.SetUserOnlineResponse{
		Success: true,
		Message: "User set to online",
	}, nil
}

func (s *server) SetUserOffline(ctx context.Context, req *pb.SetUserOfflineRequest) (*pb.SetUserOfflineResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	if status, exists := s.userStatuses[req.UserId]; exists {
		status.Online = false
		status.LastSeen = now
	} else {
		s.userStatuses[req.UserId] = &UserStatus{
			UserID:   req.UserId,
			Online:   false,
			LastSeen: now,
		}
	}

	// Publish status update to NATS
	event := &pb.UserStatusEvent{
		UserId:    req.UserId,
		Online:    false,
		Timestamp: timestamppb.New(now),
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal status event: %v", err)
	} else {
		s.natsConn.Publish("users.status", eventData)
	}

	return &pb.SetUserOfflineResponse{
		Success: true,
		Message: "User set to offline",
	}, nil
}

func (s *server) GetUserStatus(ctx context.Context, req *pb.GetUserStatusRequest) (*pb.GetUserStatusResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	status, exists := s.userStatuses[req.UserId]
	if !exists {
		return &pb.GetUserStatusResponse{
			UserId:   req.UserId,
			Online:   false,
			LastSeen: timestamppb.New(time.Now()),
		}, nil
	}

	return &pb.GetUserStatusResponse{
		UserId:   status.UserID,
		Online:   status.Online,
		LastSeen: timestamppb.New(status.LastSeen),
	}, nil
}

func (s *server) GetOnlineUsers(ctx context.Context, req *pb.GetOnlineUsersRequest) (*pb.GetOnlineUsersResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var onlineUsers []string
	for userID, status := range s.userStatuses {
		if status.Online {
			onlineUsers = append(onlineUsers, userID)
		}
	}

	return &pb.GetOnlineUsersResponse{
		UserIds: onlineUsers,
	}, nil
}

func main() {
	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	presenceServer := &server{
		userStatuses: make(map[string]*UserStatus),
		natsConn:     nc,
	}

	pb.RegisterPresenceServiceServer(s, presenceServer)

	log.Println("Presence service listening on :50052")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}