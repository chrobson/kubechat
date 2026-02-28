# KubeChat - Microservices Chat Application

A real-time chat application built with Go microservices, Kubernetes, gRPC, and NATS.

## ✅ Completed Features

### Core Services
- ✅ **users-service**: User registration, authentication with JWT tokens.
- ✅ **chat-service**: Real-time messaging with NATS integration.
- ✅ **presence-service**: User online/offline status tracking.
- ✅ **message-store-service**: PostgreSQL message persistence.
- ✅ **media-service**: File upload and storage management using MinIO (S3-compatible).
- ✅ **api-gateway**: REST API, WebSocket gateway, and Media Proxy.

### Real-time Features
- ✅ **Secure WebSockets**: JWT-based authentication for real-time connections.
- ✅ **Typing Indicators**: Real-time "is typing..." status via NATS.
- ✅ **Read Receipts**: Visual confirmation (✓✓) when messages are read.
- ✅ **Multimedia Support**: Image sharing with automatic preview in the web client.
- ✅ **Presence Updates**: Live updates of user online/offline status.

### Infrastructure & Security
- ✅ **JWT Authentication**: Secure endpoints and session management.
- ✅ **Rate Limiting**: Protection against spam and DoS at the API Gateway level.
- ✅ **S3 Storage**: MinIO integration for persistent multimedia storage.
- ✅ **gRPC APIs**: High-performance inter-service communication.
- ✅ **Docker & Kubernetes**: Containerized services with ready-to-use manifests.

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Frontend  │───▶│ API Gateway  │───▶│    NATS     │
│ (WebSocket) │    │   (Port 8080)│    │ (Port 4222) │
└─────────────┘    └──────────────┘    └─────────────┘
          │               │                    │
          ▼               ▼                    ▼
   ┌────────────┐  ┌─────────────┐    ┌─────────────┐
   │   MinIO    │◀─│ Media       │    │ Presence    │
   │ (Port 9000)│  │ Service     │    │ Service     │
   └────────────┘  │ (Port 50055)│    │ (Port 50052)│
                   └─────────────┘    └─────────────┘
                          │                    │
          ┌───────────────┴──────────┐         │
          ▼                          ▼         ▼
   ┌─────────────┐            ┌─────────────┐  ┌─────────────┐
   │ Users       │            │ Chat        │  │ Message     │
   │ Service     │            │ Service     │  │ Store       │
   │ (Port 50051)│            │ (Port 50053)│  │ (Port 50054)│
   └─────────────┘            └─────────────┘  └─────────────┘
                                                      │
                                                      ▼
                                             ┌─────────────┐
                                             │ PostgreSQL  │
                                             │ (Port 5432) │
                                             └─────────────┘
```

## Technologies

- **Go 1.24** with gRPC for microservices
- **NATS** for real-time messaging and ephemeral events
- **MinIO** for S3-compatible object storage
- **WebSocket** for real-time client communication
- **PostgreSQL** for persistent message storage
- **Docker** and **Docker Compose** for containerization
- **Kubernetes** for orchestration
- **JWT** for secure authentication
- **Prometheus** for metrics collection

## Quick Start

### 🎯 Try the Demo (Recommended)
```bash
./demo/start-demo.sh
```
Then open http://localhost:8080 in your browser!

### Using Docker Compose
```bash
cd docker
docker-compose up --build
```

### Using Kubernetes
```bash
kubectl apply -f k8s/
```

## API Endpoints

- **POST /register** - User registration
- **POST /login** - User authentication (returns JWT)
- **POST /upload** - Upload media (requires JWT)
- **GET /ws?token=JWT** - Secure WebSocket connection

## License

This project is licensed under the **MIT License** - see the [LICENSE](LICENSE) file for details.

## Documentation

- 🎯 [Demo Guide](DEMO.md) - Quick demo setup
- 📖 [Deployment Guide](DEPLOYMENT.md) - Local and K8s deployment
- 🧪 [Testing Guide](TESTING.md) - Integration tests
- 🔧 [Makefile](Makefile) - Build commands
