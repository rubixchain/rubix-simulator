#!/bin/bash

echo "Shutting down services..."

tmux kill-session -t rubix-backend 2>/dev/null
tmux kill-session -t rubix-frontend 2>/dev/null

echo "Services shut down."
