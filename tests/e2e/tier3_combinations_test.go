package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
	"dezuxk/internal/server"
	"dezuxk/internal/storage/sqlite"
)

// ============================================================================
// Tier 3: Cross-Feature Combinations (Pairwise & Multi-Feature Interactions)
// ============================================================================

func TestTier3_CrossFeatureCombinations(t *testing.T) {
	t.Run("T3_C1_AuthAndPoolAndProxy", func(t *testing.T) {
		// Combination 1: API Key Auth Gate -> Account Pool Selection -> Secret Decryption
		apiKey := "admin-prod-api-key-999"
		harness := NewTestHarness(t, WithAPIKey(apiKey))
		ctx := context.Background()

		provID := "auth-pool-p"
		_ = harness.CreateProvider(ctx, &providers.Provider{
			ID:         provID,
			Name:       "Auth Pool Provider",
			BaseURL:    "https://authpool.test",
			Status:     providers.ProviderStatusActive,
			AuthDomain: ".authpool.test",
		})

		accID := "acc-auth-pool-1"
		acc := &providers.Account{ID: accID, ProviderID: provID, Status: providers.AccountStatusReady}
		cookies := []providers.Cookie{{Name: "SESSION", Value: "valid_session_cookie", Domain: ".authpool.test", Path: "/"}}
		tokens := map[string]string{"bearer": "upstream_token_123"}
		if err := harness.CreateAccount(ctx, acc, cookies, tokens); err != nil {
			t.Fatalf("create account: %v", err)
		}

		// 1. Request without gateway auth token -> Rejected 401
		resp, _, err := harness.PostJSON("/v1/images/generations", map[string]any{
			"prompt":   "test prompt",
			"provider": provID,
		}, nil)
		if err != nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized without API key, got %d", resp.StatusCode)
		}

		// 2. Request with valid gateway auth token -> Success 200 and leased account
		resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{
			"prompt":   "test prompt",
			"provider": provID,
		}, map[string]string{
			"Authorization": "Bearer " + apiKey,
		})
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 with valid API key, got %d (body: %s)", resp.StatusCode, body)
		}

		var genResp server.ImageGenerationResponse
		if err := json.Unmarshal(body, &genResp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if genResp.AccountID != accID {
			t.Errorf("expected selected account %s, got %s", accID, genResp.AccountID)
		}

		// 3. Confirm secret remains decryptable
		sec, err := harness.Repo.GetAccountSecrets(ctx, genResp.AccountID)
		if err != nil || sec.Cookies[0].Value != "valid_session_cookie" {
			t.Errorf("account secrets corrupted or missing after request dispatch")
		}
	})

	t.Run("T3_C2_ManifestAndRegistryAndProxy", func(t *testing.T) {
		// Combination 2: Manifest Loading -> Registry Sync -> Dynamic Upstream Routing
		upstream := NewMockUpstream(t)
		upstream.SetResponse(http.StatusOK, []byte(`{"status":"success","url":"https://storage.cdn/img.png"}`))

		manifestJSON := fmt.Sprintf(`{
			"id": "manifest-prov",
			"name": "Dynamic Manifest Provider",
			"base_url": %q,
			"auth_domain": ".test",
			"status": "active"
		}`, upstream.URL)

		var p providers.Provider
		if err := json.Unmarshal([]byte(manifestJSON), &p); err != nil {
			t.Fatalf("unmarshal manifest: %v", err)
		}

		harness := NewTestHarness(t)
		ctx := context.Background()
		if err := harness.CreateProvider(ctx, &p); err != nil {
			t.Fatalf("register manifest provider: %v", err)
		}

		// Verify provider is resolvable in registry
		retrieved, err := harness.Repo.GetProvider(ctx, "manifest-prov")
		if err != nil || retrieved == nil {
			t.Fatalf("provider not found in registry: %v", err)
		}
		if retrieved.BaseURL != upstream.URL {
			t.Errorf("expected base_url %s, got %s", upstream.URL, retrieved.BaseURL)
		}

		// Send request targeting this dynamically registered provider
		resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{
			"prompt":   "dynamic upstream prompt",
			"provider": "manifest-prov",
		}, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("dispatch to manifest provider failed: %v (body: %s)", err, body)
		}
	})

	t.Run("T3_C3_PoolRotationAndRateLimitFailover", func(t *testing.T) {
		// Combination 3: Pool Rotation + Rate-Limit Handling (429) + Automatic Account Failover
		harness := NewTestHarness(t)
		ctx := context.Background()

		provID := "failover-pool-prov"
		_ = harness.CreateProvider(ctx, &providers.Provider{ID: provID, Name: "P", BaseURL: "https://p.test", Status: providers.ProviderStatusActive})

		// 3 Accounts in pool
		acc1 := &providers.Account{ID: "acc-1", ProviderID: provID, Status: providers.AccountStatusReady}
		acc2 := &providers.Account{ID: "acc-2", ProviderID: provID, Status: providers.AccountStatusReady}
		acc3 := &providers.Account{ID: "acc-3", ProviderID: provID, Status: providers.AccountStatusReady}

		_ = harness.CreateAccount(ctx, acc1, nil, nil)
		_ = harness.CreateAccount(ctx, acc2, nil, nil)
		_ = harness.CreateAccount(ctx, acc3, nil, nil)

		// Initially all 3 accounts are ready
		ready, _ := harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 3 {
			t.Fatalf("expected 3 ready accounts, got %d", len(ready))
		}

		// Simulate account 1 hitting rate-limit 429
		pause1 := time.Now().UTC().Add(10 * time.Minute)
		_ = harness.Repo.UpdateAccountStatus(ctx, "acc-1", providers.AccountStatusRateLimited, &pause1)

		// Ready accounts now 2: acc-2 and acc-3
		ready, _ = harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 2 {
			t.Fatalf("expected 2 ready accounts after acc-1 rate limit, got %d", len(ready))
		}
		if ready[0].ID != "acc-2" {
			t.Errorf("expected acc-2 next, got %s", ready[0].ID)
		}

		// Simulate account 2 hitting auth expired 401
		_ = harness.Repo.UpdateAccountStatus(ctx, "acc-2", providers.AccountStatusSignedOut, nil)

		// Ready accounts now 1: acc-3
		ready, _ = harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 1 || ready[0].ID != "acc-3" {
			t.Fatalf("expected only acc-3 ready after acc-2 sign out, got %+v", ready)
		}

		// Generation request selects the remaining ready account
		resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{
			"prompt":   "failover test prompt",
			"provider": provID,
		}, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("generation failed: %v", err)
		}
		var genResp server.ImageGenerationResponse
		_ = json.Unmarshal(body, &genResp)
		if genResp.AccountID != "acc-3" {
			t.Errorf("expected failover to select acc-3, got %s", genResp.AccountID)
		}
	})

	t.Run("T3_C4_ConfigPrecedenceAndMasterKeyAndCrypto", func(t *testing.T) {
		// Combination 4: Config Layering Precedence -> Dynamic Master Key Resolution -> SQLite Secret Encryption
		tempDir := t.TempDir()
		cfgFile := filepath.Join(tempDir, "dezuxk.json")
		dbFile := filepath.Join(tempDir, "secure.db")

		// JSON file sets key A
		keyA := "key-from-config-file-32b-length!"
		_ = os.WriteFile(cfgFile, []byte(fmt.Sprintf(`{"security":{"master_key":%q},"storage":{"db_file":%q}}`, keyA, dbFile)), 0600)

		// ENV sets key B (should override JSON file)
		keyB := "key-from-environment-32b-length!"
		t.Setenv("DEZUXK_SECURITY_MASTER_KEY", keyB)

		// CLI flag sets key C (highest precedence)
		keyC := "key-from-cli-flag-32b-length-ok!"
		cfg, err := config.Load([]string{"--config=" + cfgFile, "--master-key=" + keyC})
		if err != nil {
			t.Fatalf("load config: %v", err)
		}

		if cfg.Security.MasterKey != keyC {
			t.Fatalf("expected CLI flag master key %q, got %q", keyC, cfg.Security.MasterKey)
		}

		// Initialize cipher and repository with resolved key
		cipher, err := security.NewAdaptiveCipher(cfg.Security.MasterKey, false)
		if err != nil {
			t.Fatalf("cipher init: %v", err)
		}

		ctx := context.Background()
		db, err := sqlite.Open(ctx, sqlite.Options{Path: dbFile})
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()

		repo := sqlite.NewProvidersRepository(db, cipher)

		prov := &providers.Provider{ID: "p", Name: "Prov P", BaseURL: "https://p.test", Status: providers.ProviderStatusActive}
		if err := repo.SaveProvider(ctx, prov); err != nil {
			t.Fatalf("save provider: %v", err)
		}

		// Save account with secret encrypted with keyC
		acc := &providers.Account{ID: "acc-crypto-combo", ProviderID: "p", Status: providers.AccountStatusReady}
		if err := repo.SaveAccount(ctx, acc); err != nil {
			t.Fatalf("save account: %v", err)
		}
		sec := &providers.AccountSecrets{
			AccountID:  "acc-crypto-combo",
			Cookies:    []providers.Cookie{{Name: "COMBO_COOKIE", Value: "COMBO_VAL"}},
			Tokens:     map[string]string{"COMBO_TOKEN": "TOKEN_VAL"},
			CookieHash: "hash123",
			UpdatedAt:  time.Now().UTC(),
		}
		if err := repo.SaveAccountSecrets(ctx, sec); err != nil {
			t.Fatalf("save secrets: %v", err)
		}

		// Decrypt with keyC -> succeeds
		retrieved, err := repo.GetAccountSecrets(ctx, "acc-crypto-combo")
		if err != nil || retrieved.Cookies[0].Value != "COMBO_VAL" {
			t.Fatalf("failed to decrypt with correct keyC: %v", err)
		}

		// Attempting decrypt with keyA -> must fail
		wrongCipher, _ := security.NewAdaptiveCipher(keyA, false)
		wrongRepo := sqlite.NewProvidersRepository(db, wrongCipher)
		_, err = wrongRepo.GetAccountSecrets(ctx, "acc-crypto-combo")
		if err == nil {
			t.Fatalf("expected decryption failure with mismatched keyA")
		}
	})

	t.Run("T3_C5_ProviderHealthAndPoolAccounting", func(t *testing.T) {
		// Combination 5: Provider Health Monitoring + Dynamic Account Readiness under Active Updates
		harness := NewTestHarness(t)
		ctx := context.Background()

		provID := "health-pool-combo"
		_ = harness.CreateProvider(ctx, &providers.Provider{ID: provID, Name: "HP", BaseURL: "https://hp.test", Status: providers.ProviderStatusActive})

		// Add 5 accounts
		for i := 1; i <= 5; i++ {
			acc := &providers.Account{
				ID:         fmt.Sprintf("health-acc-%d", i),
				ProviderID: provID,
				Status:     providers.AccountStatusReady,
			}
			_ = harness.CreateAccount(ctx, acc, nil, nil)
		}

		// Verify health endpoint reports 5 ready, 5 total
		resp, body, err := harness.GetJSON("/health", nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("health check failed: %v", err)
		}
		var hr server.HealthResponse
		_ = json.Unmarshal(body, &hr)
		if hr.Accounts.Ready != 5 || hr.Accounts.Total != 5 {
			t.Errorf("expected 5 ready, 5 total; got ready=%d, total=%d", hr.Accounts.Ready, hr.Accounts.Total)
		}

		// Pause 2 accounts
		pauseTime := time.Now().UTC().Add(15 * time.Minute)
		_ = harness.Repo.UpdateAccountStatus(ctx, "health-acc-1", providers.AccountStatusRateLimited, &pauseTime)
		_ = harness.Repo.UpdateAccountStatus(ctx, "health-acc-2", providers.AccountStatusSignedOut, nil)

		// Query health endpoint again -> should report 3 ready, 5 total
		_, body, _ = harness.GetJSON("/health", nil)
		_ = json.Unmarshal(body, &hr)
		if hr.Accounts.Ready != 3 || hr.Accounts.Total != 5 {
			t.Errorf("expected 3 ready, 5 total after status updates; got ready=%d, total=%d", hr.Accounts.Ready, hr.Accounts.Total)
		}
	})

	t.Run("T3_C6_PanicRecoveryAndErrorPropagation", func(t *testing.T) {
		// Combination 6: Middleware Recoverer handles bad requests and errors without server crash
		harness := NewTestHarness(t)

		// 1. Send invalid JSON payload
		resp1, body1, err := harness.RawRequest(http.MethodPost, "/v1/images/generations", []byte("{malformed"), map[string]string{
			"Content-Type": "application/json",
		})
		if err != nil || resp1.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for malformed json, got %d (body: %s)", resp1.StatusCode, body1)
		}

		// 2. Server remains fully healthy immediately after bad request
		resp2, _, err := harness.GetJSON("/health", nil)
		if err != nil || resp2.StatusCode != http.StatusOK {
			t.Errorf("server crashed or unhealthy after malformed request: %v", err)
		}
	})

	t.Run("T3_C7_MultiProviderConcurrentDispatch", func(t *testing.T) {
		// Combination 7: Multi-Provider Concurrent Dispatch
		harness := NewTestHarness(t)
		ctx := context.Background()

		// Register 2 distinct providers
		_ = harness.CreateProvider(ctx, &providers.Provider{ID: "provider-alpha", Name: "Alpha", BaseURL: "https://alpha.test", Status: providers.ProviderStatusActive})
		_ = harness.CreateProvider(ctx, &providers.Provider{ID: "provider-beta", Name: "Beta", BaseURL: "https://beta.test", Status: providers.ProviderStatusActive})

		_ = harness.CreateAccount(ctx, &providers.Account{ID: "acc-alpha", ProviderID: "provider-alpha", Status: providers.AccountStatusReady}, nil, nil)
		_ = harness.CreateAccount(ctx, &providers.Account{ID: "acc-beta", ProviderID: "provider-beta", Status: providers.AccountStatusReady}, nil, nil)

		var wg sync.WaitGroup
		var successAlpha, successBeta atomic.Int32

		for i := 0; i < 10; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{
					"prompt":   "prompt alpha",
					"provider": "provider-alpha",
				}, nil)
				if err == nil && resp.StatusCode == http.StatusOK {
					var r server.ImageGenerationResponse
					if json.Unmarshal(body, &r) == nil && r.Provider == "provider-alpha" {
						successAlpha.Add(1)
					}
				}
			}()

			go func() {
				defer wg.Done()
				resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{
					"prompt":   "prompt beta",
					"provider": "provider-beta",
				}, nil)
				if err == nil && resp.StatusCode == http.StatusOK {
					var r server.ImageGenerationResponse
					if json.Unmarshal(body, &r) == nil && r.Provider == "provider-beta" {
						successBeta.Add(1)
					}
				}
			}()
		}
		wg.Wait()

		if successAlpha.Load() != 10 {
			t.Errorf("expected 10 alpha successes, got %d", successAlpha.Load())
		}
		if successBeta.Load() != 10 {
			t.Errorf("expected 10 beta successes, got %d", successBeta.Load())
		}
	})
}
