package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dezuxk/internal/db"
	"dezuxk/internal/models"
	"dezuxk/internal/services/chrome"
)

func TestBulkAddAndExportBackup(t *testing.T) {
	tempDB := filepath.Join(os.TempDir(), "test_features.db")
	defer os.Remove(tempDB)

	database, err := db.InitDB(tempDB)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	tempData := filepath.Join(os.TempDir(), "test_features_data")
	defer os.RemoveAll(tempData)

	svc := NewAccountService(database, tempData)

	// 1. Test Bulk Add Accounts
	bulkInput := models.BulkAddInput{
		RawList: `
test1@gmail.com|Pass123
test2@gmail.com|Pass456|recovery@gmail.com|http://103.1.2.3:8080
test3@gmail.com:Pass789
{"email": "test4@gmail.com", "cookie": "__Secure-1PSID=testcookie123; SID=abc;", "proxy": "127.0.0.1:8888"}
`,
		DefaultProxy: "http://10.0.0.1:8080",
		SkipExisting: true,
		DefaultTier:  "PRO",
		Service:      "flow,gemini",
	}

	result, err := svc.BulkAddAccounts(bulkInput)
	if err != nil {
		t.Fatalf("BulkAddAccounts failed: %v", err)
	}

	if result.TotalParsed != 4 {
		t.Errorf("expected 4 accounts parsed, got %d", result.TotalParsed)
	}
	if result.AddedCount != 4 {
		t.Errorf("expected 4 accounts added, got %d", result.AddedCount)
	}

	accounts, err := svc.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(accounts) != 4 {
		t.Fatalf("expected 4 accounts in db, got %d", len(accounts))
	}

	// 2. Test Manual Add Account
	manualInput := models.ManualAccountInput{
		Email:       "manual@gmail.com",
		Cookies:     "__Secure-1PSID=fakepsid; SID=fakesid;",
		SNlM0eToken: "fakesnlm0e",
		Proxy:       "http://manual.proxy:8080",
		Tier:        "PRO",
		Credits:     1050,
		Service:     "flow",
	}
	manualRes, err := svc.AddAccountManual(manualInput)
	if err != nil {
		t.Fatalf("AddAccountManual failed: %v", err)
	}
	if manualRes.Email != "manual@gmail.com" {
		t.Errorf("expected manual@gmail.com, got %s", manualRes.Email)
	}

	// 3. Test Export Backup ZIP
	zipOut := filepath.Join(tempData, "test_backup.zip")
	exportRes, err := svc.ExportBackup(zipOut)
	if err != nil {
		t.Fatalf("ExportBackup failed: %v", err)
	}

	if exportRes.AccountCount != 5 {
		t.Errorf("expected 5 accounts in backup, got %d", exportRes.AccountCount)
	}

	// Inspect ZIP contents
	zipReader, err := zip.OpenReader(zipOut)
	if err != nil {
		t.Fatalf("failed to open exported zip: %v", err)
	}
	defer zipReader.Close()

	foundAccountsJSON := false
	foundManifestJSON := false
	for _, f := range zipReader.File {
		if f.Name == "accounts.json" {
			foundAccountsJSON = true
		}
		if f.Name == "manifest.json" {
			foundManifestJSON = true
		}
	}

	if !foundAccountsJSON {
		t.Errorf("accounts.json missing in exported backup zip")
	}
	if !foundManifestJSON {
		t.Errorf("manifest.json missing in exported backup zip")
	}

	t.Logf("Exported backup verified successfully: %s (%d bytes)", zipOut, exportRes.SizeBytes)
}

func TestProxyParsing(t *testing.T) {
	// Test format ip:port:user:pass
	flag, user, pass := chrome.FormatChromeProxyFlag("103.14.22.1:8080:admin:secret123")
	if flag != "http://103.14.22.1:8080" {
		t.Errorf("expected http://103.14.22.1:8080, got %s", flag)
	}
	if user != "admin" || pass != "secret123" {
		t.Errorf("user/pass mismatch: got %s / %s", user, pass)
	}

	// Test format http://user:pass@host:port
	flag2, user2, pass2 := chrome.FormatChromeProxyFlag("http://myuser:mypass@proxy.net:3128")
	if !strings.Contains(flag2, "proxy.net:3128") {
		t.Errorf("expected proxy.net:3128 in flag, got %s", flag2)
	}
	if user2 != "myuser" || pass2 != "mypass" {
		t.Errorf("user2/pass2 mismatch: got %s / %s", user2, pass2)
	}
}

func TestDynamicCreditsAndTier(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_dyn.db")
	database, err := db.InitDB(tempDB)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	svc := NewAccountService(database, t.TempDir())

	// 1. Add account with 0 credits and FREE tier
	res, err := svc.AddAccountManual(models.ManualAccountInput{
		Email:   "zero_credit@gmail.com",
		Cookies: "SID=abc;",
		Tier:    "FREE",
		Credits: 0,
	})
	if err != nil {
		t.Fatalf("AddAccountManual failed: %v", err)
	}

	// Verify credits is strictly 0 and NOT forced to 1050
	if res.Credits != 0 {
		t.Errorf("expected 0 credits, got %d", res.Credits)
	}
	if res.Tier != "FREE" {
		t.Errorf("expected FREE tier, got %s", res.Tier)
	}

	// 2. Test UpdateCredits method
	if err := svc.UpdateCredits(res.ID, 3420); err != nil {
		t.Fatalf("UpdateCredits failed: %v", err)
	}

	updatedAcc, err := database.GetGoogleAccountByID(res.ID)
	if err != nil {
		t.Fatalf("GetGoogleAccountByID failed: %v", err)
	}
	if updatedAcc.Credits != 3420 {
		t.Errorf("expected 3420 credits after update, got %d", updatedAcc.Credits)
	}

	// 3. Test dynamic session update with new tier and credits
	if err := database.UpdateGoogleAccountSessionData(res.ID, "SID=newcookie;", "", 7500, "ULTRA"); err != nil {
		t.Fatalf("UpdateGoogleAccountSessionData failed: %v", err)
	}

	refreshedAcc, err := database.GetGoogleAccountByID(res.ID)
	if err != nil {
		t.Fatalf("GetGoogleAccountByID after session update failed: %v", err)
	}
	if refreshedAcc.Credits != 7500 {
		t.Errorf("expected 7500 credits, got %d", refreshedAcc.Credits)
	}
	if refreshedAcc.Tier != "ULTRA" {
		t.Errorf("expected ULTRA tier, got %s", refreshedAcc.Tier)
	}
}


