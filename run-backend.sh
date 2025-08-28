#!/bin/bash

echo "Starting Rubix Backend Server..."
echo ""

cd backend

# Build the backend (optional, for production)
# echo "Building backend..."
# go build -o rubix-simulator cmd/server/main.go

# Run the backend
echo "Starting server on port 8080..."
go run cmd/server/main.go