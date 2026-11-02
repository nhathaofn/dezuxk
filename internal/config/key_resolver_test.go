package config_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"dezuxk/internal/config"
	"dezuxk/internal/security"
)

func TestKeyResolver_PrecedenceHierarchy(t *testing.T) {
	t.Run("Tier 1: CLI flag overrides ENV, JSON, File, and Auto-gen", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyFile := filepath.Join(tmpDir, "file.key")
		if err := os.WriteFile(keyFile, []byte("file-secret"), 0o600); err != nil {
			t.Fatalf("write key file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, ".master.key"), []byte("persisted-secret"), 0o600); err != nil {
			t.Fatalf("write persisted key: %v", err)
		}

		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "env-secret")

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKey:     "json-secret",
				MasterKeyFile: keyFile,
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKey:     "cli-secret",
			MasterKeyFile: keyFile,
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if res.Key != "cli-secret" {
			t.Errorf("expected key 'cli-secret', got %q", res.Key)
		}
		if res.Source != config.SourceCLIFlag {
			t.Errorf("expected source %q, got %q", config.SourceCLIFlag, res.Source)
		}
	})

	t.Run("Tier 2: ENV overrides JSON, File, and Auto-gen when CLI is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyFile := filepath.Join(tmpDir, "file.key")
		_ = os.WriteFile(keyFile, []byte("file-secret"), 0o600)
		_ = os.WriteFile(filepath.Join(tmpDir, ".master.key"), []byte("persisted-secret"), 0o600)

		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "env-secret")

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKey:     "json-secret",
				MasterKeyFile: keyFile,
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKey:     "",
			MasterKeyFile: keyFile,
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if res.Key != "env-secret" {
			t.Errorf("expected key 'env-secret', got %q", res.Key)
		}
		if res.Source != config.SourceEnv {
			t.Errorf("expected source %q, got %q", config.SourceEnv, res.Source)
		}
	})

	t.Run("Tier 3: Config JSON overrides File and Auto-gen when CLI and ENV empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyFile := filepath.Join(tmpDir, "file.key")
		_ = os.WriteFile(keyFile, []byte("file-secret"), 0o600)
		_ = os.WriteFile(filepath.Join(tmpDir, ".master.key"), []byte("persisted-secret"), 0o600)

		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "")

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKey:     "json-secret",
				MasterKeyFile: keyFile,
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKey:     "",
			MasterKeyFile: keyFile,
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if res.Key != "json-secret" {
			t.Errorf("expected key 'json-secret', got %q", res.Key)
		}
		if res.Source != config.SourceConfigFile {
			t.Errorf("expected source %q, got %q", config.SourceConfigFile, res.Source)
		}
	})

	t.Run("Tier 4: Key file overrides Auto-gen when CLI, ENV, and JSON empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyFile := filepath.Join(tmpDir, "custom.key")
		if err := os.WriteFile(keyFile, []byte("  file-explicit-secret  \r\n"), 0o600); err != nil {
			t.Fatalf("write key file: %v", err)
		}
		_ = os.WriteFile(filepath.Join(tmpDir, ".master.key"), []byte("persisted-secret"), 0o600)

		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "")

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKey:     "",
				MasterKeyFile: "",
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKeyFile: keyFile,
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if res.Key != "file-explicit-secret" {
			t.Errorf("expected trimmed key 'file-explicit-secret', got %q", res.Key)
		}
		if res.Source != config.SourceKeyFile {
			t.Errorf("expected source %q, got %q", config.SourceKeyFile, res.Source)
		}
		if res.FilePath != keyFile {
			t.Errorf("expected file path %q, got %q", keyFile, res.FilePath)
		}
	})

	t.Run("Tier 5: Auto-generates key in DataDir when all above are empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "")
		t.Setenv("DEZUXK_SECURITY_MASTER_KEY_FILE", "")

		cfg := &config.Config{
			Storage:  config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}

		if res.Source != config.SourceAutoGeneratedPersistent {
			t.Errorf("expected source %q, got %q", config.SourceAutoGeneratedPersistent, res.Source)
		}

		// Verify file was written
		expectedPath := filepath.Join(tmpDir, ".master.key")
		content, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("failed to read generated key file %s: %v", expectedPath, err)
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed != res.Key {
			t.Errorf("persisted file content %q does not match resolved key %q", trimmed, res.Key)
		}
	})

	t.Run("Tier 5: Reads existing .master.key in DataDir when present", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", "")
		t.Setenv("DEZUXK_SECURITY_MASTER_KEY_FILE", "")

		existingKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		keyPath := filepath.Join(tmpDir, ".master.key")
		if err := os.WriteFile(keyPath, []byte(existingKey+"\n"), 0o600); err != nil {
			t.Fatalf("write master.key: %v", err)
		}

		cfg := &config.Config{
			Storage:  config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}

		if res.Key != existingKey {
			t.Errorf("expected existing key %q, got %q", existingKey, res.Key)
		}
		if res.Source != config.SourceAutoGeneratedPersistent {
			t.Errorf("expected source %q, got %q", config.SourceAutoGeneratedPersistent, res.Source)
		}
	})
}

func TestKeyResolver_AutoGeneration_Properties(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	cfg1 := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir1}}
	cfg2 := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir2}}

	resolver := config.NewKeyResolver()

	res1, err := resolver.Resolve(context.Background(), cfg1)
	if err != nil {
		t.Fatalf("Resolve 1 failed: %v", err)
	}

	res2, err := resolver.Resolve(context.Background(), cfg2)
	if err != nil {
		t.Fatalf("Resolve 2 failed: %v", err)
	}

	// 1. Length check: 32 bytes hex-encoded = 64 hex characters
	if len(res1.Key) != 64 {
		t.Errorf("expected 64 hex characters, got length %d: %q", len(res1.Key), res1.Key)
	}

	// 2. Decode check: hex decoding must yield exactly 32 raw bytes
	rawBytes, err := hex.DecodeString(res1.Key)
	if err != nil {
		t.Fatalf("key is not valid hex: %v", err)
	}
	if len(rawBytes) != 32 {
		t.Errorf("expected 32 decoded bytes, got %d", len(rawBytes))
	}

	// 3. Entropy check: separate auto-generations must be distinct
	if res1.Key == res2.Key {
		t.Fatalf("entropy failure: two auto-generated keys are identical: %s", res1.Key)
	}
}

func TestKeyResolver_AutoGeneration_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
	resolver := config.NewKeyResolver()

	// Initial resolution creates the file
	res1, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first Resolve failed: %v", err)
	}

	keyPath := filepath.Join(tmpDir, ".master.key")
	stat1, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat generated key file failed: %v", err)
	}

	// Second resolution must reuse existing file without overwriting
	res2, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second Resolve failed: %v", err)
	}

	if res1.Key != res2.Key {
		t.Fatalf("persistence failed: first key %s != second key %s", res1.Key, res2.Key)
	}

	stat2, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("second stat failed: %v", err)
	}

	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("persisted key file was overwritten on subsequent resolve call")
	}
}

func TestKeyResolver_AutoGeneration_Permissions(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
	resolver := config.NewKeyResolver()

	_, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	keyPath := filepath.Join(tmpDir, ".master.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("expected restricted file permissions 0600, got %#o", perm)
		}
	} else {
		// On Windows, verify file is created and readable
		if info.Size() == 0 {
			t.Errorf("expected non-empty key file on Windows")
		}
	}
}

func TestKeyResolver_AutoGeneration_NestedDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDataDir := filepath.Join(tmpDir, "nested", "storage", "path")

	cfg := &config.Config{Storage: config.StorageConfig{DataDir: nestedDataDir}}
	resolver := config.NewKeyResolver()

	res, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Resolve with non-existent nested data dir failed: %v", err)
	}

	keyPath := filepath.Join(nestedDataDir, ".master.key")
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected key file to exist at %s: %v", keyPath, err)
	}
	if len(res.Key) != 64 {
		t.Errorf("expected 64 hex characters, got %d", len(res.Key))
	}
}

func TestKeyResolver_KeyFile_WhitespaceTrimming(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "spaced.key")
	content := " \t\r\n secret-key-with-whitespace \n\r\t "
	if err := os.WriteFile(keyFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
	resolver := config.NewKeyResolver()

	res, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{MasterKeyFile: keyFile})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if res.Key != "secret-key-with-whitespace" {
		t.Errorf("expected trimmed key 'secret-key-with-whitespace', got %q", res.Key)
	}
}

func TestKeyResolver_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	resolver := config.NewKeyResolver()

	t.Run("nil config returns error", func(t *testing.T) {
		_, err := resolver.Resolve(context.Background(), nil)
		if err == nil {
			t.Errorf("expected error for nil config, got nil")
		}
	})

	t.Run("nonexistent key file returns error", func(t *testing.T) {
		cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
		_, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKeyFile: filepath.Join(tmpDir, "missing.key"),
		})
		if err == nil {
			t.Errorf("expected error for nonexistent key file, got nil")
		}
	})

	t.Run("empty key file returns error", func(t *testing.T) {
		emptyKeyFile := filepath.Join(tmpDir, "empty.key")
		if err := os.WriteFile(emptyKeyFile, []byte(" \r\n\t "), 0o600); err != nil {
			t.Fatalf("write empty file: %v", err)
		}

		cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
		_, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKeyFile: emptyKeyFile,
		})
		if err == nil {
			t.Errorf("expected error for empty key file, got nil")
		}
	})

	t.Run("key file pointing to directory returns error", func(t *testing.T) {
		dirKeyFile := filepath.Join(tmpDir, "a_directory")
		if err := os.Mkdir(dirKeyFile, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
		_, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKeyFile: dirKeyFile,
		})
		if err == nil {
			t.Errorf("expected error when key file is directory, got nil")
		}
	})

	t.Run("uncreatable DataDir returns error", func(t *testing.T) {
		blockedFile := filepath.Join(tmpDir, "blocked_file")
		if err := os.WriteFile(blockedFile, []byte("data"), 0o600); err != nil {
			t.Fatalf("write blocked file: %v", err)
		}
		// Point DataDir to a path inside a regular file, making os.MkdirAll fail
		invalidDataDir := filepath.Join(blockedFile, "sub_directory")

		cfg := &config.Config{Storage: config.StorageConfig{DataDir: invalidDataDir}}
		_, err := resolver.Resolve(context.Background(), cfg)
		if err == nil {
			t.Errorf("expected error when DataDir cannot be created, got nil")
		}
	})

	t.Run("persistent key path is directory returns error", func(t *testing.T) {
		dirAsKey := filepath.Join(tmpDir, "dir_as_key")
		if err := os.Mkdir(dirAsKey, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Create .master.key as a directory
		if err := os.Mkdir(filepath.Join(dirAsKey, ".master.key"), 0o700); err != nil {
			t.Fatalf("mkdir .master.key: %v", err)
		}

		cfg := &config.Config{Storage: config.StorageConfig{DataDir: dirAsKey}}
		_, err := resolver.Resolve(context.Background(), cfg)
		if err == nil {
			t.Errorf("expected error when .master.key is a directory, got nil")
		}
	})
}

func TestKeyResolver_SourceAudit(t *testing.T) {
	sources := []config.ResolutionSource{
		config.SourceCLIFlag,
		config.SourceCLI,
		config.SourceEnv,
		config.SourceConfigFile,
		config.SourceKeyFile,
		config.SourceAutoGeneratedPersistent,
		config.SourceAutoGenerated,
	}

	expectedStrings := []string{
		"cli_flag",
		"cli_flag",
		"env",
		"config_file",
		"key_file",
		"auto_generated_persistent",
		"auto_generated_persistent",
	}

	for i, s := range sources {
		if string(s) != expectedStrings[i] {
			t.Errorf("expected source %d string %q, got %q", i, expectedStrings[i], string(s))
		}
	}
}

func TestKeyResolver_IntegrationWithAdaptiveCipher(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
	resolver := config.NewKeyResolver()

	res, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	cipher, err := security.NewAdaptiveCipher(res.Key, false)
	if err != nil {
		t.Fatalf("failed to create AdaptiveCipher with auto-generated key: %v", err)
	}

	plaintext := []byte("confidential-upstream-tokens-and-session-cookies")
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	decrypted, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted payload mismatch:\nWant: %s\nGot:  %s", string(plaintext), string(decrypted))
	}
}

func TestKeyResolver_ResolveWithOptions(t *testing.T) {
	resolver := config.NewKeyResolver()

	t.Run("ResolveWithOptions CLI", func(t *testing.T) {
		res, err := resolver.ResolveWithOptions(context.Background(), config.ResolveOptions{
			CLIMasterKey: "opt-cli-key",
		})
		if err != nil {
			t.Fatalf("ResolveWithOptions failed: %v", err)
		}
		if res.Key != "opt-cli-key" || res.Source != config.SourceCLIFlag {
			t.Errorf("expected opt-cli-key / cli_flag, got %q / %q", res.Key, res.Source)
		}
	})

	t.Run("ResolveWithOptions ENV", func(t *testing.T) {
		res, err := resolver.ResolveWithOptions(context.Background(), config.ResolveOptions{
			EnvMasterKey: "opt-env-key",
		})
		if err != nil {
			t.Fatalf("ResolveWithOptions failed: %v", err)
		}
		if res.Key != "opt-env-key" || res.Source != config.SourceEnv {
			t.Errorf("expected opt-env-key / env, got %q / %q", res.Key, res.Source)
		}
	})

	t.Run("ResolveWithOptions ConfigMasterKey", func(t *testing.T) {
		res, err := resolver.ResolveWithOptions(context.Background(), config.ResolveOptions{
			ConfigMasterKey: "opt-config-key",
		})
		if err != nil {
			t.Fatalf("ResolveWithOptions failed: %v", err)
		}
		if res.Key != "opt-config-key" || res.Source != config.SourceConfigFile {
			t.Errorf("expected opt-config-key / config_file, got %q / %q", res.Key, res.Source)
		}
	})
}

func TestResolveKey_Helper(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{DataDir: tmpDir},
		Security: config.SecurityConfig{
			MasterKey: "helper-test-key",
		},
	}

	res, err := config.ResolveKey(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ResolveKey helper failed: %v", err)
	}
	if res.Key != "helper-test-key" {
		t.Errorf("expected helper-test-key, got %q", res.Key)
	}
	if res.Source != config.SourceConfigFile {
		t.Errorf("expected SourceConfigFile, got %q", res.Source)
	}
}

func TestKeyResolver_Concurrency_MultiResolverRaceOnSameDirectory(t *testing.T) {
	const concurrency = 20
	sameDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: sameDir}}

	var wg sync.WaitGroup
	keys := make(chan string, concurrency)
	errs := make(chan error, concurrency)

	// In this test, each goroutine creates its own independent NewKeyResolver() instance
	// to test canonical path locking across distinct resolver objects.
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resolver := config.NewKeyResolver()
			res, err := resolver.Resolve(context.Background(), cfg)
			if err != nil {
				errs <- err
				return
			}
			if len(res.Key) != 64 {
				errs <- os.ErrInvalid
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
		t.Fatalf("failed to read persisted key: %v", err)
	}
	diskKey := strings.TrimSpace(string(diskContent))

	for k := range keys {
		if k != diskKey {
			t.Errorf("inconsistent key: caller received %q but disk holds %q", k, diskKey)
		}
	}
}

func TestKeyResolver_CacheDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{DataDir: tmpDir}}
	resolver := config.NewKeyResolver()

	res1, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first Resolve failed: %v", err)
	}

	// Corrupt file on disk or remove it to test that subsequent in-memory queries return cached key
	keyPath := filepath.Join(tmpDir, ".master.key")
	if err := os.Remove(keyPath); err != nil {
		t.Fatalf("failed to remove key file: %v", err)
	}

	// Second resolve should hit in-memory cache and return identical key
	res2, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second Resolve (cache hit) failed: %v", err)
	}

	if res1.Key != res2.Key {
		t.Errorf("cache deduplication mismatch: %q != %q", res1.Key, res2.Key)
	}
}
