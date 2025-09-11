package rubix

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rubix-simulator/backend/config"
)

// NodeInfo represents information about a Rubix node
type NodeInfo struct {
	ID         string `json:"id"`
	ServerPort int    `json:"server_port"`
	GrpcPort   int    `json:"grpc_port"`
	DID        string `json:"did"`
	PeerID     string `json:"peer_id"`
	IsQuorum   bool   `json:"is_quorum"`
	Status     string `json:"status"`
	Process    *exec.Cmd `json:"-"`
}

// Manager manages multiple Rubix nodes
type Manager struct {
	nodes        map[string]*NodeInfo
	mu           sync.RWMutex
	config       *config.RubixConfig
	dataDir      string
	metadataFile string
	rubixPath    string
}

// NewManager creates a new Rubix node manager
func NewManager() *Manager {
	return NewManagerWithConfig(config.DefaultRubixConfig())
}

// NewManagerWithConfig creates a new Rubix node manager with custom configuration
func NewManagerWithConfig(cfg *config.RubixConfig) *Manager {
	// Create a dedicated directory for all Rubix-related data
	os.MkdirAll(cfg.DataDir, 0755)

	return &Manager{
		nodes:        make(map[string]*NodeInfo),
		config:       cfg,
		dataDir:      cfg.DataDir,
		metadataFile: filepath.Join(cfg.DataDir, "node_metadata.json"),
		rubixPath:    filepath.Join(cfg.DataDir, "rubixgoplatform"),
	}
}

// StartNodes starts the specified number of nodes
func (m *Manager) StartNodes(transactionNodeCount int, fresh bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if transactionNodeCount < m.config.MinTransactionNodes {
		return fmt.Errorf("minimum %d transaction nodes required", m.config.MinTransactionNodes)
	}
	if transactionNodeCount > m.config.MaxTransactionNodes {
		return fmt.Errorf("maximum %d transaction nodes allowed", m.config.MaxTransactionNodes)
	}

	// Check if this is first run or restart
	if !fresh && m.nodeMetadataExists() {
		log.Println("Found existing node setup, checking if adjustment needed...")
		return m.adjustNodeCount(transactionNodeCount)
	}

	// Clean up if fresh start requested
	if fresh {
		log.Println("Fresh start requested, cleaning up existing data...")
		m.cleanup()
	}

	// Setup rubixgoplatform - this will handle existing installations gracefully
	if err := m.setupRubixPlatform(); err != nil {
		return fmt.Errorf("failed to setup rubix platform: %w", err)
	}

	totalNodes := m.config.QuorumNodeCount + transactionNodeCount
	log.Printf("Starting %d nodes (%d quorum + %d transaction)", totalNodes, m.config.QuorumNodeCount, transactionNodeCount)

	// Start all nodes
	var quorumList []QuorumData
	log.Printf("================== PHASE 1: Starting Nodes ==================")
	log.Printf("Total nodes to start: %d (Quorum: %d, Transaction: %d)", 
		totalNodes, m.config.QuorumNodeCount, totalNodes-m.config.QuorumNodeCount)
	
	for i := 0; i < totalNodes; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		serverPort := m.config.BaseServerPort + i
		grpcPort := m.config.BaseGrpcPort + i
		isQuorum := i < m.config.QuorumNodeCount
		
		nodeType := "transaction"
		if isQuorum {
			nodeType = "quorum"
		}

		log.Printf("[%d/%d] Starting %s (%s node) on port %d", i+1, totalNodes, nodeID, nodeType, serverPort)

		// Start the node process
		if err := m.startNodeProcess(nodeID, i); err != nil {
			return fmt.Errorf("failed to start %s: %w", nodeID, err)
		}

		// Wait for node to be ready
		client := NewClient(serverPort)
		timeout := time.Duration(m.config.NodeStartupTimeout) * time.Second
		log.Printf("  Waiting for %s to be ready (timeout: %v)...", nodeID, timeout)
		if err := client.WaitForNode(timeout); err != nil {
			return fmt.Errorf("node %s failed to start: %w", nodeID, err)
		}
		log.Printf("  ✓ %s is ready", nodeID)

		// Initialize the node
		log.Printf("  Initializing %s core...", nodeID)
		if err := client.Start(); err != nil {
			log.Printf("  ⚠ Warning: failed to initialize %s: %v", nodeID, err)
		} else {
			log.Printf("  ✓ %s core initialized", nodeID)
		}

		// Create DID
		log.Printf("  Creating DID for %s with password...", nodeID)
		did, peerID, err := client.CreateDID(m.config.DefaultPrivKeyPassword)
		if err != nil {
			return fmt.Errorf("failed to create DID for %s: %w", nodeID, err)
		}
		
		// Log raw values for debugging
		log.Printf("  DEBUG: Raw DID value: '%s' (length: %d)", did, len(did))
		log.Printf("  DEBUG: Raw PeerID value: '%s' (length: %d)", peerID, len(peerID))
		
		// Safe string slicing to avoid panic
		didDisplay := did
		if len(did) > 16 {
			didDisplay = did[:16] + "..."
		}
		peerIDDisplay := peerID
		if len(peerID) > 8 {
			peerIDDisplay = peerID[:8] + "..."
		}
		
		if peerID == "" {
			log.Printf("  ⚠ DID created for %s: %s (WARNING: PeerID is empty!)", nodeID, didDisplay)
		} else {
			log.Printf("  ✓ DID created for %s: %s (PeerID: %s)", nodeID, didDisplay, peerIDDisplay)
		}

		// Store node info (DID registration will happen later after all DIDs are created)
		nodeInfo := &NodeInfo{
			ID:         nodeID,
			ServerPort: serverPort,
			GrpcPort:   grpcPort,
			DID:        did,
			PeerID:     peerID,
			IsQuorum:   isQuorum,
			Status:     "running",
		}

		m.nodes[nodeID] = nodeInfo
		
		if isQuorum {
			// Add to quorum list
			log.Printf("  DEBUG: Adding %s to quorum list with DID: '%s' (length: %d)", nodeID, nodeInfo.DID, len(nodeInfo.DID))
			quorumList = append(quorumList, QuorumData{
				Type:    2,
				Address: nodeInfo.DID,  // Fixed: use nodeInfo.DID instead of did
			})
			log.Printf("  Added %s to quorum list (total quorum members: %d)", nodeID, len(quorumList))
		}
	}

	// Now that all DIDs are created, register them with the network
	// This allows the pub/sub mechanism to properly distribute node information
	log.Printf("\n================== PHASE 2: DID Registration ==================")
	log.Printf("Registering all %d DIDs with the network (pub/sub distribution)...", len(m.nodes))
	registrationSuccess := 0
	for nodeID, nodeInfo := range m.nodes {
		nodeType := "transaction"
		if nodeInfo.IsQuorum {
			nodeType = "quorum"
		}
		log.Printf("  DEBUG: About to register DID for %s: '%s' (length: %d)", nodeID, nodeInfo.DID, len(nodeInfo.DID))
		didDisplay := nodeInfo.DID
		if len(nodeInfo.DID) > 16 {
			didDisplay = nodeInfo.DID[:16] + "..."
		}
		log.Printf("[%s] Registering %s node DID: %s", nodeID, nodeType, didDisplay)
		client := NewClient(nodeInfo.ServerPort)
		if err := client.RegisterDID(nodeInfo.DID, m.config.DefaultPrivKeyPassword); err != nil {
			log.Printf("  ✗ ERROR: Failed to register DID for %s: %v", nodeID, err)
		} else {
			log.Printf("  ✓ Successfully registered DID for %s", nodeID)
			registrationSuccess++
		}
	}
	log.Printf("DID registration phase complete: %d/%d successful", registrationSuccess, len(m.nodes))
	if registrationSuccess < len(m.nodes) {
		log.Printf("⚠ WARNING: Not all DIDs registered successfully!")
	}

	// Add quorum list to all nodes
	log.Printf("\n================== PHASE 3: Quorum Configuration ==================")
	log.Printf("Building quorum list with %d members:", len(quorumList))
	for i, q := range quorumList {
		log.Printf("  DEBUG: Quorum[%d] Address: '%s' (length: %d, Type: %d)", i, q.Address, len(q.Address), q.Type)
		addrDisplay := q.Address
		if len(q.Address) > 16 {
			addrDisplay = q.Address[:16] + "..."
		}
		log.Printf("  [%d] Quorum DID: %s (Type: %d)", i+1, addrDisplay, q.Type)
	}
	
	quorumAddSuccess := 0
	for nodeID, nodeInfo := range m.nodes {
		nodeType := "transaction"
		if nodeInfo.IsQuorum {
			nodeType = "quorum"
		}
		client := NewClient(nodeInfo.ServerPort)
		log.Printf("[%s] Adding quorum list to %s node...", nodeID, nodeType)
		if err := client.AddQuorum(quorumList); err != nil {
			log.Printf("  ✗ ERROR: Failed to add quorum to %s: %v", nodeID, err)
		} else {
			log.Printf("  ✓ Successfully added quorum list to %s", nodeID)
			quorumAddSuccess++
			
			// Verify quorum was added correctly
			addedQuorum, err := client.GetAllQuorum()
			if err != nil {
				log.Printf("  ⚠ WARNING: Could not verify quorum for %s: %v", nodeID, err)
			} else {
				log.Printf("  ✓ Verified %s has %d quorum members", nodeID, len(addedQuorum))
			}
		}
	}
	log.Printf("Quorum configuration complete: %d/%d nodes configured", quorumAddSuccess, len(m.nodes))

	// Setup quorum for quorum nodes
	log.Printf("\n================== PHASE 4: Quorum Setup ==================")
	log.Printf("Setting up %d quorum nodes with quorum-specific configuration...", m.config.QuorumNodeCount)
	quorumSetupSuccess := 0
	for nodeID, nodeInfo := range m.nodes {
		if nodeInfo.IsQuorum {
			client := NewClient(nodeInfo.ServerPort)
			log.Printf("[%s] Setting up quorum configuration...", nodeID)
			if err := client.SetupQuorum(nodeInfo.DID, m.config.DefaultQuorumKeyPassword, m.config.DefaultPrivKeyPassword); err != nil {
				log.Printf("  ✗ WARNING: Failed to setup quorum for %s: %v", nodeID, err)
			} else {
				log.Printf("  ✓ Successfully setup quorum for %s", nodeID)
				quorumSetupSuccess++
			}
		}
	}
	log.Printf("Quorum setup complete: %d/%d quorum nodes configured", quorumSetupSuccess, m.config.QuorumNodeCount)

	// Generate test tokens for all nodes
	log.Printf("\n================== PHASE 5: Token Generation ==================")
	log.Printf("Generating 100 test RBT tokens for all %d nodes...", len(m.nodes))
	tokenGenSuccess := 0
	for nodeID, nodeInfo := range m.nodes {
		nodeType := "transaction"
		if nodeInfo.IsQuorum {
			nodeType = "quorum"
		}
		client := NewClient(nodeInfo.ServerPort)
		didDisplay := nodeInfo.DID
		if len(nodeInfo.DID) > 16 {
			didDisplay = nodeInfo.DID[:16] + "..."
		}
		log.Printf("[%s] Generating test tokens for %s node (DID: %s)...", nodeID, nodeType, didDisplay)
		maxRetries := 2
		tokenGenerated := false
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt > 1 {
				log.Printf("  Retry %d/%d for %s...", attempt, maxRetries, nodeID)
			}
			if err := client.GenerateTestTokens(nodeInfo.DID, 100, m.config.DefaultPrivKeyPassword); err != nil {
				log.Printf("  ✗ Failed to generate tokens (attempt %d): %v", attempt, err)
				if attempt == maxRetries {
					break
				}
				continue
			}
			
			// Verify tokens were generated
			log.Printf("  Checking balance for %s...", nodeID)
			balance, err := client.GetAccountBalance(nodeInfo.DID)
			if err != nil {
				log.Printf("  ✗ Failed to check balance: %v", err)
				break
			}
			
			log.Printf("  Balance for %s: %.3f RBT", nodeID, balance)
			
			if balance > 0 {
				log.Printf("  ✓ Successfully generated tokens for %s (Balance: %.3f RBT)", nodeID, balance)
				tokenGenerated = true
				tokenGenSuccess++
				break
			} else if attempt < maxRetries {
				log.Printf("  ⚠ Balance is 0, retrying token generation...")
				time.Sleep(5 * time.Second) // Wait a bit before retry
			} else {
				log.Printf("  ✗ ERROR: %s still has 0 balance after %d attempts!", nodeID, maxRetries)
			}
		}
		if !tokenGenerated {
			log.Printf("  ✗ FAILED: Token generation failed for %s", nodeID)
		}
	}
	log.Printf("Token generation complete: %d/%d nodes have tokens", tokenGenSuccess, len(m.nodes))

	// Save metadata
	log.Printf("\n================== PHASE 6: Finalization ==================")
	if err := m.saveMetadata(); err != nil {
		log.Printf("⚠ Warning: failed to save metadata: %v", err)
	} else {
		log.Printf("✓ Metadata saved successfully")
	}

	log.Printf("\n================== SETUP COMPLETE ==================")
	log.Printf("Summary:")
	log.Printf("  - Nodes started: %d/%d", len(m.nodes), totalNodes)
	log.Printf("  - DIDs registered: %d/%d", registrationSuccess, len(m.nodes))
	log.Printf("  - Quorum configured: %d/%d", quorumAddSuccess, len(m.nodes))
	log.Printf("  - Quorum setup: %d/%d", quorumSetupSuccess, m.config.QuorumNodeCount)
	log.Printf("  - Tokens generated: %d/%d", tokenGenSuccess, len(m.nodes))
	
	if registrationSuccess < len(m.nodes) || quorumAddSuccess < len(m.nodes) || tokenGenSuccess < len(m.nodes) {
		log.Printf("⚠ WARNING: Some operations failed. Check logs above for details.")
	} else {
		log.Printf("✓ All nodes successfully configured and ready!")
	}
	
	return nil
}

// startNodeProcess starts a rubixgoplatform process
func (m *Manager) startNodeProcess(nodeID string, index int) error {
	buildDir := m.getBuildDir()
	
	// Get absolute paths
	absDataDir, _ := filepath.Abs(m.dataDir)
	absRubixPath := filepath.Join(absDataDir, "rubixgoplatform")
	
	// Define binary names
	rubixBinName := "rubixgoplatform"
	ipfsBinName := "ipfs"
	if runtime.GOOS == "windows" {
		rubixBinName += ".exe"
		ipfsBinName += ".exe"
	}
	
	// Source paths in build directory
	srcRubixPath := filepath.Join(absRubixPath, buildDir, rubixBinName)
	srcIPFSPath := filepath.Join(absRubixPath, buildDir, ipfsBinName)
	srcSwarmKeyPath := filepath.Join(absRubixPath, buildDir, "testswarm.key")
	
	// Verify source files exist
	if _, err := os.Stat(srcRubixPath); err != nil {
		return fmt.Errorf("rubixgoplatform not found at %s - please ensure platform is built", srcRubixPath)
	}
	if _, err := os.Stat(srcIPFSPath); err != nil {
		return fmt.Errorf("IPFS binary not found at %s - please ensure IPFS is downloaded", srcIPFSPath)
	}
	if _, err := os.Stat(srcSwarmKeyPath); err != nil {
		return fmt.Errorf("testswarm.key not found at %s - please ensure swarm key is downloaded", srcSwarmKeyPath)
	}

	// Create node directory
	nodeDir := filepath.Join(absDataDir, "nodes", nodeID)
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}
	
	// Copy all required files to node directory
	nodeRubixPath := filepath.Join(nodeDir, rubixBinName)
	nodeIPFSPath := filepath.Join(nodeDir, ipfsBinName)
	nodeSwarmKeyPath := filepath.Join(nodeDir, "testswarm.key")
	
	// Copy rubixgoplatform
	if _, err := os.Stat(nodeRubixPath); err != nil {
		log.Printf("Copying rubixgoplatform to %s", nodeDir)
		if err := copyFile(srcRubixPath, nodeRubixPath); err != nil {
			return fmt.Errorf("failed to copy rubixgoplatform: %w", err)
		}
		if runtime.GOOS != "windows" {
			os.Chmod(nodeRubixPath, 0755)
		}
	}
	
	// Copy IPFS binary
	if _, err := os.Stat(nodeIPFSPath); err != nil {
		log.Printf("Copying IPFS binary to %s", nodeDir)
		if err := copyFile(srcIPFSPath, nodeIPFSPath); err != nil {
			return fmt.Errorf("failed to copy IPFS: %w", err)
		}
		if runtime.GOOS != "windows" {
			os.Chmod(nodeIPFSPath, 0755)
		}
	}
	
	// Copy testswarm.key
	if _, err := os.Stat(nodeSwarmKeyPath); err != nil {
		log.Printf("Copying testswarm.key to %s", nodeDir)
		if err := copyFile(srcSwarmKeyPath, nodeSwarmKeyPath); err != nil {
			return fmt.Errorf("failed to copy swarm key: %w", err)
		}
	}

	// Calculate ports
	port := m.config.BaseServerPort + index
	grpcPort := m.config.BaseGrpcPort + index

	// Build args (removed -dir flag)
	args := []string{
		"run",
		"-p", nodeID,
		"-n", fmt.Sprintf("%d", index),
		"-s",
		"-port", fmt.Sprintf("%d", port),                 
		"-testNet",
		"-grpcPort", fmt.Sprintf("%d", grpcPort),
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// On Windows, create a batch file to run the node in a new window
		windowTitle := fmt.Sprintf("Rubix Node %s - Port %d", nodeID, port)
		
		// Create batch file content - run from node directory using local copy
		batchContent := fmt.Sprintf(`@echo off
title %s
echo Starting %s on port %d...
echo Node directory: %s
echo.
cd /d "%s"
if not exist "%s" (
    echo ERROR: rubixgoplatform.exe not found in node directory
    echo Please ensure all files are copied correctly.
    pause > nul
    exit /b 1
)
if not exist "ipfs.exe" (
    echo ERROR: ipfs.exe not found in node directory
    echo Please ensure IPFS is copied correctly.
    pause > nul
    exit /b 1
)
if not exist "testswarm.key" (
    echo ERROR: testswarm.key not found in node directory
    echo Please ensure swarm key is copied correctly.
    pause > nul
    exit /b 1
)
"%s" %s
echo.
echo Node stopped. Press any key to close this window...
pause > nul`,
			windowTitle,
			nodeID, 
			port,
			nodeDir,
			nodeDir,
			rubixBinName,
			rubixBinName,
			strings.Join(args, " "))
		
		// Write batch file
		batchPath := filepath.Join(m.dataDir, fmt.Sprintf("node_%s.bat", nodeID))
		if err := os.WriteFile(batchPath, []byte(batchContent), 0755); err != nil {
			return fmt.Errorf("failed to create batch file: %w", err)
		}
		
		// Start the batch file in a new window
		cmd = exec.Command("cmd", "/c", "start", "", batchPath)
	} else {
		// On Linux/Mac, run in a tmux session
		sessionName := fmt.Sprintf("rubix-node-%s", nodeID)
		nodeCommand := fmt.Sprintf("cd %s && %s %s", nodeDir, filepath.Join(nodeDir, rubixBinName), strings.Join(args, " "))
		cmd = exec.Command("tmux", "new-session", "-d", "-s", sessionName, nodeCommand)
	}

	// Environment vars
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("RUBIX_NODE_DIR=%s", nodeDir),
		fmt.Sprintf("RUBIX_NODE_ID=%s", nodeID),
	)

	// Improved logging
	log.Printf("Starting node %s from directory: %s",
		nodeID,
		nodeDir,
	)
	log.Printf("Command: %s %s",
		rubixBinName,
		strings.Join(args, " "),
	)

	// Start process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start node process: %w", err)
	}

	log.Printf("Node %s process started successfully", nodeID)

	// Store process handle
	if nodeInfo, exists := m.nodes[nodeID]; exists {
		nodeInfo.Process = cmd
	}

	// Give node some time to boot
	time.Sleep(30 * time.Second)

	return nil
}


// StopAllNodes stops all running nodes
func (m *Manager) StopAllNodes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("Stopping %d nodes...", len(m.nodes))

	for nodeID, nodeInfo := range m.nodes {
		// Try graceful shutdown first with a short timeout
		client := NewClient(nodeInfo.ServerPort)
		
		// Create a channel to handle the shutdown attempt
		done := make(chan bool, 1)
		go func() {
			if err := client.Shutdown(); err != nil {
				log.Printf("Warning: graceful shutdown failed for %s: %v", nodeID, err)
			}
			done <- true
		}()
		
		// Wait for graceful shutdown but only for 2 seconds
		select {
		case <-done:
			log.Printf("Node %s shut down gracefully", nodeID)
		case <-time.After(2 * time.Second):
			log.Printf("Graceful shutdown timed out for %s, force killing", nodeID)
		}

		// Force kill the process if it exists
		if runtime.GOOS == "windows" {
		    // On Windows, the process is the `start` command, which has already exited.
		    // The actual node is in a separate window. The user is expected to close the windows manually.
		    log.Printf("Skipping process kill for %s on Windows. Please close the node window manually.", nodeID)
		} else {
		    // On Linux/Mac, kill the tmux session
		    sessionName := fmt.Sprintf("rubix-node-%s", nodeID)
		    if err := exec.Command("tmux", "kill-session", "-t", sessionName).Run(); err != nil {
		        log.Printf("Warning: failed to kill tmux session for %s: %v", nodeID, err)
		    } else {
		        log.Printf("TMUX session killed for %s", nodeID)
		    }
		}
	}

	// Clear nodes
	m.nodes = make(map[string]*NodeInfo)

	log.Printf("All nodes stopped")
	return nil
}

// restartExistingNodes restarts nodes from saved metadata with retry logic
func (m *Manager) restartExistingNodes() error {
	metadata, err := m.loadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	log.Printf("Restarting %d existing nodes...", len(metadata))

	// Restart nodes with retry logic
	var failedNodes []string
	for nodeID, nodeInfo := range metadata {
		index := 0
		fmt.Sscanf(nodeID, "node%d", &index)

		// Try to restart with retries
		var lastErr error
		for retry := 0; retry < 3; retry++ {
			if retry > 0 {
				log.Printf("Retry %d/3 for node %s", retry+1, nodeID)
				time.Sleep(time.Duration(retry*5) * time.Second)
			}

			// Start the node process
			if err := m.startNodeProcess(nodeID, index); err != nil {
				lastErr = err
				continue
			}

			// Wait for node to be ready with increased timeout for restarts
			client := NewClient(nodeInfo.ServerPort)
			timeout := time.Duration(m.config.NodeStartupTimeout) * time.Second
			if err := client.WaitForNode(timeout); err != nil {
				lastErr = err
				continue
			}

			// Store node info
			m.nodes[nodeID] = nodeInfo
			nodeInfo.Status = "running"
			lastErr = nil
			break
		}

		if lastErr != nil {
			log.Printf("Failed to restart %s after 3 retries: %v", nodeID, lastErr)
			failedNodes = append(failedNodes, nodeID)
			nodeInfo.Status = "failed"
		}
	}

	// Re-setup quorum for successfully restarted quorum nodes
	for nodeID, nodeInfo := range m.nodes {
		if nodeInfo.IsQuorum && nodeInfo.Status == "running" {
			client := NewClient(nodeInfo.ServerPort)
			if err := client.SetupQuorum(nodeInfo.DID, m.config.DefaultQuorumKeyPassword, m.config.DefaultPrivKeyPassword); err != nil {
				log.Printf("Warning: failed to setup quorum for %s: %v", nodeID, err)
			}
		}
	}

	if len(failedNodes) > 0 {
		return fmt.Errorf("failed to restart nodes: %v", failedNodes)
	}

	log.Printf("Successfully restarted %d nodes", len(m.nodes))
	return nil
}

// adjustNodeCount handles dynamic adjustment of node count based on user request
func (m *Manager) adjustNodeCount(requestedTransactionNodes int) error {
	metadata, err := m.loadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	// Count existing transaction nodes (non-quorum)
	existingTransactionNodes := 0
	for _, nodeInfo := range metadata {
		if !nodeInfo.IsQuorum {
			existingTransactionNodes++
		}
	}

	// If counts match, check if nodes are already running
	if requestedTransactionNodes == existingTransactionNodes {
		log.Println("Node count matches, checking status of existing nodes...")
		allRunning := true
		for nodeID, nodeInfo := range metadata {
			client := NewClient(nodeInfo.ServerPort)
			if err := client.Ping(); err != nil {
				log.Printf("Node %s is not responding: %v", nodeID, err)
				allRunning = false
				break
			}
		}

		if allRunning {
			log.Println("All nodes are already running. Skipping restart.")
			// Important: Load the running nodes into the manager's state
			m.nodes = metadata
			return nil
		}

		log.Println("One or more nodes not running, proceeding with restart...")
		return m.restartExistingNodes()
	}

	requestedTotal := m.config.QuorumNodeCount + requestedTransactionNodes
	existingTotal := len(metadata)

	log.Printf("Node count adjustment: Existing: %d nodes (%d quorum + %d transaction), Requested: %d nodes (%d quorum + %d transaction)",
		existingTotal, m.config.QuorumNodeCount, existingTransactionNodes,
		requestedTotal, m.config.QuorumNodeCount, requestedTransactionNodes)

	// If more nodes requested, start existing and add new ones
	if requestedTransactionNodes > existingTransactionNodes {
		log.Printf("Adding %d additional transaction nodes...", requestedTransactionNodes-existingTransactionNodes)
		
		// First, restart all existing nodes
		if err := m.restartExistingNodes(); err != nil {
			return fmt.Errorf("failed to restart existing nodes: %w", err)
		}

		// Then add new transaction nodes
		return m.addTransactionNodes(requestedTransactionNodes - existingTransactionNodes)
	}

	// If fewer nodes requested, stop excess nodes
	log.Printf("Removing %d excess transaction nodes...", existingTransactionNodes-requestedTransactionNodes)
	
	// Identify which transaction nodes to stop (remove the highest numbered ones)
	nodesToStop := []string{}
	nodesToKeep := make(map[string]*NodeInfo)
	
	// Sort node IDs to ensure consistent ordering
	transactionNodeIDs := []string{}
	for nodeID, nodeInfo := range metadata {
		if !nodeInfo.IsQuorum {
			transactionNodeIDs = append(transactionNodeIDs, nodeID)
		} else {
			// Always keep quorum nodes
			nodesToKeep[nodeID] = nodeInfo
		}
	}
	
	// Sort transaction node IDs numerically
	sort.Slice(transactionNodeIDs, func(i, j int) bool {
		var indexI, indexJ int
		fmt.Sscanf(transactionNodeIDs[i], "node%d", &indexI)
		fmt.Sscanf(transactionNodeIDs[j], "node%d", &indexJ)
		return indexI < indexJ
	})
	
	// Keep the first N transaction nodes, stop the rest
	for i, nodeID := range transactionNodeIDs {
		if i < requestedTransactionNodes {
			nodesToKeep[nodeID] = metadata[nodeID]
		} else {
			nodesToStop = append(nodesToStop, nodeID)
		}
	}
	
	log.Printf("Stopping nodes: %v", nodesToStop)
	log.Printf("Keeping nodes: %d", len(nodesToKeep))
	
	// Stop excess nodes
	for _, nodeID := range nodesToStop {
		if nodeInfo, exists := m.nodes[nodeID]; exists && nodeInfo.Process != nil {
			log.Printf("Stopping node %s", nodeID)
			nodeInfo.Process.Process.Kill()
			delete(m.nodes, nodeID)
		}
	}
	
	// Restart remaining nodes
	for nodeID, nodeInfo := range nodesToKeep {
		index := 0
		fmt.Sscanf(nodeID, "node%d", &index)

		// Start the node process
		if err := m.startNodeProcess(nodeID, index); err != nil {
			log.Printf("Failed to restart %s: %v", nodeID, err)
			continue
		}

		// Wait for node to be ready
		client := NewClient(nodeInfo.ServerPort)
		timeout := time.Duration(m.config.NodeStartupTimeout) * time.Second
		if err := client.WaitForNode(timeout); err != nil {
			log.Printf("Node %s failed to start: %v", nodeID, err)
			continue
		}

		// Store node info
		m.nodes[nodeID] = nodeInfo
		nodeInfo.Status = "running"
	}

	// Re-setup quorum for quorum nodes
	for nodeID, nodeInfo := range m.nodes {
		if nodeInfo.IsQuorum && nodeInfo.Status == "running" {
			client := NewClient(nodeInfo.ServerPort)
			if err := client.SetupQuorum(nodeInfo.DID, m.config.DefaultQuorumKeyPassword, m.config.DefaultPrivKeyPassword); err != nil {
				log.Printf("Warning: failed to setup quorum for %s: %v", nodeID, err)
			}
		}
	}

	// Save updated metadata
	if err := m.saveMetadata(); err != nil {
		log.Printf("Warning: failed to save updated metadata: %v", err)
	}

	log.Printf("Successfully adjusted to %d nodes (%d quorum + %d transaction)", 
		len(m.nodes), m.config.QuorumNodeCount, requestedTransactionNodes)
	return nil
}

// addTransactionNodes adds additional transaction nodes to the existing setup
func (m *Manager) addTransactionNodes(additionalCount int) error {
	if additionalCount <= 0 {
		return nil
	}

	log.Printf("Adding %d additional transaction nodes to existing setup", additionalCount)

	// Find the highest node index to continue numbering from there
	highestIndex := -1
	for nodeID := range m.nodes {
		var index int
		fmt.Sscanf(nodeID, "node%d", &index)
		if index > highestIndex {
			highestIndex = index
		}
	}

	// Collect all existing DIDs for quorum list
	var quorumList []QuorumData
	for _, nodeInfo := range m.nodes {
		if nodeInfo.IsQuorum {
			quorumList = append(quorumList, QuorumData{
				Type:    2,
				Address: nodeInfo.DID,
			})
		}
	}

	// Start new transaction nodes
	newNodes := make([]*NodeInfo, 0)
	for i := 0; i < additionalCount; i++ {
		nodeIndex := highestIndex + 1 + i
		nodeID := fmt.Sprintf("node%d", nodeIndex)
		serverPort := m.config.BaseServerPort + nodeIndex
		grpcPort := m.config.BaseGrpcPort + nodeIndex

		log.Printf("Starting additional transaction node %s (ports: server=%d, grpc=%d)", 
			nodeID, serverPort, grpcPort)

		// Start the node process
		if err := m.startNodeProcess(nodeID, nodeIndex); err != nil {
			log.Printf("Failed to start %s: %v", nodeID, err)
			continue
		}

		// Wait for node to be ready
		client := NewClient(serverPort)
		timeout := time.Duration(m.config.NodeStartupTimeout) * time.Second
		if err := client.WaitForNode(timeout); err != nil {
			log.Printf("Node %s failed to become ready: %v", nodeID, err)
			continue
		}

		// Create NodeInfo
		nodeInfo := &NodeInfo{
			ID:         nodeID,
			ServerPort: serverPort,
			GrpcPort:   grpcPort,
			IsQuorum:   false,
			Status:     "running",
		}

		// Create DID for the new node
		log.Printf("Creating DID for %s...", nodeID)
		did, peerID, err := client.CreateDID(m.config.DefaultPrivKeyPassword)
		if err != nil {
			log.Printf("Failed to create DID for %s: %v", nodeID, err)
			// Continue anyway, node might work without DID
		} else {
			nodeInfo.DID = did
			// Handle peerID gracefully - it may be empty
			if peerID != "" {
				nodeInfo.PeerID = peerID
				log.Printf("✓ Created DID for %s with peerID", nodeID)
			} else {
				log.Printf("✓ Created DID for %s (no peerID returned)", nodeID)
			}
		}

		m.nodes[nodeID] = nodeInfo
		newNodes = append(newNodes, nodeInfo)
	}

	if len(newNodes) == 0 {
		return fmt.Errorf("failed to add any new transaction nodes")
	}

	// Phase 2: Register DIDs for new nodes
	log.Printf("Registering DIDs for %d new nodes...", len(newNodes))
	for _, nodeInfo := range newNodes {
		if nodeInfo.DID == "" {
			continue
		}
		client := NewClient(nodeInfo.ServerPort)
		if err := client.RegisterDID(nodeInfo.DID, m.config.DefaultPrivKeyPassword); err != nil {
			log.Printf("⚠ Warning: Failed to register DID for %s: %v", nodeInfo.ID, err)
		} else {
			log.Printf("✓ Registered DID for %s", nodeInfo.ID)
		}
	}

	// Phase 3: Add quorum list to new nodes
	log.Printf("Adding quorum list to new nodes...")
	for _, nodeInfo := range newNodes {
		client := NewClient(nodeInfo.ServerPort)
		if err := client.AddQuorum(quorumList); err != nil {
			log.Printf("⚠ Warning: Failed to add quorum list to %s: %v", nodeInfo.ID, err)
		} else {
			log.Printf("✓ Added quorum list to %s", nodeInfo.ID)
		}
	}

	// Phase 4: Generate test tokens for new nodes
	log.Printf("Generating test tokens for new nodes...")
	for _, nodeInfo := range newNodes {
		if nodeInfo.DID == "" {
			continue
		}
		client := NewClient(nodeInfo.ServerPort)
		
		// Try to generate tokens with retries
		tokenGenerated := false
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt > 1 {
				log.Printf("  Retry %d/%d for %s...", attempt, maxRetries, nodeInfo.ID)
			}
			if err := client.GenerateTestTokens(nodeInfo.DID, 100, m.config.DefaultPrivKeyPassword); err != nil {
				log.Printf("  ✗ Failed to generate tokens for %s (attempt %d): %v", nodeInfo.ID, attempt, err)
				if attempt == maxRetries {
					break
				}
				time.Sleep(time.Second * time.Duration(attempt))
				continue
			}
			
			// Verify tokens were generated
			balance, err := client.GetAccountBalance(nodeInfo.DID)
			if err != nil {
				log.Printf("  ✗ Failed to check balance for %s: %v", nodeInfo.ID, err)
				break
			}
			
			if balance > 0 {
				log.Printf("  ✓ Generated %.2f tokens for %s", balance, nodeInfo.ID)
				tokenGenerated = true
				break
			}
		}
		
		if !tokenGenerated {
			log.Printf("  ⚠ Warning: Could not generate tokens for %s", nodeInfo.ID)
		}
	}

	// Save updated metadata
	if err := m.saveMetadata(); err != nil {
		log.Printf("Warning: failed to save metadata: %v", err)
	}

	log.Printf("Successfully added %d transaction nodes", len(newNodes))
	return nil
}

// RestartNodes restarts specific nodes
func (m *Manager) RestartNodes(nodeIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, nodeID := range nodeIDs {
		nodeInfo, exists := m.nodes[nodeID]
		if !exists {
			log.Printf("Node %s not found, skipping", nodeID)
			continue
		}

		// Stop the node first
		if nodeInfo.Process != nil {
			nodeInfo.Process.Process.Kill()
		}

		// Extract index from nodeID
		index := 0
		fmt.Sscanf(nodeID, "node%d", &index)

		// Restart the node
		if err := m.startNodeProcess(nodeID, index); err != nil {
			return fmt.Errorf("failed to restart %s: %w", nodeID, err)
		}

		// Wait for node to be ready
		client := NewClient(nodeInfo.ServerPort)
		timeout := time.Duration(m.config.NodeStartupTimeout) * time.Second
		if err := client.WaitForNode(timeout); err != nil {
			return fmt.Errorf("node %s failed to restart: %w", nodeID, err)
		}

		nodeInfo.Status = "running"
		log.Printf("Successfully restarted node %s", nodeID)
	}

	return nil
}

// RecoverNode attempts to recover a failed node
func (m *Manager) RecoverNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nodeInfo, exists := m.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	log.Printf("Attempting to recover node %s", nodeID)

	// Check if node is actually responding
	client := NewClient(nodeInfo.ServerPort)
	if err := client.Ping(); err == nil {
		log.Printf("Node %s is already running", nodeID)
		nodeInfo.Status = "running"
		return nil
	}

	// Kill any existing process
	if nodeInfo.Process != nil {
		nodeInfo.Process.Process.Kill()
		time.Sleep(2 * time.Second)
	}

	// Clean node directory
	nodeDir := filepath.Join(m.dataDir, "nodes", nodeID)
	tempDir := nodeDir + "_backup"
	
	// Backup existing data
	if err := os.Rename(nodeDir, tempDir); err != nil {
		log.Printf("Warning: failed to backup node directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Recreate node directory
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}

	// Extract index from nodeID
	index := 0
	fmt.Sscanf(nodeID, "node%d", &index)

	// Restart the node
	if err := m.startNodeProcess(nodeID, index); err != nil {
		// Restore backup if restart fails
		os.RemoveAll(nodeDir)
		os.Rename(tempDir, nodeDir)
		return fmt.Errorf("failed to recover node: %w", err)
	}

	// Wait for node to be ready
	timeout := time.Duration(m.config.NodeStartupTimeout) * time.Second
	if err := client.WaitForNode(timeout); err != nil {
		return fmt.Errorf("node recovery failed: %w", err)
	}

	// Recreate DID if needed
	if nodeInfo.DID == "" {
		log.Printf("Recreating DID for recovered node %s", nodeID)
		did, peerID, err := client.CreateDID(m.config.DefaultPrivKeyPassword)
		if err != nil {
			log.Printf("Warning: failed to recreate DID: %v", err)
		} else {
			nodeInfo.DID = did
			nodeInfo.PeerID = peerID
		}
	}

	// Re-setup quorum if needed
	if nodeInfo.IsQuorum {
		if err := client.SetupQuorum(nodeInfo.DID, m.config.DefaultQuorumKeyPassword, m.config.DefaultPrivKeyPassword); err != nil {
			log.Printf("Warning: failed to setup quorum for recovered node: %v", err)
		}
	}

	nodeInfo.Status = "running"
	log.Printf("Successfully recovered node %s", nodeID)
	
	// Save updated metadata
	m.saveMetadata()
	
	return nil
}

// setupRubixPlatform downloads and builds rubixgoplatform
func (m *Manager) setupRubixPlatform() error {
	log.Println("Setting up rubixgoplatform...")

	needsBuild := false

	// Check if repository already exists
	if _, err := os.Stat(m.rubixPath); err == nil {
		log.Printf("Rubixgoplatform directory already exists at %s", m.rubixPath)

		// Try to pull latest changes instead of cloning
		cmd := exec.Command("git", "pull", "origin", m.config.RubixBranch)
		cmd.Dir = m.rubixPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Warning: failed to pull latest changes: %v\nOutput: %s", err, string(output))
			// Continue anyway - existing code might work
		} else {
			outputStr := string(output)
			log.Printf("Git pull output: %s", outputStr)

			// Check if there were actual updates
			if outputStr != "Already up to date.\n" && outputStr != "Already up-to-date.\n" {
				log.Println("Repository updated with new changes, will rebuild executable")
				needsBuild = true
			} else {
				log.Println("Repository already up to date")
			}
		}
	} else {
		// Clone the repository if it doesn't exist
		log.Printf("Cloning from %s...", m.config.RubixRepoURL)
		cmd := exec.Command("git", "clone", m.config.RubixRepoURL, m.rubixPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to clone rubixgoplatform: %w\nOutput: %s", err, string(output))
		}

		// Checkout the specified branch
		cmd = exec.Command("git", "checkout", m.config.RubixBranch)
		cmd.Dir = m.rubixPath
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to checkout branch %s: %v", m.config.RubixBranch, err)
		}

		// Fresh clone always needs build
		needsBuild = true
	}

	// Build the platform
	buildDir := m.getBuildDir()
	buildPath := filepath.Join(m.rubixPath, buildDir)

	// Create build directory
	if err := os.MkdirAll(buildPath, 0755); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	// Check if executable already exists
	execName := "rubixgoplatform"
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}
	execPath := filepath.Join(buildPath, execName)

	execExists := false
	if _, err := os.Stat(execPath); err == nil {
		execExists = true
	}

	// Build if needed: either doesn't exist or source was updated
	if !execExists || needsBuild {
		if needsBuild {
			log.Println("Rebuilding rubixgoplatform due to source updates...")
		} else {
			log.Println("Building rubixgoplatform for the first time...")
		}

		// Determine the make target based on OS
		var makeTarget string
		switch runtime.GOOS {
		case "windows":
			makeTarget = "compile-windows"
		case "linux":
			makeTarget = "compile-linux"
		case "darwin":
			makeTarget = "compile-mac"
		default:
			return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
		}

		log.Printf("Building rubixgoplatform using make %s...", makeTarget)
		
		// Use make command to build
		cmd := exec.Command("make", makeTarget)
		cmd.Dir = m.rubixPath
		cmd.Env = append(os.Environ(), "GO111MODULE=on")

		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to build rubixgoplatform using make %s: %w\nOutput: %s", makeTarget, err, string(output))
		}
		log.Println("Successfully built rubixgoplatform")
	} else {
		log.Printf("Using existing rubixgoplatform executable at %s", execPath)
	}

	// Download IPFS
	if err := m.downloadIPFS(); err != nil {
		return fmt.Errorf("failed to download IPFS: %w", err)
	}

	// Download test swarm key
	if err := m.downloadSwarmKey(); err != nil {
		log.Printf("Warning: failed to download swarm key: %v", err)
	}

	log.Println("Rubixgoplatform setup completed successfully")
	return nil
}

// downloadSwarmKey downloads the test swarm key with retry logic
func (m *Manager) downloadSwarmKey() error {
	log.Println("Downloading test swarm key...")

	buildDir := m.getBuildDir()
	destPath := filepath.Join(m.rubixPath, buildDir, "testswarm.key")
	
	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		log.Printf("Swarm key already exists at %s", destPath)
		return nil
	}

	// Try to copy from the repository first
	srcPath := filepath.Join(m.rubixPath, "testswarm.key")
	if _, err := os.Stat(srcPath); err == nil {
		log.Println("Copying swarm key from repository...")
		return copyFile(srcPath, destPath)
	}

	// Download from URL with retry
	log.Printf("Downloading swarm key from: %s", m.config.TestSwarmKeyURL)
	tempFile := filepath.Join(m.dataDir, "testswarm.key.tmp")
	
	if err := m.downloadWithRetry(m.config.TestSwarmKeyURL, tempFile, 3); err != nil {
		return fmt.Errorf("failed to download swarm key: %w", err)
	}
	
	// Move to final location
	if err := m.moveFile(tempFile, destPath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to move swarm key: %w", err)
	}
	
	log.Println("Successfully downloaded test swarm key")
	return nil
}

// DownloadIPFSManually forces a re-download of IPFS binary
func (m *Manager) DownloadIPFSManually() error {
	buildDir := m.getBuildDir()
	ipfsBinName := "ipfs"
	if runtime.GOOS == "windows" {
		ipfsBinName += ".exe"
	}
	
	// Remove existing IPFS binary if present
	ipfsPath := filepath.Join(m.rubixPath, buildDir, ipfsBinName)
	os.Remove(ipfsPath)
	
	// Download IPFS
	return m.downloadIPFS()
}

// downloadIPFS downloads the IPFS binary with retry logic
func (m *Manager) downloadIPFS() error {
	log.Printf("Downloading IPFS binary (version: %s)...", m.config.IPFSVersion)
	
	buildDir := m.getBuildDir()
	ipfsBinName := "ipfs"
	if runtime.GOOS == "windows" {
		ipfsBinName += ".exe"
	}
	
	// Ensure build directory exists
	buildPath := filepath.Join(m.rubixPath, buildDir)
	if err := os.MkdirAll(buildPath, 0755); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}
	
	// Check if IPFS already exists
	ipfsPath := filepath.Join(m.rubixPath, buildDir, ipfsBinName)
	if _, err := os.Stat(ipfsPath); err == nil {
		log.Printf("IPFS binary already exists at %s", ipfsPath)
		return nil
	}
	
	// Construct download URL based on OS
	var downloadURL string
	var archiveExt string
	osArch := "amd64"
	
	switch runtime.GOOS {
	case "linux":
		downloadURL = fmt.Sprintf("https://github.com/ipfs/kubo/releases/download/%s/kubo_%s_linux-%s.tar.gz",
			m.config.IPFSVersion, m.config.IPFSVersion, osArch)
		archiveExt = ".tar.gz"
	case "windows":
		downloadURL = fmt.Sprintf("https://github.com/ipfs/kubo/releases/download/%s/kubo_%s_windows-%s.zip",
			m.config.IPFSVersion, m.config.IPFSVersion, osArch)
		archiveExt = ".zip"
	case "darwin":
		downloadURL = fmt.Sprintf("https://github.com/ipfs/kubo/releases/download/%s/kubo_%s_darwin-%s.tar.gz",
			m.config.IPFSVersion, m.config.IPFSVersion, osArch)
		archiveExt = ".tar.gz"
	default:
		return fmt.Errorf("unsupported operating system for IPFS: %s", runtime.GOOS)
	}
	
	// Download with retry
	tempFile := filepath.Join(m.dataDir, fmt.Sprintf("kubo_%s%s", m.config.IPFSVersion, archiveExt))
	if err := m.downloadWithRetry(downloadURL, tempFile, 3); err != nil {
		return fmt.Errorf("failed to download IPFS: %w", err)
	}
	defer os.Remove(tempFile)
	
	// Extract archive
	log.Println("Extracting IPFS binary...")
	tempExtractDir := filepath.Join(m.dataDir, "kubo_temp")
	if err := os.MkdirAll(tempExtractDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp extraction directory: %w", err)
	}
	defer os.RemoveAll(tempExtractDir)
	
	if archiveExt == ".zip" {
		if err := m.extractZip(tempFile, tempExtractDir); err != nil {
			return fmt.Errorf("failed to extract IPFS zip: %w", err)
		}
	} else {
		if err := m.extractTarGz(tempFile, tempExtractDir); err != nil {
			return fmt.Errorf("failed to extract IPFS tar.gz: %w", err)
		}
	}
	
	// The IPFS binary is inside the kubo folder after extraction
	srcIPFS := filepath.Join(tempExtractDir, "kubo", ipfsBinName)
	
	// Check if the file exists at the expected location
	if _, err := os.Stat(srcIPFS); err != nil {
		// Try alternative location (sometimes it's directly in kubo/)
		altSrcIPFS := filepath.Join(tempExtractDir, ipfsBinName)
		if _, err2 := os.Stat(altSrcIPFS); err2 == nil {
			srcIPFS = altSrcIPFS
			log.Printf("Found IPFS binary at alternative location: %s", altSrcIPFS)
		} else {
			// List contents to debug
			log.Printf("IPFS binary not found at expected locations. Listing extraction directory contents:")
			m.listDirectory(tempExtractDir, 2)
			return fmt.Errorf("IPFS binary not found at %s or %s", srcIPFS, altSrcIPFS)
		}
	}
	
	log.Printf("Moving IPFS binary from %s to %s", srcIPFS, ipfsPath)
	if err := m.moveFile(srcIPFS, ipfsPath); err != nil {
		return fmt.Errorf("failed to move IPFS binary: %w", err)
	}
	
	// Make executable on Unix systems
	if runtime.GOOS != "windows" {
		if err := os.Chmod(ipfsPath, 0755); err != nil {
			return fmt.Errorf("failed to make IPFS executable: %w", err)
		}
	}
	
	log.Printf("Successfully downloaded and installed IPFS %s", m.config.IPFSVersion)
	return nil
}

// getBuildDir returns the build directory based on OS
func (m *Manager) getBuildDir() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	case "darwin":
		return "mac"
	default:
		return "build"
	}
}

// rubixPlatformExists checks if rubixgoplatform is already set up
func (m *Manager) rubixPlatformExists() bool {
	buildDir := m.getBuildDir()
	execPath := filepath.Join(m.rubixPath, buildDir, "rubixgoplatform")
	if runtime.GOOS == "windows" {
		execPath += ".exe"
	}
	_, err := os.Stat(execPath)
	return err == nil
}

// nodeMetadataExists checks if node metadata file exists
func (m *Manager) nodeMetadataExists() bool {
	_, err := os.Stat(m.metadataFile)
	return err == nil
}

// saveMetadata saves node metadata to file
func (m *Manager) saveMetadata() error {
	data, err := json.MarshalIndent(m.nodes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.metadataFile, data, 0644)
}

// loadMetadata loads node metadata from file
func (m *Manager) loadMetadata() (map[string]*NodeInfo, error) {
	data, err := os.ReadFile(m.metadataFile)
	if err != nil {
		return nil, err
	}

	var nodes map[string]*NodeInfo
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

// cleanup removes all node data
func (m *Manager) cleanup() {
	// Remove metadata file
	os.Remove(m.metadataFile)

	// Remove all node directories
	nodesDir := filepath.Join(m.dataDir, "nodes")
	os.RemoveAll(nodesDir)

	// Optionally remove the entire rubixgoplatform if doing a full reset
	// os.RemoveAll(m.rubixPath)
}

// CleanupAll removes all Rubix data including binaries
func (m *Manager) CleanupAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop all nodes first
	m.StopAllNodes()

	// Remove the entire data directory
	if err := os.RemoveAll(m.dataDir); err != nil {
		return fmt.Errorf("failed to cleanup data directory: %w", err)
	}

	// Recreate the data directory for future use
	os.MkdirAll(m.dataDir, 0755)

	log.Println("All Rubix data cleaned up")
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// GetNodes returns all nodes
func (m *Manager) GetNodes() map[string]*NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	nodesCopy := make(map[string]*NodeInfo)
	for k, v := range m.nodes {
		nodesCopy[k] = v
	}
	return nodesCopy
}

// GetNode returns a specific node
func (m *Manager) GetNode(nodeID string) (*NodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, exists := m.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}
	return node, nil
}

// CheckNodeStatus checks the status of a specific node
func (m *Manager) CheckNodeStatus(nodeID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeInfo, exists := m.nodes[nodeID]
	if !exists {
		return "not_found", fmt.Errorf("node %s not found", nodeID)
	}

	// Try to ping the node
	client := NewClient(nodeInfo.ServerPort)
	if err := client.Ping(); err != nil {
		nodeInfo.Status = "failed"
		return "failed", err
	}

	nodeInfo.Status = "running"
	return "running", nil
}

// CheckAllNodesStatus checks the status of all nodes
func (m *Manager) CheckAllNodesStatus() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]string)
	
	for nodeID, nodeInfo := range m.nodes {
		client := NewClient(nodeInfo.ServerPort)
		if err := client.Ping(); err != nil {
			nodeInfo.Status = "failed"
			statuses[nodeID] = "failed"
		} else {
			nodeInfo.Status = "running"
			statuses[nodeID] = "running"
		}
	}

	return statuses
}

// GetNodeMetrics retrieves metrics from a node
func (m *Manager) GetNodeMetrics(nodeID string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeInfo, exists := m.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	client := NewClient(nodeInfo.ServerPort)
	
	metrics := make(map[string]interface{})
	metrics["node_id"] = nodeID
	metrics["server_port"] = nodeInfo.ServerPort
	metrics["grpc_port"] = nodeInfo.GrpcPort
	metrics["is_quorum"] = nodeInfo.IsQuorum
	metrics["did"] = nodeInfo.DID
	metrics["peer_id"] = nodeInfo.PeerID
	metrics["status"] = nodeInfo.Status

	// Try to get additional metrics from the node
	if nodeInfo.Status == "running" {
		// Get account info
		if accountInfo, err := client.GetAccountInfo(nodeInfo.DID); err == nil {
			metrics["account_info"] = accountInfo
		}
		
		// Get peer count
		if peerCount, err := client.GetPeerCount(); err == nil {
			metrics["peer_count"] = peerCount
		}
	}

	return metrics, nil
}

// MonitorNodes continuously monitors node health
func (m *Manager) MonitorNodes(interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			statuses := m.CheckAllNodesStatus()
			
			// Log status summary
			running := 0
			failed := 0
			for _, status := range statuses {
				if status == "running" {
					running++
				} else {
					failed++
				}
			}
			
			if failed > 0 {
				log.Printf("Node Status: %d running, %d failed", running, failed)
				
				// Attempt to recover failed nodes
				for nodeID, status := range statuses {
					if status == "failed" {
						log.Printf("Attempting to auto-recover failed node %s", nodeID)
						if err := m.RecoverNode(nodeID); err != nil {
							log.Printf("Failed to auto-recover node %s: %v", nodeID, err)
						}
					}
				}
			}
			
		case <-stopCh:
			log.Println("Stopping node monitoring")
			return
		}
	}
}

// downloadWithRetry downloads a file with retry logic
func (m *Manager) downloadWithRetry(url string, destPath string, maxRetries int) error {
	var lastErr error
	
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			log.Printf("Retry %d/%d downloading from %s", i+1, maxRetries, url)
			time.Sleep(time.Duration(i*2) * time.Second) // Exponential backoff
		}
		
		if err := m.downloadFile(url, destPath); err != nil {
			lastErr = err
			log.Printf("Download attempt %d failed: %v", i+1, err)
			continue
		}
		
		return nil
	}
	
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// downloadFile downloads a file from URL to destination
func (m *Manager) downloadFile(url string, destPath string) error {
	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()
	
	// Get the data
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	
	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}

// extractZip extracts a zip file to destination
func (m *Manager) extractZip(src string, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()
	
	for _, file := range reader.File {
		path := filepath.Join(dest, file.Name)
		
		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}
		
		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		
		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()
		
		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()
		
		_, err = io.Copy(targetFile, fileReader)
		if err != nil {
			return err
		}
	}
	
	return nil
}

// extractTarGz extracts a tar.gz file to destination
func (m *Manager) extractTarGz(src string, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()
	
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()
	
	tr := tar.NewReader(gzr)
	
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		target := filepath.Join(dest, header.Name)
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Create directory if needed
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return err
			}
			file.Close()
		}
	}
	
	return nil
}

// moveFile moves a file from src to dst
func (m *Manager) moveFile(src string, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Try rename first (fastest if on same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	
	// Fall back to copy and delete
	input, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	
	if err := os.WriteFile(dst, input, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}
	
	// Remove original
	os.Remove(src)
	return nil
}

// listDirectory recursively lists directory contents for debugging
func (m *Manager) listDirectory(dir string, maxDepth int) {
	m.listDirectoryRecursive(dir, 0, maxDepth, "")
}

func (m *Manager) listDirectoryRecursive(dir string, currentDepth, maxDepth int, indent string) {
	if currentDepth > maxDepth {
		return
	}
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("%sError reading directory %s: %v", indent, dir, err)
		return
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			log.Printf("%s[DIR] %s", indent, entry.Name())
			if currentDepth < maxDepth {
				subDir := filepath.Join(dir, entry.Name())
				m.listDirectoryRecursive(subDir, currentDepth+1, maxDepth, indent+"  ")
			}
		} else {
			info, _ := entry.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			log.Printf("%s[FILE] %s (size: %d bytes)", indent, entry.Name(), size)
		}
	}
}