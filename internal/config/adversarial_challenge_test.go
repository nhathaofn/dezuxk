package config_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"dezuxk/internal/config"
	"dezuxk/internal/security"
)

// TestKeyResolver_Adversarial_LockedFile tests behavior when a master key file is locked exclusively.
func TestKeyResolver_Adversarial_LockedFile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows exclusive file locking test")
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "exclusive_locked.key")

	// Create file first
	if err := os.WriteFile(keyPath, []byte("super-secret-locked-key"), 0o600); err != nil {
		t.Fatalf("failed to write test key file: %v", err)
	}

	// Open file with exclusive lock on Windows (shareMode = 0 prevents any other process/handle from reading)
	pathPtr, err := syscall.UTF16PtrFromString(keyPath)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}

	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // 0 = FILE_SHARE_NONE (exclusive lock)
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("failed to acquire exclusive lock: %v", err)
	}
	defer syscall.CloseHandle(handle)

	// Attempt to resolve key with the locked key file specified
	cfg := &config.Config{
		Storage: config.StorageConfig{DataDir: tmpDir},
		Security: config.SecurityConfig{
			MasterKeyFile: keyPath,
		},
	}

	resolver := config.NewKeyResolver()
	res, err := resolver.Resolve(context.Background(), cfg)

	// Verification:
	// 1. MUST return an error because the key file could not be read
	// 2. MUST NOT fall back to generating a new key (which would lead to data loss)
	if err == nil {
		t.Fatalf("expected error reading exclusively locked file, but got nil; res=%+v", res)
	}

	if res != nil {
		t.Fatalf("expected nil result on locked file failure, got %+v", res)
	}

	// Error must indicate read failure
	if !strings.Contains(err.Error(), "read key file") {
		t.Errorf("expected error message to contain 'read key file', got: %v", err)
	}
}

// TestKeyResolver_Adversarial_UTF8BOM tests key resolution when the key file contains a UTF-8 BOM.
func TestKeyResolver_Adversarial_UTF8BOM(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Key file with UTF-8 BOM and alphanumeric secret", func(t *testing.T) {
		keyPath := filepath.Join(tmpDir, "bom_secret.key")
		rawSecret := "my-secret-key-12345"
		// Prepend UTF-8 BOM: 0xEF, 0xBB, 0xBF
		bomData := append([]byte{0xEF, 0xBB, 0xBF}, []byte(rawSecret)...)
		bomData = append(bomData, []byte("\r\n")...)

		if err := os.WriteFile(keyPath, bomData, 0o600); err != nil {
			t.Fatalf("write BOM file failed: %v", err)
		}

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKeyFile: keyPath,
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg)
		if err != nil {
			t.Fatalf("unexpected error resolving key with UTF-8 BOM: %v", err)
		}

		if res.Key == "" {
			t.Fatalf("expected non-empty key")
		}

		// Verify cipher roundtrip with the resolved key (even if it preserves the BOM rune U+FEFF)
		cipher, err := security.NewAdaptiveCipher(res.Key, false)
		if err != nil {
			t.Fatalf("NewAdaptiveCipher failed with BOM key: %v", err)
		}

		plaintext := []byte("payload encrypted with BOM-loaded key")
		ct, err := cipher.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("cipher.Encrypt failed: %v", err)
		}
		pt, err := cipher.Decrypt(ct)
		if err != nil {
			t.Fatalf("cipher.Decrypt failed: %v", err)
		}
		if string(pt) != string(plaintext) {
			t.Fatalf("cipher roundtrip mismatch: got %q, want %q", string(pt), string(plaintext))
		}
	})

	t.Run("Key file containing ONLY UTF-8 BOM and whitespace", func(t *testing.T) {
		keyPath := filepath.Join(tmpDir, "only_bom.key")
		bomData := []byte{0xEF, 0xBB, 0xBF, ' ', '\t', '\r', '\n'}
		if err := os.WriteFile(keyPath, bomData, 0o600); err != nil {
			t.Fatalf("write only BOM file failed: %v", err)
		}

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKeyFile: keyPath,
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg)

		// strings.TrimSpace in Go does NOT treat U+FEFF as whitespace.
		// Document empirical behavior: does it succeed with a 3-byte key or error out?
		if err != nil {
			t.Logf("Empirical result: key file with only BOM returned error: %v", err)
		} else {
			t.Logf("Empirical result: key file with only BOM resolved key len=%d (%x)", len(res.Key), []byte(res.Key))
			// Even with U+FEFF, AdaptiveCipher must be able to derive a SHA-256 key
			cipher, err := security.NewAdaptiveCipher(res.Key, false)
			if err != nil {
				t.Fatalf("cipher initialization failed: %v", err)
			}
			ct, err := cipher.Encrypt([]byte("test"))
			if err != nil {
				t.Fatalf("encrypt failed: %v", err)
			}
			pt, err := cipher.Decrypt(ct)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}
			if string(pt) != "test" {
				t.Fatalf("roundtrip mismatch")
			}
		}
	})
}

// TestKeyResolver_Adversarial_BinaryContents tests key resolution with binary contents.
func TestKeyResolver_Adversarial_BinaryContents(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Key file with raw binary non-ASCII bytes", func(t *testing.T) {
		keyPath := filepath.Join(tmpDir, "binary.key")
		// 32 arbitrary binary bytes (including null byte, high bytes, etc.)
		binaryKey := []byte{
			0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
			0x10, 0x20, 0x30, 0x40, 0x7F, 0x80, 0x90, 0xA0,
			0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE,
			0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF,
		}

		if err := os.WriteFile(keyPath, binaryKey, 0o600); err != nil {
			t.Fatalf("failed to write binary key: %v", err)
		}

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKeyFile: keyPath,
			},
		}

		resolver := config.NewKeyResolver()
		res, err := resolver.Resolve(context.Background(), cfg)
		if err != nil {
			t.Fatalf("unexpected error resolving binary key file: %v", err)
		}

		// Verify cipher operates properly with binary key string
		cipher, err := security.NewAdaptiveCipher(res.Key, false)
		if err != nil {
			t.Fatalf("NewAdaptiveCipher failed with binary key: %v", err)
		}

		plaintext := []byte("confidential-database-credentials")
		ct, err := cipher.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}
		pt, err := cipher.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}
		if string(pt) != string(plaintext) {
			t.Fatalf("payload mismatch")
		}
	})
}

// TestKeyResolver_Adversarial_ZeroLeakage tests that sensitive key strings do not leak.
func TestKeyResolver_Adversarial_ZeroLeakage(t *testing.T) {
	tmpDir := t.TempDir()
	canarySecret := "SUPER_SECRET_CANARY_VALUE_9876543210_DO_NOT_EXPOSE"

	t.Run("JSON serialization of ResolutionResult suppresses key", func(t *testing.T) {
		res := &config.ResolutionResult{
			Key:      canarySecret,
			Source:   config.SourceCLIFlag,
			FilePath: "/some/path/file.key",
		}

		data, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(data)
		if strings.Contains(jsonStr, canarySecret) {
			t.Errorf("CRITICAL SECURITY LEAK: Canary secret leaked in JSON serialization: %s", jsonStr)
		}
	})

	t.Run("Error messages do not leak configured secret keys", func(t *testing.T) {
		// Provide a non-existent key file path that contains a canary name in its path
		canaryFileName := "canary_secret_name_" + canarySecret[:8]
		nonexistentPath := filepath.Join(tmpDir, canaryFileName+".key")

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKey:     "", // Empty so Tier 4 executes
				MasterKeyFile: nonexistentPath,
			},
		}

		// When flag --master-key-file is provided, Tier 4 attempts to read the file and errors out.
		// The error message must describe the failure safely without exposing sensitive data.
		resolver := config.NewKeyResolver()
		_, err := resolver.Resolve(context.Background(), cfg, config.KeyResolverFlags{
			MasterKeyFile: nonexistentPath,
		})

		if err == nil {
			t.Fatalf("expected error for nonexistent key file")
		}

		if strings.Contains(err.Error(), canarySecret) {
			t.Errorf("CRITICAL SECURITY LEAK: Canary secret leaked in error message: %s", err.Error())
		}
	})

	t.Run("Empty key file error message does not leak contents", func(t *testing.T) {
		emptyKeyPath := filepath.Join(tmpDir, "empty_key_test.key")
		_ = os.WriteFile(emptyKeyPath, []byte("   \n\t\r  "), 0o600)

		cfg := &config.Config{
			Storage: config.StorageConfig{DataDir: tmpDir},
			Security: config.SecurityConfig{
				MasterKeyFile: emptyKeyPath,
			},
		}

		resolver := config.NewKeyResolver()
		_, err := resolver.Resolve(context.Background(), cfg)
		if err == nil {
			t.Fatalf("expected error for empty key file")
		}

		// Check error message format
		if !strings.Contains(err.Error(), "is empty") {
			t.Errorf("expected 'is empty' in error, got %v", err)
		}
	})
}

// TestKeyResolver_Adversarial_CorruptAndRestrictedPermissions tests permission failure modes.
func TestKeyResolver_Adversarial_CorruptAndRestrictedPermissions(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Persistent key in read-only directory fails gracefully on generation", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			// On Windows, setting read-only on directory attribute does not prevent file creation in standard user mode.
			// Instead, point DataDir to a non-directory path (a file), which prevents MkdirAll / file creation.
			blockedFilePath := filepath.Join(tmpDir, "file_blocking_dir")
			if err := os.WriteFile(blockedFilePath, []byte("blocker"), 0o600); err != nil {
				t.Fatalf("write blocker file: %v", err)
			}
			subDir := filepath.Join(blockedFilePath, "impossible_dir")

			cfg := &config.Config{
				Storage: config.StorageConfig{DataDir: subDir},
			}

			resolver := config.NewKeyResolver()
			res, err := resolver.Resolve(context.Background(), cfg)
			if err == nil {
				t.Fatalf("expected error when DataDir cannot be created, got res=%+v", res)
			}
			if !strings.Contains(err.Error(), "create data directory") {
				t.Errorf("expected 'create data directory' in error, got: %v", err)
			}
		} else {
			// On POSIX, chmod 0400 on dir prevents file creation
			roDir := filepath.Join(tmpDir, "ro_dir")
			if err := os.Mkdir(roDir, 0o500); err != nil {
				t.Fatalf("mkdir ro_dir: %v", err)
			}
			defer os.Chmod(roDir, 0o700)

			cfg := &config.Config{
				Storage: config.StorageConfig{DataDir: roDir},
			}

			resolver := config.NewKeyResolver()
			res, err := resolver.Resolve(context.Background(), cfg)
			if err == nil {
				t.Fatalf("expected error generating key in read-only directory, got res=%+v", res)
			}
		}
	})
}
