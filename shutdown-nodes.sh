#!/bin/bash

echo "Shutting down Rubix nodes..."
echo

BASE_PORT=20100
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

        # Handle both old format (node0) and new format (node_25000_0)
        if [[ "$node_name" =~ ^node_[0-9]+_([0-9]+)$ ]]; then
            # New format: node_{port}_{index}
            node_num="${BASH_REMATCH[1]}"
        else
            # Old format: node{index}
            node_num=${node_name#node}
        fi

        # Calculate port
        port=$((BASE_PORT + node_num))

        echo "Shutting down ${node_name} on port ${port}..."
        "$RUBIX_EXE" shutdown -port $port
    fi
done

echo
echo "All nodes shut down."