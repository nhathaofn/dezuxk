package config

import (
	"os"
	"strconv"
)

const (
	DefaultDBPath      = "gateway.db"
	DefaultGatewayPort = 8080
	DefaultAdminUser   = "admin"
	DefaultAdminPass   = "admin123"
	SettingGatewayPort = "gateway_port"
)

// AppConfig represents configuration values for the desktop application.
type AppConfig struct {
	DBPath      string `json:"dbPath"`
	GatewayPort int    `json:"gatewayPort"`
}

// LoadConfig loads configuration from environment variables or sensible defaults.
func LoadConfig() *AppConfig {
	dbPath := os.Getenv("GATEWAY_DB_PATH")
	if dbPath == "" {
		dbPath = DefaultDBPath
	}

	port := DefaultGatewayPort
	if envPort := os.Getenv("GATEWAY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 && p <= 65535 {
			port = p
		}
	}

	return &AppConfig{
		DBPath:      dbPath,
		GatewayPort: port,
	}
}
