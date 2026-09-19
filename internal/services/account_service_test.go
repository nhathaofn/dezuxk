package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dezuxk/internal/db"
	"dezuxk/internal/models"
)

func TestGoogleRequestCooldowns(t *testing.T) {
	service := &AccountService{
		refreshAttemptAt: make(map[string]time.Time),
		probeAttemptAt:   make(map[string]time.Time),
	}
	account := &models.GoogleAccount{ID: "acc_cooldown_test", Email: "test@example.com"}

	if err := service.claimRefreshAttempt(account); err != nil {
		t.Fatalf("first refresh attempt should be allowed: %v", err)
	}
	if err := service.claimRefreshAttempt(account); err == nil {
		t.Fatal("second refresh attempt inside cooldown should be rejected")
	}

	service.refreshAttemptAt[account.ID] = time.Now().UTC().Add(-refreshAttemptCooldown - time.Second)
	if err := service.claimRefreshAttempt(account); err != nil {
		t.Fatalf("refresh attempt after cooldown should be allowed: %v", err)
	}

	if err := service.claimProbeAttempt(account); err != nil {
		t.Fatalf("first probe attempt should be allowed: %v", err)
	}
	if err := service.claimProbeAttempt(account); err == nil {
		t.Fatal("second probe attempt inside cooldown should be rejected")
	}
}

func TestImportGLabsBackup(t *testing.T) {
	tempDB := filepath.Join(os.TempDir(), "test_glabs.db")
	defer os.Remove(tempDB)

	database, err := db.InitDB(tempDB)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	tempData := filepath.Join(os.TempDir(), "test_glabs_data")
	defer os.RemoveAll(tempData)

	svc := NewAccountService(database, tempData)

	// Dynamically find any backup folder in Downloads
	userProfile := os.Getenv("USERPROFILE")
	downloadsDir := filepath.Join(userProfile, "Downloads")
	var backupDir string
	if entries, err := os.ReadDir(downloadsDir); err == nil {
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Name()), "glabs-flow-accounts") && e.IsDir() {
				backupDir = filepath.Join(downloadsDir, e.Name())
				break
			}
		}
	}

	if backupDir == "" {
		t.Skip("No glabs-flow-accounts backup directory found in Downloads, skipping live import test")
	}

	res, err := svc.ImportGLabsBackup(backupDir)
	if err != nil {
		t.Fatalf("ImportGLabsBackup failed: %v", err)
	}

	if res.AccountsRestored == 0 {
		t.Errorf("expected at least 1 account imported, got %d", res.AccountsRestored)
	}

	accounts, err := svc.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}

	if len(accounts) == 0 {
		t.Fatalf("expected accounts in DB, got 0")
	}

	for _, acc := range accounts {
		t.Logf("Imported account: %s, tier: %s, credits: %d, status: %s", acc.Email, acc.Tier, acc.Credits, acc.Status)
		if acc.Tier != "PRO" && acc.Tier != "FREE" {
			t.Errorf("unexpected tier %s", acc.Tier)
		}
	}
}

func TestAddAccountManualValidation(t *testing.T) {
	tempDB := filepath.Join(os.TempDir(), "test_val.db")
	defer os.Remove(tempDB)

	database, err := db.InitDB(tempDB)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	tempData := filepath.Join(os.TempDir(), "test_val_data")
	defer os.RemoveAll(tempData)

	svc := NewAccountService(database, tempData)

	// 1. Invalid cookies (missing __Secure-1PSID and SID)
	_, err = svc.AddAccountManual(models.ManualAccountInput{
		Email:   "invalid@gmail.com",
		Cookies: "random_junk_cookie=12345; other_thing=6789;",
	})
	if err == nil {
		t.Errorf("expected error for invalid cookie without PSID or SID, but got nil")
	}

	// 2. Valid cookies
	validCookie := "__Secure-1PSID=token_secret_12345; SID=token_sid_67890; __Secure-1PSIDTS=ts_9999;"
	acc, err := svc.AddAccountManual(models.ManualAccountInput{
		Email:   "valid_user@gmail.com",
		Cookies: validCookie,
		Tier:    "PRO",
		Credits: 100,
	})
	if err != nil {
		t.Fatalf("failed to add manual account with valid cookies: %v", err)
	}

	if acc.Email != "valid_user@gmail.com" {
		t.Errorf("expected email 'valid_user@gmail.com', got '%s'", acc.Email)
	}
	if acc.Credits != 100 {
		t.Errorf("expected credits 100, got %d", acc.Credits)
	}

	// Verify lazy creation: profile folder should NOT be created on disk for manual cookie account
	profilePath := filepath.Join(tempData, "profiles", acc.ID)
	if _, statErr := os.Stat(profilePath); !os.IsNotExist(statErr) {
		t.Errorf("profile directory should NOT be eagerly created for manual cookie account")
	}
}

func TestPurgeAccountCaches(t *testing.T) {
	tempDB := filepath.Join(os.TempDir(), "test_purge.db")
	defer os.Remove(tempDB)

	database, err := db.InitDB(tempDB)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	tempData := filepath.Join(os.TempDir(), "test_purge_data")
	defer os.RemoveAll(tempData)

	svc := NewAccountService(database, tempData)

	// Create mock profile directories with cache
	pDir := filepath.Join(tempData, "profiles", "acc_test_user", "Default")
	codeCache := filepath.Join(pDir, "Code Cache")
	gpuCache := filepath.Join(pDir, "GPUCache")
	cookiesDir := filepath.Join(pDir, "Network")

	_ = os.MkdirAll(codeCache, 0755)
	_ = os.MkdirAll(gpuCache, 0755)
	_ = os.MkdirAll(cookiesDir, 0755)

	dummyData := []byte("dummy cache data filling space 1234567890")
	_ = os.WriteFile(filepath.Join(codeCache, "cache_index"), dummyData, 0644)
	_ = os.WriteFile(filepath.Join(gpuCache, "gpu_index"), dummyData, 0644)
	_ = os.WriteFile(filepath.Join(cookiesDir, "Cookies"), []byte("important session cookies"), 0644)

	res, err := svc.PurgeAccountCaches()
	if err != nil {
		t.Fatalf("PurgeAccountCaches failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected purge success")
	}
	if res.FreedBytes <= 0 {
		t.Errorf("expected freed bytes > 0, got %d", res.FreedBytes)
	}
	if res.ProfilesCleaned != 1 {
		t.Errorf("expected 1 profile cleaned, got %d", res.ProfilesCleaned)
	}

	// Verify cache dirs are gone
	if _, err := os.Stat(codeCache); !os.IsNotExist(err) {
		t.Errorf("Code Cache should be deleted")
	}
	if _, err := os.Stat(gpuCache); !os.IsNotExist(err) {
		t.Errorf("GPUCache should be deleted")
	}

	// Verify non-cache files are kept intact!
	if _, err := os.Stat(filepath.Join(cookiesDir, "Cookies")); err != nil {
		t.Errorf("Cookies file must NOT be deleted during cache purge!")
	}
}
