package sqlite_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
	"dezuxk/internal/storage/sqlite"
)

func setupTestDB(t *testing.T) (*sqlite.DB, *sqlite.ProvidersRepository, security.Cipher) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_dezuxk.db")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sqlite.Open(ctx, sqlite.Options{
		Path: dbPath,
	})
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}

	cipher, err := security.NewAdaptiveCipher("test-db-master-key-xyz", false)
	if err != nil {
		t.Fatalf("create cipher failed: %v", err)
	}

	repo := sqlite.NewProvidersRepository(db, cipher)
	return db, repo, cipher
}

func TestProvidersRepository_ProviderCRUD(t *testing.T) {
	db, repo, _ := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	p := &providers.Provider{
		ID:             "flow",
		Name:           "Google Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com", "flow.google.com"},
		ProbeRules: providers.ProbeRules{
			IdentityExpressions: []string{`[data-email]`},
			DiscoveryKeywords:   []string{"BatchGenerateImages"},
		},
		SecurityRules: map[string]any{"anti_bot": true},
		Status:        providers.ProviderStatusActive,
	}

	// 1. Save
	if err := repo.SaveProvider(ctx, p); err != nil {
		t.Fatalf("SaveProvider failed: %v", err)
	}

	// 2. Get
	fetched, err := repo.GetProvider(ctx, "flow")
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected provider 'flow', got nil")
	}
	if fetched.Name != "Google Flow" || fetched.BaseURL != "https://flow.google.com" {
		t.Errorf("provider fields mismatch: %+v", fetched)
	}
	if len(fetched.AllowedDomains) != 2 {
		t.Errorf("expected 2 allowed domains, got %d", len(fetched.AllowedDomains))
	}

	// 3. List
	list, err := repo.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != "flow" {
		t.Errorf("expected 1 provider 'flow', got %d", len(list))
	}

	// 4. Update
	p.Name = "Google Flow Updated"
	if err := repo.SaveProvider(ctx, p); err != nil {
		t.Fatalf("Update SaveProvider failed: %v", err)
	}
	fetchedUpdated, _ := repo.GetProvider(ctx, "flow")
	if fetchedUpdated.Name != "Google Flow Updated" {
		t.Errorf("expected updated name, got %s", fetchedUpdated.Name)
	}

	// 5. Delete
	if err := repo.DeleteProvider(ctx, "flow"); err != nil {
		t.Fatalf("DeleteProvider failed: %v", err)
	}
	deleted, _ := repo.GetProvider(ctx, "flow")
	if deleted != nil {
		t.Errorf("expected nil after delete, got %+v", deleted)
	}
}

func TestProvidersRepository_AccountPoolAndReadiness(t *testing.T) {
	db, repo, _ := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Seed provider
	p := &providers.Provider{
		ID:             "flow",
		Name:           "Google Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com"},
		Status:         providers.ProviderStatusActive,
	}
	if err := repo.SaveProvider(ctx, p); err != nil {
		t.Fatalf("SaveProvider failed: %v", err)
	}

	// Account 1: Ready
	acc1 := &providers.Account{
		ID:         "acc-1",
		ProviderID: "flow",
		Label:      "Ready Account 1",
		Identity:   "user1@gmail.com",
		Status:     providers.AccountStatusReady,
		ProfileDir: "/profiles/acc-1",
	}
	// Account 2: Unverified
	acc2 := &providers.Account{
		ID:         "acc-2",
		ProviderID: "flow",
		Label:      "Unverified Account 2",
		Identity:   "user2@gmail.com",
		Status:     providers.AccountStatusUnverified,
		ProfileDir: "/profiles/acc-2",
	}
	// Account 3: Paused rate-limited
	pausedUntil := time.Now().UTC().Add(1 * time.Hour)
	acc3 := &providers.Account{
		ID:          "acc-3",
		ProviderID:  "flow",
		Label:       "Paused Account 3",
		Identity:    "user3@gmail.com",
		Status:      providers.AccountStatusReady,
		ProfileDir:  "/profiles/acc-3",
		PausedUntil: &pausedUntil,
	}

	for _, acc := range []*providers.Account{acc1, acc2, acc3} {
		if err := repo.SaveAccount(ctx, acc); err != nil {
			t.Fatalf("SaveAccount failed: %v", err)
		}
	}

	// Count Total Accounts: should be 3
	total, err := repo.CountTotalAccounts(ctx)
	if err != nil {
		t.Fatalf("CountTotalAccounts failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 total accounts, got %d", total)
	}

	// Count Ready Accounts: only acc1 is ready and not paused
	ready, err := repo.CountReadyAccounts(ctx)
	if err != nil {
		t.Fatalf("CountReadyAccounts failed: %v", err)
	}
	if ready != 1 {
		t.Errorf("expected 1 ready account, got %d", ready)
	}

	// List Ready Accounts: should return only acc1
	readyList, err := repo.ListReadyAccounts(ctx, "flow")
	if err != nil {
		t.Fatalf("ListReadyAccounts failed: %v", err)
	}
	if len(readyList) != 1 || readyList[0].ID != "acc-1" {
		t.Errorf("expected [acc-1], got %+v", readyList)
	}

	// Unpause acc3 by setting paused_until in the past
	pastTime := time.Now().UTC().Add(-1 * time.Minute)
	if err := repo.UpdateAccountStatus(ctx, "acc-3", providers.AccountStatusReady, &pastTime); err != nil {
		t.Fatalf("UpdateAccountStatus failed: %v", err)
	}

	readyAfter, err := repo.CountReadyAccounts(ctx)
	if err != nil {
		t.Fatalf("CountReadyAccounts after unpause failed: %v", err)
	}
	if readyAfter != 2 {
		t.Errorf("expected 2 ready accounts after unpause, got %d", readyAfter)
	}
}

func TestProvidersRepository_TransparentSecretsEncryption(t *testing.T) {
	db, repo, cipher := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Seed provider and account
	_ = repo.SaveProvider(ctx, &providers.Provider{
		ID:             "flow",
		Name:           "Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com"},
		Status:         providers.ProviderStatusActive,
	})
	_ = repo.SaveAccount(ctx, &providers.Account{
		ID:         "acc-sec-1",
		ProviderID: "flow",
		Label:      "Secrets Test",
		ProfileDir: "/profiles/sec-1",
	})

	sec := &providers.AccountSecrets{
		AccountID: "acc-sec-1",
		Cookies: []providers.Cookie{
			{Name: "SID", Value: "secret_cookie_token_12345", Domain: ".google.com", Path: "/", Secure: true},
			{Name: "HSID", Value: "hsid_value_67890", Domain: ".google.com", Path: "/"},
		},
		Tokens: map[string]string{
			"f.sid": "flow_session_id_xyz",
			"bl":    "boq_workflow_build_label",
		},
		CookieHash: "sha256_mock_hash",
	}

	// 1. Save Secrets
	if err := repo.SaveAccountSecrets(ctx, sec); err != nil {
		t.Fatalf("SaveAccountSecrets failed: %v", err)
	}

	// 2. Verify in raw SQLite that blobs are stored encrypted, not plaintext!
	var rawCookiesBlob, rawTokensBlob []byte
	err := db.Raw().QueryRow("SELECT encrypted_cookies_blob, encrypted_tokens_blob FROM account_secrets WHERE account_id = ?", "acc-sec-1").Scan(&rawCookiesBlob, &rawTokensBlob)
	if err != nil {
		t.Fatalf("query raw secrets failed: %v", err)
	}

	if len(rawCookiesBlob) == 0 || len(rawTokensBlob) == 0 {
		t.Fatalf("expected non-empty encrypted blobs in DB")
	}

	// Neither raw blob should contain the plaintext string
	if bytes.Contains(rawCookiesBlob, []byte("secret_cookie_token_12345")) {
		t.Fatalf("SECURITY VIOLATION: Plaintext cookie found in database BLOB!")
	}
	if bytes.Contains(rawTokensBlob, []byte("flow_session_id_xyz")) {
		t.Fatalf("SECURITY VIOLATION: Plaintext token found in database BLOB!")
	}

	// Decrypt directly using cipher to confirm tag and encryption
	decryptedCookies, err := cipher.Decrypt(rawCookiesBlob)
	if err != nil {
		t.Fatalf("manual cipher.Decrypt failed: %v", err)
	}
	if !bytes.Contains(decryptedCookies, []byte("secret_cookie_token_12345")) {
		t.Fatalf("expected decrypted cookie blob to contain plaintext")
	}

	// 3. Get Account Secrets through Repository (transparent decryption)
	fetchedSec, err := repo.GetAccountSecrets(ctx, "acc-sec-1")
	if err != nil {
		t.Fatalf("GetAccountSecrets failed: %v", err)
	}
	if fetchedSec == nil {
		t.Fatalf("expected non-nil secrets")
	}
	if len(fetchedSec.Cookies) != 2 || fetchedSec.Cookies[0].Value != "secret_cookie_token_12345" {
		t.Errorf("cookies mismatch: %+v", fetchedSec.Cookies)
	}
	if fetchedSec.Tokens["f.sid"] != "flow_session_id_xyz" {
		t.Errorf("tokens mismatch: %+v", fetchedSec.Tokens)
	}
	if fetchedSec.CookieHash != "sha256_mock_hash" {
		t.Errorf("cookie hash mismatch: %s", fetchedSec.CookieHash)
	}

	// 4. Test explicit DeleteAccountSecrets
	if err := repo.DeleteAccountSecrets(ctx, "acc-sec-1"); err != nil {
		t.Fatalf("DeleteAccountSecrets failed: %v", err)
	}
	deletedSec, err := repo.GetAccountSecrets(ctx, "acc-sec-1")
	if err != nil {
		t.Fatalf("GetAccountSecrets after delete failed: %v", err)
	}
	if deletedSec != nil {
		t.Errorf("expected nil secrets after DeleteAccountSecrets, got %+v", deletedSec)
	}
}

func TestProvidersRepository_FeaturesEndpointsAndCascades(t *testing.T) {
	db, repo, _ := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	p := &providers.Provider{
		ID:             "flow",
		Name:           "Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com"},
		Status:         providers.ProviderStatusActive,
	}
	_ = repo.SaveProvider(ctx, p)

	acc := &providers.Account{
		ID:         "acc-cascade",
		ProviderID: "flow",
		Label:      "Cascade Test Account",
		ProfileDir: "/profiles/cascade",
	}
	_ = repo.SaveAccount(ctx, acc)

	// Save Feature
	feat := &providers.ProviderFeature{
		ID:               "flow.image.generate",
		ProviderID:       "flow",
		FeatureKey:       "image_gen",
		Name:             "Image Generation",
		ZoneType:         providers.ZoneExecution,
		DAGNodeType:      "flow.image.generate",
		ExecutionChannel: providers.ChannelDirectHTTP,
		InputsSchema:     json.RawMessage(`[{"id":"prompt","type":"string"}]`),
		OutputsSchema:    json.RawMessage(`[{"id":"image","type":"image"}]`),
		ParamsSchema:     json.RawMessage(`{}`),
	}
	if err := repo.SaveFeature(ctx, feat); err != nil {
		t.Fatalf("SaveFeature failed: %v", err)
	}

	features, err := repo.ListFeatures(ctx, "flow")
	if err != nil || len(features) != 1 {
		t.Fatalf("ListFeatures failed: %v, count: %d", err, len(features))
	}

	// Save Dynamic Endpoint
	endpoint := &providers.DynamicEndpoint{
		ID:                 "ep-1",
		ProviderID:         "flow",
		FeatureKey:         "image_gen",
		HTTPMethod:         "POST",
		URLTemplate:        "https://flow.google.com/batchGenerate",
		HeadersTemplate:    map[string]string{"Content-Type": "application/json"},
		RPCMethodName:      "/FlowService.BatchGenerateImages",
		ResolvedRPCID:      "ogiZ0b",
		ResolutionStrategy: "script_ast",
	}
	if err := repo.SaveEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("SaveEndpoint failed: %v", err)
	}

	fetchedEP, err := repo.GetEndpoint(ctx, "flow", "image_gen")
	if err != nil || fetchedEP == nil || fetchedEP.ResolvedRPCID != "ogiZ0b" {
		t.Fatalf("GetEndpoint mismatch: %v, %+v", err, fetchedEP)
	}

	// Save Cookie Partition Rule
	rule := &providers.CookiePartitionRule{
		ID:            "rule-1",
		ProviderID:    "flow",
		ZoneType:      providers.ZoneAuth,
		DomainPattern: "*.google.com",
		NamePattern:   "^(SID|HSID)$",
		IsRequired:    true,
	}
	if err := repo.SaveCookieRule(ctx, rule); err != nil {
		t.Fatalf("SaveCookieRule failed: %v", err)
	}

	rules, err := repo.ListCookieRules(ctx, "flow")
	if err != nil || len(rules) != 1 || !rules[0].IsRequired {
		t.Fatalf("ListCookieRules failed: %v, rules: %+v", err, rules)
	}

	// CASCADE TEST: Deleting provider 'flow' must delete its accounts, features, endpoints, and cookie rules
	if err := repo.DeleteProvider(ctx, "flow"); err != nil {
		t.Fatalf("DeleteProvider failed: %v", err)
	}

	// Verify accounts deleted
	accCheck, _ := repo.GetAccount(ctx, "acc-cascade")
	if accCheck != nil {
		t.Errorf("expected account to be deleted by cascade, got %+v", accCheck)
	}

	// Verify features deleted
	featsCheck, _ := repo.ListFeatures(ctx, "flow")
	if len(featsCheck) != 0 {
		t.Errorf("expected features to be deleted by cascade, got %d", len(featsCheck))
	}

	// Verify endpoints deleted
	epCheck, _ := repo.GetEndpoint(ctx, "flow", "image_gen")
	if epCheck != nil {
		t.Errorf("expected endpoint to be deleted by cascade, got %+v", epCheck)
	}

	// Verify cookie rules deleted
	rulesCheck, _ := repo.ListCookieRules(ctx, "flow")
	if len(rulesCheck) != 0 {
		t.Errorf("expected rules to be deleted by cascade, got %d", len(rulesCheck))
	}
}
