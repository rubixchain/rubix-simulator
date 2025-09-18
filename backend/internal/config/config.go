package config

import (
	"os"
)

type Config struct {
	Port            string
	RubixScriptPath string
	ReportsPath     string
	MaxNodes        int
	MaxTransactions int
	ExplorerBaseURL string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		RubixScriptPath: getEnv("RUBIX_SCRIPT_PATH", "./scripts/rubix_node_manager.py"),
		ReportsPath:     getEnv("REPORTS_PATH", "./reports"),
		MaxNodes:        20,
		MaxTransactions: 500,
		ExplorerBaseURL: getEnv("EXPLORER_BASE_URL", "https://testnet.rubixexplorer.com/#/transaction"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}