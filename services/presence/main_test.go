package main

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	presence "kubechat/proto/presence"
)

type publishedMsg struct {
	subject string
	data    []byte
}

type fakePublisher struct {
	published []publishedMsg
}

func (f *fakePublisher) Publish(subject string, data []byte) error {
	f.published = append(f.published, publishedMsg{subject, data})
	return nil
}

func newTestServer(t *testing.T) (*server, *miniredis.Miniredis, *fakePublisher) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { rdb.Close() })
	pub := &fakePublisher{}
	return &server{redisClient: rdb, natsConn: pub}, mr, pub
}

func setOnline(t *testing.T, s *server, userID string) {
	t.Helper()
	if _, err := s.SetUserOnline(context.Background(), &presence.SetUserOnlineRequest{UserId: userID}); err != nil {
		t.Fatalf("SetUserOnline(%q): %v", userID, err)
	}
}

func setOffline(t *testing.T, s *server, userID string) {
	t.Helper()
	if _, err := s.SetUserOffline(context.Background(), &presence.SetUserOfflineRequest{UserId: userID}); err != nil {
		t.Fatalf("SetUserOffline(%q): %v", userID, err)
	}
}

func TestSetUserOnlinePublishesEvent(t *testing.T) {
	s, mr, pub := newTestServer(t)
	setOnline(t, s, "alice")

	if ok, _ := mr.SIsMember(onlineUsersKey, "alice"); !ok {
		t.Error("alice not in online set")
	}
	if got := mr.HGet("presence:status:alice", "online"); got != "1" {
		t.Errorf("online field = %q, want %q", got, "1")
	}

	if len(pub.published) != 1 || pub.published[0].subject != "users.status" {
		t.Fatalf("published = %+v, want one users.status event", pub.published)
	}
	var event presence.UserStatusEvent
	if err := json.Unmarshal(pub.published[0].data, &event); err != nil {
		t.Fatalf("invalid event JSON: %v", err)
	}
	if event.UserId != "alice" || !event.Online || event.Timestamp == nil {
		t.Errorf("event = %+v", &event)
	}
}

func TestSetUserOffline(t *testing.T) {
	s, mr, pub := newTestServer(t)
	setOnline(t, s, "alice")
	setOffline(t, s, "alice")

	if ok, _ := mr.SIsMember(onlineUsersKey, "alice"); ok {
		t.Error("alice still in online set")
	}
	var event presence.UserStatusEvent
	if err := json.Unmarshal(pub.published[len(pub.published)-1].data, &event); err != nil {
		t.Fatal(err)
	}
	if event.UserId != "alice" || event.Online {
		t.Errorf("last event = %+v, want alice offline", &event)
	}
}

func TestGetUserStatus(t *testing.T) {
	s, _, _ := newTestServer(t)
	setOnline(t, s, "alice")
	setOnline(t, s, "bob")
	setOffline(t, s, "bob")

	tests := []struct {
		userID     string
		wantOnline bool
	}{
		{"alice", true},
		{"bob", false},
		{"carol", false}, // never seen
	}

	for _, tc := range tests {
		t.Run(tc.userID, func(t *testing.T) {
			resp, err := s.GetUserStatus(context.Background(), &presence.GetUserStatusRequest{UserId: tc.userID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.UserId != tc.userID || resp.Online != tc.wantOnline {
				t.Errorf("got user=%q online=%v, want online=%v", resp.UserId, resp.Online, tc.wantOnline)
			}
			if resp.LastSeen == nil {
				t.Error("missing last_seen")
			} else if d := time.Since(resp.LastSeen.AsTime()); d < 0 || d > time.Minute {
				t.Errorf("last_seen is %v ago, want recent", d)
			}
		})
	}
}

func TestGetOnlineUsers(t *testing.T) {
	s, _, _ := newTestServer(t)

	resp, err := s.GetOnlineUsers(context.Background(), &presence.GetOnlineUsersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.UserIds) != 0 {
		t.Errorf("expected nobody online, got %v", resp.UserIds)
	}

	setOnline(t, s, "alice")
	setOnline(t, s, "bob")
	setOnline(t, s, "carol")
	setOnline(t, s, "alice") // idempotent
	setOffline(t, s, "carol")

	resp, err = s.GetOnlineUsers(context.Background(), &presence.GetOnlineUsersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := append([]string(nil), resp.UserIds...)
	sort.Strings(got)
	if want := []string{"alice", "bob"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("online users = %v, want %v", got, want)
	}
}

func TestRedisUnavailable(t *testing.T) {
	s, mr, pub := newTestServer(t)
	mr.Close()

	if _, err := s.SetUserOnline(context.Background(), &presence.SetUserOnlineRequest{UserId: "alice"}); err == nil {
		t.Error("SetUserOnline: expected error when redis is down")
	}
	if len(pub.published) != 0 {
		t.Error("status event published even though redis write failed")
	}
	if _, err := s.GetOnlineUsers(context.Background(), &presence.GetOnlineUsersRequest{}); err == nil {
		t.Error("GetOnlineUsers: expected error when redis is down")
	}
}
