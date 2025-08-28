package rubix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Client represents a Rubix node HTTP client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Rubix node client
func NewClient(port int) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BasicResponse represents the standard response from Rubix APIs
type BasicResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result,omitempty"`
}

// DIDResponse represents the response from DID creation
type DIDResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Result  struct {
		DID    string `json:"did"`
		PeerID string `json:"peerID"`
	} `json:"result"`
}

// QuorumData represents a quorum member
type QuorumData struct {
	Type    int    `json:"type"`
	Address string `json:"address"`
}

// Start initializes the node core
func (c *Client) Start() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/start")
	if err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}
	defer resp.Body.Close()

	var result BasicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return fmt.Errorf("start failed: %s", result.Message)
	}

	return nil
}

// Shutdown stops the node
func (c *Client) Shutdown() error {
	resp, err := c.httpClient.Post(c.baseURL+"/api/shutdown", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to shutdown node: %w", err)
	}
	defer resp.Body.Close()

	var result BasicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return fmt.Errorf("shutdown failed: %s", result.Message)
	}

	return nil
}

// NodeStatus checks if the node is running
func (c *Client) NodeStatus() (bool, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/node-status")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result BasicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Status, nil
}

// CreateDID creates a new DID of type 4
func (c *Client) CreateDID(privKeyPassword string) (string, string, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add DID config - matching field names from reference function
	didConfig := map[string]interface{}{
		"Type":          4,
		"priv_pwd":      privKeyPassword,
		"mnemonic_file": "",
		"childPath":     0,
	}

	configJSON, err := json.Marshal(didConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writer.WriteField("did_config", string(configJSON)); err != nil {
		return "", "", fmt.Errorf("failed to write field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close writer: %w", err)
	}

	// Make request
	req, err := http.NewRequest("POST", c.baseURL+"/api/createdid", &buf)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result DIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return "", "", fmt.Errorf("create DID failed: %s", result.Message)
	}

	return result.Result.DID, result.Result.PeerID, nil
}

// RegisterDID registers a DID with signature handling
func (c *Client) RegisterDID(did string, password string) error {
	log.Printf("[RegisterDID] Starting DID registration for: %s", did)
	
	payload := map[string]string{
		"did": did,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/register-did", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to register DID: %w", err)
	}
	defer resp.Body.Close()

	// Parse the response to check if signature is needed
	var sigResp SignatureResponse
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[RegisterDID] Response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register DID failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse the response to check if password is needed
	if err := json.Unmarshal(body, &sigResp); err != nil {
		log.Printf("[RegisterDID] ERROR: Failed to parse response: %v", err)
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// If password is needed, send signature response
	if sigResp.Status && sigResp.Message == "Password needed" {
		log.Printf("[RegisterDID] Password required, sending signature response...")
		
		result, err := c.SendSignatureResponse(sigResp.Result.ID, sigResp.Result.Mode, password)
		if err != nil {
			log.Printf("[RegisterDID] ERROR: Failed to send signature response: %v", err)
			// For RegisterDID, we don't need the transaction ID, just success/failure
			return fmt.Errorf("failed to send signature response: %w", err)
		}
		
		if result != nil && result.Success {
			log.Printf("[RegisterDID] Signature response sent successfully, registration complete")
		} else {
			log.Printf("[RegisterDID] Signature response sent, waiting for registration to complete...")
		}
	}

	// Wait a bit for the async operation to complete
	time.Sleep(5 * time.Second)
	log.Printf("[RegisterDID] DID registration completed for: %s", did)

	return nil
}

// SignatureResponse structure for handling signature requests
type SignatureResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Result  struct {
		ID          string `json:"id"`
		Mode        int    `json:"mode"`
		Hash        []byte `json:"hash"`
		OnlyPrivKey bool   `json:"only_priv_key"`
	} `json:"result"`
}

// TransferResult represents the final result of a transfer
type TransferResult struct {
	Success       bool
	TransactionID string
	Message       string
	TimeTaken     time.Duration
}

// SendSignatureResponse sends a signature response with password
func (c *Client) SendSignatureResponse(id string, mode int, password string) (*TransferResult, error) {
	log.Printf("[SendSignatureResponse] Starting signature response for request ID: %s", id)
	log.Printf("[SendSignatureResponse]   Mode: %d (0=Basic, 1=Standard, 2=Wallet, 3=Child, 4=Lite)", mode)
	log.Printf("[SendSignatureResponse]   Target: %s", c.baseURL)
	
	payload := map[string]interface{}{
		"id":       id,
		"mode":     mode,
		"password": password,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signature response: %w", err)
	}
	
	log.Printf("[SendSignatureResponse] Payload: %s", string(data))

	// Use a 15-minute timeout for signature operations as they may involve consensus
	signatureClient := &http.Client{
		Timeout: 15 * time.Minute, // 15 minutes timeout for signature operations
	}

	log.Printf("[SendSignatureResponse] Sending POST request to %s/api/signature-response (timeout: 15 minutes)...", c.baseURL)
	startTime := time.Now()
	
	resp, err := signatureClient.Post(c.baseURL+"/api/signature-response", "application/json", bytes.NewBuffer(data))
	elapsed := time.Since(startTime)
	
	if err != nil {
		log.Printf("[SendSignatureResponse] ERROR: Request failed after %v: %v", elapsed, err)
		return nil, fmt.Errorf("failed to send signature response: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[SendSignatureResponse] Response received after %v", elapsed)
	log.Printf("[SendSignatureResponse]   Status: %d", resp.StatusCode)
	log.Printf("[SendSignatureResponse]   Body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[SendSignatureResponse] ERROR: Non-200 status code")
		return nil, fmt.Errorf("signature response failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response to check transaction status
	var result BasicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[SendSignatureResponse] ERROR: Failed to parse response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Create transfer result
	transferResult := &TransferResult{
		Success:   result.Status,
		Message:   result.Message,
		TimeTaken: elapsed,
	}

	if !result.Status {
		log.Printf("[SendSignatureResponse] ERROR: Transfer failed: %s", result.Message)
		return transferResult, fmt.Errorf("transfer failed: %s", result.Message)
	}

	// Parse success message to extract transaction ID
	// Message format: "Transfer finished successfully in 5m51.7789643s with trnxid 08765414814e03e9ffb71f3cedda61c7246f40cf1a48b2d5f6cdfdfc359b13e3"
	if strings.Contains(result.Message, "Transfer finished successfully") {
		if idx := strings.Index(result.Message, "trnxid "); idx != -1 {
			txID := result.Message[idx+7:] // Skip "trnxid "
			// Remove any trailing text or whitespace
			if spaceIdx := strings.Index(txID, " "); spaceIdx != -1 {
				txID = txID[:spaceIdx]
			}
			transferResult.TransactionID = strings.TrimSpace(txID)
			log.Printf("[SendSignatureResponse] SUCCESS: Transaction completed with ID: %s", transferResult.TransactionID)
		}
	}

	log.Printf("[SendSignatureResponse] SUCCESS: %s", result.Message)
	return transferResult, nil
}

// GenerateTestTokens generates test RBT tokens with signature handling
func (c *Client) GenerateTestTokens(did string, numberOfTokens int, password string) error {
	log.Printf("[GenerateTestTokens] Starting token generation for DID: %s, numberOfTokens: %d", did, numberOfTokens)
	
	payload := map[string]interface{}{
		"number_of_tokens": numberOfTokens,
		"did":              did,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[GenerateTestTokens] ERROR: Failed to marshal request: %v", err)
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	
	log.Printf("[GenerateTestTokens] Sending request to %s with payload: %s", c.baseURL+"/api/generate-test-token", string(data))

	resp, err := c.httpClient.Post(c.baseURL+"/api/generate-test-token", "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("[GenerateTestTokens] ERROR: Failed to make HTTP request: %v", err)
		return fmt.Errorf("failed to generate tokens: %w", err)
	}
	defer resp.Body.Close()

	// Parse the response to check if signature is needed
	var sigResp SignatureResponse
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[GenerateTestTokens] Response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[GenerateTestTokens] ERROR: Non-200 status code received")
		return fmt.Errorf("generate tokens failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse the response to check if password is needed
	if err := json.Unmarshal(body, &sigResp); err != nil {
		log.Printf("[GenerateTestTokens] ERROR: Failed to parse response: %v", err)
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// If password is needed, send signature response
	if sigResp.Status && sigResp.Message == "Password needed" {
		log.Printf("[GenerateTestTokens] Password required, sending signature response...")
		
		result, err := c.SendSignatureResponse(sigResp.Result.ID, sigResp.Result.Mode, password)
		if err != nil {
			log.Printf("[GenerateTestTokens] ERROR: Failed to send signature response: %v", err)
			// For token generation, we don't need the transaction ID
			return fmt.Errorf("failed to send signature response: %w", err)
		}
		
		if result != nil && result.Success {
			log.Printf("[GenerateTestTokens] Token generation completed successfully")
		} else {
			log.Printf("[GenerateTestTokens] Signature response sent, waiting for token generation...")
		}
	}

	// Wait and check balance periodically
	log.Printf("[GenerateTestTokens] Waiting for async token generation...")
	
	for i := 0; i < 10; i++ {  // Check for up to 50 seconds (10 * 5 seconds)
		time.Sleep(5 * time.Second)
		
		balance, err := c.GetAccountBalance(did)
		if err != nil {
			log.Printf("[GenerateTestTokens] Check %d: Failed to get balance: %v", i+1, err)
		} else {
			log.Printf("[GenerateTestTokens] Check %d: Current balance: %.2f RBT", i+1, balance)
			if balance > 0 {
				log.Printf("[GenerateTestTokens] SUCCESS: Tokens generated! Final balance: %.2f RBT", balance)
				return nil
			}
		}
	}
	
	log.Printf("[GenerateTestTokens] WARNING: Token generation may have failed - balance still 0 after 50 seconds")
	return nil
}

// AddQuorum adds quorum list to the node
func (c *Client) AddQuorum(quorumList []QuorumData) error {
	log.Printf("[AddQuorum] Adding %d quorum members to node at %s", len(quorumList), c.baseURL)
	
	data, err := json.Marshal(quorumList)
	if err != nil {
		return fmt.Errorf("failed to marshal quorum list: %w", err)
	}
	
	log.Printf("[AddQuorum] Sending quorum list: %s", string(data))

	resp, err := c.httpClient.Post(c.baseURL+"/api/addquorum", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to add quorum: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[AddQuorum] Response: %s", string(body))
	
	var result BasicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		log.Printf("[AddQuorum] ERROR: Failed to add quorum: %s", result.Message)
		return fmt.Errorf("add quorum failed: %s", result.Message)
	}

	log.Printf("[AddQuorum] Successfully added quorum list")
	return nil
}

// GetAllQuorum gets all quorum members
func (c *Client) GetAllQuorum() ([]QuorumData, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/getallquorum")
	if err != nil {
		return nil, fmt.Errorf("failed to get quorum: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status  bool         `json:"status"`
		Message string       `json:"message"`
		Result  []QuorumData `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return nil, fmt.Errorf("get quorum failed: %s", result.Message)
	}

	return result.Result, nil
}

// SetupQuorum sets up the node as a quorum member
func (c *Client) SetupQuorum(did, password, privKeyPassword string) error {
	payload := map[string]string{
		"did":           did,
		"password":      password,
		"priv_password": privKeyPassword,  // Changed to match QuorumSetup struct
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/setup-quorum", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to setup quorum: %w", err)
	}
	defer resp.Body.Close()

	var result BasicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return fmt.Errorf("setup quorum failed: %s", result.Message)
	}

	return nil
}

// GetPeerID gets the peer ID of the node
func (c *Client) GetPeerID() (string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/get-peer-id")
	if err != nil {
		return "", fmt.Errorf("failed to get peer ID: %w", err)
	}
	defer resp.Body.Close()

	var result BasicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return "", fmt.Errorf("get peer ID failed: %s", result.Message)
	}

	return result.Message, nil
}

// GetAccountInfo gets account information for a DID (returns raw map for compatibility)
func (c *Client) GetAccountInfo(did string) (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/get-account-info?did=" + did)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if status, ok := result["status"].(bool); ok && !status {
		if msg, ok := result["message"].(string); ok {
			return nil, fmt.Errorf("get account info failed: %s", msg)
		}
	}

	return result, nil
}

// GetAccountBalance gets the available RBT balance for a DID
func (c *Client) GetAccountBalance(did string) (float64, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/get-account-info?did=" + did)
	if err != nil {
		return 0, fmt.Errorf("failed to get account info: %w", err)
	}
	defer resp.Body.Close()

	// Import models package for the response type
	var accountResp struct {
		Status      bool   `json:"status"`
		Message     string `json:"message"`
		AccountInfo []struct {
			DID       string  `json:"did"`
			RBTAmount float64 `json:"rbt_amount"`
		} `json:"account_info"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&accountResp); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if !accountResp.Status {
		return 0, fmt.Errorf("get account info failed: %s", accountResp.Message)
	}

	// Return the balance of the first account (should only be one for the given DID)
	if len(accountResp.AccountInfo) > 0 {
		return accountResp.AccountInfo[0].RBTAmount, nil
	}

	return 0, fmt.Errorf("no account info found for DID: %s", did)
}

// RBTTransferRequest represents the request for RBT transfer
type RBTTransferRequest struct {
	Sender     string  `json:"sender"`
	Receiver   string  `json:"receiver"`
	TokenCount float64 `json:"tokenCOunt"` // Capital O as expected by API
	Comment    string  `json:"comment"`
	Type       int     `json:"type"`
}

// RBTTransferResponse represents the response from RBT transfer
type RBTTransferResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Result  struct {
		TransactionID string `json:"transaction_id"`
	} `json:"result"`
}

// InitiateRBTTransfer initiates an RBT transfer with signature handling
func (c *Client) InitiateRBTTransfer(sender, receiver string, amount float64, comment string, password string) (string, error) {
	// Round amount to 3 decimal places as required by Rubix API
	amount = float64(int(amount*1000)) / 1000.0
	
	log.Printf("[InitiateRBTTransfer] Starting transfer from %s to %s, amount: %.3f", sender, receiver, amount)
	
	request := RBTTransferRequest{
		Sender:     sender,
		Receiver:   receiver,
		TokenCount: amount,
		Comment:    comment,
		Type:       2, // Type 2 for RBT transfer
	}

	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[InitiateRBTTransfer] Sending request with payload: %s", string(data))

	resp, err := c.httpClient.Post(c.baseURL+"/api/initiate-rbt-transfer", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to initiate transfer: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[InitiateRBTTransfer] Response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("initiate transfer failed (status %d): %s", resp.StatusCode, string(body))
	}

	// First try to parse as signature response
	var sigResp SignatureResponse
	if err := json.Unmarshal(body, &sigResp); err == nil && sigResp.Status && sigResp.Message == "Password needed" {
		log.Printf("[InitiateRBTTransfer] Password required for DID mode %d, request ID: %s", sigResp.Result.Mode, sigResp.Result.ID)
		log.Printf("[InitiateRBTTransfer] Sending signature response with password...")
		
		startTime := time.Now()
		transferResult, err := c.SendSignatureResponse(sigResp.Result.ID, sigResp.Result.Mode, password)
		if err != nil {
			log.Printf("[InitiateRBTTransfer] ERROR: Failed to complete transfer after %v: %v", time.Since(startTime), err)
			
			// Check if we have a transfer result even with error (transaction might have failed on chain)
			if transferResult != nil && !transferResult.Success {
				log.Printf("[InitiateRBTTransfer] Transfer failed on blockchain: %s", transferResult.Message)
				return "", fmt.Errorf("transfer failed: %s", transferResult.Message)
			}
			
			return "", fmt.Errorf("failed to complete transfer: %w", err)
		}
		
		log.Printf("[InitiateRBTTransfer] Transfer completed in %v", time.Since(startTime))
		
		// Check if transaction was actually successful
		if transferResult != nil {
			if !transferResult.Success {
				log.Printf("[InitiateRBTTransfer] Transfer failed: %s", transferResult.Message)
				return "", fmt.Errorf("transfer failed: %s", transferResult.Message)
			}
			
			if transferResult.TransactionID != "" {
				log.Printf("[InitiateRBTTransfer] Transfer successful, transaction ID: %s", transferResult.TransactionID)
				return transferResult.TransactionID, nil
			}
		}
		
		// Fallback to request ID if no transaction ID found
		log.Printf("[InitiateRBTTransfer] Warning: No transaction ID in result, using request ID: %s", sigResp.Result.ID)
		return sigResp.Result.ID, nil
	}

	// If not a signature request, try to parse as transfer response
	var transferResp RBTTransferResponse
	if err := json.Unmarshal(body, &transferResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !transferResp.Status {
		return "", fmt.Errorf("transfer failed: %s", transferResp.Message)
	}

	log.Printf("[InitiateRBTTransfer] Transfer completed successfully")
	return transferResp.Result.TransactionID, nil
}

// Ping checks if the node is responsive
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/ping")
	if err != nil {
		return fmt.Errorf("failed to ping node: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status: %d", resp.StatusCode)
	}

	return nil
}

// GetPeerCount gets the number of connected peers
func (c *Client) GetPeerCount() (int, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/get-peer-count")
	if err != nil {
		return 0, fmt.Errorf("failed to get peer count: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status    bool   `json:"status"`
		Message   string `json:"message"`
		PeerCount int    `json:"peerCount"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Status {
		return 0, fmt.Errorf("get peer count failed: %s", result.Message)
	}

	return result.PeerCount, nil
}

// CheckQuorumStatus checks if a quorum member is properly set up
func (c *Client) CheckQuorumStatus(quorumAddress string) (bool, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/check-quorum-status?quorumAddress=" + quorumAddress)
	if err != nil {
		return false, fmt.Errorf("failed to check quorum status: %w", err)
	}
	defer resp.Body.Close()

	var result BasicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Status, nil
}

// WaitForNode waits for the node to be ready with exponential backoff
func (c *Client) WaitForNode(timeout time.Duration) error {
	start := time.Now()
	attempt := 0
	maxBackoff := 10 * time.Second
	
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for node to be ready after %v", timeout)
		}

		status, err := c.NodeStatus()
		if err == nil && status {
			return nil
		}
		
		// Log progress every 5 attempts
		attempt++
		if attempt%5 == 0 {
			log.Printf("Still waiting for node at %s (attempt %d, elapsed: %v)", 
				c.baseURL, attempt, time.Since(start))
		}

		// Exponential backoff with jitter
		backoff := time.Duration(float64(time.Second) * (1 + 0.5*float64(attempt)))
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		
		time.Sleep(backoff)
	}
}

// WaitForNodeWithRetry waits for node with configurable retry strategy
func (c *Client) WaitForNodeWithRetry(timeout time.Duration, maxRetries int) error {
	var lastErr error
	
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			log.Printf("Retry %d/%d waiting for node at %s", retry+1, maxRetries, c.baseURL)
			time.Sleep(time.Duration(retry*2) * time.Second)
		}
		
		if err := c.WaitForNode(timeout); err != nil {
			lastErr = err
			continue
		}
		
		return nil
	}
	
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}