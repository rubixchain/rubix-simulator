package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rubix-simulator/backend/internal/models"
)

type SimulationService struct {
	nodeManager         *NodeManager
	transactionExecutor *TransactionExecutor
	reportGenerator     *ReportGenerator
	simulations         map[string]*models.SimulationReport
	mu                  sync.RWMutex
	isSimulationRunning bool
	simMu               sync.Mutex // Mutex for isSimulationRunning flag
}

func NewSimulationService(nm *NodeManager, te *TransactionExecutor, rg *ReportGenerator) *SimulationService {
	return &SimulationService{
		nodeManager:         nm,
		transactionExecutor: te,
		reportGenerator:     rg,
		simulations:         make(map[string]*models.SimulationReport),
		isSimulationRunning: false,
	}
}

func (ss *SimulationService) GetNodeManager() *NodeManager {
	return ss.nodeManager
}

func (ss *SimulationService) StartSimulation(nodeCount, transactionCount int) (string, error) {
	ss.simMu.Lock()
	if ss.isSimulationRunning {
		ss.simMu.Unlock()
		return "", fmt.Errorf("All servers are busy, please try again after some time.")
	}
	// Validate parameters before marking simulation as running
	// nodeCount represents additional non-quorum nodes beyond the 7 quorum nodes
	// Minimum 2 non-quorum nodes required for transactions
	if nodeCount < 2 || nodeCount > 20 {
		ss.simMu.Unlock()
		return "", fmt.Errorf("non-quorum node count must be between 2 and 20 (need at least 2 for sender/receiver)")
	}
	
	if transactionCount < 1 || transactionCount > 500 {
		ss.simMu.Unlock()
		return "", fmt.Errorf("transaction count must be between 1 and 500")
	}

	ss.isSimulationRunning = true
	ss.simMu.Unlock()

	// Pause token monitoring during simulation
	ss.nodeManager.SetSimulationActive(true)

	simulationID := uuid.New().String()
	
	report := &models.SimulationReport{
		SimulationID: simulationID,
		Config: models.SimulationConfig{
			ID:           simulationID,
			Nodes:        nodeCount + 7, // Total nodes (7 quorum + additional)
			Transactions: transactionCount,
			StartedAt:    time.Now(),
		},
		TotalTransactions: transactionCount,
		IsFinished:        false,
		CreatedAt:         time.Now(),
	}
	
	ss.mu.Lock()
	ss.simulations[simulationID] = report
	ss.mu.Unlock()

	// Run simulation in background
	go ss.runSimulation(simulationID, nodeCount, transactionCount)
	
	return simulationID, nil
}

func (ss *SimulationService) runSimulation(simulationID string, nodeCount, transactionCount int) {
	defer func() {
		// Handle any panic to ensure simulation state is cleaned up
		if r := recover(); r != nil {
			log.Printf("ERROR: Simulation %s panicked: %v", simulationID, r)
			ss.updateReport(simulationID, func(report *models.SimulationReport) {
				report.IsFinished = true
				report.Error = fmt.Sprintf("Simulation panicked: %v", r)
			})
		}
		
		ss.simMu.Lock()
		ss.isSimulationRunning = false
		ss.simMu.Unlock()
		
		// Resume token monitoring after simulation completes (even if it panicked)
		ss.nodeManager.SetSimulationActive(false)
	}()

	// Safely truncate ID for logging
	simID := simulationID
	if len(simID) > 8 {
		simID = simID[:8]
	}
	log.Printf("Starting simulation %s with %d non-quorum nodes and %d transactions", 
		simID, nodeCount, transactionCount)
	
	startTime := time.Now()
	
	ss.updateReport(simulationID, func(report *models.SimulationReport) {
		report.Config.StartedAt = startTime
	})

	// Ensure nodes are running
	if _, err := ss.nodeManager.StartNodes(nodeCount); err != nil {
		log.Printf("ERROR: Failed to start nodes: %v", err)
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.IsFinished = true
			report.Error = fmt.Sprintf("Failed to start nodes: %v", err)
		})
		return
	}

	// Get available nodes from the node manager
	nodes, err := ss.nodeManager.GetAvailableNodes(nodeCount)
    if err != nil {
		log.Printf("ERROR: Failed to get available nodes: %v", err)
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.IsFinished = true
			report.Error = fmt.Sprintf("Failed to get available nodes: %v", err)
		})
		return
	}

	// Mark nodes as busy
	ss.nodeManager.MarkNodesAsBusy(nodes)
	defer ss.nodeManager.MarkNodesAsAvailable(nodes)
	
	// Verify we have nodes
	if len(nodes) == 0 {
		log.Printf("ERROR: No nodes were started")
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.IsFinished = true
			report.Error = "No Rubix nodes could be started. Check rubixgoplatform installation."
		})
		return
	}
	
	// Count transaction nodes (non-quorum)
	transactionNodeCount := 0
	for _, node := range nodes {
		if !node.IsQuorum {
			transactionNodeCount++
		}
	}
	
	if transactionNodeCount < 2 {
		log.Printf("ERROR: Only %d transaction nodes available, need at least 2", transactionNodeCount)
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.IsFinished = true
			report.Error = fmt.Sprintf("Insufficient transaction nodes: %d (need minimum 2)", transactionNodeCount)
		})
		return
	}
	
	// Update report with node information
	ss.updateReport(simulationID, func(report *models.SimulationReport) {
		nodeList := make([]models.Node, len(nodes))
		for i, n := range nodes {
			nodeList[i] = *n
		}
		report.Nodes = nodeList
	})

	log.Printf("Executing %d real transactions on %d transaction nodes...", transactionCount, transactionNodeCount)
	
	// Execute real transactions on real nodes with progress reporting
	progressCallback := func(executorCompleted int, transactions []models.Transaction) {
		// Recompute progress strictly as Success + Failed across the whole slice
		successCount := 0
		failureCount := 0
		totalLatency := time.Duration(0)
		totalTokens := float64(0)
		completedTxs := make([]models.Transaction, 0, len(transactions))

		for _, tx := range transactions {
			if tx.Status == "success" {
				successCount++
				totalTokens += tx.TokenAmount
				if tx.TimeTaken > 0 {
					totalLatency += tx.TimeTaken
				}
				completedTxs = append(completedTxs, tx)
			} else if tx.Status == "failed" {
				failureCount++
				if tx.TimeTaken > 0 {
					totalLatency += tx.TimeTaken
				}
				completedTxs = append(completedTxs, tx)
			}
		}

		computedCompleted := successCount + failureCount

		// Update report with recomputed progress and metrics
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.TransactionsCompleted = computedCompleted
			report.SuccessCount = successCount
			report.FailureCount = failureCount
			report.TotalTokensTransferred = totalTokens
			if computedCompleted > 0 {
				report.AverageTransactionTime = float64(totalLatency.Milliseconds()) / float64(computedCompleted)
			}
			// Store only completed transactions
			report.Transactions = completedTxs
		})

		log.Printf("Progress: executor=%d, computed=%d/%d (success=%d, failed=%d)", executorCompleted, computedCompleted, transactionCount, successCount, failureCount)
	}
	
	transactions := ss.transactionExecutor.ExecuteTransactionsWithProgress(nodes, transactionCount, progressCallback)
	
	if len(transactions) == 0 {
		log.Printf("ERROR: No transactions were executed")
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.IsFinished = true
			report.Error = "Failed to execute transactions. Check if nodes are running with valid DIDs."
		})
		return
	}
	
	// Process final transaction results
	report := ss.processTransactions(simulationID, transactions)
	
	endTime := time.Now()
	totalTime := endTime.Sub(startTime)
	
	ss.updateReport(simulationID, func(r *models.SimulationReport) {
		r.Config.EndedAt = &endTime
		r.TotalTime = totalTime
		r.IsFinished = true
		*r = *report
	})

	// Generate PDF report
	pdfFilename, err := ss.reportGenerator.GeneratePDF(report)
	if err != nil {
		log.Printf("Failed to generate PDF report: %v", err)
	} else {
		log.Printf("PDF report generated: %s", pdfFilename)
	}
	
	// NOTE: Nodes are NOT stopped after simulation - they remain running for subsequent simulations
	// Users can manually stop nodes using the shutdown button in the UI
	log.Printf("Nodes remain running for next simulation. Use shutdown button to stop them.")
	
	// Reuse simID from earlier for logging
	log.Printf("Simulation %s completed in %v", simID, totalTime)
}

func (ss *SimulationService) processTransactions(simulationID string, transactions []models.Transaction) *models.SimulationReport {
	ss.mu.RLock()
	report := ss.simulations[simulationID]
	ss.mu.RUnlock()

	successCount := 0
	failureCount := 0
	totalLatency := time.Duration(0)
	minTransactionTime := time.Duration(1<<63 - 1)
	maxTransactionTime := time.Duration(0)
	totalTokensTransferred := float64(0)
	nodeStats := make(map[string]*models.NodeStats)

	for _, tx := range transactions {
		if tx.Status == "success" {
			successCount++
			totalTokensTransferred += tx.TokenAmount
		} else {
			failureCount++
		}

		totalLatency += tx.TimeTaken
		if tx.TimeTaken < minTransactionTime {
			minTransactionTime = tx.TimeTaken
		}
		if tx.TimeTaken > maxTransactionTime {
			maxTransactionTime = tx.TimeTaken
		}

		// Track node stats
		if _, exists := nodeStats[tx.NodeID]; !exists {
			nodeStats[tx.NodeID] = &models.NodeStats{
				NodeID:                 tx.NodeID,
				TransactionsHandled:    0,
				SuccessfulTransactions: 0,
				FailedTransactions:     0,
				AverageTransactionTime:         0,
				TotalTokensTransferred: float64(0),
			}
		}
		
		stats := nodeStats[tx.NodeID]
		stats.TransactionsHandled++
		if tx.Status == "success" {
			stats.SuccessfulTransactions++
			stats.TotalTokensTransferred += tx.TokenAmount
		} else {
			stats.FailedTransactions++
		}
		// We'll calculate average latency later
		stats.AverageTransactionTime += tx.TimeTaken
	}

	// Calculate averages
	avgLatency := float64(0)
	if len(transactions) > 0 {
		avgLatency = float64(totalLatency.Milliseconds()) / float64(len(transactions))
	}

	// Convert map to slice and calculate average latency for each node
	nodeBreakdown := make([]models.NodeStats, 0, len(nodeStats))
	for _, stats := range nodeStats {
		if stats.TransactionsHandled > 0 {
			stats.AverageTransactionTime = stats.AverageTransactionTime / time.Duration(stats.TransactionsHandled)
		}
		nodeBreakdown = append(nodeBreakdown, *stats)
	}

	report.Transactions = transactions
	report.TransactionsCompleted = len(transactions)
	report.SuccessCount = successCount
	report.FailureCount = failureCount
	report.AverageTransactionTime = avgLatency
	report.MinTransactionTime = minTransactionTime
	report.MaxTransactionTime = maxTransactionTime
	report.TotalTokensTransferred = totalTokensTransferred
	report.NodeBreakdown = nodeBreakdown

	return report
}

func (ss *SimulationService) GetReport(simulationID string) (*models.SimulationReport, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	report, exists := ss.simulations[simulationID]
	if !exists {
		return nil, fmt.Errorf("simulation %s not found", simulationID)
	}

	return report, nil
}

// GetSimulationReport is an alias for GetReport to match the handler's expectation
func (ss *SimulationService) GetSimulationReport(simulationID string) (*models.SimulationReport, error) {
	return ss.GetReport(simulationID)
}

func (ss *SimulationService) updateReport(simulationID string, updateFunc func(*models.SimulationReport)) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	
	if report, exists := ss.simulations[simulationID]; exists {
		updateFunc(report)
	}
}