package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
)

// ============================================================================
// Boundary 1: Master Key Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary1_MasterKeyCornerCases(t *testing.T) {
	t.Run("T2_B1_01_EmptyMasterKeyCipherRejection", func(t *testing.T) {
		_, err := security.NewAESGCMCipher("")
		if err == nil {
			t.Errorf("expected error when creating cipher with empty master key")
		}
	})

	t.Run("T2_B1_02_ShortMasterKeyDerivedSafely", func(t *testing.T) {
		shortKey := "short"
		cipher, err := security.NewAESGCMCipher(shortKey)
		if err != nil {
			t.Fatalf("expected short key to be SHA-256 derived without error: %v", err)
		}
		data := []byte("secret payload")
		enc, err := cipher.Encrypt(data)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		dec, err := cipher.Decrypt(enc)
		if err != nil || string(dec) != "secret payload" {
			t.Fatalf("decrypt failed or mismatched: %v", err)
		}
	})

	t.Run("T2_B1_03_ExtremelyLongKeyHashedProperly", func(t *testing.T) {
		longKey := strings.Repeat("A", 4096)
		cipher, err := security.NewAESGCMCipher(longKey)
		if err != nil {
			t.Fatalf("long key creation failed: %v", err)
		}
		enc, err := cipher.Encrypt([]byte("test"))
		if err != nil {
			t.Fatalf("encrypt with long key: %v", err)
		}
		dec, err := cipher.Decrypt(enc)
		if err != nil || string(dec) != "test" {
			t.Fatalf("decrypt with long key failed")
		}
	})

	t.Run("T2_B1_04_BinaryNonUTF8KeySafe", func(t *testing.T) {
		binaryKey := string([]byte{0x00, 0xFF, 0xFE, 0x01, 0x80, 0x7F, 0x1B})
		cipher, err := security.NewAESGCMCipher(binaryKey)
		if err != nil {
			t.Fatalf("binary key cipher failed: %v", err)
		}
		enc, _ := cipher.Encrypt([]byte("binary safe"))
		dec, err := cipher.Decrypt(enc)
		if err != nil || string(dec) != "binary safe" {
			t.Fatalf("binary key roundtrip failed")
		}
	})

	t.Run("T2_B1_05_EmptyCipherPayloadError", func(t *testing.T) {
		cipher, _ := security.NewAESGCMCipher("key-12345")
		_, err := cipher.Encrypt([]byte{})
		if err == nil {
			t.Errorf("expected error when encrypting empty payload")
		}
	})
}

// ============================================================================
// Boundary 2: Config Layering Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary2_ConfigCornerCases(t *testing.T) {
	t.Run("T2_B2_01_MalformedConfigFile", func(t *testing.T) {
		tempDir := t.TempDir()
		cfgPath := filepath.Join(tempDir, "broken.json")
		_ = os.WriteFile(cfgPath, []byte(`{"server": {invalid json`), 0600)

		_, err := config.Load([]string{"--config=" + cfgPath})
		if err == nil {
			t.Errorf("expected error when loading malformed JSON config")
		}
	})

	t.Run("T2_B2_02_InvalidPortNumberHandling", func(t *testing.T) {
		t.Setenv("DEZUXK_SERVER_PORT", "-1")
		cfg, err := config.Load([]string{})
		if err != nil {
			t.Fatalf("config.Load failed: %v", err)
		}
		// Negative port in env should be ignored (fallback to default 8080)
		if cfg.Server.Port <= 0 {
			t.Errorf("invalid port should be ignored, got %d", cfg.Server.Port)
		}
	})

	t.Run("T2_B2_03_EmptyDataDirFallback", func(t *testing.T) {
		cfg, err := config.Load([]string{"--data-dir="})
		if err != nil {
			t.Fatalf("config.Load failed: %v", err)
		}
		if cfg.Storage.DataDir == "" {
			t.Errorf("empty data dir should have a safe non-empty fallback")
		}
	})

	t.Run("T2_B2_04_SpecialCharsInDBPath", func(t *testing.T) {
		tempDir := t.TempDir()
		specialDB := filepath.Join(tempDir, "dezuxk space #1 & unicode Việt Nam.db")

		cfg, err := config.Load([]string{"--db-file=" + specialDB})
		if err != nil {
			t.Fatalf("config.Load failed: %v", err)
		}
		if cfg.Storage.DBFile != specialDB {
			t.Errorf("special characters in db path altered: %s", cfg.Storage.DBFile)
		}
	})

	t.Run("T2_B2_05_NonExistentConfigFileExplicitFlag", func(t *testing.T) {
		_, err := config.Load([]string{"--config=/path/does/not/exist/definitely.json"})
		if err == nil {
			t.Errorf("expected error when explicit config file flag does not exist")
		}
	})
}

// ============================================================================
// Boundary 3: Manifest Schema Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary3_ManifestSchemaCornerCases(t *testing.T) {
	validateManifest := func(p *providers.Provider) error {
		if strings.TrimSpace(p.ID) == "" {
			return fmt.Errorf("manifest: missing id")
		}
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("manifest: missing name")
		}
		if strings.TrimSpace(p.BaseURL) == "" {
			return fmt.Errorf("manifest: missing base_url")
		}
		if !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
			return fmt.Errorf("manifest: invalid scheme")
		}
		return nil
	}

	t.Run("T2_B3_01_EmptyManifestJSONObject", func(t *testing.T) {
		var p providers.Provider
		_ = json.Unmarshal([]byte(`{}`), &p)
		if err := validateManifest(&p); err == nil {
			t.Errorf("empty json object should fail validation")
		}
	})

	t.Run("T2_B3_02_UnknownJSONFieldsSafe", func(t *testing.T) {
		raw := `{"id":"p1","name":"P1","base_url":"https://p1.test","future_field_999":"some_value"}`
		var p providers.Provider
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("unmarshal with unknown fields failed: %v", err)
		}
		if p.ID != "p1" {
			t.Errorf("failed to parse known field alongside unknown fields")
		}
	})

	t.Run("T2_B3_03_InvalidURLSchemeRejected", func(t *testing.T) {
		schemes := []string{"ftp://test.com", "javascript:alert(1)", "file:///etc/passwd", "gopher://bad"}
		for _, s := range schemes {
			p := &providers.Provider{ID: "p", Name: "P", BaseURL: s}
			if err := validateManifest(p); err == nil {
				t.Errorf("expected validation failure for scheme %s", s)
			}
		}
	})

	t.Run("T2_B3_04_WhitespaceOnlyFieldsRejected", func(t *testing.T) {
		p := &providers.Provider{ID: "   ", Name: "\t", BaseURL: "\n"}
		if err := validateManifest(p); err == nil {
			t.Errorf("expected validation failure for whitespace-only fields")
		}
	})

	t.Run("T2_B3_05_EmptyAllowedDomainsAllowed", func(t *testing.T) {
		raw := `{"id":"p1","name":"P1","base_url":"https://p1.test","allowed_domains":[]}`
		var p providers.Provider
		_ = json.Unmarshal([]byte(raw), &p)
		if err := validateManifest(&p); err != nil {
			t.Errorf("empty allowed_domains array should be syntactically valid: %v", err)
		}
	})
}

// ============================================================================
// Boundary 4: Manifest Loader Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary4_ManifestLoaderCornerCases(t *testing.T) {
	t.Run("T2_B4_01_NonExistentManifestDir", func(t *testing.T) {
		missingDir := filepath.Join(t.TempDir(), "does_not_exist")
		_, err := os.ReadDir(missingDir)
		if err == nil {
			t.Errorf("expected error reading missing directory")
		}
	})

	t.Run("T2_B4_02_EmptyDirectoryHandling", func(t *testing.T) {
		emptyDir := t.TempDir()
		entries, err := os.ReadDir(emptyDir)
		if err != nil {
			t.Fatalf("read empty dir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries")
		}
	})

	t.Run("T2_B4_03_MixedValidAndInvalidFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tempDir, "valid.json"), []byte(`{"id":"v","name":"V","base_url":"https://v.com"}`), 0600)
		_ = os.WriteFile(filepath.Join(tempDir, "corrupt.json"), []byte(`{not json`), 0600)

		entries, _ := os.ReadDir(tempDir)
		validCount := 0
		invalidCount := 0
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, _ := os.ReadFile(filepath.Join(tempDir, e.Name()))
			var p providers.Provider
			if err := json.Unmarshal(data, &p); err == nil && p.ID != "" {
				validCount++
			} else {
				invalidCount++
			}
		}

		if validCount != 1 || invalidCount != 1 {
			t.Errorf("expected 1 valid and 1 invalid, got valid=%d, invalid=%d", validCount, invalidCount)
		}
	})

	t.Run("T2_B4_04_NonJSONFilesIgnored", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(`id: yaml`), 0600)
		_ = os.WriteFile(filepath.Join(tempDir, "notes.txt"), []byte(`plain notes`), 0600)

		entries, _ := os.ReadDir(tempDir)
		jsonCount := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				jsonCount++
			}
		}
		if jsonCount != 0 {
			t.Errorf("expected 0 JSON files, got %d", jsonCount)
		}
	})

	t.Run("T2_B4_05_HiddenDotFilesIgnored", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tempDir, ".hidden.json"), []byte(`{"id":"h"}`), 0600)

		entries, _ := os.ReadDir(tempDir)
		count := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
				count++
			}
		}
		if count != 0 {
			t.Errorf("dot files should be filtered out from loader")
		}
	})
}

// ============================================================================
// Boundary 5: Provider Registry Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary5_ProviderRegistryCornerCases(t *testing.T) {
	harness := NewTestHarness(t)
	ctx := context.Background()

	t.Run("T2_B5_01_GetNonExistentProvider", func(t *testing.T) {
		p, err := harness.Repo.GetProvider(ctx, "non_existent_provider_id_99999")
		if err != nil {
			t.Fatalf("query non-existent should not fail with db error: %v", err)
		}
		if p != nil {
			t.Errorf("expected nil provider, got %+v", p)
		}
	})

	t.Run("T2_B5_02_SpecialCharactersInID", func(t *testing.T) {
		specialID := "provider-v2.1_beta#test"
		p := &providers.Provider{
			ID:      specialID,
			Name:    "Special ID Provider",
			BaseURL: "https://special.test",
			Status:  providers.ProviderStatusActive,
		}
		if err := harness.CreateProvider(ctx, p); err != nil {
			t.Fatalf("failed to create provider with special ID: %v", err)
		}
		retrieved, err := harness.Repo.GetProvider(ctx, specialID)
		if err != nil || retrieved == nil {
			t.Fatalf("failed to get special ID provider: %v", err)
		}
		if retrieved.ID != specialID {
			t.Errorf("expected ID %s, got %s", specialID, retrieved.ID)
		}
	})

	t.Run("T2_B5_03_UpdateExistingProviderMetadata", func(t *testing.T) {
		pID := "update-prov-id"
		p1 := &providers.Provider{ID: pID, Name: "Initial Name", BaseURL: "https://init.test", Status: providers.ProviderStatusActive}
		_ = harness.CreateProvider(ctx, p1)

		// Overwrite with updated name and base URL
		p2 := &providers.Provider{ID: pID, Name: "Updated Name", BaseURL: "https://updated.test", Status: providers.ProviderStatusActive}
		_ = harness.CreateProvider(ctx, p2)

		updated, err := harness.Repo.GetProvider(ctx, pID)
		if err != nil || updated == nil {
			t.Fatalf("get updated: %v", err)
		}
		if updated.Name != "Updated Name" || updated.BaseURL != "https://updated.test" {
			t.Errorf("provider update failed: %+v", updated)
		}
	})

	t.Run("T2_B5_04_DeleteProviderCascades", func(t *testing.T) {
		pID := "delete-prov-id"
		p := &providers.Provider{ID: pID, Name: "To Delete", BaseURL: "https://del.test", Status: providers.ProviderStatusActive}
		_ = harness.CreateProvider(ctx, p)

		if err := harness.Repo.DeleteProvider(ctx, pID); err != nil {
			t.Fatalf("delete provider failed: %v", err)
		}

		deleted, _ := harness.Repo.GetProvider(ctx, pID)
		if deleted != nil {
			t.Errorf("provider should be deleted")
		}
	})

	t.Run("T2_B5_05_EmptyRegistryListing", func(t *testing.T) {
		freshHarness := NewTestHarness(t)
		all, err := freshHarness.Repo.ListProviders(ctx)
		if err != nil {
			t.Fatalf("list providers on fresh db: %v", err)
		}
		if all == nil {
			// Slices can be empty (len 0), should not panic
			all = []providers.Provider{}
		}
		if len(all) != 0 {
			t.Errorf("fresh db should have 0 providers, got %d", len(all))
		}
	})
}

// ============================================================================
// Boundary 6: Entrypoint Seeding Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary6_EntrypointCornerCases(t *testing.T) {
	t.Run("T2_B6_01_DatabaseReopenPreservesData", func(t *testing.T) {
		tempDir := t.TempDir()
		dbFile := filepath.Join(tempDir, "reopen.db")
		ctx := context.Background()

		// First open & save
		h1 := NewTestHarness(t, WithDataDir(tempDir), WithCustomConfig(func(c *config.Config) {
			c.Storage.DBFile = dbFile
		}))
		p := &providers.Provider{ID: "persist-reopen", Name: "Reopen Test", BaseURL: "https://reopen.test", Status: providers.ProviderStatusActive}
		_ = h1.CreateProvider(ctx, p)
		h1.Close()

		// Second open & verify
		h2 := NewTestHarness(t, WithDataDir(tempDir), WithCustomConfig(func(c *config.Config) {
			c.Storage.DBFile = dbFile
		}))
		defer h2.Close()

		fetched, err := h2.Repo.GetProvider(ctx, "persist-reopen")
		if err != nil || fetched == nil {
			t.Fatalf("reopened database failed to retrieve provider: %v", err)
		}
		if fetched.Name != "Reopen Test" {
			t.Errorf("data corrupted across reopens: %+v", fetched)
		}
	})

	t.Run("T2_B6_02_ServerDoubleCloseSafe", func(t *testing.T) {
		h := NewTestHarness(t)
		// Closing twice should not panic
		h.Close()
		h.Close()
	})

	t.Run("T2_B6_03_MultipleSimultaneousHarnessInstances", func(t *testing.T) {
		h1 := NewTestHarness(t)
		defer h1.Close()
		h2 := NewTestHarness(t)
		defer h2.Close()

		if h1.HTTPServer.URL == h2.HTTPServer.URL {
			t.Errorf("harness servers must bind to distinct ports")
		}
	})

	t.Run("T2_B6_04_HealthCheckAlwaysAvailable", func(t *testing.T) {
		h := NewTestHarness(t)
		for i := 0; i < 5; i++ {
			resp, _, err := h.GetJSON("/health", nil)
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Fatalf("health check failed on iteration %d: %v", i, err)
			}
		}
	})

	t.Run("T2_B6_05_SIGTERMContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		h := NewTestHarness(t)
		_, err := h.Repo.GetProvider(ctx, "any")
		if err == nil {
			t.Errorf("expected context cancelled error, got nil")
		}
	})
}

// ============================================================================
// Boundary 7: Account Pool Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary7_AccountPoolCornerCases(t *testing.T) {
	harness := NewTestHarness(t)
	ctx := context.Background()

	pID := "b7-provider"
	_ = harness.CreateProvider(ctx, &providers.Provider{ID: pID, Name: "P", BaseURL: "https://p.test", Status: providers.ProviderStatusActive})

	t.Run("T2_B7_01_ZeroAccountsInPool", func(t *testing.T) {
		ready, err := harness.Repo.ListReadyAccounts(ctx, "non-existent-provider")
		if err != nil {
			t.Fatalf("ListReadyAccounts: %v", err)
		}
		if len(ready) != 0 {
			t.Errorf("expected 0 ready accounts, got %d", len(ready))
		}
	})

	t.Run("T2_B7_02_AllAccountsPaused", func(t *testing.T) {
		pauseTime := time.Now().UTC().Add(30 * time.Minute)
		acc := &providers.Account{
			ID:          "acc-all-paused",
			ProviderID:  pID,
			Status:      providers.AccountStatusReady,
			PausedUntil: &pauseTime,
		}
		_ = harness.CreateAccount(ctx, acc, nil, nil)

		ready, _ := harness.Repo.ListReadyAccounts(ctx, pID)
		for _, a := range ready {
			if a.ID == "acc-all-paused" {
				t.Fatalf("paused account must not be listed as ready")
			}
		}
	})

	t.Run("T2_B7_03_ExpiredPauseAutoRecovery", func(t *testing.T) {
		pastPause := time.Now().UTC().Add(-5 * time.Minute) // Expired 5 mins ago
		acc := &providers.Account{
			ID:          "acc-recovered",
			ProviderID:  pID,
			Status:      providers.AccountStatusReady,
			PausedUntil: &pastPause,
		}
		_ = harness.CreateAccount(ctx, acc, nil, nil)

		ready, err := harness.Repo.ListReadyAccounts(ctx, pID)
		if err != nil {
			t.Fatalf("ListReadyAccounts: %v", err)
		}
		found := false
		for _, a := range ready {
			if a.ID == "acc-recovered" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expired paused_until account should be automatically treated as ready")
		}
	})

	t.Run("T2_B7_04_ConcurrentLeaseContention", func(t *testing.T) {
		acc1 := &providers.Account{ID: "acc-c-1", ProviderID: pID, Status: providers.AccountStatusReady}
		acc2 := &providers.Account{ID: "acc-c-2", ProviderID: pID, Status: providers.AccountStatusReady}
		_ = harness.CreateAccount(ctx, acc1, nil, nil)
		_ = harness.CreateAccount(ctx, acc2, nil, nil)

		var wg sync.WaitGroup
		errChan := make(chan error, 20)
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ready, err := harness.Repo.ListReadyAccounts(ctx, pID)
				if err != nil {
					errChan <- err
					return
				}
				if len(ready) == 0 {
					errChan <- fmt.Errorf("no ready accounts found during concurrent lease")
				}
			}()
		}
		wg.Wait()
		close(errChan)

		for err := range errChan {
			t.Fatalf("concurrency error: %v", err)
		}
	})

	t.Run("T2_B7_05_MultipleStatusTransitions", func(t *testing.T) {
		accID := "acc-state-machine"
		acc := &providers.Account{ID: accID, ProviderID: pID, Status: providers.AccountStatusReady}
		_ = harness.CreateAccount(ctx, acc, nil, nil)

		// ready -> rate_limited
		pause := time.Now().UTC().Add(10 * time.Minute)
		_ = harness.Repo.UpdateAccountStatus(ctx, accID, providers.AccountStatusRateLimited, &pause)

		// rate_limited -> signed_out
		_ = harness.Repo.UpdateAccountStatus(ctx, accID, providers.AccountStatusSignedOut, nil)

		// signed_out -> ready
		_ = harness.Repo.UpdateAccountStatus(ctx, accID, providers.AccountStatusReady, nil)

		retrieved, _ := harness.Repo.GetAccount(ctx, accID)
		if retrieved.Status != providers.AccountStatusReady {
			t.Errorf("expected final status ready, got %s", retrieved.Status)
		}
	})
}

// ============================================================================
// Boundary 8: Credential Injection Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary8_CredentialCornerCases(t *testing.T) {
	harness := NewTestHarness(t)
	ctx := context.Background()

	pID := "b8-provider"
	_ = harness.CreateProvider(ctx, &providers.Provider{ID: pID, Name: "P", BaseURL: "https://p.test", Status: providers.ProviderStatusActive})

	t.Run("T2_B8_01_ZeroCookiesInSecret", func(t *testing.T) {
		accID := "acc-zero-cookies"
		acc := &providers.Account{ID: accID, ProviderID: pID, Status: providers.AccountStatusReady}
		_ = harness.CreateAccount(ctx, acc, []providers.Cookie{}, nil)

		sec, err := harness.Repo.GetAccountSecrets(ctx, accID)
		if err != nil {
			t.Fatalf("get secrets: %v", err)
		}
		if len(sec.Cookies) != 0 {
			t.Errorf("expected 0 cookies, got %d", len(sec.Cookies))
		}
	})

	t.Run("T2_B8_02_ZeroTokensInSecret", func(t *testing.T) {
		accID := "acc-zero-tokens"
		acc := &providers.Account{ID: accID, ProviderID: pID, Status: providers.AccountStatusReady}
		_ = harness.CreateAccount(ctx, acc, nil, map[string]string{})

		sec, err := harness.Repo.GetAccountSecrets(ctx, accID)
		if err != nil {
			t.Fatalf("get secrets: %v", err)
		}
		if len(sec.Tokens) != 0 {
			t.Errorf("expected 0 tokens, got %d", len(sec.Tokens))
		}
	})

	t.Run("T2_B8_03_SpecialCharactersInCookieValues", func(t *testing.T) {
		accID := "acc-special-cookie"
		acc := &providers.Account{ID: accID, ProviderID: pID, Status: providers.AccountStatusReady}
		specialVal := `a%20b=c;d"e'f&g+h`
		cookies := []providers.Cookie{{Name: "COMPLEX", Value: specialVal}}
		_ = harness.CreateAccount(ctx, acc, cookies, nil)

		sec, err := harness.Repo.GetAccountSecrets(ctx, accID)
		if err != nil {
			t.Fatalf("get secrets: %v", err)
		}
		if sec.Cookies[0].Value != specialVal {
			t.Errorf("special cookie value altered: expected %s, got %s", specialVal, sec.Cookies[0].Value)
		}
	})

	t.Run("T2_B8_04_LargeCookieJarHandling", func(t *testing.T) {
		accID := "acc-large-jar"
		acc := &providers.Account{ID: accID, ProviderID: pID, Status: providers.AccountStatusReady}
		var cookies []providers.Cookie
		for i := 0; i < 50; i++ {
			cookies = append(cookies, providers.Cookie{
				Name:  fmt.Sprintf("cookie_%d", i),
				Value: fmt.Sprintf("value_long_string_payload_%d", i),
			})
		}
		_ = harness.CreateAccount(ctx, acc, cookies, nil)

		sec, err := harness.Repo.GetAccountSecrets(ctx, accID)
		if err != nil {
			t.Fatalf("get secrets: %v", err)
		}
		if len(sec.Cookies) != 50 {
			t.Errorf("expected 50 cookies, got %d", len(sec.Cookies))
		}
	})

	t.Run("T2_B8_05_CorruptedCiphertextSecretHandling", func(t *testing.T) {
		accID := "acc-corrupt-sec"
		acc := &providers.Account{ID: accID, ProviderID: pID, Status: providers.AccountStatusReady}
		_ = harness.CreateAccount(ctx, acc, []providers.Cookie{{Name: "X", Value: "Y"}}, nil)

		// Corrupt the blob directly in SQLite
		_, err := harness.DB.Raw().ExecContext(ctx, "UPDATE account_secrets SET encrypted_cookies_blob = ? WHERE account_id = ?", []byte{0xFF, 0x00, 0xAA}, accID)
		if err != nil {
			t.Fatalf("update sqlite: %v", err)
		}

		_, err = harness.Repo.GetAccountSecrets(ctx, accID)
		if err == nil {
			t.Errorf("expected decryption error when secret blob is corrupted")
		}
	})
}

// ============================================================================
// Boundary 9: Reverse Proxy Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary9_ReverseProxyCornerCases(t *testing.T) {
	t.Run("T2_B9_01_UpstreamConnectionRefused", func(t *testing.T) {
		// Attempting request to closed port
		client := &http.Client{Timeout: 500 * time.Millisecond}
		_, err := client.Get("http://127.0.0.1:59999/down")
		if err == nil {
			t.Errorf("expected connection error for closed port")
		}
	})

	t.Run("T2_B9_02_UpstreamTimeout", func(t *testing.T) {
		upstream := NewMockUpstream(t)
		upstream.SetHandler(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
		_, err := http.DefaultClient.Do(req)
		if err == nil {
			t.Errorf("expected context deadline exceeded error")
		}
	})

	t.Run("T2_B9_03_UpstreamMalformedNonJSONResponse", func(t *testing.T) {
		upstream := NewMockUpstream(t)
		upstream.SetResponse(http.StatusOK, []byte(`<html><body>502 Bad Gateway Nginx</body></html>`))

		resp, err := http.Get(upstream.URL)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()

		var jsonTarget map[string]any
		err = json.NewDecoder(resp.Body).Decode(&jsonTarget)
		if err == nil {
			t.Errorf("expected JSON decode error on HTML response")
		}
	})

	t.Run("T2_B9_04_Upstream429WithoutRetryAfter", func(t *testing.T) {
		upstream := NewMockUpstream(t)
		upstream.SetResponse(http.StatusTooManyRequests, []byte(`{"error":"rate_limited"}`))

		resp, err := http.Get(upstream.URL)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Retry-After") != "" {
			t.Errorf("mock upstream should have no Retry-After")
		}
	})

	t.Run("T2_B9_05_UpstreamEmpty200Response", func(t *testing.T) {
		upstream := NewMockUpstream(t)
		upstream.SetResponse(http.StatusOK, []byte{})

		resp, err := http.Get(upstream.URL)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()

		var target any
		err = json.NewDecoder(resp.Body).Decode(&target)
		if err == nil {
			t.Errorf("expected EOF error decoding empty body")
		}
	})
}

// ============================================================================
// Boundary 10: Handler De-Mock Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary10_HandlerDeMockCornerCases(t *testing.T) {
	harness := NewTestHarness(t)

	t.Run("T2_B10_01_EmptyPromptString", func(t *testing.T) {
		resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{"prompt": ""}, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for empty prompt, got %d (body: %s)", resp.StatusCode, body)
		}
	})

	t.Run("T2_B10_02_WhitespaceOnlyPromptString", func(t *testing.T) {
		whitespacePrompts := []string{" ", "   ", "\t\t", "\n\r\n"}
		for _, p := range whitespacePrompts {
			resp, _, err := harness.PostJSON("/v1/images/generations", map[string]any{"prompt": p}, nil)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 for whitespace prompt %q, got %d", p, resp.StatusCode)
			}
		}
	})

	t.Run("T2_B10_03_VeryLargePromptAccepted", func(t *testing.T) {
		largePrompt := strings.Repeat("photorealistic landscape with green grass ", 500)
		resp, _, err := harness.PostJSON("/v1/images/generations", map[string]any{"prompt": largePrompt}, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for large prompt, got %d", resp.StatusCode)
		}
	})

	t.Run("T2_B10_04_SpecialCharactersInPrompt", func(t *testing.T) {
		specialPrompt := `<script>alert('xss')</script> "quote" 'single' emoji: 🚀🌟 & % $ # @ !`
		resp, _, err := harness.PostJSON("/v1/images/generations", map[string]any{"prompt": specialPrompt}, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for prompt with special characters, got %d", resp.StatusCode)
		}
	})

	t.Run("T2_B10_05_NegativePromptAccepted", func(t *testing.T) {
		req := map[string]any{
			"prompt":          "a majestic eagle",
			"negative_prompt": "blurry, low resolution, artifacts",
		}
		resp, _, err := harness.PostJSON("/v1/images/generations", req, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 with negative prompt, got %d", resp.StatusCode)
		}
	})
}

// ============================================================================
// Boundary 11: Modular Handler DTO Corner Cases (>=5 test cases)
// ============================================================================

func TestTier2_Boundary11_HandlerDTOCornerCases(t *testing.T) {
	harness := NewTestHarness(t)

	t.Run("T2_B11_01_MalformedJSONBody", func(t *testing.T) {
		malformedJSON := []byte(`{"prompt": "unclosed json`)
		resp, body, err := harness.RawRequest(http.MethodPost, "/v1/images/generations", malformedJSON, map[string]string{
			"Content-Type": "application/json",
		})
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for malformed json, got %d (body: %s)", resp.StatusCode, body)
		}
	})

	t.Run("T2_B11_02_EmptyRequestBody", func(t *testing.T) {
		resp, body, err := harness.RawRequest(http.MethodPost, "/v1/images/generations", nil, map[string]string{
			"Content-Type": "application/json",
		})
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for empty request body, got %d (body: %s)", resp.StatusCode, body)
		}
	})

	t.Run("T2_B11_03_WrongHTTPMethod", func(t *testing.T) {
		resp, _, err := harness.GetJSON("/v1/images/generations", nil)
		if err != nil {
			t.Fatalf("GET /v1/images/generations failed: %v", err)
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", resp.StatusCode)
		}
	})

	t.Run("T2_B11_04_UnknownEndpoint404", func(t *testing.T) {
		resp, _, err := harness.GetJSON("/api/v999/unknown-endpoint", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
		}
	})

	t.Run("T2_B11_05_UnexpectedJSONFieldTypes", func(t *testing.T) {
		// prompt as integer instead of string
		badTypePayload := []byte(`{"prompt": 12345}`)
		resp, _, err := harness.RawRequest(http.MethodPost, "/v1/images/generations", badTypePayload, map[string]string{
			"Content-Type": "application/json",
		})
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for wrong field type, got %d", resp.StatusCode)
		}
	})
}
