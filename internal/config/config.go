package config

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	DefaultDBPath      = "gateway.db"
	DefaultDataDir     = "data"
	DefaultGatewayPort = 8080
	DefaultGatewayBind = "127.0.0.1"
	DefaultAdminUser   = "admin"
	AdminPasswordEnv   = "GATEWAY_ADMIN_PASSWORD"
	MinPasswordLength  = 5
	SettingGatewayPort = "gateway_port"
)

// AppConfig represents configuration values for the desktop application.
type AppConfig struct {
	DBPath      string `json:"dbPath"`
	DataPath    string `json:"dataPath"`
	GatewayPort int    `json:"gatewayPort"`
	GatewayBind string `json:"gatewayBind"`
}

// LoadConfig loads configuration from environment variables or sensible defaults.
func LoadConfig() *AppConfig {
	dbPath := os.Getenv("GATEWAY_DB_PATH")
	if dbPath == "" {
		dbPath = DefaultDBPath
	}
	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}

	dataPath := os.Getenv("GATEWAY_DATA_PATH")
	if dataPath == "" {
		dataPath = filepath.Join(filepath.Dir(dbPath), DefaultDataDir)
	} else if abs, err := filepath.Abs(dataPath); err == nil {
		dataPath = abs
	}

	port := DefaultGatewayPort
	if envPort := os.Getenv("GATEWAY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 && p <= 65535 {
			port = p
		}
	}

	return &AppConfig{
		DBPath:      dbPath,
		DataPath:    dataPath,
		GatewayPort: port,
		GatewayBind: gatewayBindAddress(),
	}
}

func gatewayBindAddress() string {
	if bind := os.Getenv("GATEWAY_BIND_ADDRESS"); bind != "" {
		return bind
	}
	return DefaultGatewayBind
}
