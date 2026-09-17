package services

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dezuxk/internal/db"
	"dezuxk/internal/models"
	"dezuxk/internal/services/chrome"
)

// AccountService manages Google account business logic, authentication lifecycle, and verification.
type AccountService struct {
	database        *db.DB
	loginManager    *chrome.LoginManager
	keepAliveWorker *chrome.KeepAliveWorker
	baseDataDir     string
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

	return &AccountService{
		database:        database,
		loginManager:    loginMgr,
		keepAliveWorker: worker,
		baseDataDir:     baseDataDir,
	}
}

// StartBackgroundWorkers starts periodic background maintenance tasks.
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

// StartLogin begins an interactive login session using the system's real Chrome.
func (s *AccountService) StartLogin(service string) (*models.LoginSessionStatus, error) {
	if s.loginManager == nil {
		return nil, errors.New("login manager not initialized")
	}
	return s.loginManager.StartLogin(service)
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

	// Remove persistent profile directory on disk
	if acc.ProfileDir != "" {
		_ = os.RemoveAll(acc.ProfileDir)
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

	if err := s.keepAliveWorker.RefreshAccount(acc); err != nil {
		return nil, fmt.Errorf("làm mới phiên thất bại: %w", err)
	}

	updated, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}

	return updated.ToResponse(), nil
}

// TestSession verifies the account's credentials against an upstream Google endpoint.
func (s *AccountService) TestSession(accountID string) (*models.AccountTestResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	// Perform a lightweight probe to Gemini Web using the stored cookies
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.Contains(req.URL.String(), "accounts.google.com") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", "https://flow.google.com", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", acc.Cookies)

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &models.AccountTestResult{
			Success:       false,
			Message:       fmt.Sprintf("Không thể kết nối tới Flow Google: %v", err),
			LatencyMs:     latency,
			TestedService: "flow.google.com",
			Timestamp:     time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &models.AccountTestResult{
			Success:       true,
			Message:       "Phiên đăng nhập Flow Google hoàn toàn hợp lệ!",
			LatencyMs:     latency,
			TestedService: "flow.google.com",
			Timestamp:     time.Now().UTC(),
		}, nil
	}

	if resp.StatusCode == 302 || resp.StatusCode == 401 || resp.StatusCode == 403 {
		_ = s.database.UpdateGoogleAccountStatus(acc.ID, "EXPIRED", "Google yêu cầu xác thực lại (HTTP 302/401/403)")
		return &models.AccountTestResult{
			Success:       false,
			Message:       fmt.Sprintf("Phiên đăng nhập đã hết hạn trên Google (Status %d)", resp.StatusCode),
			LatencyMs:     latency,
			TestedService: "flow.google.com",
			Timestamp:     time.Now().UTC(),
		}, nil
	}

	return &models.AccountTestResult{
		Success:       true,
		Message:       fmt.Sprintf("Phản hồi từ Flow Google: Status %d", resp.StatusCode),
		LatencyMs:     latency,
		TestedService: "flow.google.com",
		Timestamp:     time.Now().UTC(),
	}, nil
}

// OpenAccountBrowser launches Chrome with the account's dedicated profile directory.
func (s *AccountService) OpenAccountBrowser(accountID string) error {
	if s.database == nil || s.loginManager == nil {
		return errors.New("service not ready")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return err
	}

	targetURL := "https://flow.google.com"
	return s.loginManager.OpenBrowser(acc.ProfileDir, targetURL)
}

// ToggleAccount switches an account between ACTIVE and DISABLED.
func (s *AccountService) ToggleAccount(accountID string, active bool) (*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
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
		if acc.Status != "DISABLED" {
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


// GLabsBackupAccount represents the account schema in G-Labs accounts.json.
type GLabsBackupAccount struct {
	Email        string  `json:"email"`
	Cookie       string  `json:"cookie"`
	Tier         string  `json:"tier"`
	Credits      int     `json:"credits"`
	Token        string  `json:"token"`
	Status       string  `json:"status"`
	Proxy        *string `json:"proxy"`
	Enabled      bool    `json:"enabled"`
	ImageEnabled bool    `json:"image_enabled"`
	VideoEnabled bool    `json:"video_enabled"`
	HasProfile   bool    `json:"has_profile"`
	UserAgent    string  `json:"user_agent"`
}

// ImportGLabsBackup imports accounts and profiles from a G-Labs backup folder or zip.
func (s *AccountService) ImportGLabsBackup(backupPath string) (int, error) {
	if s.database == nil {
		return 0, errors.New("database not initialized")
	}

	// 1. If backupPath is empty, attempt auto-discovery in user's Downloads directory
	if strings.TrimSpace(backupPath) == "" {
		userProfile := os.Getenv("USERPROFILE")
		downloadsDir := filepath.Join(userProfile, "Downloads")
		candidates := []string{
			filepath.Join(downloadsDir, "glabs-flow-accounts-20260917-160519"),
			filepath.Join(downloadsDir, "glabs-flow-accounts-20260917-160519.zip"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				backupPath = c
				break
			}
		}
		if backupPath == "" {
			return 0, errors.New("không tìm thấy gói backup glabs-flow-accounts trong Downloads")
		}
	}

	workDir := backupPath
	isZip := strings.HasSuffix(strings.ToLower(backupPath), ".zip")

	if isZip {
		tempDir := filepath.Join(s.baseDataDir, "temp_backup_import")
		_ = os.RemoveAll(tempDir)
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return 0, fmt.Errorf("không thể tạo thư mục tạm: %w", err)
		}
		defer os.RemoveAll(tempDir)

		if err := unzipTo(backupPath, tempDir); err != nil {
			return 0, fmt.Errorf("không thể giải nén file zip: %w", err)
		}
		workDir = tempDir
	}

	// 2. Locate accounts.json
	accountsJsonPath := filepath.Join(workDir, "accounts.json")
	if _, err := os.Stat(accountsJsonPath); err != nil {
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
		return 0, fmt.Errorf("không tìm thấy accounts.json: %w", err)
	}

	var rawAccounts []GLabsBackupAccount
	if err := json.Unmarshal(accountsData, &rawAccounts); err != nil {
		return 0, fmt.Errorf("lỗi đọc JSON tài khoản: %w", err)
	}

	profilesDir := filepath.Join(workDir, "flow_profiles")
	targetProfilesRoot := filepath.Join(s.baseDataDir, "profiles")
	_ = os.MkdirAll(targetProfilesRoot, 0755)

	importedCount := 0
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, raw := range rawAccounts {
		email := strings.TrimSpace(raw.Email)
		if email == "" {
			continue
		}

		// Find existing or allocate ID
		var accountID string
		var profileDir string

		existing, _ := s.database.GetGoogleAccountByEmail(email)
		if existing != nil {
			accountID = existing.ID
			profileDir = existing.ProfileDir
		} else {
			accountID = generateAccID("acc")
			profileDir = filepath.Join(targetProfilesRoot, accountID)
		}

		// Copy Chrome profile if exists
		srcProfile := filepath.Join(profilesDir, email)
		if _, err := os.Stat(srcProfile); err == nil {
			_ = copyProfileTree(srcProfile, profileDir)
		}

		tier := raw.Tier
		if tier == "" {
			tier = "PRO"
		}
		credits := raw.Credits
		if credits == 0 {
			credits = 1050
		}
		proxyVal := ""
		if raw.Proxy != nil {
			proxyVal = *raw.Proxy
		}

		status := "ACTIVE"
		if !raw.Enabled || strings.ToLower(raw.Status) == "expired" {
			status = "DISABLED"
		}

		name := strings.Split(email, "@")[0]

		acc := &models.GoogleAccount{
			ID:            accountID,
			Email:         email,
			Name:          name,
			ProfileDir:    profileDir,
			Cookies:       raw.Cookie,
			SNlM0eToken:   "",
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
			continue
		}

		importedCount++
	}

	return importedCount, nil
}

func generateAccID(prefix string) string {
	bytes := make([]byte, 6)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(bytes))
}

func copyProfileTree(src, dst string) error {
	_ = os.MkdirAll(dst, 0755)
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		// Skip bulky caches
		name := info.Name()
		if info.IsDir() && (name == "Code Cache" || name == "CacheStorage" || name == "DawnCache" || name == "ShaderCache" || name == "GPUCache" || name == "blob_storage" || name == "Crashpad" || name == "GrShaderCache") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
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

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func unzipTo(srcZip, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, _ = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
	}
	return nil
}
