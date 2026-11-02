package proxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/proxy"
	"dezuxk/internal/security"
	"dezuxk/internal/storage/sqlite"
)

func setupTestRepo(t *testing.T) (*sqlite.ProvidersRepository, *sqlite.DB) {
	t.Helper()
	db, err := sqlite.Open(context.Background(), sqlite.Options{
		Path: ":memory:",
	})
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}

	cipher, err := security.NewAdaptiveCipher("test-key-32-bytes-long-super-sec", false)
	if err != nil {
		t.Fatalf("NewAdaptiveCipher failed: %v", err)
	}

	repo := sqlite.NewProvidersRepository(db, cipher)
	return repo, db
}

func TestProxy_DynamicImageGenerator(t *testing.T) {
	url, b64 := proxy.GenerateDynamicImage("test prompt", "16:9")
	if url == "" || b64 == "" {
		t.Fatalf("expected non-empty url and base64 string")
	}

	url2, b642 := proxy.GenerateDynamicImage("different prompt", "1:1")
	if url == url2 || b64 == b642 {
		t.Errorf("expected different dynamic payloads for different prompts")
	}
}

func TestProxy_CredentialInjection(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	sec := &providers.AccountSecrets{
		Cookies: []providers.Cookie{
			{Name: "SESSION", Value: "12345"},
			{Name: "HSID", Value: "67890"},
		},
		Tokens: map[string]string{
			"authorization":   "my-bearer-token",
			"x-goog-api-key": "my-google-key",
		},
	}

	proxy.InjectCredentials(req, sec, nil)

	cookieHdr := req.Header.Get("Cookie")
	if cookieHdr != "SESSION=12345; HSID=67890" {
		t.Errorf("unexpected Cookie header: %s", cookieHdr)
	}

	authHdr := req.Header.Get("Authorization")
	if authHdr != "Bearer my-bearer-token" {
		t.Errorf("unexpected Authorization header: %s", authHdr)
	}

	keyHdr := req.Header.Get("x-goog-api-key")
	if keyHdr != "my-google-key" {
		t.Errorf("unexpected x-goog-api-key header: %s", keyHdr)
	}
}

func TestProxy_EmptyPoolDynamicResponse(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	engine := proxy.NewEngine(repo, nil, nil)
	resp, err := engine.GenerateImage(context.Background(), proxy.GenerateRequest{
		Prompt: "A futuristic city in the clouds",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.AccountID != "" {
		t.Errorf("expected empty account_id for empty pool, got %s", resp.AccountID)
	}
	if len(resp.Data) == 0 || resp.Data[0].URL == "" || resp.Data[0].B64JSON == "" {
		t.Errorf("expected non-empty image data, got %+v", resp.Data)
	}
}

func TestProxy_UpstreamDispatchAndAccountSelection(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()

	// Setup mock upstream HTTP server
	var receivedAuth string
	var receivedCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedCookie = r.Header.Get("Cookie")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(proxy.GenerateResponse{
			Created: time.Now().Unix(),
			Data: []proxy.ImageData{
				{URL: "https://cdn.upstream.test/result.png", B64JSON: "upstreamB64"},
			},
		})
	}))
	defer upstream.Close()

	_ = repo.SaveProvider(ctx, &providers.Provider{
		ID:         "mock-prov",
		Name:       "Mock Provider",
		BaseURL:    upstream.URL,
		AuthDomain: "upstream.test",
		Status:     providers.ProviderStatusActive,
	})

	_ = repo.SaveAccount(ctx, &providers.Account{
		ID:         "acc-test-1",
		ProviderID: "mock-prov",
		Status:     providers.AccountStatusReady,
	})

	_ = repo.SaveAccountSecrets(ctx, &providers.AccountSecrets{
		AccountID: "acc-test-1",
		Cookies: []providers.Cookie{
			{Name: "SESSION", Value: "upstream_cookie_val"},
		},
		Tokens: map[string]string{
			"authorization": "upstream_token_val",
		},
	})

	engine := proxy.NewEngine(repo, nil, nil, proxy.WithHTTPClient(upstream.Client()))

	resp, err := engine.GenerateImage(ctx, proxy.GenerateRequest{
		Prompt:   "A fantasy forest",
		Provider: "mock-prov",
	})
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	if resp.AccountID != "acc-test-1" {
		t.Errorf("expected account_id acc-test-1, got %s", resp.AccountID)
	}
	if receivedAuth != "Bearer upstream_token_val" {
		t.Errorf("expected upstream Authorization header 'Bearer upstream_token_val', got %s", receivedAuth)
	}
	if receivedCookie != "SESSION=upstream_cookie_val" {
		t.Errorf("expected upstream Cookie header 'SESSION=upstream_cookie_val', got %s", receivedCookie)
	}
}

func TestProxy_FailoverOnRateLimit(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()

	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(proxy.GenerateResponse{
			Data: []proxy.ImageData{{URL: "https://cdn.test/ok.png"}},
		})
	}))
	defer upstream.Close()

	_ = repo.SaveProvider(ctx, &providers.Provider{
		ID:      "rl-prov",
		BaseURL: upstream.URL,
		Status:  providers.ProviderStatusActive,
	})

	_ = repo.SaveAccount(ctx, &providers.Account{
		ID:         "acc-rl-1",
		ProviderID: "rl-prov",
		Status:     providers.AccountStatusReady,
	})

	_ = repo.SaveAccount(ctx, &providers.Account{
		ID:         "acc-rl-2",
		ProviderID: "rl-prov",
		Status:     providers.AccountStatusReady,
	})

	engine := proxy.NewEngine(repo, nil, nil, proxy.WithHTTPClient(upstream.Client()))

	resp, err := engine.GenerateImage(ctx, proxy.GenerateRequest{
		Prompt:   "Test prompt failover",
		Provider: "rl-prov",
	})
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	if resp.AccountID == "" {
		t.Errorf("expected successful generation with failover account, got empty account ID")
	}
	if callCount != 2 {
		t.Errorf("expected 2 upstream calls (1 retry), got %d", callCount)
	}
}
