package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	presence "kubechat/proto/presence"
)

type server struct {
	presence.UnimplementedPresenceServiceServer
	redisClient *redis.Client
	natsConn    *nats.Conn
}

const (
	onlineUsersKey = "presence:online"
	userStatusKey  = "presence:status:%s"
)

func (s *server) SetUserOnline(ctx context.Context, req *presence.SetUserOnlineRequest) (*presence.SetUserOnlineResponse, error) {
	now := time.Now()

	// Store in Redis Set and Hash
	pipe := s.redisClient.Pipeline()
	pipe.SAdd(ctx, onlineUsersKey, req.UserId)
	pipe.HSet(ctx, fmt.Sprintf(userStatusKey, req.UserId), "online", true, "last_seen", now.Format(time.RFC3339))
	_, err := pipe.Exec(ctx)

	if err != nil {
		log.Printf("Failed to set user online in Redis: %v", err)
		return nil, err
	}

	// Publish status update to NATS
	event := &presence.UserStatusEvent{
		UserId:    req.UserId,
		Online:    true,
		Timestamp: timestamppb.New(now),
	}

	eventData, _ := json.Marshal(event)
	s.natsConn.Publish("users.status", eventData)

	return &presence.SetUserOnlineResponse{
		Success: true,
		Message: "User set to online",
	}, nil
}

func (s *server) SetUserOffline(ctx context.Context, req *presence.SetUserOfflineRequest) (*presence.SetUserOfflineResponse, error) {
	now := time.Now()

	// Update Redis
	pipe := s.redisClient.Pipeline()
	pipe.SRem(ctx, onlineUsersKey, req.UserId)
	pipe.HSet(ctx, fmt.Sprintf(userStatusKey, req.UserId), "online", false, "last_seen", now.Format(time.RFC3339))
	_, err := pipe.Exec(ctx)

	if err != nil {
		log.Printf("Failed to set user offline in Redis: %v", err)
		return nil, err
	}

	// Publish status update to NATS
	event := &presence.UserStatusEvent{
		UserId:    req.UserId,
		Online:    false,
		Timestamp: timestamppb.New(now),
	}

	eventData, _ := json.Marshal(event)
	s.natsConn.Publish("users.status", eventData)

	return &presence.SetUserOfflineResponse{
		Success: true,
		Message: "User set to offline",
	}, nil
}

func (s *server) GetUserStatus(ctx context.Context, req *presence.GetUserStatusRequest) (*presence.GetUserStatusResponse, error) {
	res, err := s.redisClient.HGetAll(ctx, fmt.Sprintf(userStatusKey, req.UserId)).Result()
	if err != nil || len(res) == 0 {
		return &presence.GetUserStatusResponse{
			UserId:   req.UserId,
			Online:   false,
			LastSeen: timestamppb.New(time.Now()),
		}, nil
	}

	online := res["online"] == "1"
	lastSeen, _ := time.Parse(time.RFC3339, res["last_seen"])

	return &presence.GetUserStatusResponse{
		UserId:   req.UserId,
		Online:   online,
		LastSeen: timestamppb.New(lastSeen),
	}, nil
}

func (s *server) GetOnlineUsers(ctx context.Context, req *presence.GetOnlineUsersRequest) (*presence.GetOnlineUsersResponse, error) {
	onlineUsers, err := s.redisClient.SMembers(ctx, onlineUsersKey).Result()
	if err != nil {
		log.Printf("Failed to get online users from Redis: %v", err)
		return nil, err
	}

	return &presence.GetOnlineUsersResponse{
		UserIds: onlineUsers,
	}, nil
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

	// Connect to Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	presenceServer := &server{
		redisClient: rdb,
		natsConn:    nc,
	}

	presence.RegisterPresenceServiceServer(s, presenceServer)

	log.Println("Presence service listening on :50052")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}