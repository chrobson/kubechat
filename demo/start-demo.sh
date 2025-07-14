#!/bin/bash

echo "🚀 Starting KubeChat Demo..."
echo ""

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker first."
    exit 1
fi

# Check if docker-compose is available
if ! command -v docker-compose >/dev/null 2>&1; then
    echo "❌ docker-compose is not installed. Please install docker-compose first."
    exit 1
fi

# Navigate to docker directory
cd "$(dirname "$0")/../docker" || exit 1

echo "📦 Building and starting all services..."
docker-compose -f docker-compose.demo.yml up --build -d

echo ""
echo "⏳ Waiting for services to start..."

# Wait for API Gateway to be ready
for i in {1..30}; do
    if curl -s http://localhost:8080 >/dev/null 2>&1; then
        break
    fi
    echo -n "."
    sleep 2
done

echo ""
echo ""

# Check if services are running
if curl -s http://localhost:8080 >/dev/null 2>&1; then
    echo "✅ KubeChat Demo is ready!"
    echo ""
    echo "🌐 Open your browser and go to:"
    echo "   http://localhost:8080"
    echo ""
    echo "👥 Demo users (password: password123):"
    echo "   - alice"
    echo "   - bob"
    echo "   - charlie"
    echo ""
    echo "📊 Monitoring:"
    echo "   - NATS Dashboard: http://localhost:8222"
    echo ""
    echo "🛑 To stop the demo:"
    echo "   docker-compose -f docker-compose.demo.yml down"
    echo ""
else
    echo "❌ Failed to start the demo. Check the logs:"
    echo "   docker-compose -f docker-compose.demo.yml logs"
fi