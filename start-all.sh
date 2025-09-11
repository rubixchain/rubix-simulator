#!/bin/bash

# (Ensures script runs from the correct directory)
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd "$SCRIPT_DIR"

# Kill existing sessions if they are running
tmux kill-session -t rubix-backend 2>/dev/null
tmux kill-session -t rubix-frontend 2>/dev/null

echo "==================================="
echo "Starting Rubix Network Simulator"
echo "==================================="
echo ""

# Start Backend
echo "Starting Go Backend in tmux session 'rubix-backend'..."
tmux new-session -d -s rubix-backend 'cd backend && go run cmd/server/main.go'

# Wait for backend to start
sleep 3

# Start Frontend
echo "Starting React Frontend in tmux session 'rubix-frontend'..."
tmux new-session -d -s rubix-frontend 'npx vite'

echo ""
echo "==================================="
echo "Services Running in tmux sessions:"
echo "- Backend:  tmux attach -t rubix-backend"
echo "- Frontend: tmux attach -t rubix-frontend"
echo ""
echo "Web application will be available at http://localhost:5173"
echo "==================================="
echo ""
echo "To stop all services, run: ./shutdown-services.sh"
