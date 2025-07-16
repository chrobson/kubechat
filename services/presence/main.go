package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	presence "kubechat/proto/presence"
)

type UserStatus struct {
	UserID   string
	Online   bool
	LastSeen time.Time
}

type server struct {
	presence.UnimplementedPresenceServiceServer
	userStatuses map[string]*UserStatus
	mutex        sync.RWMutex
	natsConn     *nats.Conn
}

func (s *server) SetUserOnline(ctx context.Context, req *presence.SetUserOnlineRequest) (*presence.SetUserOnlineResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	s.userStatuses[req.UserId] = &UserStatus{
		UserID:   req.UserId,
		Online:   true,
		LastSeen: now,
	}

	// Publish status update to NATS
	event := &presence.UserStatusEvent{
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

	return &presence.SetUserOnlineResponse{
		Success: true,
		Message: "User set to online",
	}, nil
}

func (s *server) SetUserOffline(ctx context.Context, req *presence.SetUserOfflineRequest) (*presence.SetUserOfflineResponse, error) {
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
	event := &presence.UserStatusEvent{
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

	return &presence.SetUserOfflineResponse{
		Success: true,
		Message: "User set to offline",
	}, nil
}

func (s *server) GetUserStatus(ctx context.Context, req *presence.GetUserStatusRequest) (*presence.GetUserStatusResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	status, exists := s.userStatuses[req.UserId]
	if !exists {
		return &presence.GetUserStatusResponse{
			UserId:   req.UserId,
			Online:   false,
			LastSeen: timestamppb.New(time.Now()),
		}, nil
	}

	return &presence.GetUserStatusResponse{
		UserId:   status.UserID,
		Online:   status.Online,
		LastSeen: timestamppb.New(status.LastSeen),
	}, nil
}

func (s *server) GetOnlineUsers(ctx context.Context, req *presence.GetOnlineUsersRequest) (*presence.GetOnlineUsersResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var onlineUsers []string
	for userID, status := range s.userStatuses {
		if status.Online {
			onlineUsers = append(onlineUsers, userID)
		}
	}

	return &presence.GetOnlineUsersResponse{
		UserIds: onlineUsers,
	}, nil
}

func (s *server) clearAllUserStatuses() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	log.Println("Clearing all user statuses on service start")
	// Mark all existing users as offline
	for userID, status := range s.userStatuses {
		status.Online = false
		status.LastSeen = time.Now()
		
		// Publish offline status for each user
		event := &presence.UserStatusEvent{
			UserId:    userID,
			Online:    false,
			Timestamp: timestamppb.New(time.Now()),
		}
		
		eventData, err := json.Marshal(event)
		if err == nil {
			s.natsConn.Publish("users.status", eventData)
		}
	}
	
	// Clear the map entirely to start fresh
	s.userStatuses = make(map[string]*UserStatus)
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

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	presenceServer := &server{
		userStatuses: make(map[string]*UserStatus),
		natsConn:     nc,
	}

	// Clear any stale user statuses from previous runs
	presenceServer.clearAllUserStatuses()

	presence.RegisterPresenceServiceServer(s, presenceServer)

	log.Println("Presence service listening on :50052")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}