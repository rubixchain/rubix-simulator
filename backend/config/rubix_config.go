package config

// RubixConfig contains configuration for Rubix node management
type RubixConfig struct {
	// DataDir is the root directory for all Rubix-related data
	DataDir string `json:"dataDir"`
	
	// Network configuration
	BaseServerPort int `json:"baseServerPort"`
	BaseGrpcPort   int `json:"baseGrpcPort"`
	
	// Node configuration
	QuorumNodeCount     int `json:"quorumNodeCount"`
	MinTransactionNodes int `json:"minTransactionNodes"`
	MaxTransactionNodes int `json:"maxTransactionNodes"`
	
	// Timeouts and delays
	NodeStartupDelay   int `json:"nodeStartupDelay"`   // Seconds to wait for node startup
	NodeStartupTimeout int `json:"nodeStartupTimeout"` // Maximum seconds to wait for node
	
	// Rubix platform settings
	RubixRepoURL    string `json:"rubixRepoUrl"`
	RubixBranch     string `json:"rubixBranch"`
	IPFSVersion     string `json:"ipfsVersion"`
	TestSwarmKeyURL string `json:"testSwarmKeyUrl"`
	
	// Default passwords (for testing only)
	DefaultPrivKeyPassword   string `json:"defaultPrivKeyPassword"`
	DefaultQuorumKeyPassword string `json:"defaultQuorumKeyPassword"`
	
	// Token monitoring configuration
	TokenMonitoringEnabled    bool    `json:"tokenMonitoringEnabled"`    // Enable/disable automatic token monitoring
	TokenMonitoringInterval   int     `json:"tokenMonitoringInterval"`   // Minutes between balance checks
	MinTokenBalance          float64 `json:"minTokenBalance"`           // Minimum balance threshold (RBT)
	TokenRefillAmount        int     `json:"tokenRefillAmount"`         // Amount to generate when below threshold
	// Note: Token monitoring automatically pauses during active simulations to avoid interfering with transaction results
}

// DefaultRubixConfig returns the default configuration
func DefaultRubixConfig() *RubixConfig {
	return &RubixConfig{
		DataDir:             "./rubix-data",
		BaseServerPort:      20000,
		BaseGrpcPort:        10500,
		QuorumNodeCount:     7,
		MinTransactionNodes: 2,
		MaxTransactionNodes: 20,
		NodeStartupDelay:    40,
		NodeStartupTimeout:  120,  // Increased to 2 minutes for slower systems
		RubixRepoURL:        "https://github.com/rubixchain/rubixgoplatform.git",
		RubixBranch:         "main",
		IPFSVersion:         "v0.21.0",
		TestSwarmKeyURL:     "https://raw.githubusercontent.com/rubixchain/rubixgoplatform/main/testswarm.key",
		DefaultPrivKeyPassword:   "mypassword",
		DefaultQuorumKeyPassword: "mypassword",
		
		// Token monitoring defaults
		TokenMonitoringEnabled:  true,
		TokenMonitoringInterval: 10,     // 10 minutes
		MinTokenBalance:        1000.0,  // 1000 RBT threshold
		TokenRefillAmount:      100,     // Generate 100 tokens when below threshold
	}
}