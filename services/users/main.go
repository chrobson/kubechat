package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	
	users "kubechat/proto/users"
)

type User struct {
	ID       string
	Username string
	Email    string
	Password string
	Online   bool
}

type server struct {
	users.UnimplementedUsersServiceServer
	users map[string]*User
	mutex sync.RWMutex
}

func (s *server) CreateUser(ctx context.Context, req *users.CreateUserRequest) (*users.CreateUserResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if username already exists
	for _, user := range s.users {
		if user.Username == req.Username {
			return &users.CreateUserResponse{
				Success: false,
				Message: "Username already exists",
			}, nil
		}
	}

	// Generate deterministic user ID from username
	userID := generateUserID(req.Username)

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return &users.CreateUserResponse{
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

	return &users.CreateUserResponse{
		UserId:  userID,
		Success: true,
		Message: "User created successfully",
	}, nil
}

func (s *server) LoginUser(ctx context.Context, req *users.LoginUserRequest) (*users.LoginUserResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var user *User
	for _, u := range s.users {
		if u.Username == req.Username {
			user = u
			break
		}
	}

	if user == nil {
		return &users.LoginUserResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Check password
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		return &users.LoginUserResponse{
			Success: false,
			Message: "Invalid password",
		}, nil
	}

	// Generate JWT token
	token, err := generateJWT(user.ID)
	if err != nil {
		return &users.LoginUserResponse{
			Success: false,
			Message: "Failed to generate token",
		}, err
	}

	user.Online = true

	return &users.LoginUserResponse{
		UserId:  user.ID,
		Token:   token,
		Success: true,
		Message: "Login successful",
	}, nil
}

func (s *server) GetUser(ctx context.Context, req *users.GetUserRequest) (*users.GetUserResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	user, exists := s.users[req.UserId]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	return &users.GetUserResponse{
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

func generateUserID(username string) string {
	hash := sha256.Sum256([]byte(username))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes
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

	users.RegisterUsersServiceServer(s, userServer)

	log.Println("Users service listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}