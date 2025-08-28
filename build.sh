#!/bin/bash

echo "Building Rubix Simulator for Production..."
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# Build backend
echo "Building backend..."
cd backend
go build -o rubix-simulator cmd/server/main.go
if [ $? -ne 0 ]; then
    echo -e "${RED}Backend build failed!${NC}"
    exit 1
fi
echo -e "${GREEN}Backend built successfully: backend/rubix-simulator${NC}"
cd ..
echo ""

# Build frontend
echo "Building frontend..."
cd frontend
if [ ! -d "node_modules" ]; then
    echo "Installing frontend dependencies..."
    npm install
fi
npm run build
if [ $? -ne 0 ]; then
    echo -e "${RED}Frontend build failed!${NC}"
    exit 1
fi
echo -e "${GREEN}Frontend built successfully: frontend/dist${NC}"
cd ..
echo ""

echo "========================================"
echo -e "${GREEN}Build completed successfully!${NC}"
echo ""
echo "Backend executable: backend/rubix-simulator"
echo "Frontend build: frontend/dist"
echo ""
echo "To run in production:"
echo "  1. Run backend/rubix-simulator"
echo "  2. Serve frontend/dist with a web server"
echo "========================================"