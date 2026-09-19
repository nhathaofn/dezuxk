package services

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dezuxk/internal/config"
	appcrypto "dezuxk/internal/crypto"
	"dezuxk/internal/db"
	"dezuxk/internal/models"
	"dezuxk/internal/services/chrome"
)

const (
	// Google quota pages are expensive browser navigations and should not be
	// polled more often than the UI's five-minute freshness window.
	liveMetricsCacheTTL        = 15 * time.Minute
	liveMetricsRequestCooldown = 15 * time.Minute
	refreshAttemptCooldown     = 10 * time.Minute
	probeAttemptCooldown       = 5 * time.Minute
)

type liveMetricsState struct {
	inFlight      bool
	done          chan struct{}
	metrics       *models.LiveAccountMetrics
	fetchedAt     time.Time
	lastAttemptAt time.Time
	lastErr       error
}

// AccountService manages Google account business logic, authentication lifecycle, and verification.
type AccountService struct {
	database         *db.DB
	loginManager     *chrome.LoginManager
	keepAliveWorker  *chrome.KeepAliveWorker
	baseDataDir      string
	accountLocks     sync.Map // key: accountID, value: *sync.Mutex
	refreshAttemptMu sync.Mutex
	refreshAttemptAt map[string]time.Time
	probeAttemptMu   sync.Mutex
	probeAttemptAt   map[string]time.Time
	liveMetricsMu    sync.Mutex
	liveMetrics      map[string]*liveMetricsState
}

// NewAccountService initializes AccountService and its sub-components.
func NewAccountService(database *db.DB, baseDataDir string) *AccountService {
	if baseDataDir == "" {
		baseDataDir = "data"
	}
	absDataDir, err := filepath.Abs(baseDataDir)
	if err == nil {
		baseDataDir = absDataDir
	}

	loginMgr := chrome.NewLoginManager(database, baseDataDir)
	worker := chrome.NewKeepAliveWorker(database)
	service := &AccountService{
		database:         database,
		loginManager:     loginMgr,
		keepAliveWorker:  worker,
		baseDataDir:      baseDataDir,
		refreshAttemptAt: make(map[string]time.Time),
		probeAttemptAt:   make(map[string]time.Time),
		liveMetrics:      make(map[string]*liveMetricsState),
	}
	worker.SetRefreshFunc(service.refreshAccountWithLock)

	return service
}

func (s *AccountService) getAccountLock(accountID string) *sync.Mutex {
	lock, _ := s.accountLocks.LoadOrStore(accountID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *AccountService) isManagedProfileDir(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	root, err := filepath.Abs(filepath.Join(s.baseDataDir, "profiles"))
	if err != nil {
		return false
	}
	candidate, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func (s *AccountService) profileStillReferenced(dir string) bool {
	accounts, err := s.database.ListGoogleAccounts()
	if err != nil {
		// On a database/read failure, preserve the profile rather than risk
		// deleting data that may already belong to a replacement account.
		return true
	}
	target := filepath.Clean(dir)
	for _, account := range accounts {
		if strings.EqualFold(filepath.Clean(account.ProfileDir), target) {
			return true
		}
	}
	return false
}

// StartBackgroundWorkers starts periodic background maintenance tasks.
// The application does not call this during normal startup; keeping it
// explicit prevents Chrome from being launched in the background by default.
func (s *AccountService) StartBackgroundWorkers() {
	if s.keepAliveWorker != nil {
		s.keepAliveWorker.Start()
	}
}

// StopBackgroundWorkers stops background tasks cleanly.
func (s *AccountService) StopBackgroundWorkers() {
	if s.keepAliveWorker != nil {
		s.keepAliveWorker.Stop()
	}
}

// CleanupActiveSessions terminates any active Chrome login sessions and cleans up processes.
func (s *AccountService) CleanupActiveSessions() {
	if s.loginManager != nil {
		s.loginManager.CloseAllSessions()
	}
}

// ListAccounts returns all registered Google accounts as sanitized responses.
func (s *AccountService) ListAccounts() ([]*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	accounts, err := s.database.ListGoogleAccounts()
	if err != nil {
		return nil, err
	}

	responses := make([]*models.GoogleAccountResponse, 0, len(accounts))
	for _, acc := range accounts {
		responses = append(responses, acc.ToResponse())
	}

	return responses, nil
}

// GetAccount returns a single account by ID.
func (s *AccountService) GetAccount(accountID string) (*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}

	return acc.ToResponse(), nil
}

// GetLiveMetrics reads Flow and Gemini quota directly from the account's
// managed Chrome profile. The returned snapshot is intentionally not persisted
// in SQLite; only the session/profile data remains durable.
func (s *AccountService) GetLiveMetrics(accountID string) (*models.LiveAccountMetrics, error) {
	if s.database == nil || s.keepAliveWorker == nil {
		return nil, errors.New("service not ready")
	}
	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}
	if !s.isManagedProfileDir(acc.ProfileDir) {
		return nil, errors.New("profile tài khoản không nằm trong thư mục được quản lý")
	}

	for {
		now := time.Now().UTC()

		s.liveMetricsMu.Lock()
		state := s.liveMetrics[accountID]
		if state == nil {
			state = &liveMetricsState{}
			s.liveMetrics[accountID] = state
		}

		if state.metrics != nil && now.Sub(state.fetchedAt) < liveMetricsCacheTTL {
			metrics := cloneLiveMetrics(state.metrics)
			s.liveMetricsMu.Unlock()
			return metrics, nil
		}

		if state.inFlight {
			done := state.done
			s.liveMetricsMu.Unlock()
			<-done
			continue
		}

		if !state.lastAttemptAt.IsZero() && now.Sub(state.lastAttemptAt) < liveMetricsRequestCooldown {
			metrics := cloneLiveMetrics(state.metrics)
			lastErr := state.lastErr
			retryAfter := liveMetricsRequestCooldown - now.Sub(state.lastAttemptAt)
			s.liveMetricsMu.Unlock()

			// Return stale data instead of opening another Chrome session. The
			// RetrievedAt value lets the UI show that it is not a fresh read.
			if metrics != nil {
				return metrics, nil
			}
			if lastErr != nil {
				return nil, fmt.Errorf("đã tạm hoãn đọc quota để tránh gửi quá nhiều yêu cầu Google; thử lại sau %s: %w", retryAfterLabel(retryAfter), lastErr)
			}
			return nil, fmt.Errorf("đã tạm hoãn đọc quota để tránh gửi quá nhiều yêu cầu Google; thử lại sau %s", retryAfterLabel(retryAfter))
		}

		state.inFlight = true
		state.done = make(chan struct{})
		state.lastAttemptAt = now
		done := state.done
		s.liveMetricsMu.Unlock()

		metrics, collectErr := s.collectLiveMetricsWithLock(acc)

		s.liveMetricsMu.Lock()
		if metrics != nil {
			state.metrics = cloneLiveMetrics(metrics)
			state.fetchedAt = time.Now().UTC()
		}
		state.lastErr = collectErr
		state.inFlight = false
		close(done)
		state.done = nil
		cachedMetrics := cloneLiveMetrics(state.metrics)
		s.liveMetricsMu.Unlock()

		return cachedMetrics, collectErr
	}
}

func (s *AccountService) collectLiveMetricsWithLock(acc *models.GoogleAccount) (*models.LiveAccountMetrics, error) {
	lock := s.getAccountLock(acc.ID)
	if !lock.TryLock() {
		return nil, errors.New("tài khoản đang được mở hoặc làm mới, vui lòng thử lại sau")
	}
	defer lock.Unlock()
	return s.keepAliveWorker.CollectLiveMetrics(acc)
}

func cloneLiveMetrics(metrics *models.LiveAccountMetrics) *models.LiveAccountMetrics {
	if metrics == nil {
		return nil
	}
	copy := *metrics
	return &copy
}

func retryAfterLabel(remaining time.Duration) string {
	if remaining <= time.Second {
		return "ít hơn 1 giây"
	}
	if remaining < time.Minute {
		seconds := int((remaining + time.Second - 1) / time.Second)
		return fmt.Sprintf("%d giây", seconds)
	}
	minutes := int((remaining + time.Minute - 1) / time.Minute)
	return fmt.Sprintf("%d phút", minutes)
}

// GetAccountProxy returns the full proxy only on an explicit authenticated edit
// request. It is deliberately not part of the account-list response.
func (s *AccountService) GetAccountProxy(accountID string) (string, error) {
	if s.database == nil {
		return "", errors.New("database not initialized")
	}
	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return "", err
	}
	return acc.Proxy, nil
}

// StartLogin begins one interactive Google login session shared by Flow and Gemini.
func (s *AccountService) StartLogin(proxy string) (*models.LoginSessionStatus, error) {
	if s.loginManager == nil {
		return nil, errors.New("login manager not initialized")
	}
	return s.loginManager.StartLogin(proxy)
}

// GetLoginStatus returns the live status of an ongoing login session.
func (s *AccountService) GetLoginStatus(sessionID string) (*models.LoginSessionStatus, error) {
	if s.loginManager == nil {
		return nil, errors.New("login manager not initialized")
	}
	return s.loginManager.GetStatus(sessionID)
}

// CancelLogin cancels an active login attempt and cleans up temporary resources.
func (s *AccountService) CancelLogin(sessionID string) error {
	if s.loginManager == nil {
		return errors.New("login manager not initialized")
	}
	return s.loginManager.CancelLogin(sessionID)
}

// DeleteAccount removes the account from SQLite and cleans up its stored profile directory.
func (s *AccountService) DeleteAccount(accountID string) error {
	if s.database == nil {
		return errors.New("database not initialized")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return err
	}

	if err := s.database.DeleteGoogleAccount(accountID); err != nil {
		return err
	}

	// Remove persistent profile directory on disk with retry to handle Windows file locks
	if s.isManagedProfileDir(acc.ProfileDir) {
		go func(dir string) {
			for i := 0; i < 5; i++ {
				if s.profileStillReferenced(dir) {
					return
				}
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					return
				}
				if err := os.RemoveAll(dir); err == nil {
					return
				}
				time.Sleep(400 * time.Millisecond)
			}
		}(acc.ProfileDir)
	}

	return nil
}

// RefreshSession forces an immediate headless refresh of an account's cookies.
func (s *AccountService) RefreshSession(accountID string) (*models.GoogleAccountResponse, error) {
	if s.database == nil || s.keepAliveWorker == nil {
		return nil, errors.New("service not ready")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}
	if acc.Status == "PENDING" {
		return nil, errors.New("tài khoản đang chờ kiểm tra phiên Google")
	}

	if err := s.refreshAccountWithLock(acc); err != nil {
		return nil, fmt.Errorf("làm mới phiên thất bại: %w", err)
	}

	updated, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}

	return updated.ToResponse(), nil
}

func (s *AccountService) refreshAccountWithLock(acc *models.GoogleAccount) error {
	if acc == nil {
		return errors.New("tài khoản không hợp lệ")
	}
	if !s.isManagedProfileDir(acc.ProfileDir) {
		return errors.New("profile tài khoản không nằm trong thư mục được quản lý")
	}
	lock := s.getAccountLock(acc.ID)
	if !lock.TryLock() {
		return fmt.Errorf("tài khoản %s đang được mở hoặc đang được làm mới, vui lòng thử lại sau", acc.Email)
	}
	defer lock.Unlock()
	if err := s.claimRefreshAttempt(acc); err != nil {
		return err
	}
	return s.keepAliveWorker.RefreshAccount(acc)
}

func (s *AccountService) claimRefreshAttempt(acc *models.GoogleAccount) error {
	if acc == nil {
		return errors.New("tài khoản không hợp lệ")
	}

	now := time.Now().UTC()
	s.refreshAttemptMu.Lock()
	defer s.refreshAttemptMu.Unlock()

	lastAttempt := s.refreshAttemptAt[acc.ID]
	if persisted, err := time.ParseInLocation("2006-01-02 15:04:05", acc.LastRefreshAt, time.UTC); err == nil && persisted.After(lastAttempt) {
		lastAttempt = persisted
	}
	if !lastAttempt.IsZero() && now.Sub(lastAttempt) < refreshAttemptCooldown {
		return fmt.Errorf("đã tạm hoãn làm mới để tránh gửi quá nhiều yêu cầu Google; thử lại sau %s", retryAfterLabel(refreshAttemptCooldown-now.Sub(lastAttempt)))
	}

	s.refreshAttemptAt[acc.ID] = now
	return nil
}

// TestSession verifies the account's credentials against an upstream Google endpoint using the account's proxy if configured.
func (s *AccountService) TestSession(accountID string) (*models.AccountTestResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}
	if err := s.claimProbeAttempt(acc); err != nil {
		return nil, err
	}
	lock := s.getAccountLock(accountID)
	if !lock.TryLock() {
		return nil, fmt.Errorf("tài khoản %s đang được mở hoặc đang được làm mới, vui lòng thử lại sau", acc.Email)
	}
	defer lock.Unlock()

	start := time.Now()
	testedService, endpoint := accountProbeTarget(acc.Services)
	if !hasSessionCookie(acc.Cookies) {
		_ = s.database.UpdateGoogleAccountStatus(acc.ID, "EXPIRED", "Không tìm thấy cookie phiên Google cần thiết")
		return &models.AccountTestResult{
			Success:       false,
			Message:       "Tài khoản chưa có cookie phiên Google hợp lệ",
			TestedService: testedService,
			Timestamp:     time.Now().UTC(),
		}, nil
	}

	transport := &http.Transport{}
	if acc.Proxy != "" {
		serverFlag, user, pass := chrome.FormatChromeProxyFlag(acc.Proxy)
		if serverFlag != "" {
			proxyURL, err := url.Parse(serverFlag)
			if err == nil {
				if user != "" && proxyURL.User == nil {
					proxyURL.User = url.UserPassword(user, pass)
				}
				transport.Proxy = http.ProxyURL(proxyURL)
			}
		}
	}

	// Perform a lightweight probe to Google Flow using the stored cookies and proxy
	client := &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Hostname() == "accounts.google.com" || strings.HasSuffix(req.URL.Hostname(), ".accounts.google.com") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(acc.UserAgent) != "" {
		req.Header.Set("User-Agent", acc.UserAgent)
	}
	req.Header.Set("Cookie", acc.Cookies)

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &models.AccountTestResult{
			Success:       false,
			Message:       fmt.Sprintf("Không thể kết nối tới %s", testedService),
			LatencyMs:     latency,
			TestedService: testedService,
			Timestamp:     time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()

	finalHost := resp.Request.URL.Hostname()
	redirectedToAuth := finalHost == "accounts.google.com" || strings.HasSuffix(finalHost, ".accounts.google.com")
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && !redirectedToAuth {
		_ = s.database.UpdateGoogleAccountStatus(acc.ID, "ACTIVE", "")
		return &models.AccountTestResult{
			Success:       true,
			Message:       fmt.Sprintf("Phiên đăng nhập %s hợp lệ", testedService),
			LatencyMs:     latency,
			TestedService: testedService,
			Timestamp:     time.Now().UTC(),
		}, nil
	}

	if redirectedToAuth || resp.StatusCode >= 300 {
		_ = s.database.UpdateGoogleAccountStatus(acc.ID, "EXPIRED", fmt.Sprintf("Google yêu cầu xác thực lại (HTTP %d)", resp.StatusCode))
		return &models.AccountTestResult{
			Success:       false,
			Message:       fmt.Sprintf("Phiên đăng nhập %s đã hết hạn hoặc bị chuyển tới trang đăng nhập (Status %d)", testedService, resp.StatusCode),
			LatencyMs:     latency,
			TestedService: testedService,
			Timestamp:     time.Now().UTC(),
		}, nil
	}

	_ = s.database.UpdateGoogleAccountStatus(acc.ID, "ERROR", fmt.Sprintf("Google trả về HTTP %d", resp.StatusCode))
	return &models.AccountTestResult{
		Success:       false,
		Message:       fmt.Sprintf("Phản hồi không hợp lệ từ %s: Status %d", testedService, resp.StatusCode),
		LatencyMs:     latency,
		TestedService: testedService,
		Timestamp:     time.Now().UTC(),
	}, nil
}

func (s *AccountService) claimProbeAttempt(acc *models.GoogleAccount) error {
	if acc == nil {
		return errors.New("tài khoản không hợp lệ")
	}

	now := time.Now().UTC()
	s.probeAttemptMu.Lock()
	defer s.probeAttemptMu.Unlock()

	if lastAttempt := s.probeAttemptAt[acc.ID]; !lastAttempt.IsZero() && now.Sub(lastAttempt) < probeAttemptCooldown {
		return fmt.Errorf("đã tạm hoãn kiểm tra để tránh gửi quá nhiều yêu cầu Google; thử lại sau %s", retryAfterLabel(probeAttemptCooldown-now.Sub(lastAttempt)))
	}
	s.probeAttemptAt[acc.ID] = now
	return nil
}

// OpenAccountBrowser launches Chrome with the account's dedicated profile directory and proxy.
func (s *AccountService) OpenAccountBrowser(accountID string) error {
	if s.database == nil || s.loginManager == nil {
		return errors.New("service not ready")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return err
	}
	if !s.isManagedProfileDir(acc.ProfileDir) {
		return errors.New("profile tài khoản không nằm trong thư mục được quản lý")
	}
	if err := os.MkdirAll(acc.ProfileDir, 0700); err != nil {
		return fmt.Errorf("không thể tạo profile tài khoản: %w", err)
	}

	lock := s.getAccountLock(accountID)
	if !lock.TryLock() {
		return fmt.Errorf("tài khoản %s đang được mở hoặc làm mới", acc.Email)
	}
	defer lock.Unlock()

	targetURL := "https://flow.google.com"
	if strings.Contains(strings.ToLower(acc.Services), "gemini") && !strings.Contains(strings.ToLower(acc.Services), "flow") {
		targetURL = "https://gemini.google.com/app"
	}

	return s.loginManager.OpenBrowserWithCookies(acc.ProfileDir, targetURL, acc.Proxy, acc.Cookies)
}

// ToggleAccount switches an account between ACTIVE and DISABLED.
func (s *AccountService) ToggleAccount(accountID string, active bool) (*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}
	if active {
		if acc, err := s.database.GetGoogleAccountByID(accountID); err != nil {
			return nil, err
		} else if acc.Status == "PENDING" {
			return nil, errors.New("hãy kiểm tra phiên Google trước khi bật tài khoản")
		}
	}

	status := "ACTIVE"
	lastError := ""
	if !active {
		status = "DISABLED"
		lastError = "Tài khoản bị tắt thủ công"
	}

	if err := s.database.UpdateGoogleAccountStatus(accountID, status, lastError); err != nil {
		return nil, err
	}

	return s.GetAccount(accountID)
}

// RefreshAllAccounts triggers a refresh across all accounts that are not disabled.
func (s *AccountService) RefreshAllAccounts() ([]*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	accounts, err := s.database.ListGoogleAccounts()
	if err != nil {
		return nil, err
	}

	for _, acc := range accounts {
		if acc.Status == "ACTIVE" {
			_, _ = s.RefreshSession(acc.ID)
		}
	}

	return s.ListAccounts()
}

// SaveAccountProxy saves a proxy string for an account.
func (s *AccountService) SaveAccountProxy(accountID, proxy string) error {
	if s.database == nil {
		return errors.New("database not initialized")
	}
	return s.database.UpdateGoogleAccountProxy(accountID, proxy)
}

// UpdateAccountFeatures updates image_enabled and video_enabled flags.
func (s *AccountService) UpdateAccountFeatures(accountID string, imageEnabled, videoEnabled bool) error {
	if s.database == nil {
		return errors.New("database not initialized")
	}
	return s.database.UpdateGoogleAccountFeatures(accountID, imageEnabled, videoEnabled)
}

// BulkUpdateFeatures updates feature flags for all accounts.
func (s *AccountService) BulkUpdateFeatures(feature string, enabled bool) error {
	if s.database == nil {
		return errors.New("database not initialized")
	}
	return s.database.BulkUpdateGoogleAccountFeatures(feature, enabled)
}

// UpdateCredits updates the credit count of an account directly.
func (s *AccountService) UpdateCredits(accountID string, credits int) error {
	if s.database == nil {
		return errors.New("database not initialized")
	}
	if credits < 0 {
		return errors.New("số tín dụng không được âm")
	}
	return s.database.UpdateGoogleAccountCredits(accountID, credits)
}

// AddAccountManual inserts a Google account provided manually with cookies/tokens.
func (s *AccountService) AddAccountManual(input models.ManualAccountInput) (*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	cookies := strings.TrimSpace(input.Cookies)
	var emailErr error
	if email, emailErr = normalizeGoogleEmail(email); emailErr != nil {
		return nil, emailErr
	}
	if cookies == "" {
		return nil, errors.New("vui lòng nhập chuỗi cookies của tài khoản Google")
	}

	// Strict cookie token validation: must contain __Secure-1PSID or SID
	if !hasSessionCookie(cookies) {
		return nil, errors.New("chuỗi cookie không hợp lệ (bắt buộc phải có __Secure-1PSID hoặc SID)")
	}

	accountID := generateAccID("acc")
	profileDir := filepath.Join(s.baseDataDir, "profiles", accountID)
	// Lazy profile initialization: do NOT eager-create empty directory for manual cookie accounts

	existing, _ := s.database.GetGoogleAccountByEmail(email)
	if existing != nil {
		accountID = existing.ID
		if s.isManagedProfileDir(existing.ProfileDir) {
			profileDir = existing.ProfileDir
		}
	}

	tier := normalizeTier(input.Tier)
	credits := input.Credits
	if credits < 0 {
		credits = 0
	}
	service := normalizeService(input.Service)

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
	acc := &models.GoogleAccount{
		ID:            accountID,
		Email:         email,
		Name:          strings.Split(email, "@")[0],
		ProfileDir:    profileDir,
		Cookies:       cookies,
		SNlM0eToken:   strings.TrimSpace(input.SNlM0eToken),
		Services:      service,
		Status:        "PENDING",
		Tier:          tier,
		Credits:       credits,
		Proxy:         strings.TrimSpace(input.Proxy),
		ImageEnabled:  true,
		VideoEnabled:  true,
		LastRefreshAt: "",
		CreatedAt:     nowStr,
		UpdatedAt:     nowStr,
	}

	if err := s.database.UpsertGoogleAccount(acc); err != nil {
		return nil, fmt.Errorf("lỗi lưu tài khoản: %w", err)
	}

	return acc.ToResponse(), nil
}

// BulkAddAccounts parses multiline inputs and registers multiple accounts into SQLite.
func (s *AccountService) BulkAddAccounts(input models.BulkAddInput) (*models.BulkAddResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	lines := strings.Split(input.RawList, "\n")
	result := &models.BulkAddResult{
		Items: make([]models.BulkAccountItem, 0, len(lines)),
	}

	targetProfilesRoot := filepath.Join(s.baseDataDir, "profiles")
	_ = os.MkdirAll(targetProfilesRoot, 0700)

	defaultTier := strings.ToUpper(strings.TrimSpace(input.DefaultTier))
	if defaultTier == "" {
		defaultTier = "FREE"
	}
	if defaultTier != "FREE" && defaultTier != "PRO" && defaultTier != "ULTRA" {
		defaultTier = "FREE"
	}
	service := normalizeService(input.Service)
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		result.TotalParsed++

		var item models.BulkAccountItem
		var parsedEmail, parsedProxy, parsedCookies string

		// Check if line is JSON format
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var jsonAcc struct {
				Email   string `json:"email"`
				Proxy   string `json:"proxy"`
				Cookie  string `json:"cookie"`
				Cookies string `json:"cookies"`
			}
			if err := json.Unmarshal([]byte(line), &jsonAcc); err == nil && jsonAcc.Email != "" {
				parsedEmail = strings.TrimSpace(jsonAcc.Email)
				parsedProxy = strings.TrimSpace(jsonAcc.Proxy)
				if jsonAcc.Cookie != "" {
					parsedCookies = strings.TrimSpace(jsonAcc.Cookie)
				} else {
					parsedCookies = strings.TrimSpace(jsonAcc.Cookies)
				}
			}
		}

		if parsedEmail == "" {
			var parts []string
			if strings.Contains(line, "|") {
				parts = strings.Split(line, "|")
			} else if strings.Contains(line, "\t") {
				parts = strings.Split(line, "\t")
			} else {
				parts = []string{line}
			}

			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}

			if len(parts) >= 2 && strings.Contains(parts[0], "@") && hasSessionCookie(parts[1]) {
				parsedEmail = parts[0]
				parsedCookies = parts[1]
				if len(parts) >= 3 {
					parsedProxy = strings.Join(parts[2:], "|")
				}
			} else {
				if len(parts) > 0 && strings.Contains(parts[0], "@") {
					item.Email = strings.ToLower(strings.TrimSpace(parts[0]))
				}
				item.Status = "ERROR"
				item.Message = "Cần email và Cookie Google hợp lệ; hệ thống không nhận mật khẩu tài khoản"
				result.FailedCount++
				result.Items = append(result.Items, item)
				continue
			}
		}

		item.Email = strings.ToLower(strings.TrimSpace(parsedEmail))
		parsedProxy = strings.TrimSpace(parsedProxy)
		parsedCookies = strings.TrimSpace(parsedCookies)
		if normalizedEmail, emailErr := normalizeGoogleEmail(item.Email); emailErr != nil {
			item.Status = "ERROR"
			item.Message = emailErr.Error()
			result.FailedCount++
			result.Items = append(result.Items, item)
			continue
		} else {
			item.Email = normalizedEmail
		}
		if parsedProxy == "" && input.DefaultProxy != "" {
			parsedProxy = strings.TrimSpace(input.DefaultProxy)
		}
		if item.Email == "" || !hasSessionCookie(parsedCookies) {
			item.Status = "ERROR"
			item.Message = "Thiếu email hoặc Cookie Google hợp lệ"
			result.FailedCount++
			result.Items = append(result.Items, item)
			continue
		}

		existing, _ := s.database.GetGoogleAccountByEmail(item.Email)
		if existing != nil && input.SkipExisting {
			item.Status = "SKIPPED"
			item.Message = "Đã bỏ qua vì tài khoản đã tồn tại"
			result.SkippedCount++
			result.Items = append(result.Items, item)
			continue
		}

		var accountID string
		var profileDir string
		status := "PENDING"
		tier := defaultTier
		credits := 0
		snlm0eToken := ""
		imageEnabled := true
		videoEnabled := true
		userAgent := ""
		if existing != nil {
			accountID = existing.ID
			if s.isManagedProfileDir(existing.ProfileDir) {
				profileDir = existing.ProfileDir
			} else {
				profileDir = filepath.Join(targetProfilesRoot, accountID)
			}
			// Replacing an existing cookie session must be re-verified, but must
			// not erase durable account settings or previously known credits.
			if existing.Status == "DISABLED" {
				status = "DISABLED"
			}
			tier = normalizeTier(existing.Tier)
			credits = existing.Credits
			snlm0eToken = existing.SNlM0eToken
			imageEnabled = existing.ImageEnabled
			videoEnabled = existing.VideoEnabled
			userAgent = existing.UserAgent
			if parsedProxy == "" {
				parsedProxy = existing.Proxy
			}
		} else {
			accountID = generateAccID("acc")
			profileDir = filepath.Join(targetProfilesRoot, accountID)
		}
		acc := &models.GoogleAccount{
			ID:            accountID,
			Email:         item.Email,
			Name:          strings.Split(item.Email, "@")[0],
			ProfileDir:    profileDir,
			Cookies:       parsedCookies,
			SNlM0eToken:   snlm0eToken,
			Services:      service,
			Status:        status,
			Tier:          tier,
			Credits:       credits,
			Proxy:         parsedProxy,
			ImageEnabled:  imageEnabled,
			VideoEnabled:  videoEnabled,
			UserAgent:     userAgent,
			LastRefreshAt: "",
			CreatedAt:     nowStr,
			UpdatedAt:     nowStr,
		}

		if err := s.database.UpsertGoogleAccount(acc); err != nil {
			item.Status = "ERROR"
			item.Message = fmt.Sprintf("Lỗi cơ sở dữ liệu: %v", err)
			result.FailedCount++
		} else {
			item.Status = "ADDED"
			item.Message = "Thêm thành công"
			result.AddedCount++
		}

		result.Items = append(result.Items, item)
	}

	return result, nil
}

// ExportBackup exports all accounts and profiles into a password-protected archive.
func (s *AccountService) ExportBackup(destZipPath, password string) (*models.BackupExportResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}
	if len(password) < config.MinPasswordLength {
		return nil, errors.New("mật khẩu backup phải có ít nhất 5 ký tự")
	}

	accounts, err := s.database.ListGoogleAccounts()
	if err != nil {
		return nil, fmt.Errorf("lỗi đọc danh sách tài khoản: %w", err)
	}
	if len(accounts) == 0 {
		return nil, errors.New("chưa có tài khoản nào trong hệ thống để sao lưu")
	}

	if err := os.MkdirAll(filepath.Dir(destZipPath), 0700); err != nil {
		return nil, fmt.Errorf("không thể tạo thư mục lưu trữ: %w", err)
	}

	stagingFile, err := os.CreateTemp(s.baseDataDir, ".dezuxk-backup-*.zip")
	if err != nil {
		return nil, fmt.Errorf("không thể tạo file backup tạm: %w", err)
	}
	stagingPath := stagingFile.Name()
	defer os.Remove(stagingPath)
	if err := stagingFile.Chmod(0600); err != nil {
		_ = stagingFile.Close()
		return nil, fmt.Errorf("không thể bảo vệ file backup tạm: %w", err)
	}

	zipFile := stagingFile
	zipWriter := zip.NewWriter(zipFile)
	defer func() {
		_ = zipWriter.Close()
		_ = zipFile.Close()
	}()

	// 1. Build accounts.json
	rawExportList := make([]GLabsBackupAccount, 0, len(accounts))
	for _, a := range accounts {
		proxyVal := a.Proxy
		var proxyPtr *string
		if proxyVal != "" {
			proxyPtr = &proxyVal
		}
		rawExportList = append(rawExportList, GLabsBackupAccount{
			Email:        a.Email,
			Cookie:       a.Cookies,
			Tier:         a.Tier,
			Credits:      a.Credits,
			Token:        a.SNlM0eToken,
			Status:       strings.ToLower(a.Status),
			Proxy:        proxyPtr,
			Enabled:      a.Status == "ACTIVE",
			ImageEnabled: a.ImageEnabled,
			VideoEnabled: a.VideoEnabled,
			HasProfile:   a.ProfileDir != "",
			UserAgent:    a.UserAgent,
		})
	}

	accountsJSONData, err := json.MarshalIndent(rawExportList, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo dữ liệu accounts.json: %w", err)
	}

	accHeader := &zip.FileHeader{
		Name:   "accounts.json",
		Method: zip.Deflate,
	}
	accWriter, err := zipWriter.CreateHeader(accHeader)
	if err != nil {
		return nil, err
	}
	if _, err := accWriter.Write(accountsJSONData); err != nil {
		return nil, err
	}

	// 2. Build manifest.json
	profilesExported := 0
	for _, a := range accounts {
		if s.isManagedProfileDir(a.ProfileDir) {
			if info, err := os.Stat(a.ProfileDir); err == nil && info.IsDir() {
				profilesExported++
			}
		}
	}

	manifestData, err := json.Marshal(map[string]interface{}{
		"kind":         "flow",
		"accounts":     len(accounts),
		"profiles_dir": "flow_profiles",
		"profiles":     profilesExported,
		"version":      1,
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo manifest backup: %w", err)
	}

	manHeader := &zip.FileHeader{
		Name:   "manifest.json",
		Method: zip.Deflate,
	}
	manWriter, err := zipWriter.CreateHeader(manHeader)
	if err != nil {
		return nil, err
	}
	if _, err := manWriter.Write(manifestData); err != nil {
		return nil, err
	}

	// 3. Compress flow_profiles/<email>/ (skipping bloated caches)
	for _, a := range accounts {
		if !s.isManagedProfileDir(a.ProfileDir) {
			continue
		}
		if info, err := os.Stat(a.ProfileDir); err != nil || !info.IsDir() {
			continue
		}

		profileFolderName := profileArchiveName(a.Email, a.ID)

		walkErr := filepath.Walk(a.ProfileDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(a.ProfileDir, path)
			if err != nil || rel == "." {
				return nil
			}

			name := info.Name()
			if info.IsDir() && (name == "Code Cache" || name == "CacheStorage" || name == "DawnCache" ||
				name == "ShaderCache" || name == "GPUCache" || name == "blob_storage" ||
				name == "Crashpad" || name == "GrShaderCache" || name == "BrowserMetrics") {
				return filepath.SkipDir
			}

			zipPath := filepath.ToSlash(filepath.Join("flow_profiles", profileFolderName, rel))
			if info.IsDir() {
				zipPath += "/"
				_, err := zipWriter.Create(zipPath)
				return err
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = zipPath
			header.Method = zip.Deflate

			w, err := zipWriter.CreateHeader(header)
			if err != nil {
				return err
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(w, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if walkErr != nil {
			return nil, fmt.Errorf("không thể đóng gói profile %s: %w", a.Email, walkErr)
		}
	}

	if err := zipWriter.Close(); err != nil {
		_ = zipFile.Close()
		return nil, fmt.Errorf("không thể hoàn tất file backup tạm: %w", err)
	}
	if err := zipFile.Close(); err != nil {
		return nil, fmt.Errorf("không thể đóng file backup tạm: %w", err)
	}
	plainArchive, err := os.ReadFile(stagingPath)
	if err != nil {
		return nil, fmt.Errorf("không thể đọc file backup tạm: %w", err)
	}
	encryptedArchive, err := appcrypto.EncryptBackup(plainArchive, password)
	for i := range plainArchive {
		plainArchive[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("không thể mã hoá backup: %w", err)
	}
	if err := os.WriteFile(destZipPath, encryptedArchive, 0600); err != nil {
		return nil, fmt.Errorf("không thể ghi file backup: %w", err)
	}

	fi, err := os.Stat(destZipPath)
	sizeBytes := int64(0)
	if err == nil {
		sizeBytes = fi.Size()
	}

	return &models.BackupExportResult{
		Success:      true,
		FilePath:     destZipPath,
		AccountCount: len(accounts),
		ProfileCount: profilesExported,
		SizeBytes:    sizeBytes,
		Message:      fmt.Sprintf("Đã sao lưu mã hoá thành công %d tài khoản và %d profiles vào %s (%d KB)", len(accounts), profilesExported, filepath.Base(destZipPath), sizeBytes/1024),
	}, nil
}

// TestProxy verifies connectivity, measures latency (ms) and queries public egress IP through the given proxy.
func (s *AccountService) TestProxy(proxyStr string) (*models.ProxyTestResult, error) {
	proxyStr = strings.TrimSpace(proxyStr)
	if proxyStr == "" {
		return &models.ProxyTestResult{
			Success:   false,
			Message:   "Địa chỉ proxy rỗng",
			LatencyMs: 0,
		}, errors.New("địa chỉ proxy rỗng")
	}

	serverFlag, user, pass := chrome.FormatChromeProxyFlag(proxyStr)
	if serverFlag == "" {
		return &models.ProxyTestResult{
			Success:   false,
			Message:   "Định dạng proxy không hợp lệ (hỗ trợ http://user:pass@ip:port, ip:port:user:pass, hoặc ip:port)",
			LatencyMs: 0,
		}, errors.New("định dạng proxy không hợp lệ")
	}

	proxyURL, err := url.Parse(serverFlag)
	if err != nil {
		return &models.ProxyTestResult{
			Success:   false,
			Message:   "Địa chỉ proxy không hợp lệ",
			LatencyMs: 0,
		}, err
	}

	if user != "" && proxyURL.User == nil {
		proxyURL.User = url.UserPassword(user, pass)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
	}

	start := time.Now()
	resp, err := client.Get("https://api.ipify.org?format=json")
	latency := time.Since(start).Milliseconds()

	if err != nil {
		// Fallback checking google.com
		start = time.Now()
		resp2, err2 := client.Get("https://www.google.com")
		latency2 := time.Since(start).Milliseconds()
		if err2 != nil {
			return &models.ProxyTestResult{
				Success:   false,
				LatencyMs: latency,
				Message:   "Kết nối proxy thất bại",
			}, nil
		}
		defer resp2.Body.Close()
		return &models.ProxyTestResult{
			Success:   true,
			LatencyMs: latency2,
			Message:   fmt.Sprintf("Kết nối proxy thành công tới Google (%d ms)!", latency2),
		}, nil
	}
	defer resp.Body.Close()

	var ipData struct {
		IP string `json:"ip"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ipData)

	return &models.ProxyTestResult{
		Success:   true,
		LatencyMs: latency,
		EgressIP:  ipData.IP,
		Message:   fmt.Sprintf("Proxy hoạt động hoàn hảo! IP Egress: %s, Độ trễ: %d ms", ipData.IP, latency),
	}, nil
}

// GLabsBackupAccount represents the account schema in G-Labs accounts.json.
type GLabsBackupAccount struct {
	Email        string          `json:"email"`
	Cookie       string          `json:"cookie"`
	Cookies      json.RawMessage `json:"cookies,omitempty"`
	Tier         string          `json:"tier"`
	Credits      int             `json:"credits"`
	Token        string          `json:"token"`
	Status       string          `json:"status"`
	Proxy        *string         `json:"proxy"`
	Enabled      bool            `json:"enabled"`
	ImageEnabled bool            `json:"image_enabled"`
	VideoEnabled bool            `json:"video_enabled"`
	HasProfile   bool            `json:"has_profile"`
	UserAgent    string          `json:"user_agent"`
}

// ImportGLabsBackup imports accounts and profiles from an encrypted export or
// a compatible external G-Labs folder/zip. Password is required only for the
// encrypted format.
func (s *AccountService) ImportGLabsBackup(backupPath string, password ...string) (*models.RestoreResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	// 1. If backupPath is empty, dynamically auto-discover newest backup in user's Downloads directory
	if strings.TrimSpace(backupPath) == "" {
		userProfile := os.Getenv("USERPROFILE")
		downloadsDir := filepath.Join(userProfile, "Downloads")
		if entries, err := os.ReadDir(downloadsDir); err == nil {
			var bestMatch string
			var newestModTime time.Time
			for _, e := range entries {
				lower := strings.ToLower(e.Name())
				if strings.Contains(lower, "glabs") || strings.Contains(lower, "flow-accounts") {
					fullPath := filepath.Join(downloadsDir, e.Name())
					info, sErr := os.Stat(fullPath)
					if sErr == nil && info.ModTime().After(newestModTime) {
						newestModTime = info.ModTime()
						bestMatch = fullPath
					}
				}
			}
			if bestMatch != "" {
				backupPath = bestMatch
			}
		}
		if backupPath == "" {
			return nil, errors.New("không tìm thấy gói backup glabs-flow-accounts trong Downloads")
		}
	}

	workDir := backupPath
	isZip := strings.HasSuffix(strings.ToLower(backupPath), ".zip")

	if isZip {
		tempDir := filepath.Join(s.baseDataDir, "temp_backup_import_"+generateAccID("job"))
		if err := os.MkdirAll(tempDir, 0700); err != nil {
			return nil, fmt.Errorf("không thể tạo thư mục tạm: %w", err)
		}
		defer os.RemoveAll(tempDir)

		archivePath := backupPath
		archiveBytes, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return nil, fmt.Errorf("không thể đọc file backup: %w", readErr)
		}
		if bytes.HasPrefix(archiveBytes, []byte(appcrypto.BackupPrefix)) {
			backupPassword := ""
			if len(password) > 0 {
				backupPassword = password[0]
			}
			decrypted, decryptErr := appcrypto.DecryptBackup(archiveBytes, backupPassword)
			for i := range archiveBytes {
				archiveBytes[i] = 0
			}
			if decryptErr != nil {
				return nil, fmt.Errorf("không thể mở backup mã hoá: %w", decryptErr)
			}
			archivePath = filepath.Join(tempDir, "decrypted.zip")
			if err := os.WriteFile(archivePath, decrypted, 0600); err != nil {
				return nil, fmt.Errorf("không thể tạo file backup giải mã tạm: %w", err)
			}
			for i := range decrypted {
				decrypted[i] = 0
			}
		}

		if err := unzipTo(archivePath, tempDir); err != nil {
			return nil, fmt.Errorf("không thể giải nén file zip: %w", err)
		}
		workDir = tempDir
	}

	// 2. Locate accounts.json
	accountsJsonPath := filepath.Join(workDir, "accounts.json")
	if strings.HasSuffix(strings.ToLower(workDir), "accounts.json") {
		accountsJsonPath = workDir
		workDir = filepath.Dir(workDir)
	} else if _, err := os.Stat(accountsJsonPath); err != nil {
		// Try looking inside first child directory
		entries, _ := os.ReadDir(workDir)
		for _, e := range entries {
			if e.IsDir() {
				sub := filepath.Join(workDir, e.Name(), "accounts.json")
				if _, subErr := os.Stat(sub); subErr == nil {
					accountsJsonPath = sub
					workDir = filepath.Join(workDir, e.Name())
					break
				}
			}
		}
	}

	accountsData, err := os.ReadFile(accountsJsonPath)
	if err != nil {
		return nil, fmt.Errorf("không tìm thấy accounts.json: %w", err)
	}

	var rawAccounts []GLabsBackupAccount
	if err := json.Unmarshal(accountsData, &rawAccounts); err != nil {
		return nil, fmt.Errorf("lỗi đọc JSON tài khoản: %w", err)
	}

	profilesDir := filepath.Join(workDir, "flow_profiles")
	if _, err := os.Stat(profilesDir); err != nil {
		// Also check browser_profiles
		bDir := filepath.Join(workDir, "browser_profiles")
		if _, bErr := os.Stat(bDir); bErr == nil {
			profilesDir = bDir
		}
	}

	targetProfilesRoot := filepath.Join(s.baseDataDir, "profiles")
	_ = os.MkdirAll(targetProfilesRoot, 0700)

	importedCount := 0
	profilesRestored := 0
	var restoreErrors []string
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, raw := range rawAccounts {
		email := strings.ToLower(strings.TrimSpace(raw.Email))
		if normalizedEmail, emailErr := normalizeGoogleEmail(email); emailErr != nil {
			restoreErrors = append(restoreErrors, "bỏ qua một tài khoản có email không hợp lệ")
			continue
		} else {
			email = normalizedEmail
		}

		var accountID string
		var profileDir string

		existing, _ := s.database.GetGoogleAccountByEmail(email)
		if existing != nil {
			accountID = existing.ID
			if s.isManagedProfileDir(existing.ProfileDir) {
				profileDir = existing.ProfileDir
			} else {
				profileDir = filepath.Join(targetProfilesRoot, accountID)
			}
		} else {
			accountID = generateAccID("acc")
			profileDir = filepath.Join(targetProfilesRoot, accountID)
		}

		// Copy Chrome profile if exists
		srcProfile := filepath.Join(profilesDir, profileArchiveName(email, accountID))
		if _, err := os.Stat(srcProfile); err == nil {
			if err := copyProfileTree(srcProfile, profileDir); err == nil {
				profilesRestored++
			}
		}

		tier := normalizeTier(raw.Tier)
		credits := raw.Credits
		if credits < 0 {
			credits = 0
		}
		proxyVal := ""
		if raw.Proxy != nil {
			proxyVal = *raw.Proxy
		}
		cookieHeader := strings.TrimSpace(raw.Cookie)
		if cookieHeader == "" {
			cookieHeader = backupCookieHeader(raw.Cookies)
		}
		if !hasSessionCookie(cookieHeader) {
			restoreErrors = append(restoreErrors, fmt.Sprintf("bỏ qua tài khoản %s vì thiếu cookie phiên Google", email))
			continue
		}

		status := "PENDING"
		rawStatus := strings.ToLower(strings.TrimSpace(raw.Status))
		if rawStatus == "disabled" || rawStatus == "expired" || (rawStatus == "" && !raw.Enabled) {
			status = "DISABLED"
		}
		token := strings.TrimSpace(raw.Token)
		if strings.EqualFold(token, "flow") {
			token = ""
		}

		name := strings.Split(email, "@")[0]

		acc := &models.GoogleAccount{
			ID:            accountID,
			Email:         email,
			Name:          name,
			ProfileDir:    profileDir,
			Cookies:       cookieHeader,
			SNlM0eToken:   token,
			Services:      "flow,gemini",
			Status:        status,
			Tier:          tier,
			Credits:       credits,
			Proxy:         proxyVal,
			ImageEnabled:  raw.ImageEnabled,
			VideoEnabled:  raw.VideoEnabled,
			UserAgent:     raw.UserAgent,
			LastRefreshAt: nowStr,
			CreatedAt:     nowStr,
			UpdatedAt:     nowStr,
		}

		if err := s.database.UpsertGoogleAccount(acc); err != nil {
			restoreErrors = append(restoreErrors, fmt.Sprintf("không thể nạp tài khoản %s", email))
			continue
		}

		importedCount++
	}

	return &models.RestoreResult{
		Success:          len(restoreErrors) == 0,
		AccountsRestored: importedCount,
		ProfilesRestored: profilesRestored,
		Errors:           restoreErrors,
		Message:          fmt.Sprintf("Đã nạp %d tài khoản và %d hồ sơ trình duyệt từ bản sao lưu.", importedCount, profilesRestored),
	}, nil
}

func backupCookieHeader(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var header string
	if json.Unmarshal(raw, &header) == nil {
		return strings.TrimSpace(header)
	}

	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return strings.TrimSpace(strings.Join(values, "; "))
	}

	var cookies []chrome.CDPCookie
	if json.Unmarshal(raw, &cookies) == nil {
		header, _, _ := chrome.FilterGoogleCookies(cookies)
		return header
	}
	return ""
}

func normalizeService(raw string) string {
	// A Google session is shared by both products. Keep the input parameter for
	// backwards-compatible imports, but never create a single-service account.
	_ = raw
	return "flow,gemini"
}

func normalizeGoogleEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" {
		return "", errors.New("email Google là bắt buộc")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.ContainsAny(email, `/\\`) {
		return "", errors.New("email Google không hợp lệ")
	}
	return email, nil
}

func normalizeTier(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "PRO":
		return "PRO"
	case "ULTRA":
		return "ULTRA"
	default:
		return "FREE"
	}
}

func hasSessionCookie(cookies string) bool {
	return hasCookieName(cookies, "__Secure-1PSID") || hasCookieName(cookies, "SID")
}

func hasCookieName(header, wanted string) bool {
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(name) == wanted && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func accountProbeTarget(services string) (service, endpoint string) {
	services = strings.ToLower(services)
	if strings.Contains(services, "flow") {
		return "flow.google.com", "https://flow.google.com/"
	}
	return "gemini.google.com", "https://gemini.google.com/app"
}

func generateAccID(prefix string) string {
	bytes := make([]byte, 6)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(bytes))
}

func profileArchiveName(email, accountID string) string {
	name := strings.TrimSpace(email)
	if name == "" {
		name = accountID
	}
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, `\`, "_")
	name = strings.Map(func(r rune) rune {
		switch r {
		case ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			if r < 32 {
				return '_'
			}
			return r
		}
	}, name)
	if name == "" {
		return accountID
	}
	return name
}

func copyProfileTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Skip bulky caches
		name := info.Name()
		if info.IsDir() && (name == "Code Cache" || name == "CacheStorage" || name == "DawnCache" || name == "ShaderCache" || name == "GPUCache" || name == "blob_storage" || name == "Crashpad" || name == "GrShaderCache") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		return copySingleFile(path, target)
	})
}

func copySingleFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func unzipTo(srcZip, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	destRoot, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip entry dạng symlink không được phép: %s", f.Name)
		}
		fpath, err := filepath.Abs(filepath.Join(destRoot, filepath.FromSlash(f.Name)))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(destRoot, fpath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry nằm ngoài thư mục đích: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0700); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			_ = outFile.Close()
			_ = rc.Close()
			return err
		}
		if err := outFile.Close(); err != nil {
			_ = rc.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			return err
		}
	}
	return nil
}

// PurgeAccountCaches scans all account profile directories and removes bulky browser caches (GPUCache, Code Cache, etc.)
func (s *AccountService) PurgeAccountCaches() (*models.CachePurgeResult, error) {
	profilesDir := filepath.Join(s.baseDataDir, "profiles")
	if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
		return &models.CachePurgeResult{
			Success:         true,
			FreedBytes:      0,
			ProfilesCleaned: 0,
			Message:         "Thư mục profiles trống, không có dữ liệu cache cần dọn dẹp.",
		}, nil
	}

	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, fmt.Errorf("lỗi đọc thư mục profiles: %w", err)
	}

	cacheNames := map[string]bool{
		"Code Cache":     true,
		"CacheStorage":   true,
		"DawnCache":      true,
		"ShaderCache":    true,
		"GPUCache":       true,
		"blob_storage":   true,
		"Crashpad":       true,
		"GrShaderCache":  true,
		"BrowserMetrics": true,
	}

	var totalFreed int64
	profilesCleaned := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		accProfileDir := filepath.Join(profilesDir, e.Name())
		cleanedThisProfile := false

		_ = filepath.Walk(accProfileDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			if cacheNames[info.Name()] {
				// Calculate size before deleting
				var size int64
				_ = filepath.Walk(path, func(_ string, f os.FileInfo, _ error) error {
					if f != nil && !f.IsDir() {
						size += f.Size()
					}
					return nil
				})
				if err := os.RemoveAll(path); err == nil {
					totalFreed += size
					cleanedThisProfile = true
				}
				return filepath.SkipDir
			}
			return nil
		})

		if cleanedThisProfile {
			profilesCleaned++
		}
	}

	freedMB := float64(totalFreed) / (1024 * 1024)
	return &models.CachePurgeResult{
		Success:         true,
		FreedBytes:      totalFreed,
		ProfilesCleaned: profilesCleaned,
		Message:         fmt.Sprintf("Đã dọn dẹp thành công %.2f MB dữ liệu cache từ %d hồ sơ Chrome!", freedMB, profilesCleaned),
	}, nil
}
