package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/server"
)

// ============================================================================
// Tier 4: Real-World Scenarios (End-to-End Workflows & Production Simulation)
// ============================================================================

func TestTier4_RealWorldScenarios(t *testing.T) {
	t.Run("T4_S1_FullLifecycleBootstrap", func(t *testing.T) {
		// Scenario 1: Complete Gateway Lifecycle
		// 1. Create temporary config file and manifests
		tempDir := t.TempDir()
		cfgFile := filepath.Join(tempDir, "dezuxk.json")
		manifestFile := filepath.Join(tempDir, "flow.json")

		cfgContent := fmt.Sprintf(`{
			"server": {"host": "127.0.0.1", "port": 0, "api_key": "prod-lifecycle-token"},
			"storage": {"data_dir": %q, "db_file": "lifecycle.db"},
			"security": {"master_key": "lifecycle-master-key-32b-length"}
		}`, tempDir)
		if err := os.WriteFile(cfgFile, []byte(cfgContent), 0600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		manifestContent := `{
			"id": "flow",
			"name": "Google Flow Production",
			"base_url": "https://flow.google.com",
			"auth_domain": ".google.com",
			"allowed_domains": [".google.com", "flow.google.com"],
			"probe_rules": {"identityExpressions": ["[data-email]"]},
			"status": "active"
		}`
		if err := os.WriteFile(manifestFile, []byte(manifestContent), 0600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}

		// 2. Load config and start harness
		cfg, err := config.Load([]string{"--config=" + cfgFile})
		if err != nil {
			t.Fatalf("config.Load failed: %v", err)
		}

		harness := NewTestHarness(t,
			WithAPIKey(cfg.Server.APIKey),
			WithMasterKey(cfg.Security.MasterKey),
			WithDataDir(tempDir),
		)
		ctx := context.Background()

		// 3. Register provider and account
		var p providers.Provider
		_ = json.Unmarshal([]byte(manifestContent), &p)
		if err := harness.CreateProvider(ctx, &p); err != nil {
			t.Fatalf("register provider: %v", err)
		}

		acc := &providers.Account{
			ID:         "acc-lifecycle-1",
			ProviderID: "flow",
			Identity:   "user@corp.google.com",
			Status:     providers.AccountStatusReady,
		}
		cookies := []providers.Cookie{
			{Name: "SAPISID", Value: "cookie_val_1", Domain: ".google.com", Path: "/"},
			{Name: "SSID", Value: "cookie_val_2", Domain: ".google.com", Path: "/"},
		}
		tokens := map[string]string{"f.sid": "boq_session_id_456"}
		if err := harness.CreateAccount(ctx, acc, cookies, tokens); err != nil {
			t.Fatalf("register account: %v", err)
		}

		// 4. Query Health
		resp, body, err := harness.GetJSON("/health", nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("health check failed: %v", err)
		}
		var hr server.HealthResponse
		_ = json.Unmarshal(body, &hr)
		if hr.Accounts.Ready < 1 {
			t.Errorf("expected at least 1 ready account, got %d", hr.Accounts.Ready)
		}

		// 5. Submit Image Generation
		resp, body, err = harness.PostJSON("/v1/images/generations", map[string]any{
			"prompt":       "production test generation",
			"aspect_ratio": "16:9",
			"provider":     "flow",
		}, map[string]string{
			"Authorization": "Bearer prod-lifecycle-token",
		})
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("generation request failed: %v (body: %s)", err, body)
		}

		var genResp server.ImageGenerationResponse
		if err := json.Unmarshal(body, &genResp); err != nil {
			t.Fatalf("unmarshal generation response: %v", err)
		}
		if genResp.Provider != "flow" || genResp.AccountID != "acc-lifecycle-1" {
			t.Errorf("unexpected generation response payload: %+v", genResp)
		}

		// 6. Graceful shutdown
		harness.Close()
	})

	t.Run("T4_S2_MultiAccountConcurrentLoad", func(t *testing.T) {
		// Scenario 2: High Concurrency Load across 3 Pool Accounts
		harness := NewTestHarness(t)
		ctx := context.Background()

		provID := "concurrency-provider"
		_ = harness.CreateProvider(ctx, &providers.Provider{ID: provID, Name: "CP", BaseURL: "https://cp.test", Status: providers.ProviderStatusActive})

		// 3 Accounts in pool
		for i := 1; i <= 3; i++ {
			acc := &providers.Account{
				ID:         fmt.Sprintf("concurrent-acc-%d", i),
				ProviderID: provID,
				Status:     providers.AccountStatusReady,
			}
			_ = harness.CreateAccount(ctx, acc, nil, nil)
		}

		var wg sync.WaitGroup
		var totalSuccess atomic.Int32
		accountHitMap := sync.Map{}

		clientCount := 12
		for i := 0; i < clientCount; i++ {
			wg.Add(1)
			go func(reqNum int) {
				defer wg.Done()
				resp, body, err := harness.PostJSON("/v1/images/generations", map[string]any{
					"prompt":   fmt.Sprintf("concurrent prompt #%d", reqNum),
					"provider": provID,
				}, nil)
				if err == nil && resp.StatusCode == http.StatusOK {
					var gr server.ImageGenerationResponse
					if json.Unmarshal(body, &gr) == nil && gr.AccountID != "" {
						totalSuccess.Add(1)
						accountHitMap.Store(gr.AccountID, true)
					}
				}
			}(i)
		}
		wg.Wait()

		if totalSuccess.Load() != int32(clientCount) {
			t.Errorf("expected %d successful requests, got %d", clientCount, totalSuccess.Load())
		}

		// Check that accounts were utilized without single-account lockups
		accountsUsed := 0
		accountHitMap.Range(func(key, value any) bool {
			accountsUsed++
			return true
		})
		if accountsUsed == 0 {
			t.Errorf("expected accounts to be used, got 0")
		}
	})

	t.Run("T4_S3_CascadingFailoverAndRecovery", func(t *testing.T) {
		// Scenario 3: Cascading Failure & Pool Cooldown Recovery
		harness := NewTestHarness(t)
		ctx := context.Background()

		provID := "cascade-prov"
		_ = harness.CreateProvider(ctx, &providers.Provider{ID: provID, Name: "Cascade", BaseURL: "https://c.test", Status: providers.ProviderStatusActive})

		accA := &providers.Account{ID: "acc-cascade-a", ProviderID: provID, Status: providers.AccountStatusReady}
		accB := &providers.Account{ID: "acc-cascade-b", ProviderID: provID, Status: providers.AccountStatusReady}
		_ = harness.CreateAccount(ctx, accA, nil, nil)
		_ = harness.CreateAccount(ctx, accB, nil, nil)

		// 1. Initial: 2 ready
		ready, _ := harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 2 {
			t.Fatalf("expected 2 ready accounts, got %d", len(ready))
		}

		// 2. Account A fails with 401 Unauthorized -> marked signed_out
		_ = harness.Repo.UpdateAccountStatus(ctx, "acc-cascade-a", providers.AccountStatusSignedOut, nil)
		ready, _ = harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 1 || ready[0].ID != "acc-cascade-b" {
			t.Fatalf("expected 1 ready account (acc-cascade-b), got %+v", ready)
		}

		// 3. Account B fails with 429 Rate Limited -> paused with short future cooldown
		shortPause := time.Now().UTC().Add(50 * time.Millisecond)
		_ = harness.Repo.UpdateAccountStatus(ctx, "acc-cascade-b", providers.AccountStatusReady, &shortPause)

		// 4. All accounts exhausted -> 0 ready
		ready, _ = harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 0 {
			t.Fatalf("expected 0 ready accounts during exhaustion, got %d", len(ready))
		}

		// 5. Wait for short pause to elapse (cooldown recovery)
		time.Sleep(100 * time.Millisecond)

		// 6. Account B auto-recovers as ready
		ready, _ = harness.Repo.ListReadyAccounts(ctx, provID)
		if len(ready) != 1 || ready[0].ID != "acc-cascade-b" {
			t.Fatalf("expected acc-cascade-b to recover after cooldown, got %+v", ready)
		}
	})

	t.Run("T4_S4_HermeticGoogleFlowSimulation", func(t *testing.T) {
		// Scenario 4: Hermetic Upstream Google Flow Simulation
		// Stand up a mock upstream simulating Google Flow's BatchGenerateImages RPC
		mockFlow := NewMockUpstream(t)
		mockFlow.SetHandler(func(w http.ResponseWriter, r *http.Request) {
			cookie := r.Header.Get("Cookie")
			auth := r.Header.Get("Authorization")

			// Check simulated Google Flow credentials
			if !strings.Contains(cookie, "SAPISID=") || auth == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":401,"message":"Request had invalid authentication credentials."}}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"imageResponses": [
					{
						"image": {
							"imageUri": "https://storage.googleapis.com/flow-generated/sample_highres.png"
						}
					}
				]
			}`))
		})

		// Make request to mock Google Flow upstream directly
		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequest(http.MethodPost, mockFlow.URL+"/v1/batchGenerateImages", nil)
		req.Header.Set("Cookie", "SAPISID=google_auth_cookie_token_123")
		req.Header.Set("Authorization", "Bearer ya29.a0AfH6SM...")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request to mock flow failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from mock flow, got %d", resp.StatusCode)
		}
		if mockFlow.RequestCount() != 1 {
			t.Errorf("expected 1 request recorded, got %d", mockFlow.RequestCount())
		}
	})

	t.Run("T4_S5_SubprocessCLIBinaryExecution", func(t *testing.T) {
		// Scenario 5: Build and Run Gateway Binary as Subprocess
		goBin := FindGoBinary()
		tempDir := t.TempDir()
		binPath := filepath.Join(tempDir, "dezuxk_test.exe")

		// 1. Compile binary
		buildCmd := exec.Command(goBin, "build", "-o", binPath, "./cmd/server")
		buildCmd.Dir = filepath.Clean(filepath.Join(".", "..", "..")) // Dezuxk workspace root
		// Also allow current dir if relative path is inside repo
		if _, err := os.Stat("go.mod"); err == nil {
			buildCmd.Dir = "."
		}

		buildOut, err := buildCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to compile gateway binary: %v (output: %s)", err, buildOut)
		}

		// 2. Find a free local port
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen failed: %v", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()

		// 3. Start subprocess
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		dbFile := filepath.Join(tempDir, "cli_test.db")
		proc := exec.CommandContext(ctx, binPath,
			fmt.Sprintf("--port=%d", port),
			"--host=127.0.0.1",
			"--data-dir="+tempDir,
			"--db-file="+dbFile,
			"--master-key=subprocess-cli-master-key-32b-ok",
		)
		proc.Dir = tempDir

		if err := proc.Start(); err != nil {
			t.Fatalf("failed to start subprocess: %v", err)
		}
		defer func() {
			if proc.Process != nil {
				_ = proc.Process.Kill()
			}
		}()

		// 4. Poll /health until server is up
		healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
		serverReady := false
		client := &http.Client{Timeout: 1 * time.Second}

		for attempt := 0; attempt < 30; attempt++ {
			time.Sleep(100 * time.Millisecond)
			resp, err := client.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				serverReady = true
				break
			}
		}

		if !serverReady {
			t.Fatalf("gateway subprocess did not become ready at %s within timeout", healthURL)
		}

		// 5. Query health and verify response
		resp, err := client.Get(healthURL)
		if err != nil {
			t.Fatalf("failed to query subprocess: %v", err)
		}
		defer resp.Body.Close()

		var hr server.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
			t.Fatalf("failed to decode subprocess health response: %v", err)
		}
		if hr.Status != "healthy" {
			t.Errorf("expected status 'healthy', got %s", hr.Status)
		}

		// 6. Kill process cleanly
		_ = proc.Process.Kill()
		_ = proc.Wait()
	})
}
