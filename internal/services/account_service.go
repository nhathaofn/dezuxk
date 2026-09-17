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
	"net/url"
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

// OpenAccountBrowser launches Chrome with the account's dedicated profile directory and proxy.
func (s *AccountService) OpenAccountBrowser(accountID string) error {
	if s.database == nil || s.loginManager == nil {
		return errors.New("service not ready")
	}

	acc, err := s.database.GetGoogleAccountByID(accountID)
	if err != nil {
		return err
	}

	targetURL := "https://flow.google.com"
	if strings.Contains(strings.ToLower(acc.Services), "gemini") && !strings.Contains(strings.ToLower(acc.Services), "flow") {
		targetURL = "https://gemini.google.com/app"
	}

	return s.loginManager.OpenBrowser(acc.ProfileDir, targetURL, acc.Proxy)
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

// AddAccountManual inserts a Google account provided manually with cookies/tokens.
func (s *AccountService) AddAccountManual(input models.ManualAccountInput) (*models.GoogleAccountResponse, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}
	email := strings.TrimSpace(input.Email)
	cookies := strings.TrimSpace(input.Cookies)
	if cookies == "" {
		return nil, errors.New("vui lòng nhập chuỗi cookies của tài khoản Google")
	}

	if email == "" {
		email = fmt.Sprintf("google_user_%s@gmail.com", generateAccID("u")[2:])
	}

	accountID := generateAccID("acc")
	profileDir := filepath.Join(s.baseDataDir, "profiles", accountID)
	_ = os.MkdirAll(profileDir, 0755)

	existing, _ := s.database.GetGoogleAccountByEmail(email)
	if existing != nil {
		accountID = existing.ID
		if existing.ProfileDir != "" {
			profileDir = existing.ProfileDir
		}
	}

	tier := input.Tier
	if tier == "" {
		tier = "PRO"
	}
	credits := input.Credits
	if credits <= 0 {
		credits = 1050
	}
	service := input.Service
	if service == "" {
		service = "flow,gemini"
	}

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
	acc := &models.GoogleAccount{
		ID:            accountID,
		Email:         email,
		Name:          strings.Split(email, "@")[0],
		ProfileDir:    profileDir,
		Cookies:       cookies,
		SNlM0eToken:   strings.TrimSpace(input.SNlM0eToken),
		Services:      service,
		Status:        "ACTIVE",
		Tier:          tier,
		Credits:       credits,
		Proxy:         strings.TrimSpace(input.Proxy),
		ImageEnabled:  true,
		VideoEnabled:  true,
		LastRefreshAt: nowStr,
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
	_ = os.MkdirAll(targetProfilesRoot, 0755)

	defaultTier := input.DefaultTier
	if defaultTier == "" {
		defaultTier = "PRO"
	}
	service := input.Service
	if service == "" {
		service = "flow,gemini"
	}
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		result.TotalParsed++

		var item models.BulkAccountItem

		// Check if line is JSON format
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var jsonAcc struct {
				Email         string `json:"email"`
				Password      string `json:"password"`
				RecoveryEmail string `json:"recovery_email"`
				Proxy         string `json:"proxy"`
				Cookie        string `json:"cookie"`
				Cookies       string `json:"cookies"`
			}
			if err := json.Unmarshal([]byte(line), &jsonAcc); err == nil && jsonAcc.Email != "" {
				item.Email = jsonAcc.Email
				item.Password = jsonAcc.Password
				item.RecoveryEmail = jsonAcc.RecoveryEmail
				item.Proxy = jsonAcc.Proxy
				if jsonAcc.Cookie != "" {
					item.Cookies = jsonAcc.Cookie
				} else {
					item.Cookies = jsonAcc.Cookies
				}
			}
		}

		if item.Email == "" {
			var parts []string
			if strings.Contains(line, "|") {
				parts = strings.Split(line, "|")
			} else if strings.Contains(line, "\t") {
				parts = strings.Split(line, "\t")
			} else if strings.Count(line, ":") >= 1 && !strings.HasPrefix(line, "http") {
				parts = strings.Split(line, ":")
			} else {
				parts = []string{line}
			}

			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}

			if len(parts) >= 1 && strings.Contains(parts[0], "@") {
				item.Email = parts[0]
				if len(parts) >= 2 {
					item.Password = parts[1]
				}
				if len(parts) >= 3 {
					if strings.Contains(parts[2], "@") {
						item.RecoveryEmail = parts[2]
					} else if strings.Contains(parts[2], ":") || strings.HasPrefix(parts[2], "http") {
						item.Proxy = parts[2]
					}
				}
				if len(parts) >= 4 {
					item.Proxy = parts[3]
				}
			} else if len(parts) == 1 && (strings.Contains(parts[0], "__Secure-1PSID") || strings.Contains(parts[0], "SID=")) {
				item.Cookies = parts[0]
				item.Email = fmt.Sprintf("google_user_%s@gmail.com", generateAccID("u")[2:])
			} else {
				item.Status = "ERROR"
				item.Message = "Định dạng dòng không hợp lệ"
				result.FailedCount++
				result.Items = append(result.Items, item)
				continue
			}
		}

		if item.Proxy == "" && input.DefaultProxy != "" {
			item.Proxy = strings.TrimSpace(input.DefaultProxy)
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
		if existing != nil {
			accountID = existing.ID
			profileDir = existing.ProfileDir
			if profileDir == "" {
				profileDir = filepath.Join(targetProfilesRoot, accountID)
			}
		} else {
			accountID = generateAccID("acc")
			profileDir = filepath.Join(targetProfilesRoot, accountID)
		}
		_ = os.MkdirAll(profileDir, 0755)

		acc := &models.GoogleAccount{
			ID:            accountID,
			Email:         item.Email,
			Name:          strings.Split(item.Email, "@")[0],
			ProfileDir:    profileDir,
			Cookies:       item.Cookies,
			Services:      service,
			Status:        "ACTIVE",
			Tier:          defaultTier,
			Credits:       1050,
			Proxy:         item.Proxy,
			Password:      item.Password,
			RecoveryEmail: item.RecoveryEmail,
			ImageEnabled:  true,
			VideoEnabled:  true,
			LastRefreshAt: nowStr,
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

// ExportBackup exports all accounts, manifest.json, and flow_profiles to a zip file.
func (s *AccountService) ExportBackup(destZipPath string) (*models.BackupExportResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
	}

	accounts, err := s.database.ListGoogleAccounts()
	if err != nil {
		return nil, fmt.Errorf("lỗi đọc danh sách tài khoản: %w", err)
	}
	if len(accounts) == 0 {
		return nil, errors.New("chưa có tài khoản nào trong hệ thống để sao lưu")
	}

	if err := os.MkdirAll(filepath.Dir(destZipPath), 0755); err != nil {
		return nil, fmt.Errorf("không thể tạo thư mục lưu trữ: %w", err)
	}

	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return nil, fmt.Errorf("không thể tạo file zip: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

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
			Token:        "flow",
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
		if a.ProfileDir != "" {
			if _, err := os.Stat(a.ProfileDir); err == nil {
				profilesExported++
			}
		}
	}

	manifestData, _ := json.Marshal(map[string]interface{}{
		"kind":         "flow",
		"accounts":     len(accounts),
		"profiles_dir": "flow_profiles",
		"profiles":     profilesExported,
		"version":      1,
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
	})

	manHeader := &zip.FileHeader{
		Name:   "manifest.json",
		Method: zip.Deflate,
	}
	manWriter, err := zipWriter.CreateHeader(manHeader)
	if err == nil {
		_, _ = manWriter.Write(manifestData)
	}

	// 3. Compress flow_profiles/<email>/ (skipping bloated caches)
	for _, a := range accounts {
		if a.ProfileDir == "" {
			continue
		}
		if _, err := os.Stat(a.ProfileDir); err != nil {
			continue
		}

		profileFolderName := a.Email
		if profileFolderName == "" {
			profileFolderName = a.ID
		}

		_ = filepath.Walk(a.ProfileDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
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
				_, _ = zipWriter.Create(zipPath)
				return nil
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return nil
			}
			header.Name = zipPath
			header.Method = zip.Deflate

			w, err := zipWriter.CreateHeader(header)
			if err != nil {
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			_, _ = io.Copy(w, f)
			return nil
		})
	}

	_ = zipWriter.Close()
	_ = zipFile.Close()

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
		Message:      fmt.Sprintf("Đã sao lưu thành công %d tài khoản và %d profiles vào %s (%d KB)", len(accounts), profilesExported, filepath.Base(destZipPath), sizeBytes/1024),
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
			Message:   fmt.Sprintf("Không thể phân tích URL proxy: %v", err),
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
				Message:   fmt.Sprintf("Kết nối proxy thất bại: %v", err),
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
func (s *AccountService) ImportGLabsBackup(backupPath string) (*models.RestoreResult, error) {
	if s.database == nil {
		return nil, errors.New("database not initialized")
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
			return nil, errors.New("không tìm thấy gói backup glabs-flow-accounts trong Downloads")
		}
	}

	workDir := backupPath
	isZip := strings.HasSuffix(strings.ToLower(backupPath), ".zip")

	if isZip {
		tempDir := filepath.Join(s.baseDataDir, "temp_backup_import")
		_ = os.RemoveAll(tempDir)
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return nil, fmt.Errorf("không thể tạo thư mục tạm: %w", err)
		}
		defer os.RemoveAll(tempDir)

		if err := unzipTo(backupPath, tempDir); err != nil {
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
	_ = os.MkdirAll(targetProfilesRoot, 0755)

	importedCount := 0
	profilesRestored := 0
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, raw := range rawAccounts {
		email := strings.TrimSpace(raw.Email)
		if email == "" {
			continue
		}

		var accountID string
		var profileDir string

		existing, _ := s.database.GetGoogleAccountByEmail(email)
		if existing != nil {
			accountID = existing.ID
			profileDir = existing.ProfileDir
			if profileDir == "" {
				profileDir = filepath.Join(targetProfilesRoot, accountID)
			}
		} else {
			accountID = generateAccID("acc")
			profileDir = filepath.Join(targetProfilesRoot, accountID)
		}

		// Copy Chrome profile if exists
		srcProfile := filepath.Join(profilesDir, email)
		if _, err := os.Stat(srcProfile); err == nil {
			if err := copyProfileTree(srcProfile, profileDir); err == nil {
				profilesRestored++
			}
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
			SNlM0eToken:   raw.Token,
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

	return &models.RestoreResult{
		Success:          true,
		AccountsRestored: importedCount,
		ProfilesRestored: profilesRestored,
		Message:          fmt.Sprintf("Đã nạp thành công %d tài khoản và %d hồ sơ trình duyệt từ bản sao lưu!", importedCount, profilesRestored),
	}, nil
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
