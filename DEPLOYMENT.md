# KubeChat Deployment Guide

## Prerequisites

- Go 1.21+
- Docker and Docker Compose
- Kubernetes cluster (local or cloud)
- kubectl configured
- NATS server
- PostgreSQL (for production)

## Local Development

### Using Docker Compose (Recommended)

1. Build and start all services:
```bash
cd docker
docker-compose up --build
```

2. Services will be available at:
- API Gateway: http://localhost:8080
- Users Service: localhost:50051
- Presence Service: localhost:50052
- Chat Service: localhost:50053
- Message Store Service: localhost:50054
- NATS: localhost:4222
- PostgreSQL: localhost:5432

### Manual Setup

1. Start NATS:
```bash
docker run -p 4222:4222 -p 6222:6222 -p 8222:8222 nats:latest
```

2. Start PostgreSQL:
```bash
docker run -e POSTGRES_DB=kubechat -e POSTGRES_USER=user -e POSTGRES_PASSWORD=password -p 5432:5432 postgres:13
```

3. Generate protobuf files:
```bash
make proto
```

4. Install dependencies:
```bash
go mod tidy
```

5. Start services in separate terminals:
```bash
make run-users
make run-presence
make run-chat
make run-messagestore
make run-gateway
```

## Kubernetes Deployment

### Deploy to Kubernetes

1. Create namespace:
```bash
kubectl apply -f k8s/namespace.yaml
```

2. Deploy infrastructure (NATS, PostgreSQL):
```bash
kubectl apply -f k8s/nats.yaml
kubectl apply -f k8s/postgres.yaml
```

3. Wait for infrastructure to be ready:
```bash
kubectl wait --for=condition=ready pod -l app=nats -n kubechat --timeout=60s
kubectl wait --for=condition=ready pod -l app=postgres -n kubechat --timeout=60s
```

4. Build Docker images:

### Option A: Local Development (Recommended)
For local development without external registry:
```bash
docker build -f docker/Dockerfile.users -t kubechat/users-service:latest .
docker build -f docker/Dockerfile.presence -t kubechat/presence-service:latest .
docker build -f docker/Dockerfile.chat -t kubechat/chat-service:latest .
docker build -f docker/Dockerfile.messagestore -t kubechat/message-store-service:latest .
docker build -f docker/Dockerfile.gateway -t kubechat/api-gateway:latest .
```

**Note:** All deployment files use `imagePullPolicy: IfNotPresent` by default, so locally built images will be used without requiring an external registry.

### Option B: External Registry (Production)
For production deployment with external registry:
```bash
# Build and tag with your registry
docker build -f docker/Dockerfile.users -t your-registry/kubechat/users-service:latest .
docker build -f docker/Dockerfile.presence -t your-registry/kubechat/presence-service:latest .
docker build -f docker/Dockerfile.chat -t your-registry/kubechat/chat-service:latest .
docker build -f docker/Dockerfile.messagestore -t your-registry/kubechat/message-store-service:latest .
docker build -f docker/Dockerfile.gateway -t your-registry/kubechat/api-gateway:latest .

# Push to your registry
docker push your-registry/kubechat/users-service:latest
docker push your-registry/kubechat/presence-service:latest
docker push your-registry/kubechat/chat-service:latest
docker push your-registry/kubechat/message-store-service:latest
docker push your-registry/kubechat/api-gateway:latest
```

Then update the image names in the deployment files and change `imagePullPolicy: IfNotPresent` to `imagePullPolicy: Always` if needed.

5. Deploy services:
```bash
kubectl apply -f k8s/users-service.yaml
kubectl apply -f k8s/presence-service.yaml
kubectl apply -f k8s/message-store-service.yaml
kubectl apply -f k8s/chat-service.yaml
kubectl apply -f k8s/api-gateway.yaml
```

6. Check deployment status:
```bash
kubectl get pods -n kubechat
kubectl get services -n kubechat
```

7. Access the application:
```bash
kubectl port-forward service/api-gateway 8080:80 -n kubechat
```

## Service Ports

- users-service: 50051
- presence-service: 50052
- chat-service: 50053
- message-store-service: 50054
- api-gateway: 8080
- NATS: 4222
- PostgreSQL: 5432

## Environment Variables

### chat-service
- `NATS_URL`: NATS connection URL (default: nats://localhost:4222)
- `MESSAGE_STORE_URL`: Message store service URL (default: localhost:50054)

### presence-service
- `NATS_URL`: NATS connection URL (default: nats://localhost:4222)

### message-store-service
- `NATS_URL`: NATS connection URL (default: nats://localhost:4222)
- `DATABASE_URL`: PostgreSQL connection URL

### api-gateway
- `NATS_URL`: NATS connection URL (default: nats://localhost:4222)
- `USERS_SERVICE_URL`: Users service URL (default: localhost:50051)
- `CHAT_SERVICE_URL`: Chat service URL (default: localhost:50053)
- `PRESENCE_SERVICE_URL`: Presence service URL (default: localhost:50052)

## Troubleshooting

### Check service logs
```bash
kubectl logs -f deployment/users-service -n kubechat
kubectl logs -f deployment/presence-service -n kubechat
kubectl logs -f deployment/chat-service -n kubechat
kubectl logs -f deployment/message-store-service -n kubechat
kubectl logs -f deployment/api-gateway -n kubechat
```

### Check NATS connectivity
```bash
kubectl exec -it deployment/nats -n kubechat -- nats pub test "hello"
```

### Check database connectivity
```bash
kubectl exec -it deployment/postgres -n kubechat -- psql -U user -d kubechat
```