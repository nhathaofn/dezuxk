package config_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if cfg.Storage.DataDir != "." {
		t.Errorf("expected default data_dir '.', got %s", cfg.Storage.DataDir)
	}
	// ZERO-HARDCODE: MasterKey must be empty in default config (no hardcoded secret)
	if cfg.Security.MasterKey != "" {
		t.Errorf("expected default master key to be empty (zero-hardcode), got %q", cfg.Security.MasterKey)
	}
	if cfg.Security.MasterKeyFile != "" {
		t.Errorf("expected default master_key_file to be empty, got %q", cfg.Security.MasterKeyFile)
	}
	// Manifests directory default
	if cfg.ManifestsDir != "manifests" {
		t.Errorf("expected default manifests_dir 'manifests', got %q", cfg.ManifestsDir)
	}
	if cfg.GetManifestsDir() != "manifests" {
		t.Errorf("expected GetManifestsDir() 'manifests', got %q", cfg.GetManifestsDir())
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
		"--master-key-file=/tmp/custom.key",
		"--manifests-dir=/tmp/custom-manifests",
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
	if cfg.Security.MasterKeyFile != "/tmp/custom.key" {
		t.Errorf("expected master_key_file /tmp/custom.key, got %s", cfg.Security.MasterKeyFile)
	}
	if cfg.ManifestsDir != "/tmp/custom-manifests" {
		t.Errorf("expected manifests_dir /tmp/custom-manifests, got %s", cfg.ManifestsDir)
	}
	if cfg.GetManifestsDir() != "/tmp/custom-manifests" {
		t.Errorf("expected GetManifestsDir() /tmp/custom-manifests, got %s", cfg.GetManifestsDir())
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
	t.Setenv("DEZUXK_STORAGE_DATA_DIR", "env-data")
	t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "env-master-key")
	t.Setenv("DEZUXK_SECURITY_MASTER_KEY_FILE", "env-key.txt")
	t.Setenv("DEZUXK_MANIFESTS_DIR", "env-manifests")
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
	if cfg.Storage.DataDir != "env-data" {
		t.Errorf("expected data_dir env-data, got %s", cfg.Storage.DataDir)
	}
	if cfg.Security.MasterKey != "env-master-key" {
		t.Errorf("expected master_key env-master-key, got %s", cfg.Security.MasterKey)
	}
	if cfg.Security.MasterKeyFile != "env-key.txt" {
		t.Errorf("expected master_key_file env-key.txt, got %s", cfg.Security.MasterKeyFile)
	}
	if cfg.ManifestsDir != "env-manifests" {
		t.Errorf("expected manifests_dir env-manifests, got %s", cfg.ManifestsDir)
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
		"storage": {"db_file": "file.db", "data_dir": "file-data"},
		"manifests_dir": "file-manifests",
		"security": {"master_key": "file-master-key", "master_key_file": "file-key.txt"}
	}`
	if err := os.WriteFile(configPath, []byte(fileContent), 0o600); err != nil {
		t.Fatalf("write config file failed: %v", err)
	}

	// Env sets port to 7171, but flag sets port to 7272.
	// Flag should win for port, File should supply db_file.
	t.Setenv("DEZUXK_SERVER_PORT", "7171")
	t.Setenv("DEZUXK_MANIFESTS_DIR", "env-manifests")

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
	if cfg.ManifestsDir != "env-manifests" {
		t.Errorf("expected env manifests_dir 'env-manifests' to win over file 'file-manifests', got %s", cfg.ManifestsDir)
	}
	if cfg.Server.APIKey != "file-key" {
		t.Errorf("expected file api_key file-key, got %s", cfg.Server.APIKey)
	}
	if cfg.Storage.DBFile != "file.db" {
		t.Errorf("expected file db_file file.db, got %s", cfg.Storage.DBFile)
	}
	if cfg.Security.MasterKey != "file-master-key" {
		t.Errorf("expected file master_key file-master-key, got %s", cfg.Security.MasterKey)
	}
	if cfg.Security.MasterKeyFile != "file-key.txt" {
		t.Errorf("expected file master_key_file file-key.txt, got %s", cfg.Security.MasterKeyFile)
	}
}

func TestNoHardcodedMasterKeyInSource(t *testing.T) {
	// Dynamically assemble the old forbidden key string to avoid self-match
	forbiddenParts := []string{"dezuxk", "default", "secret", "master", "key", "32b"}
	forbiddenKey := strings.Join(forbiddenParts, "-")

	// Find repository root traversing up until go.mod is found
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := dir
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatalf("could not locate go.mod from %s", dir)
		}
		repoRoot = parent
	}

	checkedFiles := 0
	err = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".json", ".yaml", ".yml", ".toml", ".env":
			// Skip test files themselves to avoid matching test definitions
			if strings.HasSuffix(info.Name(), "_test.go") {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			checkedFiles++
			if strings.Contains(string(content), forbiddenKey) {
				t.Errorf("CRITICAL SECURITY VIOLATION: forbidden hardcoded key found in %s", path)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("source tree scan failed: %v", err)
	}
	if checkedFiles == 0 {
		t.Fatalf("expected to scan source files, found 0")
	}
}

func TestLoad_CLIKeyFileOverridesConfigJSONMasterKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "dezuxk.json")
	fileContent := `{
		"security": {
			"master_key": "stale-json-master-key"
		}
	}`
	if err := os.WriteFile(configPath, []byte(fileContent), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	keyFilePath := filepath.Join(tmpDir, "authoritative.key")
	if err := os.WriteFile(keyFilePath, []byte("authoritative-key-from-cli-file\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	args := []string{
		"--config=" + configPath,
		"--master-key-file=" + keyFilePath,
	}

	cfg, err := config.Load(args)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.Security.MasterKey != "" {
		t.Errorf("expected cfg.Security.MasterKey to be cleared by CLI --master-key-file, got %q", cfg.Security.MasterKey)
	}
	if cfg.Security.MasterKeyFile != keyFilePath {
		t.Errorf("expected cfg.Security.MasterKeyFile %q, got %q", keyFilePath, cfg.Security.MasterKeyFile)
	}

	// Downstream KeyResolver resolution must resolve from the key file
	res, err := config.ResolveKey(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ResolveKey failed: %v", err)
	}
	if res.Key != "authoritative-key-from-cli-file" {
		t.Errorf("expected resolved key 'authoritative-key-from-cli-file', got %q", res.Key)
	}
	if res.Source != config.SourceKeyFile {
		t.Errorf("expected source %q, got %q", config.SourceKeyFile, res.Source)
	}
}
