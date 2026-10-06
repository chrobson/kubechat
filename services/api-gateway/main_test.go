package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"

	chat "kubechat/proto/chat"
	presence "kubechat/proto/presence"
	users "kubechat/proto/users"
)

var testSecret = []byte("test-secret")

// --- fakes ---------------------------------------------------------------

type fakeBus struct {
	mu        sync.Mutex
	published []*nats.Msg
	handlers  map[string][]nats.MsgHandler
}

func newFakeBus() *fakeBus {
	return &fakeBus{handlers: make(map[string][]nats.MsgHandler)}
}

func (b *fakeBus) Publish(subject string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, &nats.Msg{Subject: subject, Data: data})
	return nil
}

func (b *fakeBus) Subscribe(subject string, cb nats.MsgHandler) (*nats.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[subject] = append(b.handlers[subject], cb)
	return nil, nil
}

// deliver invokes every handler subscribed to subject, like NATS would.
func (b *fakeBus) deliver(subject string, data []byte) {
	b.mu.Lock()
	hs := append([]nats.MsgHandler(nil), b.handlers[subject]...)
	b.mu.Unlock()
	for _, h := range hs {
		h(&nats.Msg{Subject: subject, Data: data})
	}
}

func (b *fakeBus) messages() []*nats.Msg {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*nats.Msg(nil), b.published...)
}

type fakeUsers struct {
	users.UsersServiceClient
	loginResp  *users.LoginUserResponse
	createResp *users.CreateUserResponse
	getResp    *users.GetUserResponse
	err        error
	lastGetID  string
}

func (f *fakeUsers) LoginUser(_ context.Context, _ *users.LoginUserRequest, _ ...grpc.CallOption) (*users.LoginUserResponse, error) {
	return f.loginResp, f.err
}

func (f *fakeUsers) CreateUser(_ context.Context, _ *users.CreateUserRequest, _ ...grpc.CallOption) (*users.CreateUserResponse, error) {
	return f.createResp, f.err
}

func (f *fakeUsers) GetUser(_ context.Context, in *users.GetUserRequest, _ ...grpc.CallOption) (*users.GetUserResponse, error) {
	f.lastGetID = in.UserId
	return f.getResp, f.err
}

type fakeChat struct {
	chat.ChatServiceClient
	mu         sync.Mutex
	sent       []*chat.SendMessageRequest
	historyReq *chat.GetMessageHistoryRequest
	history    *chat.GetMessageHistoryResponse
	err        error
}

func (f *fakeChat) SendMessage(_ context.Context, in *chat.SendMessageRequest, _ ...grpc.CallOption) (*chat.SendMessageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, in)
	if f.err != nil {
		return nil, f.err
	}
	return &chat.SendMessageResponse{MessageId: "m1", Success: true}, nil
}

func (f *fakeChat) GetMessageHistory(_ context.Context, in *chat.GetMessageHistoryRequest, _ ...grpc.CallOption) (*chat.GetMessageHistoryResponse, error) {
	f.historyReq = in
	if f.err != nil {
		return nil, f.err
	}
	return f.history, nil
}

type fakePresence struct {
	presence.PresenceServiceClient
	mu      sync.Mutex
	online  []string
	offline []string
	users   []string
}

func (f *fakePresence) SetUserOnline(_ context.Context, in *presence.SetUserOnlineRequest, _ ...grpc.CallOption) (*presence.SetUserOnlineResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online = append(f.online, in.UserId)
	return &presence.SetUserOnlineResponse{Success: true}, nil
}

func (f *fakePresence) SetUserOffline(_ context.Context, in *presence.SetUserOfflineRequest, _ ...grpc.CallOption) (*presence.SetUserOfflineResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offline = append(f.offline, in.UserId)
	return &presence.SetUserOfflineResponse{Success: true}, nil
}

func (f *fakePresence) GetOnlineUsers(_ context.Context, _ *presence.GetOnlineUsersRequest, _ ...grpc.CallOption) (*presence.GetOnlineUsersResponse, error) {
	return &presence.GetOnlineUsersResponse{UserIds: f.users}, nil
}

func (f *fakePresence) offlineCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.offline...)
}

// --- helpers -------------------------------------------------------------

type testGateway struct {
	*Gateway
	bus      *fakeBus
	users    *fakeUsers
	chat     *fakeChat
	presence *fakePresence
}

func newTestGateway() *testGateway {
	tg := &testGateway{
		bus:      newFakeBus(),
		users:    &fakeUsers{},
		chat:     &fakeChat{history: &chat.GetMessageHistoryResponse{}},
		presence: &fakePresence{},
	}
	tg.Gateway = &Gateway{
		usersClient:    tg.users,
		chatClient:     tg.chat,
		presenceClient: tg.presence,
		natsConn:       tg.bus,
		clients:        make(map[string]*Client),
		jwtSecret:      testSecret,
	}
	return tg
}

func makeToken(t *testing.T, method jwt.SigningMethod, key interface{}, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func validToken(t *testing.T, userID string) string {
	return makeToken(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
}

func newTestClient(userID string) *Client {
	return &Client{
		UserID:  userID,
		Send:    make(chan []byte, 16),
		Limiter: rate.NewLimiter(rate.Inf, 1),
	}
}

func recvMessage(t *testing.T, c *Client) Message {
	t.Helper()
	select {
	case data := <-c.Send:
		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("invalid JSON sent to client: %v", err)
		}
		return m
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for client message")
		return Message{}
	}
}

func assertNoMessage(t *testing.T, c *Client) {
	t.Helper()
	select {
	case data := <-c.Send:
		t.Fatalf("unexpected message sent to client: %s", data)
	default:
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// --- auth ----------------------------------------------------------------

func TestVerifyToken(t *testing.T) {
	g := newTestGateway()
	future := time.Now().Add(time.Hour).Unix()

	tests := []struct {
		name    string
		token   string
		wantID  string
		wantErr bool
	}{
		{
			name:   "valid token",
			token:  validToken(t, "alice"),
			wantID: "alice",
		},
		{
			name: "expired token",
			token: makeToken(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{
				"user_id": "alice", "exp": time.Now().Add(-time.Minute).Unix(),
			}),
			wantErr: true,
		},
		{
			name: "wrong secret",
			token: makeToken(t, jwt.SigningMethodHS256, []byte("other"), jwt.MapClaims{
				"user_id": "alice", "exp": future,
			}),
			wantErr: true,
		},
		{
			name: "alg none is rejected",
			token: makeToken(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, jwt.MapClaims{
				"user_id": "alice", "exp": future,
			}),
			wantErr: true,
		},
		{
			name:    "missing user_id claim",
			token:   makeToken(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{"exp": future}),
			wantErr: true,
		},
		{
			name: "non-string user_id claim",
			token: makeToken(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{
				"user_id": 42, "exp": future,
			}),
			wantErr: true,
		},
		{
			name:    "garbage",
			token:   "not.a.jwt",
			wantErr: true,
		},
		{
			name:    "empty",
			token:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := g.verifyToken(tc.token)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if id != tc.wantID {
				t.Errorf("user id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	g := newTestGateway()
	tok := validToken(t, "alice")

	tests := []struct {
		name       string
		header     string
		query      string
		wantStatus int
	}{
		{"bearer header", "Bearer " + tok, "", http.StatusOK},
		{"query param", "", "?token=" + tok, http.StatusOK},
		{"no token", "", "", http.StatusUnauthorized},
		{"invalid token", "Bearer nope", "", http.StatusUnauthorized},
		{"non-bearer header falls back to query", "Basic abc", "?token=" + tok, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotUser string
			h := g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				gotUser = userIDFromContext(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/x"+tc.query, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && gotUser != "alice" {
				t.Errorf("user in context = %q, want alice", gotUser)
			}
		})
	}
}

// --- HTTP handlers -------------------------------------------------------

func TestHandleGetChatHistory(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		query      string
		chatErr    error
		wantStatus int
	}{
		{"participant as user1", http.MethodGet, "?user1=alice&user2=bob", nil, http.StatusOK},
		{"participant as user2", http.MethodGet, "?user1=bob&user2=alice", nil, http.StatusOK},
		{"not a participant", http.MethodGet, "?user1=bob&user2=carol", nil, http.StatusForbidden},
		{"missing user2", http.MethodGet, "?user1=alice", nil, http.StatusBadRequest},
		{"wrong method", http.MethodPost, "?user1=alice&user2=bob", nil, http.StatusMethodNotAllowed},
		{"chat service error", http.MethodGet, "?user1=alice&user2=bob", errors.New("down"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGateway()
			g.chat.err = tc.chatErr
			g.chat.history = &chat.GetMessageHistoryResponse{Messages: []*chat.Message{{MessageId: "m1", Content: "hi"}}}

			req := httptest.NewRequest(tc.method, "/chat/history"+tc.query, nil)
			req.Header.Set("Authorization", "Bearer "+validToken(t, "alice"))
			rec := httptest.NewRecorder()
			g.authMiddleware(g.handleGetChatHistory)(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			var body struct {
				Messages []struct {
					MessageID string `json:"message_id"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Messages) != 1 || body.Messages[0].MessageID != "m1" {
				t.Errorf("body = %+v", body)
			}
		})
	}
}

func TestHandleGetUser(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		err        error
		wantStatus int
		wantID     string
	}{
		{"existing user", "/user/abc123", nil, http.StatusOK, "abc123"},
		{"missing id", "/user/", nil, http.StatusBadRequest, ""},
		{"unknown user", "/user/nope", errors.New("not found"), http.StatusNotFound, "nope"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGateway()
			g.users.getResp = &users.GetUserResponse{UserId: "abc123", Username: "alice"}
			g.users.err = tc.err

			rec := httptest.NewRecorder()
			g.handleGetUser(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if g.users.lastGetID != tc.wantID {
				t.Errorf("looked up %q, want %q", g.users.lastGetID, tc.wantID)
			}
		})
	}
}

func TestHandleLoginAndRegister(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(*testGateway) http.HandlerFunc
		method     string
		body       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "login ok",
			handler:    func(g *testGateway) http.HandlerFunc { return g.handleLogin },
			method:     http.MethodPost,
			body:       `{"username":"alice","password":"pw"}`,
			wantStatus: http.StatusOK,
			wantBody:   `"token":"tok"`,
		},
		{
			name:       "login bad json",
			handler:    func(g *testGateway) http.HandlerFunc { return g.handleLogin },
			method:     http.MethodPost,
			body:       `{`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "login wrong method",
			handler:    func(g *testGateway) http.HandlerFunc { return g.handleLogin },
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "login backend error",
			handler:    func(g *testGateway) http.HandlerFunc { return g.handleLogin },
			method:     http.MethodPost,
			body:       `{}`,
			err:        errors.New("down"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "register ok",
			handler:    func(g *testGateway) http.HandlerFunc { return g.handleRegister },
			method:     http.MethodPost,
			body:       `{"username":"alice","password":"pw"}`,
			wantStatus: http.StatusOK,
			wantBody:   `"user_id":"u1"`,
		},
		{
			name:       "register bad json",
			handler:    func(g *testGateway) http.HandlerFunc { return g.handleRegister },
			method:     http.MethodPost,
			body:       `nope`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGateway()
			g.users.loginResp = &users.LoginUserResponse{Token: "tok", Success: true}
			g.users.createResp = &users.CreateUserResponse{UserId: "u1", Success: true}
			g.users.err = tc.err

			rec := httptest.NewRecorder()
			tc.handler(g)(rec, httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body)))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body %q does not contain %q", rec.Body, tc.wantBody)
			}
		})
	}
}

// --- WebSocket message routing -------------------------------------------

func TestHandleMessageSendMessage(t *testing.T) {
	tests := []struct {
		name     string
		content  interface{}
		wantSent bool
	}{
		{"valid", map[string]interface{}{"recipient_id": "bob", "message": "hi"}, true},
		{"missing recipient", map[string]interface{}{"message": "hi"}, false},
		{"missing message", map[string]interface{}{"recipient_id": "bob"}, false},
		{"wrong content type", "hi bob", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGateway()
			c := newTestClient("alice")
			g.handleMessage(c, &Message{Type: "send_message", Content: tc.content})

			if !tc.wantSent {
				if len(g.chat.sent) != 0 {
					t.Fatalf("unexpected SendMessage: %+v", g.chat.sent)
				}
				return
			}
			if len(g.chat.sent) != 1 {
				t.Fatalf("SendMessage calls = %d, want 1", len(g.chat.sent))
			}
			req := g.chat.sent[0]
			if req.SenderId != "alice" || req.RecipientId != "bob" || req.Message != "hi" {
				t.Errorf("request = %+v", req)
			}
		})
	}
}

func TestHandleMessageSenderCannotBeSpoofed(t *testing.T) {
	g := newTestGateway()
	c := newTestClient("alice")
	g.handleMessage(c, &Message{
		Type:    "send_message",
		UserID:  "mallory",
		Content: map[string]interface{}{"recipient_id": "bob", "message": "hi", "sender_id": "mallory"},
	})
	if len(g.chat.sent) != 1 || g.chat.sent[0].SenderId != "alice" {
		t.Fatalf("sender must come from the authenticated connection, got %+v", g.chat.sent)
	}
}

func TestHandleMessageEvents(t *testing.T) {
	tests := []struct {
		name      string
		msg       Message
		wantField string
		wantValue interface{}
	}{
		{
			name:      "typing started",
			msg:       Message{Type: "typing_event", Content: map[string]interface{}{"recipient_id": "bob", "is_typing": true}},
			wantField: "is_typing",
			wantValue: true,
		},
		{
			name:      "read receipt",
			msg:       Message{Type: "read_receipt", Content: map[string]interface{}{"recipient_id": "bob", "message_id": "m1"}},
			wantField: "message_id",
			wantValue: "m1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGateway()
			g.handleMessage(newTestClient("alice"), &tc.msg)

			msgs := g.bus.messages()
			if len(msgs) != 1 {
				t.Fatalf("published %d messages, want 1", len(msgs))
			}
			if msgs[0].Subject != "chat.events.bob" {
				t.Errorf("subject = %q, want chat.events.bob", msgs[0].Subject)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(msgs[0].Data, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["sender_id"] != "alice" {
				t.Errorf("sender_id = %v, want alice", payload["sender_id"])
			}
			if payload[tc.wantField] != tc.wantValue {
				t.Errorf("%s = %v, want %v", tc.wantField, payload[tc.wantField], tc.wantValue)
			}
		})
	}
}

func TestHandleMessageGetOnlineUsers(t *testing.T) {
	g := newTestGateway()
	g.presence.users = []string{"alice", "bob"}
	c := newTestClient("alice")

	g.handleMessage(c, &Message{Type: "get_online_users"})

	m := recvMessage(t, c)
	if m.Type != "online_users" {
		t.Fatalf("type = %q, want online_users", m.Type)
	}
	ids, _ := m.Content.([]interface{})
	if len(ids) != 2 {
		t.Errorf("content = %v, want 2 users", m.Content)
	}
}

func TestHandleMessageUnknownType(t *testing.T) {
	g := newTestGateway()
	c := newTestClient("alice")
	g.handleMessage(c, &Message{Type: "nope"})
	assertNoMessage(t, c)
	if len(g.bus.messages()) != 0 || len(g.chat.sent) != 0 {
		t.Error("unknown message type should be ignored")
	}
}

// --- NATS -> WebSocket fan-out -------------------------------------------

func registerClient(g *testGateway, c *Client) {
	g.subscribeToUserMessages(c.UserID, c)
	g.subscribeToUserEvents(c.UserID, c)
	g.clientsMutex.Lock()
	g.clients[c.UserID] = c
	g.clientsMutex.Unlock()
}

func TestUserMessagesDelivered(t *testing.T) {
	g := newTestGateway()
	c := newTestClient("bob")
	registerClient(g, c)

	data, _ := json.Marshal(&chat.Message{MessageId: "m1", SenderId: "alice", RecipientId: "bob", Content: "hi"})
	g.bus.deliver("chat.messages.bob", data)

	m := recvMessage(t, c)
	if m.Type != "new_message" {
		t.Fatalf("type = %q, want new_message", m.Type)
	}
	content, _ := m.Content.(map[string]interface{})
	if content["message_id"] != "m1" || content["content"] != "hi" {
		t.Errorf("content = %v", m.Content)
	}
}

func TestStaleClientIgnoresMessages(t *testing.T) {
	g := newTestGateway()
	old := newTestClient("bob")
	registerClient(g, old)
	current := newTestClient("bob")
	registerClient(g, current)

	data, _ := json.Marshal(&chat.Message{MessageId: "m1", Content: "hi"})
	g.bus.deliver("chat.messages.bob", data)

	recvMessage(t, current)
	assertNoMessage(t, old)
}

func TestUserEventsDelivered(t *testing.T) {
	g := newTestGateway()
	c := newTestClient("bob")
	registerClient(g, c)

	tests := []struct {
		name     string
		payload  interface{}
		wantType string
	}{
		{"typing", &chat.TypingEvent{SenderId: "alice", RecipientId: "bob", IsTyping: true}, "typing_event"},
		{"receipt", &chat.ReadReceipt{SenderId: "alice", RecipientId: "bob", MessageId: "m1"}, "read_receipt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(tc.payload)
			g.bus.deliver("chat.events.bob", data)
			if m := recvMessage(t, c); m.Type != tc.wantType {
				t.Errorf("type = %q, want %q", m.Type, tc.wantType)
			}
		})
	}
}

func TestDecodeUserEvent(t *testing.T) {
	marshal := func(v interface{}) []byte {
		b, _ := json.Marshal(v)
		return b
	}

	tests := []struct {
		name     string
		data     []byte
		wantType string
		wantOK   bool
	}{
		{"typing started", marshal(&chat.TypingEvent{SenderId: "a", IsTyping: true}), "typing_event", true},
		// is_typing=false is dropped by omitempty, so the payload has no is_typing key.
		{"typing stopped", marshal(&chat.TypingEvent{SenderId: "a", IsTyping: false}), "typing_event", true},
		{"read receipt", marshal(&chat.ReadReceipt{SenderId: "a", MessageId: "m1"}), "read_receipt", true},
		{"invalid json", []byte("{"), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := decodeUserEvent(tc.data)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if m.Type != tc.wantType {
				t.Errorf("type = %q, want %q", m.Type, tc.wantType)
			}
		})
	}
}

func TestTypingStoppedReachesClient(t *testing.T) {
	g := newTestGateway()
	alice, bob := newTestClient("alice"), newTestClient("bob")
	registerClient(g, bob)

	g.handleMessage(alice, &Message{Type: "typing_event", Content: map[string]interface{}{"recipient_id": "bob", "is_typing": false}})
	for _, m := range g.bus.messages() {
		g.bus.deliver(m.Subject, m.Data)
	}

	if m := recvMessage(t, bob); m.Type != "typing_event" {
		t.Fatalf("type = %q, want typing_event", m.Type)
	}
}

func TestUserStatusBroadcast(t *testing.T) {
	g := newTestGateway()
	g.subscribeToUserStatus()
	a, b := newTestClient("alice"), newTestClient("bob")
	registerClient(g, a)
	registerClient(g, b)

	data, _ := json.Marshal(&presence.UserStatusEvent{UserId: "carol", Online: true})
	g.bus.deliver("users.status", data)

	for _, c := range []*Client{a, b} {
		m := recvMessage(t, c)
		if m.Type != "user_status" {
			t.Errorf("%s got type %q, want user_status", c.UserID, m.Type)
		}
	}
}

func TestSlowClientDoesNotPanic(t *testing.T) {
	g := newTestGateway()
	g.subscribeToUserStatus()
	slow := &Client{UserID: "bob", Send: make(chan []byte, 1), Limiter: rate.NewLimiter(rate.Inf, 1)}
	registerClient(g, slow)

	status, _ := json.Marshal(&presence.UserStatusEvent{UserId: "carol", Online: true})
	msg, _ := json.Marshal(&chat.Message{MessageId: "m1"})

	// The buffer holds one message; everything after that must be dropped
	// rather than closing the channel (which would panic on the next send).
	for i := 0; i < 3; i++ {
		g.bus.deliver("users.status", status)
		g.bus.deliver("chat.messages.bob", msg)
	}

	if got := len(slow.Send); got != 1 {
		t.Errorf("buffered = %d, want 1", got)
	}
	g.clientsMutex.RLock()
	defer g.clientsMutex.RUnlock()
	if g.clients["bob"] != slow {
		t.Error("slow client should stay registered until its connection closes")
	}
}

// --- WebSocket end-to-end ------------------------------------------------

func TestWebSocketRejectsInvalidToken(t *testing.T) {
	g := newTestGateway()
	srv := httptest.NewServer(http.HandlerFunc(g.handleWebSocket))
	defer srv.Close()

	for _, q := range []string{"", "?token=bad"} {
		resp, err := http.Get(srv.URL + "/ws" + q)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("query %q: status = %d, want 401", q, resp.StatusCode)
		}
	}
}

func dialWS(t *testing.T, srv *httptest.Server, userID string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=" + validToken(t, userID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func (g *testGateway) currentClient(userID string) *Client {
	g.clientsMutex.RLock()
	defer g.clientsMutex.RUnlock()
	return g.clients[userID]
}

func readWS(t *testing.T, conn *websocket.Conn) Message {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var m Message
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatalf("read: %v", err)
	}
	return m
}

func TestWebSocketDeliversMessages(t *testing.T) {
	g := newTestGateway()
	srv := httptest.NewServer(http.HandlerFunc(g.handleWebSocket))
	defer srv.Close()

	conn := dialWS(t, srv, "bob")
	waitFor(t, "bob to register", func() bool { return g.currentClient("bob") != nil })

	data, _ := json.Marshal(&chat.Message{MessageId: "m1", SenderId: "alice", Content: "hi"})
	g.bus.deliver("chat.messages.bob", data)

	if m := readWS(t, conn); m.Type != "new_message" {
		t.Fatalf("type = %q, want new_message", m.Type)
	}

	// Client -> server: send_message is routed to the chat service.
	if err := conn.WriteJSON(Message{Type: "send_message", Content: map[string]string{"recipient_id": "alice", "message": "yo"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "SendMessage", func() bool {
		g.chat.mu.Lock()
		defer g.chat.mu.Unlock()
		return len(g.chat.sent) == 1
	})

	// Disconnect marks the user offline and unregisters it.
	conn.Close()
	waitFor(t, "bob to unregister", func() bool { return g.currentClient("bob") == nil })
	waitFor(t, "SetUserOffline", func() bool { return len(g.presence.offlineCalls()) == 1 })
}

func TestWebSocketReconnectKeepsNewConnection(t *testing.T) {
	g := newTestGateway()
	srv := httptest.NewServer(http.HandlerFunc(g.handleWebSocket))
	defer srv.Close()

	first := dialWS(t, srv, "bob")
	waitFor(t, "first connection", func() bool { return g.currentClient("bob") != nil })
	firstClient := g.currentClient("bob")

	second := dialWS(t, srv, "bob")
	waitFor(t, "second connection", func() bool {
		c := g.currentClient("bob")
		return c != nil && c != firstClient
	})

	// The gateway closes the old socket; wait for its read loop to finish.
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := first.ReadMessage(); err != nil {
			break
		}
	}
	time.Sleep(50 * time.Millisecond) // let the old read loop run its cleanup

	if g.currentClient("bob") == nil {
		t.Fatal("old connection teardown unregistered the new connection")
	}
	if calls := g.presence.offlineCalls(); len(calls) != 0 {
		t.Fatalf("user marked offline while still connected: %v", calls)
	}

	data, _ := json.Marshal(&chat.Message{MessageId: "m1", Content: "hi"})
	g.bus.deliver("chat.messages.bob", data)
	if m := readWS(t, second); m.Type != "new_message" {
		t.Fatalf("type = %q, want new_message", m.Type)
	}
}

func TestWebSocketDisconnectStopsGoroutines(t *testing.T) {
	g := newTestGateway()
	srv := httptest.NewServer(http.HandlerFunc(g.handleWebSocket))
	defer srv.Close()
	srv.Client() // start any lazily created server goroutines up front

	base := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		conn := dialWS(t, srv, "bob")
		waitFor(t, "registration", func() bool { return g.currentClient("bob") != nil })
		conn.Close()
		waitFor(t, "unregistration", func() bool { return g.currentClient("bob") == nil })
	}

	// Both pumps must exit once the client disconnects.
	waitFor(t, "pump goroutines to exit", func() bool { return runtime.NumGoroutine() <= base })
}
