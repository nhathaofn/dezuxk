package services

import (
	"os"
	"path/filepath"
	"testing"

	"dezuxk/internal/db"
)

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

	backupDir := `C:\Users\PC\Downloads\glabs-flow-accounts-20260917-160519`
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Skip("glabs-flow-accounts backup directory not found, skipping live import test")
	}

	count, err := svc.ImportGLabsBackup(backupDir)
	if err != nil {
		t.Fatalf("ImportGLabsBackup failed: %v", err)
	}

	if count != 6 {
		t.Errorf("expected 6 accounts imported, got %d", count)
	}

	accounts, err := svc.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}

	if len(accounts) != 6 {
		t.Fatalf("expected 6 accounts in DB, got %d", len(accounts))
	}

	for _, acc := range accounts {
		t.Logf("Imported account: %s, tier: %s, credits: %d, status: %s", acc.Email, acc.Tier, acc.Credits, acc.Status)
		if acc.Tier != "PRO" {
			t.Errorf("expected tier PRO, got %s", acc.Tier)
		}
		if acc.Credits <= 0 {
			t.Errorf("expected credits > 0, got %d", acc.Credits)
		}
	}
}
