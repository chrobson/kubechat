package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	chat "kubechat/proto/chat"
	messagestore "kubechat/proto/messagestore"
)

type publishedMsg struct {
	subject string
	data    []byte
}

type fakePublisher struct {
	published []publishedMsg
	failOn    map[string]error
}

func (f *fakePublisher) Publish(subject string, data []byte) error {
	if err := f.failOn[subject]; err != nil {
		return err
	}
	f.published = append(f.published, publishedMsg{subject, data})
	return nil
}

type fakeStore struct {
	messagestore.MessageStoreServiceClient

	stored     []*messagestore.StoreMessageRequest
	storeErr   error
	historyReq *messagestore.GetMessageHistoryRequest
	history    []*messagestore.StoredMessage
	historyErr error
}

func (f *fakeStore) StoreMessage(_ context.Context, in *messagestore.StoreMessageRequest, _ ...grpc.CallOption) (*messagestore.StoreMessageResponse, error) {
	f.stored = append(f.stored, in)
	if f.storeErr != nil {
		return nil, f.storeErr
	}
	return &messagestore.StoreMessageResponse{Success: true}, nil
}

func (f *fakeStore) GetMessageHistory(_ context.Context, in *messagestore.GetMessageHistoryRequest, _ ...grpc.CallOption) (*messagestore.GetMessageHistoryResponse, error) {
	f.historyReq = in
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return &messagestore.GetMessageHistoryResponse{Messages: f.history}, nil
}

func TestSendMessage(t *testing.T) {
	pub := &fakePublisher{}
	store := &fakeStore{}
	s := &server{natsConn: pub, messageStoreConn: store}

	resp, err := s.SendMessage(context.Background(), &chat.SendMessageRequest{
		SenderId:    "alice",
		RecipientId: "bob",
		Message:     "hi",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success || resp.MessageId == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	wantSubjects := []string{"chat.messages.bob", "chat.messages.alice"}
	if len(pub.published) != len(wantSubjects) {
		t.Fatalf("published %d messages, want %d", len(pub.published), len(wantSubjects))
	}
	for i, want := range wantSubjects {
		got := pub.published[i]
		if got.subject != want {
			t.Errorf("publish[%d] subject = %q, want %q", i, got.subject, want)
		}
		var msg chat.Message
		if err := json.Unmarshal(got.data, &msg); err != nil {
			t.Fatalf("payload is not valid JSON: %v", err)
		}
		if msg.MessageId != resp.MessageId || msg.SenderId != "alice" || msg.RecipientId != "bob" || msg.Content != "hi" {
			t.Errorf("publish[%d] payload = %+v", i, &msg)
		}
		if msg.Timestamp == nil {
			t.Errorf("publish[%d] payload has no timestamp", i)
		}
	}

	if len(store.stored) != 1 {
		t.Fatalf("stored %d messages, want 1", len(store.stored))
	}
	if got := store.stored[0]; got.MessageId != resp.MessageId || got.Content != "hi" {
		t.Errorf("stored message = %+v", got)
	}
}

func TestSendMessageFailures(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		name        string
		failOn      map[string]error
		storeErr    error
		nilStore    bool
		wantErr     bool
		wantStored  int
		wantPublish int
	}{
		{
			name:    "recipient publish fails",
			failOn:  map[string]error{"chat.messages.bob": boom},
			wantErr: true,
		},
		{
			name:        "sender echo fails is not fatal",
			failOn:      map[string]error{"chat.messages.alice": boom},
			wantStored:  1,
			wantPublish: 1,
		},
		{
			name:        "store failure is not fatal",
			storeErr:    boom,
			wantStored:  1,
			wantPublish: 2,
		},
		{
			name:        "no message store configured",
			nilStore:    true,
			wantPublish: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pub := &fakePublisher{failOn: tc.failOn}
			store := &fakeStore{storeErr: tc.storeErr}
			s := &server{natsConn: pub}
			if !tc.nilStore {
				s.messageStoreConn = store
			}

			resp, err := s.SendMessage(context.Background(), &chat.SendMessageRequest{
				SenderId: "alice", RecipientId: "bob", Message: "hi",
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if resp == nil || resp.Success {
					t.Errorf("expected unsuccessful response, got %+v", resp)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !resp.Success {
				t.Errorf("expected success, got %+v", resp)
			}
			if len(store.stored) != tc.wantStored {
				t.Errorf("stored = %d, want %d", len(store.stored), tc.wantStored)
			}
			if len(pub.published) != tc.wantPublish {
				t.Errorf("published = %d, want %d", len(pub.published), tc.wantPublish)
			}
		})
	}
}

func TestGetMessageHistory(t *testing.T) {
	ts := timestamppb.New(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	store := &fakeStore{history: []*messagestore.StoredMessage{
		{MessageId: "m2", SenderId: "bob", RecipientId: "alice", Content: "yo", Timestamp: ts},
		{MessageId: "m1", SenderId: "alice", RecipientId: "bob", Content: "hi", Timestamp: ts},
	}}
	s := &server{messageStoreConn: store}

	cursor := timestamppb.New(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC))
	resp, err := s.GetMessageHistory(context.Background(), &chat.GetMessageHistoryRequest{
		UserId1:       "alice",
		UserId2:       "bob",
		Limit:         20,
		Offset:        5,
		LastMessageId: "m3",
		LastTimestamp: cursor,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := store.historyReq
	if req.UserId1 != "alice" || req.UserId2 != "bob" || req.Limit != 20 || req.Offset != 5 ||
		req.LastMessageId != "m3" || !req.LastTimestamp.AsTime().Equal(cursor.AsTime()) {
		t.Errorf("pagination not forwarded to store: %+v", req)
	}

	if len(resp.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(resp.Messages))
	}
	if m := resp.Messages[0]; m.MessageId != "m2" || m.SenderId != "bob" || m.Content != "yo" || m.Timestamp != ts {
		t.Errorf("message not mapped correctly: %+v", m)
	}
}

func TestGetMessageHistoryErrors(t *testing.T) {
	t.Run("no store returns empty history", func(t *testing.T) {
		s := &server{}
		resp, err := s.GetMessageHistory(context.Background(), &chat.GetMessageHistoryRequest{UserId1: "a", UserId2: "b"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Messages) != 0 {
			t.Errorf("expected no messages, got %d", len(resp.Messages))
		}
	})

	t.Run("store error is propagated", func(t *testing.T) {
		s := &server{messageStoreConn: &fakeStore{historyErr: errors.New("db down")}}
		if _, err := s.GetMessageHistory(context.Background(), &chat.GetMessageHistoryRequest{}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestGenerateMessageID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateMessageID()
		if len(id) != 32 {
			t.Fatalf("id %q has length %d, want 32", id, len(id))
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("id %q is not hex: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}
