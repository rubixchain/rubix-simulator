package models

import (
	"time"
)

type Node struct {
	ID       string    `json:"id"`
	Port     int       `json:"port"`
	GrpcPort int       `json:"grpcPort,omitempty"`
	PID      int       `json:"pid,omitempty"`
	DID      string    `json:"did,omitempty"`
	IsQuorum bool      `json:"isQuorum"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started"`
}

type Transaction struct {
	ID          string        `json:"id"`
	Sender      string        `json:"sender"`
	Receiver    string        `json:"receiver"`
	TokenAmount float64       `json:"tokenAmount"`  // Changed to float64 for RBT transfers
	Comment     string        `json:"comment"`
	Status      string        `json:"status"`
	TimeTaken   time.Duration `json:"timeTaken"`
	Error       string        `json:"error,omitempty"`
	NodeID      string        `json:"nodeId"`
	Timestamp   time.Time     `json:"timestamp"`
}

type SimulationConfig struct {
	ID           string    `json:"id"`
	Nodes        int       `json:"nodes"`
	Transactions int       `json:"transactions"`
	StartedAt    time.Time `json:"startedAt"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`
}

type SimulationReport struct {
	SimulationID          string          `json:"simulationId"`
	Config               SimulationConfig `json:"config"`
	Nodes                []Node          `json:"nodes"`
	Transactions         []Transaction   `json:"transactions"`
	TransactionsCompleted int            `json:"transactionsCompleted"`
	TotalTransactions    int            `json:"totalTransactions"`
	SuccessCount         int            `json:"successCount"`
	FailureCount         int            `json:"failureCount"`
	AverageLatency       float64        `json:"averageLatency"`
	MinLatency           time.Duration  `json:"minLatency"`
	MaxLatency           time.Duration  `json:"maxLatency"`
	TotalTokensTransferred float64       `json:"totalTokensTransferred"`
	TotalTime            time.Duration  `json:"totalTime"`
	IsFinished           bool           `json:"isFinished"`
	Error                string         `json:"error,omitempty"`
	NodeBreakdown        []NodeStats    `json:"nodeBreakdown"`
	CreatedAt            time.Time      `json:"createdAt"`
}

type NodeStats struct {
	NodeID               string        `json:"nodeId"`
	TransactionsHandled  int          `json:"transactionsHandled"`
	SuccessfulTransactions int        `json:"successfulTransactions"`
	FailedTransactions   int          `json:"failedTransactions"`
	AverageLatency       time.Duration `json:"averageLatency"`
	TotalTokensTransferred float64    `json:"totalTokensTransferred"`
}

type SimulationRequest struct {
	Nodes        int `json:"nodes"`
	Transactions int `json:"transactions"`
}

type SimulationResponse struct {
	SimulationID string `json:"simulationId"`
	Message      string `json:"message"`
}

type ReportInfo struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	CreatedAt time.Time `json:"createdAt"`
	Size      int64     `json:"size"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

type RubixTransferRequest struct {
	Receiver    string  `json:"receiver"`
	Sender      string  `json:"sender"`
	TokenCount  float64 `json:"tokenCOunt"`  // Capital O as expected by API
	Comment     string  `json:"comment"`
	Type        int     `json:"type"`
}

type RubixTransferResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  struct {
		TransactionID string `json:"transactionID"`
	} `json:"result"`
}

// AccountInfo represents the response from get-account-info API
type AccountInfoResponse struct {
	Status      bool              `json:"status"`
	Message     string            `json:"message"`
	AccountInfo []DIDAccountInfo  `json:"account_info"`
}

type DIDAccountInfo struct {
	DID        string  `json:"did"`
	DIDType    int     `json:"did_type"`
	RBTAmount  float64 `json:"rbt_amount"`   // Available balance
	PledgedRBT float64 `json:"pledged_rbt"`  // Pledged tokens
	LockedRBT  float64 `json:"locked_rbt"`   // Locked tokens
	PinnedRBT  float64 `json:"pinned_rbt"`   // Pinned tokens
}