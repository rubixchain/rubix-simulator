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
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		RubixScriptPath: getEnv("RUBIX_SCRIPT_PATH", "./scripts/rubix_node_manager.py"),
		ReportsPath:     getEnv("REPORTS_PATH", "./reports"),
		MaxNodes:        20,
		MaxTransactions: 500,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}