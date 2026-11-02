// Package config implements layered, zero-hardcode configuration management for dezuxk.
package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	APIKey string `json:"api_key"`
}

// StorageConfig defines database and persistence settings.
type StorageConfig struct {
	DBFile  string `json:"db_file"`
	DataDir string `json:"data_dir"`
}

// SecurityConfig defines encryption and secret protection settings.
type SecurityConfig struct {
	MasterKey string `json:"master_key"`
	UseDPAPI  bool   `json:"use_dpapi"`
}

// Config is the root configuration structure for dezuxk.
type Config struct {
	Server   ServerConfig   `json:"server"`
	Storage  StorageConfig  `json:"storage"`
	Security SecurityConfig `json:"security"`
}

// Default returns a Config populated with production-safe default values.
func Default() *Config {
	useDPAPI := runtime.GOOS == "windows"
	return &Config{
		Server: ServerConfig{
			Host:   "0.0.0.0",
			Port:   8080,
			APIKey: "",
		},
		Storage: StorageConfig{
			DBFile:  "dezuxk.db",
			DataDir: ".",
		},
		Security: SecurityConfig{
			MasterKey: "dezuxk-default-secret-master-key-32b",
			UseDPAPI:  useDPAPI,
		},
	}
}

// Load loads configuration with precedence:
// 1. Defaults
// 2. Config JSON file (if path provided or exists at default path)
// 3. Environment variables (prefix DEZUXK_)
// 4. Command line arguments
func Load(args []string) (*Config, error) {
	cfg := Default()

	// Parse flags using a local FlagSet to avoid polluting global state
	fs := flag.NewFlagSet("dezuxk", flag.ContinueOnError)

	var (
		flagConfigFile = fs.String("config", "", "path to json config file")
		flagHost       = fs.String("host", "", "http listen host")
		flagPort       = fs.Int("port", 0, "http listen port")
		flagAPIKey     = fs.String("api-key", "", "master api key for bearer authentication")
		flagDBFile     = fs.String("db-file", "", "path to sqlite database file")
		flagDataDir    = fs.String("data-dir", "", "data directory")
		flagMasterKey  = fs.String("master-key", "", "master encryption key")
		flagUseDPAPI   = fs.String("use-dpapi", "", "use windows dpapi if true")
	)

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("config: parse flags: %w", err)
	}

	// 2. Load from config file if specified or default exists
	configFile := *flagConfigFile
	if configFile == "" {
		if _, err := os.Stat("dezuxk.json"); err == nil {
			configFile = "dezuxk.json"
		}
	}
	if configFile != "" {
		if data, err := os.ReadFile(configFile); err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("config: parse config file %s: %w", configFile, err)
			}
		} else if *flagConfigFile != "" {
			return nil, fmt.Errorf("config: read config file %s: %w", configFile, err)
		}
	}

	// 3. Apply Environment Variables
	applyEnv(cfg)

	// 4. Apply Command Line Flags (highest precedence)
	if *flagHost != "" {
		cfg.Server.Host = *flagHost
	}
	if *flagPort != 0 {
		cfg.Server.Port = *flagPort
	}
	if *flagAPIKey != "" {
		cfg.Server.APIKey = *flagAPIKey
	}
	if *flagDBFile != "" {
		cfg.Storage.DBFile = *flagDBFile
	}
	if *flagDataDir != "" {
		cfg.Storage.DataDir = *flagDataDir
	}
	if *flagMasterKey != "" {
		cfg.Security.MasterKey = *flagMasterKey
	}
	if *flagUseDPAPI != "" {
		cfg.Security.UseDPAPI = strings.ToLower(*flagUseDPAPI) == "true" || *flagUseDPAPI == "1"
	}

	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("DEZUXK_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("DEZUXK_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("DEZUXK_SERVER_API_KEY"); v != "" {
		cfg.Server.APIKey = v
	}
	if v := os.Getenv("DEZUXK_STORAGE_DB_FILE"); v != "" {
		cfg.Storage.DBFile = v
	}
	if v := os.Getenv("DEZUXK_STORAGE_DATA_DIR"); v != "" {
		cfg.Storage.DataDir = v
	}
	if v := os.Getenv("DEZUXK_SECURITY_MASTER_KEY"); v != "" {
		cfg.Security.MasterKey = v
	}
	if v := os.Getenv("DEZUXK_SECURITY_USE_DPAPI"); v != "" {
		cfg.Security.UseDPAPI = strings.ToLower(v) == "true" || v == "1"
	}
}
