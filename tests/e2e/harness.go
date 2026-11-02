package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
	"dezuxk/internal/server"
	"dezuxk/internal/storage/sqlite"
)

// HarnessOption configures a TestHarness instance.
type HarnessOption func(*harnessOptions)

type harnessOptions struct {
	apiKey     string
	masterKey  string
	useDPAPI   bool
	dataDir    string
	customCfg  func(*config.Config)
	nullLogger bool
}

// WithAPIKey sets the server API key for authentication testing.
func WithAPIKey(key string) HarnessOption {
	return func(o *harnessOptions) {
		o.apiKey = key
	}
}

// WithMasterKey sets an explicit master key for crypto testing.
func WithMasterKey(key string) HarnessOption {
	return func(o *harnessOptions) {
		o.masterKey = key
	}
}

// WithUseDPAPI toggles DPAPI usage.
func WithUseDPAPI(use bool) HarnessOption {
	return func(o *harnessOptions) {
		o.useDPAPI = use
	}
}

// WithDataDir specifies a custom data directory.
func WithDataDir(dir string) HarnessOption {
	return func(o *harnessOptions) {
		o.dataDir = dir
	}
}

// WithCustomConfig provides arbitrary adjustments to the Config struct.
func WithCustomConfig(fn func(*config.Config)) HarnessOption {
	return func(o *harnessOptions) {
		o.customCfg = fn
	}
}

// TestHarness encapsulates an isolated, running dezuxk gateway server instance
// backed by a temporary SQLite database and an HTTP test client.
type TestHarness struct {
	T          *testing.T
	TempDir    string
	DBPath     string
	DB         *sqlite.DB
	Cipher     security.Cipher
	Repo       providers.Repository
	Config     *config.Config
	Server     *server.Server
	HTTPServer *httptest.Server
	Client     *http.Client
}

// NewTestHarness builds and starts a complete isolated server harness for E2E testing.
func NewTestHarness(t *testing.T, opts ...HarnessOption) *TestHarness {
	t.Helper()

	options := &harnessOptions{
		masterKey:  "test-secret-master-key-32-bytes!",
		useDPAPI:   false, // Default to AES-GCM for portable, reproducible testing
		nullLogger: true,
	}
	for _, opt := range opts {
		opt(options)
	}

	tempDir := t.TempDir()
	dataDir := tempDir
	if options.dataDir != "" {
		dataDir = options.dataDir
	}

	dbPath := filepath.Join(dataDir, "test_dezuxk.db")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sqlite.Open(ctx, sqlite.Options{
		Path:   dbPath,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite database at %s: %v", dbPath, err)
	}

	cipher, err := security.NewAdaptiveCipher(options.masterKey, options.useDPAPI)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create adaptive cipher: %v", err)
	}

	repo := sqlite.NewProvidersRepository(db, cipher)

	cfg := config.Default()
	cfg.Server.APIKey = options.apiKey
	cfg.Storage.DBFile = dbPath
	cfg.Storage.DataDir = dataDir
	cfg.Security.MasterKey = options.masterKey
	cfg.Security.UseDPAPI = options.useDPAPI

	if options.customCfg != nil {
		options.customCfg(cfg)
	}

	srv := server.NewServer(cfg, repo)
	httpServer := httptest.NewServer(srv)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	h := &TestHarness{
		T:          t,
		TempDir:    tempDir,
		DBPath:     dbPath,
		DB:         db,
		Cipher:     cipher,
		Repo:       repo,
		Config:     cfg,
		Server:     srv,
		HTTPServer: httpServer,
		Client:     client,
	}

	t.Cleanup(func() {
		h.Close()
	})

	return h
}

// Close gracefully terminates the HTTP test server and SQLite database connection.
func (h *TestHarness) Close() {
	if h.HTTPServer != nil {
		h.HTTPServer.Close()
		h.HTTPServer = nil
	}
	if h.DB != nil {
		_ = h.DB.Close()
		h.DB = nil
	}
}

// CreateProvider registers a provider in the repository.
func (h *TestHarness) CreateProvider(ctx context.Context, p *providers.Provider) error {
	return h.Repo.SaveProvider(ctx, p)
}

// CreateAccount registers an account along with its encrypted secrets in the repository.
func (h *TestHarness) CreateAccount(ctx context.Context, acc *providers.Account, cookies []providers.Cookie, tokens map[string]string) error {
	if err := h.Repo.SaveAccount(ctx, acc); err != nil {
		return err
	}
	if cookies != nil || tokens != nil {
		sec := &providers.AccountSecrets{
			AccountID:  acc.ID,
			Cookies:    cookies,
			Tokens:     tokens,
			CookieHash: "testhash",
			UpdatedAt:  time.Now().UTC(),
		}
		if err := h.Repo.SaveAccountSecrets(ctx, sec); err != nil {
			return err
		}
	}
	return nil
}

// PostJSON executes an HTTP POST request to the specified endpoint with a JSON body.
func (h *TestHarness) PostJSON(endpoint string, reqBody any, headers map[string]string) (*http.Response, []byte, error) {
	h.T.Helper()

	var bodyBytes []byte
	if reqBody != nil {
		var err error
		bodyBytes, err = json.Marshal(reqBody)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal request: %w", err)
		}
	}

	url := h.HTTPServer.URL + endpoint
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read response body: %w", err)
	}

	return resp, respBody, nil
}

// GetJSON executes an HTTP GET request to the specified endpoint.
func (h *TestHarness) GetJSON(endpoint string, headers map[string]string) (*http.Response, []byte, error) {
	h.T.Helper()

	url := h.HTTPServer.URL + endpoint
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read response body: %w", err)
	}

	return resp, respBody, nil
}

// RawRequest executes an arbitrary HTTP request against the gateway test server.
func (h *TestHarness) RawRequest(method, endpoint string, body []byte, headers map[string]string) (*http.Response, []byte, error) {
	h.T.Helper()

	url := h.HTTPServer.URL + endpoint
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("create raw request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("execute raw request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read raw response: %w", err)
	}

	return resp, respBody, nil
}

// MockUpstreamRequest captures an incoming HTTP request sent to a MockUpstream.
type MockUpstreamRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// MockUpstream provides a hermetic mock HTTP server simulating external providers (e.g. Google Flow).
type MockUpstream struct {
	Server        *httptest.Server
	URL           string
	mu            sync.Mutex
	Requests      []MockUpstreamRequest
	StatusCode    int
	ResponseBody  []byte
	CustomHandler http.HandlerFunc
}

// NewMockUpstream creates and starts a hermetic MockUpstream server.
func NewMockUpstream(t *testing.T) *MockUpstream {
	t.Helper()

	mock := &MockUpstream{
		StatusCode:   http.StatusOK,
		ResponseBody: []byte(`{"status":"success","images":[{"url":"https://mock.storage/generated.png"}]}`),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		mock.mu.Lock()
		mock.Requests = append(mock.Requests, MockUpstreamRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Header: r.Header.Clone(),
			Body:   body,
		})
		custom := mock.CustomHandler
		status := mock.StatusCode
		respBody := mock.ResponseBody
		mock.mu.Unlock()

		if custom != nil {
			custom(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
	})

	server := httptest.NewServer(handler)
	mock.Server = server
	mock.URL = server.URL

	t.Cleanup(func() {
		mock.Close()
	})

	return mock
}

// SetResponse updates the default status code and body returned by the mock server.
func (m *MockUpstream) SetResponse(status int, body []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StatusCode = status
	m.ResponseBody = body
}

// SetHandler configures a dynamic handler for flexible response handling.
func (m *MockUpstream) SetHandler(fn http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CustomHandler = fn
}

// GetRequests returns a copy of all requests received by the mock server.
func (m *MockUpstream) GetRequests() []MockUpstreamRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]MockUpstreamRequest, len(m.Requests))
	copy(copied, m.Requests)
	return copied
}

// RequestCount returns the total number of requests received by the mock server.
func (m *MockUpstream) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Requests)
}

// Close shuts down the mock server.
func (m *MockUpstream) Close() {
	if m.Server != nil {
		m.Server.Close()
		m.Server = nil
	}
}

// FindGoBinary resolves the path to the Go toolchain executable.
func FindGoBinary() string {
	candidates := []string{
		`C:\Users\PC\AppData\Local\Programs\go1.27.3\go\bin\go.exe`,
		`C:\Program Files\Go\bin\go.exe`,
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	return "go"
}

// GenerateRandomSecret returns a 32-byte cryptographically secure hex string.
func GenerateRandomSecret() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
