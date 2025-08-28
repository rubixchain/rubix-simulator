#!/bin/bash

echo "Starting Rubix Simulator..."
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Start backend server
echo "Starting backend server..."
(cd backend && go run cmd/server/main.go) &
BACKEND_PID=$!

# Wait for backend to start
echo "Waiting for backend to initialize..."
sleep 5

# Check if backend is running
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}Backend started successfully on http://localhost:8080${NC}"
else
    echo -e "${RED}Backend failed to start!${NC}"
    exit 1
fi

echo ""

# Start frontend
echo "Starting frontend..."
cd frontend

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo "Installing frontend dependencies..."
    npm install
fi

echo -e "${GREEN}Frontend starting on http://localhost:5173${NC}"
echo ""
echo "========================================"
echo -e "${GREEN}Rubix Simulator is running!${NC}"
echo "Backend: http://localhost:8080"
echo "Frontend: http://localhost:5173"
echo ""
echo "Press Ctrl+C to stop"
echo "========================================"
echo ""

# Trap Ctrl+C and cleanup
trap 'echo "Stopping..."; kill $BACKEND_PID 2>/dev/null; exit' INT

# Run frontend (this will block)
npm run dev