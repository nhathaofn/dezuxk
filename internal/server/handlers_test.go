package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
	"dezuxk/internal/server"
	"dezuxk/internal/storage/sqlite"
)

func setupTestServer(t *testing.T, apiKey string) (*server.Server, *sqlite.ProvidersRepository, *sqlite.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_server.db")

	ctx := context.Background()
	db, err := sqlite.Open(ctx, sqlite.Options{
		Path: dbPath,
	})
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}

	cipher, err := security.NewAdaptiveCipher("test-master-key", false)
	if err != nil {
		t.Fatalf("NewAdaptiveCipher failed: %v", err)
	}

	repo := sqlite.NewProvidersRepository(db, cipher)

	cfg := config.Default()
	cfg.Server.APIKey = apiKey
	cfg.Storage.DBFile = dbPath

	srv := server.NewServer(cfg, repo)
	return srv, repo, db
}

func TestHealthEndpoint(t *testing.T) {
	srv, repo, db := setupTestServer(t, "test-api-key")
	defer db.Close()

	ctx := context.Background()

	// Seed one ready and one unverified account
	_ = repo.SaveProvider(ctx, &providers.Provider{
		ID:             "flow",
		Name:           "Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com"},
		Status:         providers.ProviderStatusActive,
	})
	_ = repo.SaveAccount(ctx, &providers.Account{
		ID:         "acc-1",
		ProviderID: "flow",
		Label:      "Ready Acc",
		Status:     providers.AccountStatusReady,
		ProfileDir: "/profiles/1",
	})
	_ = repo.SaveAccount(ctx, &providers.Account{
		ID:         "acc-2",
		ProviderID: "flow",
		Label:      "Unverified Acc",
		Status:     providers.AccountStatusUnverified,
		ProfileDir: "/profiles/2",
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp server.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %s", resp.Status)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", resp.Version)
	}
	if resp.Accounts.Ready != 1 {
		t.Errorf("expected 1 ready account, got %d", resp.Accounts.Ready)
	}
	if resp.Accounts.Total != 2 {
		t.Errorf("expected 2 total accounts, got %d", resp.Accounts.Total)
	}
}

func TestImageGenerations_Authentication(t *testing.T) {
	srv, _, db := setupTestServer(t, "expected-secret-token")
	defer db.Close()

	body := []byte(`{"prompt":"a majestic dragon flying over mountains"}`)

	// 1. Missing Authorization header -> 401
	{
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected HTTP 401 for missing auth header, got %d", rr.Code)
		}
	}

	// 2. Invalid Bearer token -> 401
	{
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer invalid-wrong-token")
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected HTTP 401 for wrong bearer token, got %d", rr.Code)
		}
	}

	// 3. Valid Bearer token -> succeeds (HTTP 200)
	{
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer expected-secret-token")
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected HTTP 200 for valid token, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	}
}

func TestImageGenerations_Authentication_EmptyAPIKey(t *testing.T) {
	srv, _, db := setupTestServer(t, "")
	defer db.Close()

	body := []byte(`{"prompt":"a tranquil lake at sunrise"}`)

	// 1. Unauthenticated request without Authorization header -> succeeds with HTTP 200
	{
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200 for empty API key without auth header, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	}

	// 2. Request with arbitrary Authorization header -> also succeeds with HTTP 200
	{
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer arbitrary-token")
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200 for empty API key with arbitrary auth header, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	}

	// 3. Direct verification of RequireBearer("") middleware
	{
		mw := server.RequireBearer("")
		called := false
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !called || rr.Code != http.StatusOK {
			t.Fatalf("expected RequireBearer(\"\") to pass through, called=%v, code=%d", called, rr.Code)
		}
	}
}

func TestImageGenerations_Validation(t *testing.T) {
	srv, _, db := setupTestServer(t, "valid-token")
	defer db.Close()

	// Empty prompt
	emptyPromptBody := []byte(`{"prompt": "   ", "provider": "flow"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(emptyPromptBody))
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for empty prompt, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var errResp server.ErrorResponse
	_ = json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp.Error != "bad_request" {
		t.Errorf("expected error 'bad_request', got %s", errResp.Error)
	}
}

func TestImageGenerations_PoolSelectionAndMockFallback(t *testing.T) {
	srv, repo, db := setupTestServer(t, "valid-token")
	defer db.Close()

	ctx := context.Background()

	// Scenario A: Empty pool -> returns mock generation with HTTP 200
	{
		body := []byte(`{"prompt": "cyberpunk city, neon lights", "aspect_ratio": "16:9"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp server.ImageGenerationResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Data) == 0 || resp.Data[0].URL == "" || resp.Data[0].B64JSON == "" {
			t.Errorf("expected image data with url and b64_json, got %+v", resp.Data)
		}
		if resp.Provider != "flow" {
			t.Errorf("expected default provider 'flow', got %s", resp.Provider)
		}
		if resp.AccountID != "" {
			t.Errorf("expected empty account_id for empty pool mock, got %s", resp.AccountID)
		}
	}

	// Scenario B: With ready account in pool -> selects account and updates last_checked_at
	{
		_ = repo.SaveProvider(ctx, &providers.Provider{
			ID:             "flow",
			Name:           "Flow",
			BaseURL:        "https://flow.google.com",
			AuthDomain:     ".google.com",
			AllowedDomains: []string{".google.com"},
			Status:         providers.ProviderStatusActive,
		})
		_ = repo.SaveAccount(ctx, &providers.Account{
			ID:         "ready-acc-42",
			ProviderID: "flow",
			Label:      "Production Account",
			Status:     providers.AccountStatusReady,
			ProfileDir: "/profiles/prod",
		})

		body := []byte(`{"prompt": "ancient temple hidden in misty jungle", "provider": "flow"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		srv.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp server.ImageGenerationResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.AccountID != "ready-acc-42" {
			t.Errorf("expected selected account_id 'ready-acc-42', got %s", resp.AccountID)
		}

		// Verify account last_checked_at was updated
		acc, err := repo.GetAccount(ctx, "ready-acc-42")
		if err != nil || acc == nil || acc.LastCheckedAt == nil {
			t.Fatalf("expected account last_checked_at to be updated: %v, %+v", err, acc)
		}
		if time.Since(*acc.LastCheckedAt) > 10*time.Second {
			t.Errorf("expected recently updated last_checked_at timestamp, got %v", acc.LastCheckedAt)
		}
	}
}
