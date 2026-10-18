# KubeChat Testing Guide

## Unit Tests

Unit tests live next to each service (`services/*/main_test.go`) and need no
running infrastructure: NATS, gRPC clients and MinIO are replaced with
in-process fakes, Redis with [miniredis](https://github.com/alicebob/miniredis)
and Postgres with [sqlmock](https://github.com/DATA-DOG/go-sqlmock).

```bash
make test                        # go test -race ./...
go test -cover ./...             # with coverage
go test ./services/api-gateway/  # single service
```

What is covered:

- **api-gateway**: JWT verification (expiry, wrong secret, `alg: none`, bad
  claims), auth middleware, HTTP handlers, history pagination parameters,
  WebSocket message routing, NATS fan-out to clients, and end-to-end WebSocket
  sessions (delivery, reconnect, disconnect cleanup).
- **users**: registration, duplicate/invalid input, login and issued JWT claims.
- **chat**: publishing to recipient and sender subjects, persistence, failure
  handling, history mapping.
- **message-store**: keyset vs offset pagination queries, limit clamping,
  idempotent inserts, error propagation, delete authorization.
- **presence**: online/offline state in Redis and status events.
- **media**: uploads (size limit, response encoding) and metadata lookup.

CI (`.github/workflows/ci.yml`) runs `gofmt`, `go build`, `go vet` and
`go test -race` on every push and pull request.

The sections below describe manual checks against a running stack
(`docker-compose up` in `docker/`).

## API Testing

### 1. User Registration

```bash
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "alice",
    "password": "password123",
    "email": "alice@example.com"
  }'
```

Expected response:
```json
{
  "user_id": "abc123...",
  "success": true,
  "message": "User created successfully"
}
```

### 2. User Login

```bash
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "alice",
    "password": "password123"
  }'
```

Expected response:
```json
{
  "user_id": "abc123...",
  "token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9...",
  "success": true,
  "message": "Login successful"
}
```

### 3. Chat History

History is returned newest first (`limit` defaults to 50, max 200). To load
older messages, pass the last message of the previous page as the cursor:
`before_id` is its `message_id` and `before` its `timestamp` in RFC 3339 form,
keeping the fractional seconds.

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/chat/history?user1=$ALICE&user2=$BOB&limit=20"

curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/chat/history?user1=$ALICE&user2=$BOB&limit=20&before=2024-01-01T12:00:00Z&before_id=MESSAGE_ID"
```

## WebSocket Testing

### Connect to WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/ws?token=JWT_TOKEN');

ws.onopen = function(event) {
    console.log('Connected to WebSocket');
    
    // Send a message
    ws.send(JSON.stringify({
        type: 'send_message',
        content: {
            recipient_id: 'RECIPIENT_USER_ID',
            message: 'Hello, World!'
        }
    }));
};

ws.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Received:', data);
};
```

### Get Online Users

```javascript
ws.send(JSON.stringify({
    type: 'get_online_users'
}));
```

## gRPC Testing

### Install grpcurl

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

### Test Users Service

```bash
# Create user
grpcurl -plaintext -d '{
  "username": "bob",
  "password": "password123",
  "email": "bob@example.com"
}' localhost:50051 users.UsersService/CreateUser

# Login user
grpcurl -plaintext -d '{
  "username": "bob",
  "password": "password123"
}' localhost:50051 users.UsersService/LoginUser

# Get user
grpcurl -plaintext -d '{
  "user_id": "USER_ID_HERE"
}' localhost:50051 users.UsersService/GetUser
```

### Test Presence Service

```bash
# Set user online
grpcurl -plaintext -d '{
  "user_id": "USER_ID_HERE"
}' localhost:50052 presence.PresenceService/SetUserOnline

# Get user status
grpcurl -plaintext -d '{
  "user_id": "USER_ID_HERE"
}' localhost:50052 presence.PresenceService/GetUserStatus

# Get online users
grpcurl -plaintext -d '{}' localhost:50052 presence.PresenceService/GetOnlineUsers
```

### Test Chat Service

```bash
# Send message
grpcurl -plaintext -d '{
  "sender_id": "SENDER_ID",
  "recipient_id": "RECIPIENT_ID",
  "message": "Hello from gRPC!"
}' localhost:50053 chat.ChatService/SendMessage

# Get message history
grpcurl -plaintext -d '{
  "user_id1": "USER1_ID",
  "user_id2": "USER2_ID",
  "limit": 10
}' localhost:50053 chat.ChatService/GetMessageHistory
```

## NATS Testing

### Install NATS CLI

```bash
go install github.com/nats-io/natscli/nats@latest
```

### Subscribe to Messages

```bash
# Subscribe to user status updates
nats sub "users.status"

# Subscribe to chat messages for a specific user
nats sub "chat.messages.USER_ID"
```

### Publish Test Messages

```bash
# Publish user status
nats pub "users.status" '{"user_id":"test123","online":true,"timestamp":{"seconds":1704110400}}'

# Publish chat message
nats pub "chat.messages.USER_ID" '{"message_id":"msg123","sender_id":"sender123","recipient_id":"USER_ID","content":"Test message","timestamp":{"seconds":1704110400}}'
```

## Integration Testing

### Test Complete Chat Flow

1. Register two users (Alice and Bob)
2. Login both users to get user IDs
3. Connect both users via WebSocket
4. Send message from Alice to Bob
5. Verify Bob receives the message
6. Check message history
7. Verify presence updates

### Load Testing

```bash
# Install hey for load testing
go install github.com/rakyll/hey@latest

# Test user registration endpoint
hey -n 100 -c 10 -m POST -H "Content-Type: application/json" \
  -d '{"username":"user","password":"pass","email":"user@test.com"}' \
  http://localhost:8080/register

# Test user login endpoint
hey -n 100 -c 10 -m POST -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"password123"}' \
  http://localhost:8080/login
```

## Monitoring

### Check Service Health

```bash
# Check if all services are running
docker-compose ps

# Or in Kubernetes
kubectl get pods -n kubechat
```

### NATS Monitoring

Access NATS monitoring at: http://localhost:8222

### Database Monitoring

```bash
# Connect to PostgreSQL
docker-compose -f docker/docker-compose.yml exec postgres psql -U user -d kubechat

# Check messages table
SELECT COUNT(*) FROM messages;
SELECT * FROM messages ORDER BY timestamp DESC LIMIT 10;
```

## Performance Testing

### WebSocket Connection Test

```javascript
// Test multiple simultaneous connections (tokens obtained via /login)
const connections = [];
for (let i = 0; i < 100; i++) {
    const ws = new WebSocket(`ws://localhost:8080/ws?token=${tokens[i]}`);
    connections.push(ws);
}
```

### Message Throughput Test

```bash
# Use NATS bench for testing message throughput
nats bench "chat.messages.test" --msgs 10000 --size 1024
```