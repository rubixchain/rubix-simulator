package main

import (
	"fmt"
	"log"
	"time"

	"github.com/rubix-simulator/backend/internal/rubix"
)

func main() {
	log.Println("Testing Rubix Go implementation...")

	// Test 1: Test single node operations
	testSingleNode()

	// Test 2: Test manager with multiple nodes
	// testManager()
}

func testSingleNode() {
	log.Println("\n=== Testing Single Node Operations ===")

	// Create a client for node on port 20000
	client := rubix.NewClient(20000)

	// Check node status
	log.Println("Checking node status...")
	status, err := client.NodeStatus()
	if err != nil {
		log.Printf("Node is not running (expected): %v", err)
	} else {
		log.Printf("Node status: %v", status)
	}

	// Test API endpoints when node is running
	// This assumes you have a node running on port 20000
	if status {
		// Get peer ID
		peerID, err := client.GetPeerID()
		if err != nil {
			log.Printf("Failed to get peer ID: %v", err)
		} else {
			log.Printf("Peer ID: %s", peerID)
		}
	}

	log.Println("Single node test completed")
}

func testManager() {
	log.Println("\n=== Testing Node Manager ===")

	manager := rubix.NewManager()

	// Start nodes (7 quorum + 2 transaction)
	log.Println("Starting nodes...")
	err := manager.StartNodes(2, true)
	if err != nil {
		log.Fatalf("Failed to start nodes: %v", err)
	}

	// Get all nodes
	nodes := manager.GetNodes()
	log.Printf("Started %d nodes", len(nodes))

	// Display node information
	for nodeID, node := range nodes {
		fmt.Printf("Node %s:\n", nodeID)
		fmt.Printf("  Port: %d\n", node.ServerPort)
		fmt.Printf("  GRPC Port: %d\n", node.GrpcPort)
		fmt.Printf("  DID: %s\n", node.DID)
		fmt.Printf("  Is Quorum: %v\n", node.IsQuorum)
		fmt.Printf("  Status: %s\n", node.Status)
	}

	// Wait a bit
	log.Println("Nodes running... waiting 30 seconds")
	time.Sleep(30 * time.Second)

	// Stop all nodes
	log.Println("Stopping all nodes...")
	err = manager.StopAllNodes()
	if err != nil {
		log.Printf("Warning: failed to stop nodes: %v", err)
	}

	log.Println("Manager test completed")
}