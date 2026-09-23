package main

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/protobuf/types/known/timestamppb"

	messagestore "kubechat/proto/messagestore"
)

func newMockServer(t *testing.T) (*server, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
		db.Close()
	})
	return &server{db: db}, mock
}

var historyColumns = []string{"message_id", "sender_id", "recipient_id", "content", "timestamp", "created_at"}

func TestHistoryQuery(t *testing.T) {
	cursor := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		req         *messagestore.GetMessageHistoryRequest
		wantArgs    []interface{}
		wantKeyset  bool
		wantOffsetQ bool
	}{
		{
			name:        "first page uses default limit",
			req:         &messagestore.GetMessageHistoryRequest{UserId1: "a", UserId2: "b"},
			wantArgs:    []interface{}{"a", "b", int32(defaultHistoryLimit), int32(0)},
			wantOffsetQ: true,
		},
		{
			name:        "explicit limit and offset",
			req:         &messagestore.GetMessageHistoryRequest{UserId1: "a", UserId2: "b", Limit: 10, Offset: 30},
			wantArgs:    []interface{}{"a", "b", int32(10), int32(30)},
			wantOffsetQ: true,
		},
		{
			name:        "negative limit and offset are normalised",
			req:         &messagestore.GetMessageHistoryRequest{UserId1: "a", UserId2: "b", Limit: -5, Offset: -1},
			wantArgs:    []interface{}{"a", "b", int32(defaultHistoryLimit), int32(0)},
			wantOffsetQ: true,
		},
		{
			name:        "limit is capped",
			req:         &messagestore.GetMessageHistoryRequest{UserId1: "a", UserId2: "b", Limit: 100000},
			wantArgs:    []interface{}{"a", "b", int32(maxHistoryLimit), int32(0)},
			wantOffsetQ: true,
		},
		{
			name: "cursor switches to keyset pagination and ignores offset",
			req: &messagestore.GetMessageHistoryRequest{
				UserId1: "a", UserId2: "b", Limit: 10, Offset: 30,
				LastMessageId: "m9", LastTimestamp: timestamppb.New(cursor),
			},
			wantArgs:   []interface{}{"a", "b", cursor, "m9", int32(10)},
			wantKeyset: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, args := historyQuery(tc.req)

			if got := strings.Contains(query, "message_id < $4"); got != tc.wantKeyset {
				t.Errorf("keyset condition present = %v, want %v\n%s", got, tc.wantKeyset, query)
			}
			if got := strings.Contains(query, "OFFSET"); got != tc.wantOffsetQ {
				t.Errorf("OFFSET present = %v, want %v\n%s", got, tc.wantOffsetQ, query)
			}
			if !strings.Contains(query, "ORDER BY timestamp DESC, message_id DESC") {
				t.Errorf("query must order by (timestamp, message_id) for stable pages:\n%s", query)
			}

			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
			for i := range args {
				if gotT, ok := args[i].(time.Time); ok {
					if !gotT.Equal(tc.wantArgs[i].(time.Time)) {
						t.Errorf("arg[%d] = %v, want %v", i, gotT, tc.wantArgs[i])
					}
					continue
				}
				if args[i] != tc.wantArgs[i] {
					t.Errorf("arg[%d] = %#v, want %#v", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func TestGetMessageHistory(t *testing.T) {
	s, mock := newMockServer(t)
	ts := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("FROM messages")).
		WithArgs("alice", "bob", int32(defaultHistoryLimit), int32(0)).
		WillReturnRows(sqlmock.NewRows(historyColumns).
			AddRow("m2", "bob", "alice", "yo", ts.Add(time.Second), ts.Add(time.Second)).
			AddRow("m1", "alice", "bob", "hi", ts, ts))

	resp, err := s.GetMessageHistory(context.Background(), &messagestore.GetMessageHistoryRequest{
		UserId1: "alice", UserId2: "bob",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(resp.Messages))
	}
	first := resp.Messages[0]
	if first.MessageId != "m2" || first.SenderId != "bob" || first.Content != "yo" {
		t.Errorf("first message = %+v", first)
	}
	if !first.Timestamp.AsTime().Equal(ts.Add(time.Second)) {
		t.Errorf("timestamp = %v", first.Timestamp.AsTime())
	}
}

func TestGetMessageHistoryCursor(t *testing.T) {
	s, mock := newMockServer(t)
	cursor := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("timestamp < $3 OR (timestamp = $3 AND message_id < $4)")).
		WithArgs("alice", "bob", cursor, "m5", int32(2)).
		WillReturnRows(sqlmock.NewRows(historyColumns))

	resp, err := s.GetMessageHistory(context.Background(), &messagestore.GetMessageHistoryRequest{
		UserId1: "alice", UserId2: "bob", Limit: 2,
		LastMessageId: "m5", LastTimestamp: timestamppb.New(cursor),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Messages) != 0 {
		t.Errorf("got %d messages, want 0", len(resp.Messages))
	}
}

func TestGetMessageHistoryErrors(t *testing.T) {
	t.Run("query error", func(t *testing.T) {
		s, mock := newMockServer(t)
		mock.ExpectQuery(regexp.QuoteMeta("FROM messages")).WillReturnError(errors.New("db down"))

		if _, err := s.GetMessageHistory(context.Background(), &messagestore.GetMessageHistoryRequest{}); err == nil {
			t.Fatal("expected query error to be returned, not an empty page")
		}
	})

	t.Run("row iteration error", func(t *testing.T) {
		s, mock := newMockServer(t)
		ts := time.Now()
		mock.ExpectQuery(regexp.QuoteMeta("FROM messages")).
			WillReturnRows(sqlmock.NewRows(historyColumns).
				AddRow("m1", "a", "b", "hi", ts, ts).
				AddRow("m2", "a", "b", "yo", ts, ts).
				RowError(1, errors.New("connection lost")))

		if _, err := s.GetMessageHistory(context.Background(), &messagestore.GetMessageHistoryRequest{}); err == nil {
			t.Fatal("expected error for truncated result set")
		}
	})
}

func TestStoreMessage(t *testing.T) {
	s, mock := newMockServer(t)
	ts := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO messages")).
		WithArgs("m1", "alice", "bob", "hi", ts, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	resp, err := s.StoreMessage(context.Background(), &messagestore.StoreMessageRequest{
		MessageId: "m1", SenderId: "alice", RecipientId: "bob", Content: "hi",
		Timestamp: timestamppb.New(ts),
	})
	if err != nil || !resp.Success {
		t.Fatalf("StoreMessage = %+v, %v", resp, err)
	}
}

// The chat service stores each message over gRPC and also publishes it to
// chat.messages.<recipient> and chat.messages.<sender>, both of which this
// service consumes. Inserts must therefore be idempotent on message_id.
func TestStoreMessageIsIdempotent(t *testing.T) {
	s, mock := newMockServer(t)
	insert := regexp.QuoteMeta("ON CONFLICT (message_id) DO NOTHING")
	mock.ExpectExec(insert).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(insert).WillReturnResult(sqlmock.NewResult(0, 0))

	req := &messagestore.StoreMessageRequest{MessageId: "m1", Timestamp: timestamppb.Now()}
	for i := 0; i < 2; i++ {
		resp, err := s.StoreMessage(context.Background(), req)
		if err != nil || !resp.Success {
			t.Fatalf("attempt %d: StoreMessage = %+v, %v", i+1, resp, err)
		}
	}
}

func TestStoreMessageDBError(t *testing.T) {
	s, mock := newMockServer(t)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO messages")).WillReturnError(errors.New("connection reset"))

	resp, err := s.StoreMessage(context.Background(), &messagestore.StoreMessageRequest{
		MessageId: "m1", Timestamp: timestamppb.Now(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if resp.Success || resp.Error == "" {
		t.Errorf("expected failed response with error, got %+v", resp)
	}
}

func TestDeleteMessage(t *testing.T) {
	tests := []struct {
		name        string
		result      driver.Result
		execErr     error
		wantSuccess bool
		wantErr     bool
	}{
		{"own message deleted", sqlmock.NewResult(0, 1), nil, true, false},
		{"not found or not owner", sqlmock.NewResult(0, 0), nil, false, false},
		{"database error", nil, errors.New("db down"), false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := newMockServer(t)
			exp := mock.ExpectExec(regexp.QuoteMeta("DELETE FROM messages WHERE message_id = $1 AND sender_id = $2")).
				WithArgs("m1", "alice")
			if tc.execErr != nil {
				exp.WillReturnError(tc.execErr)
			} else {
				exp.WillReturnResult(tc.result)
			}

			resp, err := s.DeleteMessage(context.Background(), &messagestore.DeleteMessageRequest{
				MessageId: "m1", UserId: "alice",
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if resp.Success != tc.wantSuccess {
				t.Errorf("success = %v, want %v", resp.Success, tc.wantSuccess)
			}
		})
	}
}

func TestWithoutDatabase(t *testing.T) {
	s := &server{}

	store, err := s.StoreMessage(context.Background(), &messagestore.StoreMessageRequest{MessageId: "m1"})
	if err != nil || !store.Success {
		t.Errorf("StoreMessage without db = %+v, %v", store, err)
	}

	hist, err := s.GetMessageHistory(context.Background(), &messagestore.GetMessageHistoryRequest{UserId1: "a", UserId2: "b"})
	if err != nil || len(hist.Messages) != 0 {
		t.Errorf("GetMessageHistory without db = %+v, %v", hist, err)
	}

	del, err := s.DeleteMessage(context.Background(), &messagestore.DeleteMessageRequest{MessageId: "m1", UserId: "a"})
	if err != nil || del.Success {
		t.Errorf("DeleteMessage without db = %+v, %v", del, err)
	}
}
