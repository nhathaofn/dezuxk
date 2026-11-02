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

// ProvidersConfig defines upstream provider manifest loading settings.
type ProvidersConfig struct {
	ManifestsDir string `json:"manifests_dir"`
}

// StorageConfig defines database and persistence settings.
type StorageConfig struct {
	DBFile       string `json:"db_file"`
	DataDir      string `json:"data_dir"`
	ManifestsDir string `json:"manifests_dir,omitempty"`
}

// SecurityConfig defines encryption and secret protection settings.
type SecurityConfig struct {
	MasterKey     string `json:"master_key"`
	MasterKeyFile string `json:"master_key_file,omitempty"`
	UseDPAPI      bool   `json:"use_dpapi"`
}

// Config is the root configuration structure for dezuxk.
type Config struct {
	Server       ServerConfig    `json:"server"`
	Storage      StorageConfig   `json:"storage"`
	Security     SecurityConfig  `json:"security"`
	Providers    ProvidersConfig `json:"providers"`
	ManifestsDir string          `json:"manifests_dir"`

	// ResolverFlags stores parsed CLI flags for downstream KeyResolver
	ResolverFlags KeyResolverFlags `json:"-"`
}

// GetManifestsDir returns the directory containing provider manifests.
func (c *Config) GetManifestsDir() string {
	if c == nil {
		return "manifests"
	}
	if c.ManifestsDir != "" {
		return c.ManifestsDir
	}
	if c.Providers.ManifestsDir != "" {
		return c.Providers.ManifestsDir
	}
	if c.Storage.ManifestsDir != "" {
		return c.Storage.ManifestsDir
	}
	return "manifests"
}

// Default returns a Config populated with production-safe default values.
// Notice: MasterKey is intentionally empty to guarantee zero hardcoded secrets.
func Default() *Config {
	useDPAPI := runtime.GOOS == "windows"
	return &Config{
		Server: ServerConfig{
			Host:   "0.0.0.0",
			Port:   8080,
			APIKey: "",
		},
		Storage: StorageConfig{
			DBFile:       "dezuxk.db",
			DataDir:      ".",
			ManifestsDir: "manifests",
		},
		Security: SecurityConfig{
			MasterKey:     "",
			MasterKeyFile: "",
			UseDPAPI:      useDPAPI,
		},
		Providers: ProvidersConfig{
			ManifestsDir: "manifests",
		},
		ManifestsDir: "manifests",
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
		flagConfigFile    = fs.String("config", "", "path to json config file")
		flagHost          = fs.String("host", "", "http listen host")
		flagPort          = fs.Int("port", 0, "http listen port")
		flagAPIKey        = fs.String("api-key", "", "master api key for bearer authentication")
		flagDBFile        = fs.String("db-file", "", "path to sqlite database file")
		flagDataDir       = fs.String("data-dir", "", "data directory")
		flagMasterKey     = fs.String("master-key", "", "master encryption key")
		flagMasterKeyFile = fs.String("master-key-file", "", "path to master encryption key file")
		flagManifestsDir  = fs.String("manifests-dir", "", "path to provider manifests directory")
		flagUseDPAPI      = fs.String("use-dpapi", "", "use windows dpapi if true")
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
			// Synchronize manifests dir if either field was unmarshaled
			if cfg.ManifestsDir != "manifests" {
				cfg.Providers.ManifestsDir = cfg.ManifestsDir
				cfg.Storage.ManifestsDir = cfg.ManifestsDir
			} else if cfg.Providers.ManifestsDir != "manifests" {
				cfg.ManifestsDir = cfg.Providers.ManifestsDir
				cfg.Storage.ManifestsDir = cfg.Providers.ManifestsDir
			} else if cfg.Storage.ManifestsDir != "manifests" && cfg.Storage.ManifestsDir != "" {
				cfg.ManifestsDir = cfg.Storage.ManifestsDir
				cfg.Providers.ManifestsDir = cfg.Storage.ManifestsDir
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
	} else if *flagMasterKeyFile != "" {
		// CLI key file overrides JSON config and ENV master keys
		cfg.Security.MasterKey = ""
	}
	if *flagMasterKeyFile != "" {
		cfg.Security.MasterKeyFile = *flagMasterKeyFile
	}
	if *flagManifestsDir != "" {
		cfg.ManifestsDir = *flagManifestsDir
		cfg.Providers.ManifestsDir = *flagManifestsDir
		cfg.Storage.ManifestsDir = *flagManifestsDir
	}
	if *flagUseDPAPI != "" {
		cfg.Security.UseDPAPI = strings.ToLower(*flagUseDPAPI) == "true" || *flagUseDPAPI == "1"
	}

	// Record parsed key flags for KeyResolver
	cfg.ResolverFlags = KeyResolverFlags{
		MasterKey:     *flagMasterKey,
		MasterKeyFile: *flagMasterKeyFile,
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
	if v := os.Getenv("DEZUXK_SECURITY_MASTER_KEY_FILE"); v != "" {
		cfg.Security.MasterKeyFile = v
	}
	if v := os.Getenv("DEZUXK_MANIFESTS_DIR"); v != "" {
		cfg.ManifestsDir = v
		cfg.Providers.ManifestsDir = v
		cfg.Storage.ManifestsDir = v
	} else if v := os.Getenv("DEZUXK_PROVIDERS_MANIFESTS_DIR"); v != "" {
		cfg.ManifestsDir = v
		cfg.Providers.ManifestsDir = v
		cfg.Storage.ManifestsDir = v
	}
	if v := os.Getenv("DEZUXK_SECURITY_USE_DPAPI"); v != "" {
		cfg.Security.UseDPAPI = strings.ToLower(v) == "true" || v == "1"
	}
}
