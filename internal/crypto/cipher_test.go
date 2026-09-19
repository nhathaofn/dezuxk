package crypto

import (
	"strings"
	"testing"
)

type mockStore struct {
	data map[string]string
}

func (m *mockStore) GetSetting(key, defaultValue string) string {
	if val, ok := m.data[key]; ok {
		return val
	}
	return defaultValue
}

func (m *mockStore) SetSetting(key, value string) error {
	m.data[key] = value
	return nil
}

func TestGetOrInitMasterKey(t *testing.T) {
	store := &mockStore{data: make(map[string]string)}

	key1, err := GetOrInitMasterKey(store)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("expected 32 byte key, got %d", len(key1))
	}

	// Second retrieval with same store should return identical key
	key2, err := GetOrInitMasterKey(store)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if string(key1) != string(key2) {
		t.Fatalf("keys should match for same store")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	store := &mockStore{data: make(map[string]string)}
	key, err := GetOrInitMasterKey(store)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	secret := "__Secure-1PSID=test_cookie_12345; SID=sid_token_abc;"

	encrypted, err := Encrypt(secret, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if !strings.HasPrefix(encrypted, EncryptedPrefix) {
		t.Fatalf("expected prefix %s, got: %s", EncryptedPrefix, encrypted)
	}
	if encrypted == secret {
		t.Fatalf("encrypted text must not equal plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != secret {
		t.Fatalf("expected decrypted '%s', got '%s'", secret, decrypted)
	}
}

func TestBackwardCompatibility(t *testing.T) {
	store := &mockStore{data: make(map[string]string)}
	key, _ := GetOrInitMasterKey(store)

	// Plaintext without prefix must return unchanged
	rawLegacy := "__Secure-1PSID=legacy_plaintext_cookie"
	result, err := Decrypt(rawLegacy, key)
	if err != nil {
		t.Fatalf("decrypting legacy plaintext should not error, got: %v", err)
	}
	if result != rawLegacy {
		t.Fatalf("expected '%s', got '%s'", rawLegacy, result)
	}

	// Empty string
	empty, err := Decrypt("", key)
	if err != nil || empty != "" {
		t.Fatalf("empty input should return empty without error")
	}
}
