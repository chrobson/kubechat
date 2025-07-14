package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	pb "kubechat/proto"
)

type User struct {
	ID       string
	Username string
	Email    string
	Password string
	Online   bool
}

type server struct {
	pb.UnimplementedUsersServiceServer
	users map[string]*User
	mutex sync.RWMutex
}

func (s *server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if username already exists
	for _, user := range s.users {
		if user.Username == req.Username {
			return &pb.CreateUserResponse{
				Success: false,
				Message: "Username already exists",
			}, nil
		}
	}

	// Generate user ID
	userID := generateID()

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return &pb.CreateUserResponse{
			Success: false,
			Message: "Failed to hash password",
		}, err
	}

	user := &User{
		ID:       userID,
		Username: req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
		Online:   false,
	}

	s.users[userID] = user

	return &pb.CreateUserResponse{
		UserId:  userID,
		Success: true,
		Message: "User created successfully",
	}, nil
}

func (s *server) LoginUser(ctx context.Context, req *pb.LoginUserRequest) (*pb.LoginUserResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var user *User
	for _, u := range s.users {
		if u.Username == req.Username {
			user = u
			break
		}
	}

	if user == nil {
		return &pb.LoginUserResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Check password
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		return &pb.LoginUserResponse{
			Success: false,
			Message: "Invalid password",
		}, nil
	}

	// Generate JWT token
	token, err := generateJWT(user.ID)
	if err != nil {
		return &pb.LoginUserResponse{
			Success: false,
			Message: "Failed to generate token",
		}, err
	}

	user.Online = true

	return &pb.LoginUserResponse{
		UserId:  user.ID,
		Token:   token,
		Success: true,
		Message: "Login successful",
	}, nil
}

func (s *server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	user, exists := s.users[req.UserId]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	return &pb.GetUserResponse{
		UserId:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		Online:   user.Online,
	}, nil
}

func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func generateJWT(userID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString([]byte("your-secret-key"))
	return tokenString, err
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	userServer := &server{
		users: make(map[string]*User),
	}

	pb.RegisterUsersServiceServer(s, userServer)

	log.Println("Users service listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}