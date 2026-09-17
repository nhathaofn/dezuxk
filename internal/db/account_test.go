package db

import (
	"os"
	"path/filepath"
	"testing"

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
		Services:    "gemini,flow",
		Status:      "ACTIVE",
	}

	if err := database.CreateGoogleAccount(acc); err != nil {
		t.Fatalf("CreateGoogleAccount failed: %v", err)
	}

	// 2. Get account by ID
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

	// 3. List accounts
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
