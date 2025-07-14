# KubeChat Demo Guide

## Quick Demo Setup

The fastest way to try out KubeChat is using the demo docker-compose setup.

### Prerequisites

- Docker and Docker Compose installed
- Ports 8080, 4222, 5432, and 50051-50054 available

### Start the Demo

1. **Clone and start the demo:**
```bash
cd docker
docker-compose -f docker-compose.demo.yml up --build
```

2. **Wait for all services to start** (usually takes 30-60 seconds)

3. **Open the demo in your browser:**
```
http://localhost:8080
```

### Demo Features

The demo includes:
- **Web-based chat interface** with real-time messaging
- **User authentication** (register/login)
- **Online presence indicators**
- **Multiple user support**
- **Pre-created demo users** for testing

### Testing the Demo

#### Option 1: Use Pre-created Users
The demo automatically creates these test users:
- Username: `alice`, Password: `password123`
- Username: `bob`, Password: `password123`
- Username: `charlie`, Password: `password123`

#### Option 2: Create Your Own Users
1. Click "Register" and create a new user
2. Login with your credentials

#### Multi-User Chat Test
1. **Open multiple browser tabs/windows** to `http://localhost:8080`
2. **Login as different users** in each tab
3. **Start chatting** between the users
4. **Watch real-time messages** and presence updates

### Demo Walkthrough

1. **Register/Login:**
   - Use the left panel to authenticate
   - Try both registration and login

2. **See Online Users:**
   - Check the right panel for online users
   - Open another browser tab and login as a different user
   - See the user list update in real-time

3. **Send Messages:**
   - Click on a user in the right panel to start chatting
   - Type a message and press Enter or click Send
   - Watch messages appear instantly in other browser tabs

4. **Test Presence:**
   - Close a browser tab and watch the user go offline
   - Refresh the user list to see updates

### API Testing

You can also test the APIs directly:

#### Register a new user:
```bash
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpass", "email": "test@example.com"}'
```

#### Login:
```bash
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpass"}'
```

### WebSocket Testing

Connect to WebSocket for real-time messaging:
```javascript
const ws = new WebSocket('ws://localhost:8080/ws?user_id=YOUR_USER_ID');

// Send a message
ws.send(JSON.stringify({
    type: 'send_message',
    content: {
        recipient_id: 'RECIPIENT_USER_ID',
        message: 'Hello!'
    }
}));

// Get online users
ws.send(JSON.stringify({
    type: 'get_online_users'
}));
```

### Monitoring

#### NATS Monitoring
View NATS monitoring dashboard:
```
http://localhost:8222
```

#### Service Logs
View logs for any service:
```bash
# View all logs
docker-compose -f docker-compose.demo.yml logs -f

# View specific service logs
docker-compose -f docker-compose.demo.yml logs -f api-gateway
docker-compose -f docker-compose.demo.yml logs -f chat-service
docker-compose -f docker-compose.demo.yml logs -f users-service
```

#### Database Access
Connect to PostgreSQL:
```bash
docker-compose -f docker-compose.demo.yml exec postgres psql -U user -d kubechat
```

Then run SQL queries:
```sql
-- View all messages
SELECT * FROM messages ORDER BY timestamp DESC;

-- Count messages
SELECT COUNT(*) FROM messages;
```

### Stopping the Demo

```bash
docker-compose -f docker-compose.demo.yml down

# Remove volumes (this will delete all data)
docker-compose -f docker-compose.demo.yml down -v
```

### Troubleshooting

#### Services not starting?
- Check if ports are available: `netstat -tulpn | grep -E '(8080|4222|5432|5005[1-4])'`
- Check logs: `docker-compose -f docker-compose.demo.yml logs`

#### Can't connect to chat?
- Make sure you're logged in first
- Check that WebSocket connection is established (green status)
- Try refreshing the page

#### Messages not appearing?
- Ensure both users are online
- Check browser console for errors
- Verify NATS is running: `docker-compose -f docker-compose.demo.yml logs nats`

#### Database issues?
- Wait for PostgreSQL to fully start (check healthcheck)
- Check database logs: `docker-compose -f docker-compose.demo.yml logs postgres`

### Architecture in Demo

The demo showcases the complete microservices architecture:

```
Browser (http://localhost:8080)
    ↓
API Gateway (WebSocket + REST)
    ↓
┌─────────────┬─────────────┬─────────────────┐
│ Users       │ Chat        │ Presence        │
│ Service     │ Service     │ Service         │
└─────────────┴─────────────┴─────────────────┘
    ↓               ↓               ↓
PostgreSQL    Message Store   NATS Messaging
              Service         (Real-time)
```

Each service runs in its own container and communicates via gRPC and NATS, demonstrating the scalable microservices pattern.