package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"dezuxk/internal/config"

	"golang.org/x/crypto/argon2"
)

const (
	// EncryptedPrefix marks a string as encrypted with AES-256-GCM.
	EncryptedPrefix = "enc:v1:"
	BackupPrefix    = "dezuxk-backup:v1:"
	settingKey      = "sec_device_salt"
	dpapiKeySetting = "sec_master_key_dpapi"
)

// SettingsStore abstracts reading and saving persistent key-value configuration.
type SettingsStore interface {
	GetSetting(key, defaultValue string) string
	SetSetting(key, value string) error
}

// EncryptBackup protects an exported archive with a user-supplied password.
// The password is deliberately separate from the machine-bound DB key so the
// backup can be restored on another authorized machine.
func EncryptBackup(plaintext []byte, password string) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("backup is empty")
	}
	if len(password) < config.MinPasswordLength {
		return nil, errors.New("backup password must contain at least 5 characters")
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate backup salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate backup nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(BackupPrefix))
	envelope := append(append(append([]byte{}, salt...), nonce...), ciphertext...)
	encoded := base64.RawStdEncoding.EncodeToString(envelope)
	return []byte(BackupPrefix + encoded), nil
}

// DecryptBackup opens a password-protected exported archive.
func DecryptBackup(envelope []byte, password string) ([]byte, error) {
	if !strings.HasPrefix(string(envelope), BackupPrefix) {
		return nil, errors.New("unsupported backup format")
	}
	if len(password) < config.MinPasswordLength {
		return nil, errors.New("backup password must contain at least 5 characters")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(string(envelope), BackupPrefix))
	if err != nil {
		return nil, fmt.Errorf("failed to decode backup: %w", err)
	}
	const saltLen = 16
	const nonceLen = 12
	if len(raw) <= saltLen+nonceLen {
		return nil, errors.New("backup payload is truncated")
	}
	salt := raw[:saltLen]
	nonce := raw[saltLen : saltLen+nonceLen]
	ciphertext := raw[saltLen+nonceLen:]
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(BackupPrefix))
	if err != nil {
		return nil, errors.New("backup password is incorrect or the backup is corrupted")
	}
	return plaintext, nil
}

// GetOrInitMasterKey retrieves or creates the 256-bit data key. On Windows a
// fresh random key is protected with DPAPI; the older salt+MachineGuid key is
// used only as a migration key for data written by older builds.
func GetOrInitMasterKey(store SettingsStore) ([]byte, error) {
	if store == nil {
		return nil, errors.New("settings store is required to derive the encryption key")
	}

	if protectedText := store.GetSetting(dpapiKeySetting, ""); protectedText != "" {
		protected, err := base64.StdEncoding.DecodeString(protectedText)
		if err != nil {
			return nil, fmt.Errorf("failed to decode protected master key: %w", err)
		}
		key, err := unprotectMasterKey(protected)
		if err != nil {
			return nil, fmt.Errorf("failed to unprotect master key: %w", err)
		}
		if len(key) != 32 {
			return nil, errors.New("protected master key has an invalid length")
		}
		return key, nil
	}

	var saltHex string
	saltHex = store.GetSetting(settingKey, "")

	if saltHex == "" {
		saltBytes := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, saltBytes); err != nil {
			return nil, fmt.Errorf("failed to generate crypto salt: %w", err)
		}
		saltHex = base64.StdEncoding.EncodeToString(saltBytes)
		if err := store.SetSetting(settingKey, saltHex); err != nil {
			return nil, fmt.Errorf("failed to persist device salt: %w", err)
		}
	}

	legacyKey, err := deriveLegacyMasterKey(saltHex)
	if err != nil {
		return nil, err
	}

	masterKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, masterKey); err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}
	if protected, err := protectMasterKey(masterKey); err == nil {
		encoded := base64.StdEncoding.EncodeToString(protected)
		if err := store.SetSetting(dpapiKeySetting, encoded); err != nil {
			return nil, fmt.Errorf("failed to persist protected master key: %w", err)
		}
		return masterKey, nil
	} else if masterKeyProtectionRequired() {
		return nil, fmt.Errorf("OS master-key protection is unavailable: %w", err)
	}

	return legacyKey, nil
}

// GetLegacyMasterKey derives the pre-DPAPI key so the database layer can
// decrypt and re-encrypt records created by older versions.
func GetLegacyMasterKey(store SettingsStore) ([]byte, error) {
	if store == nil {
		return nil, errors.New("settings store is required to derive the legacy key")
	}
	saltHex := store.GetSetting(settingKey, "")
	if saltHex == "" {
		return nil, errors.New("legacy device salt is unavailable")
	}
	return deriveLegacyMasterKey(saltHex)
}

func deriveLegacyMasterKey(saltHex string) ([]byte, error) {
	if saltHex == "" {
		return nil, errors.New("legacy device salt is empty")
	}
	hasher := sha256.New()
	hasher.Write([]byte(saltHex))
	hasher.Write([]byte(getMachineIdentifier()))
	return hasher.Sum(nil), nil
}

// Encrypt encrypts plaintext using AES-256-GCM and appends the prefix "enc:v1:".
// If plaintext is empty, it returns an empty string without encryption.
func Encrypt(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(key) != 32 {
		return "", errors.New("invalid key length: must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a ciphertext string prefixed with "enc:v1:".
// If the string does NOT start with "enc:v1:", it is treated as unencrypted legacy plaintext (backward compatibility).
func Decrypt(ciphertext string, key []byte) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	// Backward compatibility: if not prefixed, return plain text
	if !strings.HasPrefix(ciphertext, EncryptedPrefix) {
		return ciphertext, nil
	}

	rawB64 := strings.TrimPrefix(ciphertext, EncryptedPrefix)
	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 ciphertext: %w", err)
	}

	if len(key) != 32 {
		return "", errors.New("invalid key length: must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext payload too short")
	}

	nonce, actualCiphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (invalid key or tampered data): %w", err)
	}

	return string(plaintext), nil
}

// getMachineIdentifier queries Windows MachineGuid, falling back to hostname/OS info.
func getMachineIdentifier() string {
	// On Windows, query MachineGuid from Registry
	cmd := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "MachineGuid") && strings.Contains(trimmed, "REG_SZ") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 3 {
					return parts[len(parts)-1]
				}
			}
		}
	}

	// Fallback to computer name / user domain
	if name, err := os.Hostname(); err == nil && name != "" {
		return name
	}

	return "dezuxk_generic_machine_node"
}
