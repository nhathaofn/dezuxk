package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"dezuxk/internal/config"
)

// TestChallenger_ConcurrencyStress_50Workers tests 50 racing goroutines
// auto-generating keys on the SAME fresh directory using a single KeyResolver.
func TestChallenger_ConcurrencyStress_50Workers(t *testing.T) {
	const concurrency = 50
	sameDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: sameDir}}
	resolver := config.NewKeyResolver()

	var wg sync.WaitGroup
	keys := make(chan string, concurrency)
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			res, err := resolver.Resolve(context.Background(), cfg)
			if err != nil {
				errs <- fmt.Errorf("worker %d resolve failed: %w", id, err)
				return
			}
			if len(res.Key) != 64 {
				errs <- fmt.Errorf("worker %d invalid key length: %d", id, len(res.Key))
				return
			}
			keys <- res.Key
		}(i)
	}

	wg.Wait()
	close(keys)
	close(errs)

	for err := range errs {
		t.Errorf("racing resolve error: %v", err)
	}

	diskContent, err := os.ReadFile(filepath.Join(sameDir, ".master.key"))
	if err != nil {
		t.Fatalf("failed to read persisted key from disk: %v", err)
	}
	diskKey := strings.TrimSpace(string(diskContent))
	if len(diskKey) != 64 {
		t.Fatalf("persisted key corrupted, len=%d: %q", len(diskKey), diskKey)
	}

	var count int
	for k := range keys {
		count++
		if k != diskKey {
			t.Errorf("inconsistent key: caller received %q but disk holds %q", k, diskKey)
		}
	}
	if count != concurrency {
		t.Errorf("expected %d resolved keys, got %d", concurrency, count)
	}
}

// TestChallenger_ConcurrencyStress_50DistinctResolvers tests 50 racing goroutines
// each instantiating its OWN KeyResolver instance on the SAME fresh directory.
func TestChallenger_ConcurrencyStress_50DistinctResolvers(t *testing.T) {
	const concurrency = 50
	sameDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: sameDir}}

	var wg sync.WaitGroup
	keys := make(chan string, concurrency)
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resolver := config.NewKeyResolver()
			res, err := resolver.Resolve(context.Background(), cfg)
			if err != nil {
				errs <- fmt.Errorf("worker %d resolve failed: %w", id, err)
				return
			}
			if len(res.Key) != 64 {
				errs <- fmt.Errorf("worker %d invalid key length: %d", id, len(res.Key))
				return
			}
			keys <- res.Key
		}(i)
	}

	wg.Wait()
	close(keys)
	close(errs)

	for err := range errs {
		t.Errorf("multi-resolver racing error: %v", err)
	}

	diskContent, err := os.ReadFile(filepath.Join(sameDir, ".master.key"))
	if err != nil {
		t.Fatalf("failed to read persisted key from disk: %v", err)
	}
	diskKey := strings.TrimSpace(string(diskContent))
	if len(diskKey) != 64 {
		t.Fatalf("persisted key corrupted, len=%d: %q", len(diskKey), diskKey)
	}

	var count int
	for k := range keys {
		count++
		if k != diskKey {
			t.Errorf("inconsistent key: caller received %q but disk holds %q", k, diskKey)
		}
	}
	if count != concurrency {
		t.Errorf("expected %d resolved keys, got %d", concurrency, count)
	}
}

// TestChallenger_ConcurrencyStress_50PackageLevelResolveKey tests 50 racing goroutines
// calling the package-level config.ResolveKey helper on the SAME fresh directory.
func TestChallenger_ConcurrencyStress_50PackageLevelResolveKey(t *testing.T) {
	const concurrency = 50
	sameDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: sameDir}}

	var wg sync.WaitGroup
	keys := make(chan string, concurrency)
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			res, err := config.ResolveKey(context.Background(), cfg)
			if err != nil {
				errs <- fmt.Errorf("worker %d resolve failed: %w", id, err)
				return
			}
			if len(res.Key) != 64 {
				errs <- fmt.Errorf("worker %d invalid key length: %d", id, len(res.Key))
				return
			}
			keys <- res.Key
		}(i)
	}

	wg.Wait()
	close(keys)
	close(errs)

	for err := range errs {
		t.Errorf("ResolveKey racing error: %v", err)
	}

	diskContent, err := os.ReadFile(filepath.Join(sameDir, ".master.key"))
	if err != nil {
		t.Fatalf("failed to read persisted key: %v", err)
	}
	diskKey := strings.TrimSpace(string(diskContent))

	var count int
	for k := range keys {
		count++
		if k != diskKey {
			t.Errorf("inconsistent key: caller received %q but disk holds %q", k, diskKey)
		}
	}
	if count != concurrency {
		t.Errorf("expected %d resolved keys, got %d", concurrency, count)
	}
}

// TestChallenger_ConcurrencyStress_MassiveMultiDirectoryContention tests 10 directories
// concurrently initialized, each by 50 racing goroutines (500 goroutines total).
func TestChallenger_ConcurrencyStress_MassiveMultiDirectoryContention(t *testing.T) {
	const dirCount = 10
	const workersPerDir = 50

	var parentWg sync.WaitGroup
	allErrors := make(chan error, dirCount*workersPerDir)

	for d := 0; d < dirCount; d++ {
		parentWg.Add(1)
		go func(dirIndex int) {
			defer parentWg.Done()
			dir := t.TempDir()
			cfg := &config.Config{Storage: config.StorageConfig{DataDir: dir}}

			var dirWg sync.WaitGroup
			dirKeys := make(chan string, workersPerDir)

			for w := 0; w < workersPerDir; w++ {
				dirWg.Add(1)
				go func(workerIndex int) {
					defer dirWg.Done()
					resolver := config.NewKeyResolver()
					res, err := resolver.Resolve(context.Background(), cfg)
					if err != nil {
						allErrors <- fmt.Errorf("dir %d worker %d: %w", dirIndex, workerIndex, err)
						return
					}
					dirKeys <- res.Key
				}(w)
			}

			dirWg.Wait()
			close(dirKeys)

			diskContent, err := os.ReadFile(filepath.Join(dir, ".master.key"))
			if err != nil {
				allErrors <- fmt.Errorf("dir %d read disk key: %w", dirIndex, err)
				return
			}
			expectedKey := strings.TrimSpace(string(diskContent))

			for k := range dirKeys {
				if k != expectedKey {
					allErrors <- fmt.Errorf("dir %d inconsistent key %q != disk %q", dirIndex, k, expectedKey)
				}
			}
		}(d)
	}

	parentWg.Wait()
	close(allErrors)

	for err := range allErrors {
		t.Errorf("massive concurrency error: %v", err)
	}
}

// TestChallenger_CLIKeyFilePrecedence_OverJSONAndStorageConfig tests the CLI flag precedence
// where --master-key-file takes precedence over JSON config security.master_key.
func TestChallenger_CLIKeyFilePrecedence_OverJSONAndStorageConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "dezuxk.json")
	fileContent := `{
		"security": {
			"master_key": "stale-json-master-key",
			"master_key_file": "stale-json-master-key-file.txt"
		},
		"storage": {
			"data_dir": "` + strings.ReplaceAll(tmpDir, `\`, `\\`) + `"
		}
	}`
	if err := os.WriteFile(configPath, []byte(fileContent), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	keyFilePath := filepath.Join(tmpDir, "authoritative-cli.key")
	expectedAuthoritativeKey := "3232323232323232323232323232323232323232323232323232323232323232"
	if err := os.WriteFile(keyFilePath, []byte(expectedAuthoritativeKey+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write authoritative key file: %v", err)
	}

	args := []string{
		"--config=" + configPath,
		"--master-key-file=" + keyFilePath,
	}

	cfg, err := config.Load(args)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	// In config struct:
	if cfg.Security.MasterKey != "" {
		t.Errorf("cfg.Security.MasterKey should be empty (overridden by --master-key-file), got %q", cfg.Security.MasterKey)
	}
	if cfg.Security.MasterKeyFile != keyFilePath {
		t.Errorf("cfg.Security.MasterKeyFile expected %q, got %q", keyFilePath, cfg.Security.MasterKeyFile)
	}

	// Resolve the key using KeyResolver
	resolver := config.NewKeyResolver()
	res, err := resolver.Resolve(context.Background(), cfg, cfg.ResolverFlags)
	if err != nil {
		t.Fatalf("resolver.Resolve failed: %v", err)
	}

	if res.Key != expectedAuthoritativeKey {
		t.Errorf("expected resolved key %q, got %q", expectedAuthoritativeKey, res.Key)
	}
	if res.Source != config.SourceKeyFile {
		t.Errorf("expected source SourceKeyFile, got %q", res.Source)
	}
	if res.FilePath != keyFilePath {
		t.Errorf("expected FilePath %q, got %q", keyFilePath, res.FilePath)
	}
}

// TestChallenger_CLIKey_Overrides_CLIKeyFile tests that when BOTH --master-key and --master-key-file
// are supplied on the CLI, --master-key takes precedence over --master-key-file.
func TestChallenger_CLIKey_Overrides_CLIKeyFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyFilePath := filepath.Join(tmpDir, "secondary.key")
	if err := os.WriteFile(keyFilePath, []byte("secondary-file-key\n"), 0o600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	args := []string{
		"--master-key=primary-cli-key",
		"--master-key-file=" + keyFilePath,
	}

	cfg, err := config.Load(args)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	resolver := config.NewKeyResolver()
	res, err := resolver.Resolve(context.Background(), cfg, cfg.ResolverFlags)
	if err != nil {
		t.Fatalf("resolver.Resolve failed: %v", err)
	}

	if res.Key != "primary-cli-key" {
		t.Errorf("expected 'primary-cli-key', got %q", res.Key)
	}
	if res.Source != config.SourceCLIFlag {
		t.Errorf("expected SourceCLIFlag, got %q", res.Source)
	}
}
