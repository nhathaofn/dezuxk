package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"dezuxk/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.APIKey != "" {
		t.Errorf("expected default api key to be empty, got %s", cfg.Server.APIKey)
	}
	if cfg.Storage.DBFile != "dezuxk.db" {
		t.Errorf("expected default db_file dezuxk.db, got %s", cfg.Storage.DBFile)
	}
	if cfg.Security.MasterKey == "" {
		t.Errorf("expected non-empty default master key")
	}
	expectedDPAPI := runtime.GOOS == "windows"
	if cfg.Security.UseDPAPI != expectedDPAPI {
		t.Errorf("expected default use_dpapi %v on %s, got %v", expectedDPAPI, runtime.GOOS, cfg.Security.UseDPAPI)
	}
}

func TestLoadWithFlags(t *testing.T) {
	args := []string{
		"--host=127.0.0.1",
		"--port=9090",
		"--api-key=test-secret-key",
		"--db-file=custom.db",
		"--data-dir=/tmp/data",
		"--master-key=custom-master-key",
		"--use-dpapi=false",
	}

	cfg, err := config.Load(args)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.APIKey != "test-secret-key" {
		t.Errorf("expected api_key test-secret-key, got %s", cfg.Server.APIKey)
	}
	if cfg.Storage.DBFile != "custom.db" {
		t.Errorf("expected db_file custom.db, got %s", cfg.Storage.DBFile)
	}
	if cfg.Storage.DataDir != "/tmp/data" {
		t.Errorf("expected data_dir /tmp/data, got %s", cfg.Storage.DataDir)
	}
	if cfg.Security.MasterKey != "custom-master-key" {
		t.Errorf("expected master_key custom-master-key, got %s", cfg.Security.MasterKey)
	}
	if cfg.Security.UseDPAPI != false {
		t.Errorf("expected use_dpapi false, got %v", cfg.Security.UseDPAPI)
	}
}

func TestLoadWithEnvironment(t *testing.T) {
	t.Setenv("DEZUXK_SERVER_HOST", "10.0.0.1")
	t.Setenv("DEZUXK_SERVER_PORT", "9999")
	t.Setenv("DEZUXK_SERVER_API_KEY", "env-api-key")
	t.Setenv("DEZUXK_STORAGE_DB_FILE", "env.db")
	t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "env-master-key")
	t.Setenv("DEZUXK_SECURITY_USE_DPAPI", "true")

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.APIKey != "env-api-key" {
		t.Errorf("expected api_key env-api-key, got %s", cfg.Server.APIKey)
	}
	if cfg.Storage.DBFile != "env.db" {
		t.Errorf("expected db_file env.db, got %s", cfg.Storage.DBFile)
	}
	if cfg.Security.MasterKey != "env-master-key" {
		t.Errorf("expected master_key env-master-key, got %s", cfg.Security.MasterKey)
	}
	if cfg.Security.UseDPAPI != true {
		t.Errorf("expected use_dpapi true, got %v", cfg.Security.UseDPAPI)
	}
}

func TestLoadWithFileAndFlagPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.json")
	fileContent := `{
		"server": {"port": 7070, "api_key": "file-key"},
		"storage": {"db_file": "file.db"}
	}`
	if err := os.WriteFile(configPath, []byte(fileContent), 0o600); err != nil {
		t.Fatalf("write config file failed: %v", err)
	}

	// Env sets port to 7171, but flag sets port to 7272.
	// Flag should win for port, File should supply db_file.
	t.Setenv("DEZUXK_SERVER_PORT", "7171")

	args := []string{
		"--config=" + configPath,
		"--port=7272",
	}

	cfg, err := config.Load(args)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.Server.Port != 7272 {
		t.Errorf("expected flag port 7272 to win over env 7171 and file 7070, got %d", cfg.Server.Port)
	}
	if cfg.Server.APIKey != "file-key" {
		t.Errorf("expected file api_key file-key, got %s", cfg.Server.APIKey)
	}
	if cfg.Storage.DBFile != "file.db" {
		t.Errorf("expected file db_file file.db, got %s", cfg.Storage.DBFile)
	}
}
