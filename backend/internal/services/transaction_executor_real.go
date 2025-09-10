package services

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rubix-simulator/backend/internal/config"
	"github.com/rubix-simulator/backend/internal/models"
	"github.com/rubix-simulator/backend/internal/rubix"
)

type TransactionExecutor struct {
	config     *config.Config
	httpClient *http.Client
}

func NewTransactionExecutor(cfg *config.Config) *TransactionExecutor {
	return &TransactionExecutor{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExecuteTransactions executes real transactions using real Rubix nodes with real DIDs
// Uses paired transaction model: nodes are paired for each round to prevent conflicts
func (te *TransactionExecutor) ExecuteTransactions(nodes []*models.Node, count int) []models.Transaction {
	return te.ExecuteTransactionsWithProgress(nodes, count, nil)
}

// ExecuteTransactionsWithProgress executes transactions and reports progress via callback
func (te *TransactionExecutor) ExecuteTransactionsWithProgress(nodes []*models.Node, count int, progressCallback func(completed int, transactions []models.Transaction)) []models.Transaction {
	// Filter out quorum nodes - only use non-quorum nodes for transactions
	transactionNodes := make([]*models.Node, 0)
	for _, node := range nodes {
		if !node.IsQuorum {
			transactionNodes = append(transactionNodes, node)
		}
	}
	
	if len(transactionNodes) < 2 {
		log.Println("ERROR: Need at least 2 transaction nodes for sender and receiver")
		return []models.Transaction{}
	}
	
	// Verify all transaction nodes have DIDs (created by Python script)
	for _, node := range transactionNodes {
		if node.DID == "" {
			log.Printf("ERROR: Node %s does not have a DID. Ensure rubixgoplatform is running and DIDs are created.", node.ID)
			return []models.Transaction{}
		}
	}

	log.Printf("Executing %d real transactions using %d transaction nodes (paired model)", count, len(transactionNodes))

	// IMPORTANT: Re-register each node's own DID to ensure peer discovery
	// This triggers the pub/sub broadcast mechanism for peer discovery
	log.Printf("Re-registering DIDs to ensure peer discovery before transactions...")
	
	// Register each node's own DID (this will broadcast via pub/sub)
	for _, node := range nodes {
		if node.DID == "" {
			continue
		}
		
		client := rubix.NewClient(node.Port)
		nodeType := "transaction"
		if node.IsQuorum {
			nodeType = "quorum"
		}
		
		// Truncate DID for logging
		didDisplay := node.DID
		if len(didDisplay) > 16 {
			didDisplay = didDisplay[:16] + "..."
		}
		
		log.Printf("Registering %s node %s DID: %s", nodeType, node.ID, didDisplay)
		
		// Register this node's own DID (will broadcast via pub/sub)
		err := client.RegisterDID(node.DID, "mypassword") // Using default password
		if err != nil {
			// If already registered, that's fine - it will still trigger broadcast
			if err.Error() != "DID already registered" && err.Error() != "already_registered" {
				log.Printf("  ⚠ Warning: Failed to register DID for %s: %v", node.ID, err)
			} else {
				log.Printf("  ✓ DID already registered for %s (broadcast triggered)", node.ID)
			}
		} else {
			log.Printf("  ✓ DID registered for %s (broadcast sent)", node.ID)
		}
		
		// Small delay to avoid overwhelming the network
		time.Sleep(100 * time.Millisecond)
	}
	
	// Wait for pub/sub propagation across the network
	log.Printf("Waiting 2 seconds for pub/sub broadcast to complete...")
	time.Sleep(2 * time.Second)

	// Pre-generate all transaction plans with random pairs
	type txPlan struct {
		index        int
		senderNode   *models.Node
		receiverNode *models.Node
	}
	
	allPlans := make([]txPlan, 0, count)
	
	// Generate random transaction plans
	for i := 0; i < count; i++ {
		// Select random sender node
		senderIdx := rand.Intn(len(transactionNodes))
		senderNode := transactionNodes[senderIdx]
		
		// Select different receiver node
		receiverIdx := senderIdx
		for receiverIdx == senderIdx && len(transactionNodes) > 1 {
			receiverIdx = rand.Intn(len(transactionNodes))
		}
		receiverNode := transactionNodes[receiverIdx]
		
		allPlans = append(allPlans, txPlan{
			index:        i,
			senderNode:   senderNode,
			receiverNode: receiverNode,
		})
	}
	
	transactions := make([]models.Transaction, count)
	transactionIndex := 0
	roundNumber := 1
	
	// Process transactions in rounds with pairing
	for transactionIndex < len(allPlans) {
		// Track which nodes are busy in this round
		busyNodes := make(map[string]bool)
		roundPlans := make([]txPlan, 0)
		
		// Select transactions for this round (ensuring no node is used twice)
		for i := transactionIndex; i < len(allPlans); i++ {
			plan := allPlans[i]
			
			// Skip already processed transactions
			if plan.senderNode == nil || plan.receiverNode == nil {
				continue
			}
			
			// Check if either node is already busy in this round
			if !busyNodes[plan.senderNode.ID] && !busyNodes[plan.receiverNode.ID] {
				// Mark both nodes as busy
				busyNodes[plan.senderNode.ID] = true
				busyNodes[plan.receiverNode.ID] = true
				
				roundPlans = append(roundPlans, plan)
				
				// For even nodes, we can have n/2 pairs max
				// For odd nodes, we can have (n-1)/2 pairs max
				maxPairs := len(transactionNodes) / 2
				if len(roundPlans) >= maxPairs {
					break
				}
			}
		}
		
		if len(roundPlans) == 0 {
			// This shouldn't happen, but handle it gracefully
			log.Printf("Warning: No valid pairs found in round %d, moving to next transaction", roundNumber)
			transactionIndex++
			continue
		}
		
		log.Printf("Round %d: Executing %d parallel transaction(s)", roundNumber, len(roundPlans))
		
		// Execute this round's transactions in parallel
		var wg sync.WaitGroup
		for _, plan := range roundPlans {
			wg.Add(1)
			go func(p txPlan) {
				defer wg.Done()
				
				// Use real DIDs from nodes
				senderDID := p.senderNode.DID
				receiverDID := p.receiverNode.DID
				
				log.Printf("  Round %d: Executing transaction %d: %s -> %s", 
					roundNumber, p.index, p.senderNode.ID, p.receiverNode.ID)
				
				// Execute the transaction
				transaction := te.executeRealTransaction(
					p.senderNode, 
					senderDID, 
					p.receiverNode, 
					receiverDID, 
					p.index,
				)
				transactions[p.index] = transaction
				
				// Mark this plan as processed (set both to nil to avoid partial state)
				for j := range allPlans {
					if allPlans[j].index == p.index {
						allPlans[j].senderNode = nil
						allPlans[j].receiverNode = nil
						break
					}
				}
			}(plan)
		}
		
		// Wait for this round to complete
		wg.Wait()
		
		// Report progress after each round if callback provided
		if progressCallback != nil {
			completedCount := 0
			for _, plan := range allPlans {
				if plan.senderNode == nil { // Marked as processed
					completedCount++
				}
			}
			log.Printf("Progress update: %d/%d transactions completed", completedCount, count)
			progressCallback(completedCount, transactions)
		}
		
		// Move to next unprocessed transactions
		for transactionIndex < len(allPlans) && allPlans[transactionIndex].senderNode == nil {
			transactionIndex++
		}
		
		// Small delay between rounds to ensure blockchain state is updated
		if transactionIndex < len(allPlans) {
			time.Sleep(500 * time.Millisecond)
		}
		
		roundNumber++
	}
	
	log.Printf("Completed %d transactions in %d rounds", count, roundNumber-1)
	return transactions
}

func (te *TransactionExecutor) executeRealTransaction(senderNode *models.Node, senderDID string, receiverNode *models.Node, receiverDID string, index int) models.Transaction {
	tokenAmount := float64(rand.Intn(10) + 1)
	
	transaction := models.Transaction{
		ID:          uuid.New().String(),
		Sender:      senderDID,
		Receiver:    receiverDID,
		TokenAmount: tokenAmount,
		Comment:     fmt.Sprintf("Transaction %d from %s to %s", index, senderNode.ID, receiverNode.ID),
		NodeID:      senderNode.ID, // Transaction initiated from sender node
		Timestamp:   time.Now(),
		Status:      "pending",
	}

	startTime := time.Now()

	// Check sender's balance before attempting transaction
	client := rubix.NewClient(senderNode.Port)
	
	balance, err := client.GetAccountBalance(senderDID)
	if err != nil {
		transaction.Status = "failed"
		transaction.Error = fmt.Sprintf("Failed to check balance: %v", err)
		transaction.TimeTaken = time.Since(startTime)
		log.Printf("Failed to check balance for %s: %v", senderNode.ID, err)
		return transaction
	}

	log.Printf("Node %s balance: %.3f RBT, attempting to send: %.3f RBT", senderNode.ID, balance, tokenAmount)

	// Check if sender has sufficient balance
	if balance < tokenAmount {
		// Try with a smaller amount that the sender can afford
		if balance > 1.0 {
			// Use 80% of available balance to leave some for fees
			tokenAmount = balance * 0.8
			// Round to 3 decimal places as required by Rubix API
			tokenAmount = float64(int(tokenAmount*1000)) / 1000.0
			transaction.TokenAmount = tokenAmount
			log.Printf("Adjusted transaction amount to %.3f RBT (80%% of available %.3f RBT)", tokenAmount, balance)
		} else {
			transaction.Status = "failed"
			transaction.Error = fmt.Sprintf("Insufficient balance: have %.2f RBT, need %.2f RBT", balance, tokenAmount)
			transaction.TimeTaken = time.Since(startTime)
			log.Printf("Insufficient balance for %s: have %.2f, need %.2f", senderNode.ID, balance, tokenAmount)
			return transaction
		}
	}

	// Use the new InitiateRBTTransfer function with signature handling
	// Using hardcoded password for test environment
	transactionID, err := client.InitiateRBTTransfer(
		transaction.Sender,
		transaction.Receiver,
		transaction.TokenAmount,
		transaction.Comment,
		"mypassword", // Default password for test environment
	)
	
	transaction.TimeTaken = time.Since(startTime)
	
	if err != nil {
		transaction.Status = "failed"
		transaction.Error = fmt.Sprintf("Failed to execute transfer: %v", err)
		// Safely truncate ID for logging
		txID := transaction.ID
		if len(txID) > 8 {
			txID = txID[:8]
		}
		log.Printf("Transaction %s failed: %v", txID, err)
		return transaction
	}
	
	// Update transaction ID if we got one from the API
	if transactionID != "" {
		transaction.ID = transactionID
	}
	
	transaction.Status = "success"
	// Safely truncate ID for logging
	txID := transaction.ID
	if len(txID) > 8 {
		txID = txID[:8]
	}
	log.Printf("Transaction %s completed successfully in %v", txID, transaction.TimeTaken)

	return transaction
}