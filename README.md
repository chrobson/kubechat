# KubeChat - Microservices Chat Application

A real-time chat application built with Go microservices, Kubernetes, gRPC, and NATS.

## ✅ Completed Features

### Core Services
- ✅ **users-service**: User registration, authentication with JWT tokens
- ✅ **chat-service**: Real-time messaging with NATS integration
- ✅ **presence-service**: User online/offline status tracking
- ✅ **message-store-service**: PostgreSQL message persistence
- ✅ **api-gateway**: REST API and WebSocket gateway for clients

### Infrastructure
- ✅ **gRPC APIs**: Full protobuf definitions and implementations
- ✅ **NATS Integration**: Real-time messaging and user status updates
- ✅ **WebSocket Support**: Real-time client communication
- ✅ **Docker Configuration**: Multi-stage builds and docker-compose
- ✅ **Kubernetes Manifests**: Complete K8s deployment files
- ✅ **Documentation**: Deployment and testing guides

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Frontend  │───▶│ API Gateway  │───▶│    NATS     │
│ (WebSocket) │    │   (Port 8080)│    │ (Port 4222) │
└─────────────┘    └──────────────┘    └─────────────┘
                          │                    │
                          ▼                    ▼
                   ┌─────────────┐    ┌─────────────┐
                   │ Users       │    │ Presence    │
                   │ Service     │    │ Service     │
                   │ (Port 50051)│    │ (Port 50052)│
                   └─────────────┘    └─────────────┘
                          │                    │
                          ▼                    ▼
                   ┌─────────────┐    ┌─────────────┐
                   │ Chat        │    │ Message     │
                   │ Service     │    │ Store       │
                   │ (Port 50053)│    │ (Port 50054)│
                   └─────────────┘    └─────────────┘
                                             │
                                             ▼
                                    ┌─────────────┐
                                    │ PostgreSQL  │
                                    │ (Port 5432) │
                                    └─────────────┘
```

## Technologies

- **Go 1.21** with gRPC for microservices
- **NATS** for real-time messaging and event streaming
- **WebSocket** for real-time client communication
- **PostgreSQL** for persistent message storage
- **Docker** and **Docker Compose** for containerization
- **Kubernetes** for orchestration and scaling
- **JWT** for authentication
- **Protocol Buffers** for service definitions

## Quick Start

### 🎯 Try the Demo (Recommended)
```bash
./demo/start-demo.sh
```
Then open http://localhost:8080 in your browser!

### Using Docker Compose
```bash
cd docker
docker-compose -f docker-compose.demo.yml up --build
```

### Using Kubernetes
```bash
kubectl apply -f k8s/
```

### Manual Development
```bash
make proto    # Generate protobuf files
make deps     # Install dependencies
make build    # Build all services
make run-gateway  # Start API gateway
```

## API Endpoints

- **POST /register** - User registration
- **POST /login** - User authentication
- **GET /ws?user_id=X** - WebSocket connection for real-time chat

## WebSocket Messages

### Send Message
```json
{
  "type": "send_message",
  "content": {
    "recipient_id": "user123",
    "message": "Hello!"
  }
}
```

### Get Online Users
```json
{
  "type": "get_online_users"
}
```

## Documentation

- 🎯 [Demo Guide](DEMO.md) - Quick demo setup with web interface
- 📖 [Deployment Guide](DEPLOYMENT.md) - How to deploy locally and on Kubernetes
- 🧪 [Testing Guide](TESTING.md) - API testing, WebSocket testing, and integration tests
- 🔧 [Makefile](Makefile) - Build and development commands

## Project Structure

```
kubechat/
├── services/           # Microservices implementations
│   ├── users/         # User management service
│   ├── chat/          # Chat messaging service
│   ├── presence/      # User presence service
│   ├── message-store/ # Message persistence service
│   └── api-gateway/   # WebSocket and REST gateway
├── proto/             # Protocol buffer definitions
├── k8s/               # Kubernetes manifests
├── docker/            # Docker files and compose
├── demo/              # Demo setup and web interface
│   ├── index.html     # Web-based chat interface
│   └── start-demo.sh  # Quick demo startup script
├── DEMO.md            # Demo guide
├── DEPLOYMENT.md      # Deployment instructions
└── TESTING.md         # Testing guide
```

## Development

Each service runs independently and communicates via gRPC. NATS handles real-time messaging between services and clients. The API Gateway provides WebSocket connections for real-time client communication.

For detailed setup and testing instructions, see [DEPLOYMENT.md](DEPLOYMENT.md) and [TESTING.md](TESTING.md).