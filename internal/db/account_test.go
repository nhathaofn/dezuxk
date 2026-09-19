package db

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcrypto "dezuxk/internal/crypto"
	"dezuxk/internal/models"
)

func TestGoogleAccountCRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "db_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	// 1. Create account
	acc := &models.GoogleAccount{
		ID:          "acc_123",
		Email:       "test@gmail.com",
		Name:        "Test User",
		AvatarURL:   "https://example.com/avatar.png",
		ProfileDir:  "profiles/acc_123",
		Cookies:     "__Secure-1PSID=token1; __Secure-1PSIDTS=token2",
		SNlM0eToken: "token_snlm0e",
		Proxy:       "http://proxy-user:proxy-pass@example.com:8080",
		Services:    "gemini,flow",
		Status:      "ACTIVE",
	}

	if err := database.CreateGoogleAccount(acc); err != nil {
		t.Fatalf("CreateGoogleAccount failed: %v", err)
	}

	// 2. Verify raw storage in SQLite is encrypted with enc:v1:
	var rawCookies, rawSnlm0e, rawProxy string
	err = database.conn.QueryRow("SELECT cookies, snlm0e_token, proxy FROM google_accounts WHERE id = ?", "acc_123").Scan(&rawCookies, &rawSnlm0e, &rawProxy)
	if err != nil {
		t.Fatalf("failed querying raw row: %v", err)
	}
	if len(database.CipherKey()) == 32 {
		if !strings.HasPrefix(rawCookies, "enc:v1:") {
			t.Errorf("raw stored cookies should be encrypted with 'enc:v1:', got: %s", rawCookies)
		}
		if !strings.HasPrefix(rawSnlm0e, "enc:v1:") {
			t.Errorf("raw stored snlm0e should be encrypted with 'enc:v1:', got: %s", rawSnlm0e)
		}
		if !strings.HasPrefix(rawProxy, "enc:v1:") {
			t.Errorf("raw stored proxy should be encrypted with 'enc:v1:'")
		}
	}

	// 3. Get account by ID and verify transparent decryption
	fetched, err := database.GetGoogleAccountByID("acc_123")
	if err != nil {
		t.Fatalf("GetGoogleAccountByID failed: %v", err)
	}
	if fetched.Email != "test@gmail.com" {
		t.Errorf("expected email 'test@gmail.com', got '%s'", fetched.Email)
	}
	if fetched.Cookies != acc.Cookies {
		t.Errorf("cookies mismatch: expected '%s', got '%s'", acc.Cookies, fetched.Cookies)
	}
	if fetched.SNlM0eToken != acc.SNlM0eToken {
		t.Errorf("snlm0e mismatch: expected '%s', got '%s'", acc.SNlM0eToken, fetched.SNlM0eToken)
	}
	if fetched.Proxy != acc.Proxy {
		t.Errorf("proxy mismatch after transparent decryption")
	}
	response := fetched.ToResponse()
	if strings.Contains(response.Proxy, "proxy-pass") || response.Proxy != "http://example.com:8080" {
		t.Errorf("proxy response should be redacted, got %q", response.Proxy)
	}

	// 4. List accounts
	list, err := database.ListGoogleAccounts()
	if err != nil {
		t.Fatalf("ListGoogleAccounts failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 account in list, got %d", len(list))
	}

	// 4. Update cookies
	newCookies := "__Secure-1PSID=updated1; __Secure-1PSIDTS=updated2"
	if err := database.UpdateGoogleAccountCookies("acc_123", newCookies, "new_snlm0e"); err != nil {
		t.Fatalf("UpdateGoogleAccountCookies failed: %v", err)
	}

	updated, err := database.GetGoogleAccountByID("acc_123")
	if err != nil {
		t.Fatalf("GetGoogleAccountByID after update failed: %v", err)
	}
	if updated.Cookies != newCookies {
		t.Errorf("expected new cookies, got '%s'", updated.Cookies)
	}
	if updated.SNlM0eToken != "new_snlm0e" {
		t.Errorf("expected 'new_snlm0e', got '%s'", updated.SNlM0eToken)
	}
	if updated.LastRefreshAt == "" {
		t.Errorf("expected LastRefreshAt to be touched, but was empty")
	}

	// 5. Update Status
	if err := database.UpdateGoogleAccountStatus("acc_123", "EXPIRED", "Token expired on Google"); err != nil {
		t.Fatalf("UpdateGoogleAccountStatus failed: %v", err)
	}
	statusUpdated, _ := database.GetGoogleAccountByID("acc_123")
	if statusUpdated.Status != "EXPIRED" || statusUpdated.LastError != "Token expired on Google" {
		t.Errorf("status update failed: %+v", statusUpdated)
	}

	// 6. Delete account
	if err := database.DeleteGoogleAccount("acc_123"); err != nil {
		t.Fatalf("DeleteGoogleAccount failed: %v", err)
	}
	_, err = database.GetGoogleAccountByID("acc_123")
	if err == nil {
		t.Errorf("expected error getting deleted account, got nil")
	}
}

func TestLegacySensitiveFieldsAreMigratedAndGooglePasswordIsCleared(t *testing.T) {
	database, err := InitDB(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	_, err = database.conn.Exec(`INSERT INTO google_accounts
		(id, email, profile_dir, cookies, snlm0e_token, proxy, password, recovery_email)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy_1", "legacy@example.com", "profiles/legacy_1", "SID=legacy", "legacy-token",
		"http://user:pass@proxy.example:8080", "old-google-password", "recovery@example.com")
	if err != nil {
		t.Fatalf("failed to insert legacy row: %v", err)
	}
	if err := database.migrateSensitiveFields(); err != nil {
		t.Fatalf("migrateSensitiveFields failed: %v", err)
	}

	var cookies, token, proxy, password, recovery string
	if err := database.conn.QueryRow(`SELECT cookies, snlm0e_token, proxy, password, recovery_email
		FROM google_accounts WHERE id = ?`, "legacy_1").Scan(&cookies, &token, &proxy, &password, &recovery); err != nil {
		t.Fatalf("failed to read migrated row: %v", err)
	}
	if !strings.HasPrefix(cookies, "enc:v1:") || !strings.HasPrefix(token, "enc:v1:") || !strings.HasPrefix(proxy, "enc:v1:") {
		t.Fatal("legacy sensitive fields were not encrypted")
	}
	if password != "" || recovery != "" {
		t.Fatal("legacy Google password/recovery fields were not cleared")
	}

	acc, err := database.GetGoogleAccountByID("legacy_1")
	if err != nil {
		t.Fatalf("failed to load migrated account: %v", err)
	}
	if acc.Cookies != "SID=legacy" || acc.SNlM0eToken != "legacy-token" || acc.Proxy != "http://user:pass@proxy.example:8080" {
		t.Fatal("migrated fields did not decrypt back to their expected values")
	}
}

func TestLegacyEncryptedSensitiveFieldsAreReencryptedWithCurrentKey(t *testing.T) {
	database, err := InitDB(filepath.Join(t.TempDir(), "legacy-encrypted.db"))
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	if len(database.legacyCipherKey) != 32 || bytes.Equal(database.cipherKey, database.legacyCipherKey) {
		t.Skip("current platform uses the legacy fallback key")
	}
	legacyCookies, err := appcrypto.Encrypt("SID=legacy-encrypted", database.legacyCipherKey)
	if err != nil {
		t.Fatalf("failed to create legacy cookie ciphertext: %v", err)
	}
	legacyToken, err := appcrypto.Encrypt("legacy-token", database.legacyCipherKey)
	if err != nil {
		t.Fatalf("failed to create legacy token ciphertext: %v", err)
	}

	_, err = database.conn.Exec(`INSERT INTO google_accounts
		(id, email, profile_dir, cookies, snlm0e_token)
		VALUES (?, ?, ?, ?, ?)`,
		"legacy_enc_1", "legacy-encrypted@example.com", "profiles/legacy_enc_1", legacyCookies, legacyToken)
	if err != nil {
		t.Fatalf("failed to insert legacy encrypted row: %v", err)
	}
	if err := database.migrateSensitiveFields(); err != nil {
		t.Fatalf("migrateSensitiveFields failed: %v", err)
	}

	var rawCookies string
	if err := database.conn.QueryRow("SELECT cookies FROM google_accounts WHERE id = ?", "legacy_enc_1").Scan(&rawCookies); err != nil {
		t.Fatalf("failed to read migrated cookie ciphertext: %v", err)
	}
	if rawCookies == legacyCookies {
		t.Fatal("legacy ciphertext was not re-encrypted with the current key")
	}
	acc, err := database.GetGoogleAccountByID("legacy_enc_1")
	if err != nil {
		t.Fatalf("failed to load migrated account: %v", err)
	}
	if acc.Cookies != "SID=legacy-encrypted" || acc.SNlM0eToken != "legacy-token" {
		t.Fatal("legacy encrypted values did not decrypt after migration")
	}
}
