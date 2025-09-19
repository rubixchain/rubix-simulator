package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rubix-simulator/backend/internal/config"
	"github.com/rubix-simulator/backend/internal/models"
	"github.com/rubix-simulator/backend/internal/rubix"
)

type NodeManager struct {
	config       *config.Config
	nodes        map[string]*models.Node
	busyNodes    map[string]bool // New field
	mu           sync.RWMutex
	basePort     int
	usePython    bool
	rubixManager *rubix.Manager
	quorumNodes  int  // Fixed number of quorum nodes
}

func NewNodeManager(cfg *config.Config) *NodeManager {
	return &NodeManager{
		config:       cfg,
		nodes:        make(map[string]*models.Node),
		busyNodes:    make(map[string]bool), // New field
		basePort:     20000,
		usePython:    false, // Use Go implementation by default
		rubixManager: rubix.NewManager(),
		quorumNodes:  7,  // Fixed 7 quorum nodes as per requirement
	}
}

func (nm *NodeManager) StartNodes(count int) ([]*models.Node, error) {
	return nm.StartNodesWithOptions(count, false)
}

func (nm *NodeManager) StartNodesWithOptions(count int, fresh bool) ([]*models.Node, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Count represents additional nodes beyond the 7 quorum nodes
	transactionNodes := count
	if transactionNodes < 2 || transactionNodes > 20 {
		return nil, fmt.Errorf("transaction node count must be between 2 and 20")
	}

	// Only stop nodes if we're doing a fresh start
	if fresh {
		nm.StopAllNodesInternal()
	}

	if !nm.usePython {
		// Use the Go implementation
		log.Printf("Using Go implementation to start nodes")

		// Start nodes using the Go manager
		if err := nm.rubixManager.StartNodes(transactionNodes, fresh); err != nil {
			return nil, fmt.Errorf("failed to start nodes: %w", err)
		}

		// Convert rubix.NodeInfo to models.Node
		var nodes []*models.Node
		for _, nodeInfo := range nm.rubixManager.GetNodes() {
			node := &models.Node{
				ID:       nodeInfo.ID,
				Port:     nodeInfo.ServerPort,
				GrpcPort: nodeInfo.GrpcPort,
				DID:      nodeInfo.DID,
				IsQuorum: nodeInfo.IsQuorum,
				Status:   nodeInfo.Status,
				Started:  time.Now(),
			}
			nm.nodes[node.ID] = node
			nodes = append(nodes, node)
		}

		totalNodes := nm.quorumNodes + transactionNodes
		log.Printf("Successfully started %d nodes (%d quorum + %d transaction) via Go manager",
			totalNodes, nm.quorumNodes, transactionNodes)
		return nodes, nil
	}

	// Fallback to simulated nodes if Python is disabled and no Go implementation
	return nm.startSimulatedNodes(count)
}

func (nm *NodeManager) RestartNodes() ([]*models.Node, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.usePython {
		// Use the Go implementation to restart nodes
		log.Printf("Using Go implementation to restart nodes")

		// This will restart based on saved metadata
		if err := nm.rubixManager.StartNodes(2, false); err != nil {
			return nil, fmt.Errorf("failed to restart nodes: %w", err)
		}

		// Convert rubix.NodeInfo to models.Node
		var nodes []*models.Node
		for _, nodeInfo := range nm.rubixManager.GetNodes() {
			node := &models.Node{
				ID:       nodeInfo.ID,
				Port:     nodeInfo.ServerPort,
				GrpcPort: nodeInfo.GrpcPort,
				DID:      nodeInfo.DID,
				IsQuorum: nodeInfo.IsQuorum,
				Status:   nodeInfo.Status,
				Started:  time.Now(),
			}
			nm.nodes[node.ID] = node
			nodes = append(nodes, node)
		}

		log.Printf("Successfully restarted %d nodes", len(nodes))
		return nodes, nil
	}

	return nil, fmt.Errorf("restart not supported in simulation mode")
}

func (nm *NodeManager) ResetNodes() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.usePython {
		// Stop all nodes first
		if err := nm.rubixManager.StopAllNodes(); err != nil {
			log.Printf("Warning: failed to stop nodes: %v", err)
		}
	}

	// Clear internal state
	nm.nodes = make(map[string]*models.Node)

	return nil
}

func (nm *NodeManager) startSimulatedNodes(count int) ([]*models.Node, error) {
	var nodes []*models.Node
	for i := 0; i < count; i++ {
		node := &models.Node{
			ID:      fmt.Sprintf("sim-node-%d", i+1),
			Port:    nm.basePort + i,
			Status:  "running",
			Started: time.Now(),
		}
		nm.nodes[node.ID] = node
		nodes = append(nodes, node)
	}
	log.Printf("Created %d simulated nodes", len(nodes))
	return nodes, nil
}

func (nm *NodeManager) StopAllNodes() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	return nm.StopAllNodesInternal()
}

func (nm *NodeManager) StopAllNodesInternal() error {
	if !nm.usePython {
		// Use the Go implementation to stop nodes
		if err := nm.rubixManager.StopAllNodes(); err != nil {
			log.Printf("Warning: failed to stop nodes: %v", err)
			// Continue with cleanup even if stop fails
		} else {
			log.Printf("Stopped all nodes via Go manager")
		}
	}

	// Clean up internal state
	nm.nodes = make(map[string]*models.Node)

	return nil
}

func (nm *NodeManager) GetNodes() []*models.Node {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var nodes []*models.Node
	for _, node := range nm.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

func (nm *NodeManager) GetNode(id string) (*models.Node, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	node, exists := nm.nodes[id]
	if !exists {
		return nil, fmt.Errorf("node %s not found", id)
	}
	return node, nil
}

func (nm *NodeManager) checkNodeHealth(node *models.Node) error {
	return nil
}

func (nm *NodeManager) MarkNodesAsBusy(nodes []*models.Node) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	for _, node := range nodes {
		nm.busyNodes[node.ID] = true
	}
}

func (nm *NodeManager) MarkNodesAsAvailable(nodes []*models.Node) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	for _, node := range nodes {
		delete(nm.busyNodes, node.ID)
	}
}

func (nm *NodeManager) GetAvailableNodes(count int) ([]*models.Node, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var availableNodes []*models.Node
	for _, node := range nm.nodes {
		// Return only available, non-quorum nodes for transactions
		if !nm.busyNodes[node.ID] && !node.IsQuorum {
			availableNodes = append(availableNodes, node)
		}
	}

	if len(availableNodes) < count {
		return nil, fmt.Errorf("not enough available transaction nodes to run the simulation: have %d, need %d", len(availableNodes), count)
	}

	return availableNodes[:count], nil
}

// CheckTokenBalances triggers an immediate token balance check for all nodes
func (nm *NodeManager) CheckTokenBalances() {
	if nm.rubixManager != nil {
		nm.rubixManager.CheckBalancesNow()
	}
}

// AutoStartTokenMonitoring automatically starts token monitoring if nodes already exist
func (nm *NodeManager) AutoStartTokenMonitoring() {
	if nm.rubixManager != nil {
		nm.rubixManager.AutoStartTokenMonitoring()
	}
}

// SetSimulationActive controls whether token monitoring should be paused during simulations
func (nm *NodeManager) SetSimulationActive(active bool) {
	if nm.rubixManager != nil {
		nm.rubixManager.SetSimulationActive(active)
	}
}

// IsSimulationActive returns whether a simulation is currently running
func (nm *NodeManager) IsSimulationActive() bool {
	if nm.rubixManager != nil {
		return nm.rubixManager.IsSimulationActive()
	}
	return false
}