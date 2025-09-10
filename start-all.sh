#!/bin/bash

pwd

echo "==================================="
echo "Starting Rubix Network Simulator"
echo "==================================="
echo ""

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "Shutting down services..."
    kill $BACKEND_PID 2>/dev/null
    kill $FRONTEND_PID 2>/dev/null
    exit
}

# Set up trap for cleanup
trap cleanup INT TERM

# Start Backend
echo "Starting Go Backend on port 8080..."
cd backend && go run cmd/server/main.go &
BACKEND_PID=$!
cd ..

# Wait for backend to start
sleep 3

# Start Frontend
echo "Starting React Frontend on port 5173..."
CURRENT_DIR=$(pwd)
echo "Current directory is: $CURRENT_DIR"
npm run dev &
FRONTEND_PID=$!

echo ""
echo "==================================="
echo "Services Running:"
echo "- Backend:  http://localhost:8080"
echo "- Frontend: http://localhost:5173"
echo "==================================="
echo ""
echo "Press Ctrl+C to stop all services..."

# Wait for processes
wait