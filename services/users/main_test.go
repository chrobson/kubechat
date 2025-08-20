package main

import (
	"context"
	"testing"

	userspb "kubechat/proto/users"
)

func TestUsersService_GivenWhenThen_CreateAndLoginFlow(t *testing.T) {
	t.Run("create and login flow", func(t *testing.T) {
		// --- Given ---
		svc := &server{users: make(map[string]*User)}

		// --- When --- (creating a new unique user)
		t.Run("create unique user", func(t *testing.T) {
			createResp, err := svc.CreateUser(context.Background(), &userspb.CreateUserRequest{
				Username: "alice",
				Email:    "alice@example.com",
				Password: "Str0ngP@ss!",
			})

			// --- Then ---
			t.Run("user created and stored with hashed password", func(t *testing.T) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !createResp.GetSuccess() {
					t.Fatalf("expected success")
				}
				if createResp.GetUserId() == "" {
					t.Fatalf("expected user id")
				}

				u := svc.users[createResp.GetUserId()]
				if u == nil {
					t.Fatalf("expected user in store")
				}
				if u.Username != "alice" {
					t.Fatalf("username mismatch: %s", u.Username)
				}
				if len(u.Password) == 0 {
					t.Fatalf("expected hashed password present")
				}
			})

			// --- When --- (creating the same username again)
			t.Run("duplicate username", func(t *testing.T) {
				dupResp, dupErr := svc.CreateUser(context.Background(), &userspb.CreateUserRequest{
					Username: "alice",
					Email:    "alice2@example.com",
					Password: "whatever",
				})

				// --- Then ---
				t.Run("duplicate rejected", func(t *testing.T) {
					if dupErr != nil {
						t.Fatalf("unexpected error: %v", dupErr)
					}
					if dupResp.GetSuccess() {
						t.Fatalf("expected failure on duplicate username")
					}
				})
			})

			// --- When --- (logging in with correct credentials)
			t.Run("login correct password", func(t *testing.T) {
				loginResp, loginErr := svc.LoginUser(context.Background(), &userspb.LoginUserRequest{
					Username: "alice",
					Password: "Str0ngP@ss!",
				})

				// --- Then ---
				t.Run("token returned and user online", func(t *testing.T) {
					if loginErr != nil {
						t.Fatalf("unexpected error: %v", loginErr)
					}
					if !loginResp.GetSuccess() {
						t.Fatalf("expected login success")
					}
					if loginResp.GetToken() == "" {
						t.Fatalf("expected token")
					}

					u := svc.users[loginResp.GetUserId()]
					if u == nil {
						t.Fatalf("expected user present")
					}
					if !u.Online {
						t.Fatalf("expected user online")
					}
				})
			})

			// --- When --- (logging in with wrong password)
			t.Run("login wrong password", func(t *testing.T) {
				badResp, badErr := svc.LoginUser(context.Background(), &userspb.LoginUserRequest{
					Username: "alice",
					Password: "wrong",
				})

				// --- Then ---
				t.Run("login fails and no token", func(t *testing.T) {
					if badErr != nil {
						t.Fatalf("unexpected error: %v", badErr)
					}
					if badResp.GetSuccess() {
						t.Fatalf("expected login failure")
					}
					if badResp.GetToken() != "" {
						t.Fatalf("expected empty token on failure")
					}
				})
			})

			// --- When --- (fetching the user by id)
			t.Run("get user by id", func(t *testing.T) {
				got, getErr := svc.GetUser(context.Background(), &userspb.GetUserRequest{
					UserId: createResp.GetUserId(),
				})

				// --- Then ---
				t.Run("user data returned", func(t *testing.T) {
					if getErr != nil {
						t.Fatalf("unexpected error: %v", getErr)
					}
					if got.GetUserId() != createResp.GetUserId() {
						t.Fatalf("user id mismatch")
					}
					if got.GetUsername() != "alice" {
						t.Fatalf("username mismatch: %s", got.GetUsername())
					}
					if got.GetEmail() != "alice@example.com" {
						t.Fatalf("email mismatch: %s", got.GetEmail())
					}
				})
			})
		})
	})
}

func TestUsersService_GivenWhenThen_GetUser_NotFound(t *testing.T) {
	// --- Given ---
	svc := &server{users: make(map[string]*User)}

	// --- When ---
	_, err := svc.GetUser(context.Background(), &userspb.GetUserRequest{UserId: "nope"})

	// --- Then ---
	if err == nil {
		t.Fatalf("expected error")
	}
}
