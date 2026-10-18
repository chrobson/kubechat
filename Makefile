.PHONY: proto clean build test vet fmt run-users run-chat run-presence run-messagestore run-media run-gateway deps

# Generate protobuf files
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/**/*.proto

# Clean generated files
clean:
	find . -name "*.pb.go" -delete

# Build all services
build:
	go build -o bin/users-service ./services/users
	go build -o bin/chat-service ./services/chat
	go build -o bin/presence-service ./services/presence
	go build -o bin/messagestore-service ./services/message-store
	go build -o bin/media-service ./services/media
	go build -o bin/api-gateway ./services/api-gateway

# Run unit tests (no external services required)
test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w services

# Run individual services
run-users:
	go run ./services/users

run-chat:
	go run ./services/chat

run-presence:
	go run ./services/presence

run-messagestore:
	go run ./services/message-store

run-media:
	go run ./services/media

run-gateway:
	go run ./services/api-gateway

# Install dependencies
deps:
	go mod tidy
	go mod download