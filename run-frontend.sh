#!/bin/bash

echo "Starting Rubix Frontend..."
echo ""

cd frontend

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
    echo ""
fi

echo "Starting development server on http://localhost:5173"
echo ""
npm run dev