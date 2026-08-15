package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	users "kubechat/proto/users"
)

const testSecret = "test-secret"

func newTestServer() *server {
	return &server{
		users:     make(map[string]*User),
		jwtSecret: []byte(testSecret),
	}
}

func mustCreate(t *testing.T, s *server, username, password string) string {
	t.Helper()
	resp, err := s.CreateUser(context.Background(), &users.CreateUserRequest{
		Username: username,
		Password: password,
		Email:    username + "@example.com",
	})
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", username, err)
	}
	if !resp.Success {
		t.Fatalf("CreateUser(%q) failed: %s", username, resp.Message)
	}
	return resp.UserId
}

func TestCreateUser(t *testing.T) {
	s := newTestServer()
	id := mustCreate(t, s, "alice", "secret123")

	if id != generateUserID("alice") {
		t.Errorf("user id = %q, want deterministic id %q", id, generateUserID("alice"))
	}

	stored := s.users[id]
	if stored == nil {
		t.Fatal("user not stored")
	}
	if stored.Password == "secret123" {
		t.Error("password stored in plain text")
	}
	if stored.Email != "alice@example.com" {
		t.Errorf("email = %q", stored.Email)
	}
}

func TestCreateUserDuplicateUsername(t *testing.T) {
	s := newTestServer()
	mustCreate(t, s, "alice", "secret123")

	resp, err := s.CreateUser(context.Background(), &users.CreateUserRequest{Username: "alice", Password: "other"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected duplicate username to be rejected")
	}
	if len(s.users) != 1 {
		t.Errorf("users count = %d, want 1", len(s.users))
	}
}

func TestCreateUserValidation(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{"empty username", "", "secret"},
		{"blank username", "   ", "secret"},
		{"empty password", "alice", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer()
			resp, err := s.CreateUser(context.Background(), &users.CreateUserRequest{
				Username: tc.username,
				Password: tc.password,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Success {
				t.Fatal("expected request to be rejected")
			}
			if len(s.users) != 0 {
				t.Error("invalid user was stored")
			}
		})
	}
}

func TestCreateUserConcurrent(t *testing.T) {
	s := newTestServer()
	const n = 8

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = s.CreateUser(context.Background(), &users.CreateUserRequest{
				Username: fmt.Sprintf("user%d", i),
				Password: "pw",
			})
		}(i)
	}
	wg.Wait()

	if len(s.users) != n {
		t.Errorf("users count = %d, want %d", len(s.users), n)
	}
}

func TestLoginUser(t *testing.T) {
	s := newTestServer()
	id := mustCreate(t, s, "alice", "secret123")

	tests := []struct {
		name        string
		username    string
		password    string
		wantSuccess bool
	}{
		{"valid credentials", "alice", "secret123", true},
		{"wrong password", "alice", "nope", false},
		{"unknown user", "bob", "secret123", false},
		{"empty password", "alice", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := s.LoginUser(context.Background(), &users.LoginUserRequest{
				Username: tc.username,
				Password: tc.password,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Success != tc.wantSuccess {
				t.Fatalf("success = %v, want %v (%s)", resp.Success, tc.wantSuccess, resp.Message)
			}
			if !tc.wantSuccess {
				if resp.Token != "" {
					t.Error("token returned for failed login")
				}
				return
			}
			if resp.UserId != id {
				t.Errorf("user id = %q, want %q", resp.UserId, id)
			}
			if !s.users[id].Online {
				t.Error("user not marked online after login")
			}
			assertValidToken(t, resp.Token, id)
		})
	}
}

func assertValidToken(t *testing.T, tokenString, wantUserID string) {
	t.Helper()
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(tok *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		t.Fatalf("token invalid: %v", err)
	}
	if got := claims["user_id"]; got != wantUserID {
		t.Errorf("user_id claim = %v, want %q", got, wantUserID)
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		t.Fatalf("missing exp claim: %v", err)
	}
	if d := time.Until(exp.Time); d < 23*time.Hour || d > 25*time.Hour {
		t.Errorf("token expires in %v, want ~24h", d)
	}
}

func TestGenerateJWTRejectsOtherSecret(t *testing.T) {
	s := newTestServer()
	tokenString, err := s.generateJWT("u1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = jwt.Parse(tokenString, func(*jwt.Token) (interface{}, error) {
		return []byte("wrong-secret"), nil
	})
	if err == nil {
		t.Fatal("token verified with wrong secret")
	}
}

func TestGetUser(t *testing.T) {
	s := newTestServer()
	id := mustCreate(t, s, "alice", "secret123")

	t.Run("existing user", func(t *testing.T) {
		resp, err := s.GetUser(context.Background(), &users.GetUserRequest{UserId: id})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Username != "alice" || resp.UserId != id {
			t.Errorf("got %+v", resp)
		}
	})

	t.Run("missing user", func(t *testing.T) {
		if _, err := s.GetUser(context.Background(), &users.GetUserRequest{UserId: "missing"}); err == nil {
			t.Fatal("expected error for unknown user")
		}
	})
}

func TestGenerateUserID(t *testing.T) {
	a, b := generateUserID("alice"), generateUserID("bob")
	if a == b {
		t.Error("different usernames produced the same id")
	}
	if a != generateUserID("alice") {
		t.Error("id is not deterministic")
	}
	if len(a) != 32 {
		t.Errorf("id length = %d, want 32 hex chars", len(a))
	}
}
