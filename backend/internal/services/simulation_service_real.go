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
}

func NewSimulationService(nm *NodeManager, te *TransactionExecutor, rg *ReportGenerator) *SimulationService {
	return &SimulationService{
		nodeManager:         nm,
		transactionExecutor: te,
		reportGenerator:     rg,
		simulations:         make(map[string]*models.SimulationReport),
	}
}

func (ss *SimulationService) GetNodeManager() *NodeManager {
	return ss.nodeManager
}

func (ss *SimulationService) StartSimulation(nodeCount, transactionCount int) (string, error) {
	// nodeCount represents additional non-quorum nodes beyond the 7 quorum nodes
	// Minimum 2 non-quorum nodes required for transactions
	if nodeCount < 2 || nodeCount > 20 {
		return "", fmt.Errorf("non-quorum node count must be between 2 and 20 (need at least 2 for sender/receiver)")
	}
	
	if transactionCount < 1 || transactionCount > 500 {
		return "", fmt.Errorf("transaction count must be between 1 and 500")
	}

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

	var nodes []*models.Node
	var err error
	
	// Start real Rubix nodes (7 quorum + additional non-quorum)
	// NO FALLBACK TO SIMULATION - if nodes don't start, simulation fails
	nodes, err = ss.nodeManager.StartNodes(nodeCount)
	if err != nil {
		log.Printf("ERROR: Failed to start Rubix nodes: %v", err)
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.IsFinished = true
			report.Error = fmt.Sprintf("Failed to start Rubix nodes. Ensure rubixgoplatform is installed and running: %v", err)
		})
		return
	}
	
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
	progressCallback := func(completed int, transactions []models.Transaction) {
		// Update report with current progress
		ss.updateReport(simulationID, func(report *models.SimulationReport) {
			report.TransactionsCompleted = completed
			
			// Count successes and failures so far
			successCount := 0
			failureCount := 0
			totalLatency := time.Duration(0)
			totalTokens := float64(0)
			
			for i := 0; i < completed && i < len(transactions); i++ {
				if transactions[i].Status == "success" {
					successCount++
					totalTokens += transactions[i].TokenAmount
				} else if transactions[i].Status == "failed" {
					failureCount++
				}
				if transactions[i].TimeTaken > 0 {
					totalLatency += transactions[i].TimeTaken
				}
			}
			
			report.SuccessCount = successCount
			report.FailureCount = failureCount
			report.TotalTokensTransferred = totalTokens
			
			// Calculate average latency for completed transactions
			if completed > 0 {
				report.AverageLatency = float64(totalLatency.Milliseconds()) / float64(completed)
			}
			
			// Store transactions processed so far
			if completed > 0 && len(transactions) > 0 {
				report.Transactions = transactions[:completed]
			}
		})
		
		log.Printf("Progress: %d/%d transactions completed", completed, transactionCount)
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
	minLatency := time.Duration(1<<63 - 1)
	maxLatency := time.Duration(0)
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
		if tx.TimeTaken < minLatency {
			minLatency = tx.TimeTaken
		}
		if tx.TimeTaken > maxLatency {
			maxLatency = tx.TimeTaken
		}

		// Track node stats
		if _, exists := nodeStats[tx.NodeID]; !exists {
			nodeStats[tx.NodeID] = &models.NodeStats{
				NodeID:                 tx.NodeID,
				TransactionsHandled:    0,
				SuccessfulTransactions: 0,
				FailedTransactions:     0,
				AverageLatency:         0,
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
		stats.AverageLatency += tx.TimeTaken
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
			stats.AverageLatency = stats.AverageLatency / time.Duration(stats.TransactionsHandled)
		}
		nodeBreakdown = append(nodeBreakdown, *stats)
	}

	report.Transactions = transactions
	report.TransactionsCompleted = len(transactions)
	report.SuccessCount = successCount
	report.FailureCount = failureCount
	report.AverageLatency = avgLatency
	report.MinLatency = minLatency
	report.MaxLatency = maxLatency
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