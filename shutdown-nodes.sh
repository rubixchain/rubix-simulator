#!/bin/bash

echo "Shutting down Rubix nodes..."
echo

BASE_PORT=20000
NODES_DIR="backend/rubix-data/nodes"

# Detect OS and set rubix executable path
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    RUBIX_EXE="backend/rubix-data/rubixgoplatform/linux/rubixgoplatform"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    RUBIX_EXE="backend/rubix-data/rubixgoplatform/mac/rubixgoplatform"
else
    echo "Unsupported OS: $OSTYPE"
    exit 1
fi

# Check if nodes directory exists
if [ ! -d "$NODES_DIR" ]; then
    echo "No nodes directory found."
    exit 0
fi

# Loop through node directories
for node_dir in $NODES_DIR/node*; do
    if [ -d "$node_dir" ]; then
        # Extract node number from directory name
        node_name=$(basename "$node_dir")
        node_num=${node_name#node}
        
        # Calculate port
        port=$((BASE_PORT + node_num))
        
        echo "Shutting down node${node_num} on port ${port}..."
        "$RUBIX_EXE" shutdown -port $port
    fi
done

echo
echo "All nodes shut down."