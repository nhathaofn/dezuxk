package chrome

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dezuxk/internal/db"
	"dezuxk/internal/models"
)

// ActiveSession stores the runtime state of an interactive login process.
type ActiveSession struct {
	SessionID  string
	AccountID  string
	Service    string
	Proxy      string
	ProxyUser  string
	ProxyPass  string
	ProfileDir string
	Port       int
	Cmd        *exec.Cmd
	Status     *models.LoginSessionStatus
	CancelFunc context.CancelFunc
	CreatedAt  time.Time
}

// LoginManager orchestrates opening real Chrome with isolated profiles and listening via CDP.
type LoginManager struct {
	mu       sync.RWMutex
	sessions map[string]*ActiveSession
	database *db.DB
	baseDir  string
}

// NewLoginManager creates a new LoginManager.
func NewLoginManager(database *db.DB, baseDataDir string) *LoginManager {
	if baseDataDir == "" {
		baseDataDir = "data"
	}
	absDataDir, err := filepath.Abs(baseDataDir)
	if err == nil {
		baseDataDir = absDataDir
	}
	profilesDir := filepath.Join(baseDataDir, "profiles")
	_ = os.MkdirAll(profilesDir, 0755)

	return &LoginManager{
		sessions: make(map[string]*ActiveSession),
		database: database,
		baseDir:  profilesDir,
	}
}

// StartLogin spawns a real Chrome browser on an isolated profile and begins CDP monitoring.
func (m *LoginManager) StartLogin(service, proxy string) (*models.LoginSessionStatus, error) {
	chromePath, err := FindChromeExecutable()
	if err != nil {
		return nil, fmt.Errorf("không thể khởi chạy: %w", err)
	}

	sessionID := generateID("sess")
	accountID := generateID("acc")
	absProfileDir, err := filepath.Abs(filepath.Join(m.baseDir, accountID))
	if err != nil {
		absProfileDir = filepath.Join(m.baseDir, accountID)
	}
	if err := os.MkdirAll(absProfileDir, 0755); err != nil {
		return nil, fmt.Errorf("không thể tạo thư mục profile: %w", err)
	}

	port, err := findFreePort()
	if err != nil {
		return nil, fmt.Errorf("không tìm thấy cổng mạng trống: %w", err)
	}

	// Direct directly to Google Sign-In with auto-redirect to Gemini/Flow upon success
	targetURL := "https://accounts.google.com/ServiceLogin?continue=https%3A%2F%2Fgemini.google.com%2Fapp&hl=vi"
	if strings.ToLower(service) == "flow" {
		targetURL = "https://accounts.google.com/ServiceLogin?continue=https%3A%2F%2Fflow.google.com&hl=vi"
	}

	// Command flags to run a completely ISOLATED, DEDICATED Chrome window:
	// - Absolute --user-data-dir forces Chrome to create a brand new process and separate session sandbox
	// - --new-window forces a standalone window
	// - --remote-debugging-port allows CDP inspection without automation bot detection
	args := []string{
		fmt.Sprintf("--user-data-dir=%s", absProfileDir),
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--new-window",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--profile-directory=Default",
		"--window-size=1080,780",
		"--disable-blink-features=AutomationControlled",
	}

	var proxyUser, proxyPass string
	if strings.TrimSpace(proxy) != "" {
		serverFlag, u, p := FormatChromeProxyFlag(proxy)
		if serverFlag != "" {
			args = append(args, fmt.Sprintf("--proxy-server=%s", serverFlag))
			proxyUser = u
			proxyPass = p
		}
	}

	args = append(args, targetURL)

	cmd := exec.Command(chromePath, args...)

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(absProfileDir)
		return nil, fmt.Errorf("không thể khởi chạy Chrome: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	status := &models.LoginSessionStatus{
		SessionID: sessionID,
		Step:      models.LoginStepWaitingUserLogin,
		Message:   "Đã mở cửa sổ Chrome mới cho tài khoản. Vui lòng đăng nhập Google trên cửa sổ vừa mở...",
	}

	active := &ActiveSession{
		SessionID:  sessionID,
		AccountID:  accountID,
		Service:    service,
		Proxy:      strings.TrimSpace(proxy),
		ProxyUser:  proxyUser,
		ProxyPass:  proxyPass,
		ProfileDir: absProfileDir,
		Port:       port,
		Cmd:        cmd,
		Status:     status,
		CancelFunc: cancel,
		CreatedAt:  time.Now(),
	}

	m.mu.Lock()
	m.sessions[sessionID] = active
	m.mu.Unlock()

	// Launch background watcher to monitor login via CDP
	go m.watchLoginSession(ctx, active)

	return status, nil
}

// GetStatus returns the current status of a login session.
func (m *LoginManager) GetStatus(sessionID string) (*models.LoginSessionStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, errors.New("phiên đăng nhập không tồn tại hoặc đã hết hạn")
	}

	return session.Status, nil
}

// CancelLogin terminates the login process, closes Chrome, and cleans up temp files if incomplete.
func (m *LoginManager) CancelLogin(sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.Unlock()
		return errors.New("phiên đăng nhập không tồn tại")
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	session.CancelFunc()
	if session.Cmd != nil && session.Cmd.Process != nil {
		KillProcessTree(session.Cmd.Process.Pid)
	}

	// If never completed, clean up empty profile dir
	if session.Status.Step != models.LoginStepCompleted {
		_ = os.RemoveAll(session.ProfileDir)
	}

	return nil
}

func (m *LoginManager) watchLoginSession(ctx context.Context, s *ActiveSession) {
	defer func() {
		if s.Cmd != nil && s.Cmd.Process != nil {
			KillProcessTree(s.Cmd.Process.Pid)
		}
	}()

	processExited := make(chan struct{})
	go func() {
		if s.Cmd != nil {
			_ = s.Cmd.Wait()
		}
		close(processExited)
	}()

	// 1. Wait for debug port to become ready (up to 20 seconds)
	var readyTarget *CDPTarget
	var client *CDPClient
	startWait := time.Now()

	for {
		if ctx.Err() != nil {
			m.updateSessionStatus(s.SessionID, models.LoginStepCancelled, "Phiên đăng nhập đã bị hủy", nil, "")
			return
		}

		targets, err := QueryCDPTargets(s.Port)
		if err == nil && len(targets) > 0 {
			for _, t := range targets {
				if t.Type == "page" && t.WebSocketDebuggerURL != "" {
					targetCopy := t
					readyTarget = &targetCopy
					break
				}
			}
			if readyTarget != nil {
				break
			}
		}

		if time.Since(startWait) > 20*time.Second {
			m.updateSessionStatus(s.SessionID, models.LoginStepFailed, "Không thể kết nối tới cổng DevTools của Chrome", nil, "timeout waiting for debug port")
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Connect to page CDP WebSocket
	var err error
	client, err = ConnectCDP(ctx, readyTarget.WebSocketDebuggerURL)
	if err != nil {
		// Fallback to browser-level debugger URL if page target fails
		ver, vErr := QueryCDPVersion(s.Port)
		if vErr == nil && ver.WebSocketDebuggerURL != "" {
			client, err = ConnectCDP(ctx, ver.WebSocketDebuggerURL)
		}
	}

	if err != nil || client == nil {
		m.updateSessionStatus(s.SessionID, models.LoginStepFailed, "Không thể kết nối CDP WebSocket", nil, fmt.Sprintf("%v", err))
		return
	}
	defer client.Close()

	// Enable proxy credentials response if proxy requires authentication
	if s.ProxyUser != "" && s.ProxyPass != "" {
		_ = client.EnableProxyAuth(ctx, s.ProxyUser, s.ProxyPass)
	}

	// 2. Poll every 1.5s to detect successful Google login
	pollTicker := time.NewTicker(1500 * time.Millisecond)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.updateSessionStatus(s.SessionID, models.LoginStepCancelled, "Phiên đăng nhập đã hết thời gian hoặc bị hủy", nil, "")
			return
		case <-processExited:
			m.updateSessionStatus(s.SessionID, models.LoginStepCancelled, "Cửa sổ Chrome đã bị đóng trước khi hoàn tất đăng nhập", nil, "")
			return
		case <-pollTicker.C:
			// Read current targets to check current URL
			targets, err := QueryCDPTargets(s.Port)
			if err != nil || len(targets) == 0 {
				continue
			}

			var currentURL string
			for _, t := range targets {
				if t.Type == "page" {
					currentURL = t.URL
					break
				}
			}

			// Read all cookies via CDP
			cookies, err := client.GetCookies(ctx)
			if err != nil {
				continue
			}

			cookieHeader, hasPSID, hasPSIDTS := FilterGoogleCookies(cookies)

			// Check URL conditions:
			isSignInPage := strings.Contains(currentURL, "accounts.google.com/v3/signin") ||
				strings.Contains(currentURL, "accounts.google.com/ServiceLogin") ||
				strings.Contains(currentURL, "accounts.google.com/signin") ||
				strings.Contains(currentURL, "signin/challenge")

			isIntermediatePage := strings.Contains(currentURL, "myaccount.google.com") ||
				strings.Contains(currentURL, "accounts.google.com/b/") ||
				strings.Contains(currentURL, "gds.google.com") ||
				strings.Contains(currentURL, "policies.google.com")

			isTargetReached := false
			if strings.ToLower(s.Service) == "flow" {
				isTargetReached = strings.Contains(currentURL, "flow.google.com") || strings.Contains(currentURL, "labs.google")
			} else if strings.ToLower(s.Service) == "gemini" {
				isTargetReached = strings.Contains(currentURL, "gemini.google.com")
			}

			// Login is ready if:
			// 1. Target service page is reached, OR
			// 2. Has PSID and left sign-in pages AND left intermediate account pages
			loginReady := hasPSID && (isTargetReached || (!isSignInPage && !isIntermediatePage))

			if loginReady {
				// Login Detected! Transition to EXTRACTING_COOKIES
				m.updateSessionStatus(s.SessionID, models.LoginStepExtracting, "Đã phát hiện phiên đăng nhập! Đang trích xuất thông tin...", nil, "")

				time.Sleep(2 * time.Second)

				// Extract CSRF Token SNlM0e via JavaScript evaluation (primarily for Gemini)
				snlm0e := ""
				if strings.Contains(strings.ToLower(s.Service), "gemini") || strings.Contains(currentURL, "gemini.google.com") {
					snlm0e, _ = client.EvaluateJS(ctx, `(function() {
						if (window.WIZ_global_data && window.WIZ_global_data.SNlM0e) {
							return window.WIZ_global_data.SNlM0e;
						}
						var match = document.documentElement.innerHTML.match(/"SNlM0e":"([^"]+)"/);
						if (match && match[1]) return match[1];
						return "";
					})()`)
				}

				// Extract email and display name from page context
				extractedEmail, _ := client.EvaluateJS(ctx, `(function() {
					var el = document.querySelector('a[aria-label*="@"], button[aria-label*="@"]');
					if (el) {
						var label = el.getAttribute('aria-label') || '';
						var m = label.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
						if (m) return m[1];
					}
					return "";
				})()`)

				extractedName, _ := client.EvaluateJS(ctx, `(function() {
					var el = document.querySelector('a[aria-label*="@"], button[aria-label*="@"]');
					if (el) {
						var label = el.getAttribute('aria-label') || '';
						var parts = label.split('\n');
						if (parts.length > 0 && parts[0].trim()) return parts[0].trim();
					}
					return "";
				})()`)

				if extractedEmail == "" {
					extractedEmail = fmt.Sprintf("google_user_%s@gmail.com", s.AccountID[len(s.AccountID)-4:])
				}
				if extractedName == "" {
					extractedName = "Google Account"
				}

				// Build GoogleAccount model
				// Extract live dynamic credits and tier from the session
				liveCredits, liveTier := ExtractAccountCreditsAndTier(ctx, client)
				if liveTier == "" {
					liveTier = "FREE"
				}
				if liveCredits < 0 {
					liveCredits = 0
				}

				nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
				account := &models.GoogleAccount{
					ID:            s.AccountID,
					Email:         extractedEmail,
					Name:          extractedName,
					ProfileDir:    s.ProfileDir,
					Cookies:       cookieHeader,
					SNlM0eToken:   snlm0e,
					Services:      s.Service,
					Status:        "ACTIVE",
					Tier:          liveTier,
					Credits:       liveCredits,
					Proxy:         s.Proxy,
					ImageEnabled:  true,
					VideoEnabled:  true,
					LastRefreshAt: nowStr,
					CreatedAt:     nowStr,
					UpdatedAt:     nowStr,
				}

				if m.database != nil {
					if err := m.database.CreateGoogleAccount(account); err != nil {
						log.Printf("[LoginManager] Error saving account to DB: %v\n", err)
						m.updateSessionStatus(s.SessionID, models.LoginStepFailed, "Lỗi khi lưu tài khoản vào cơ sở dữ liệu", nil, err.Error())
						return
					}
				}

				// Gracefully close Chrome
				_ = client.CloseBrowser(ctx)
				if s.Cmd.Process != nil {
					KillProcessTree(s.Cmd.Process.Pid)
				}

				successMsg := fmt.Sprintf("Đăng nhập thành công tài khoản %s! Session đã được lưu trữ an toàn.", extractedEmail)
				if hasPSIDTS {
					successMsg += " (Tokens timestamp đã kích hoạt)"
				}

				m.updateSessionStatus(s.SessionID, models.LoginStepCompleted, successMsg, account.ToResponse(), "")
				return
			}
		}
	}
}

func (m *LoginManager) updateSessionStatus(sessionID string, step models.LoginStep, message string, account *models.GoogleAccountResponse, errStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if exists {
		session.Status.Step = step
		session.Status.Message = message
		session.Status.Account = account
		session.Status.Error = errStr
	}
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func generateID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// CleanURL removes query params for clean display
func CleanURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// FormatChromeProxyFlag parses various proxy formats (http://user:pass@ip:port, ip:port:user:pass, ip:port)
// and extracts the proxy server address suitable for Chrome's --proxy-server flag.
func FormatChromeProxyFlag(rawProxy string) (serverFlag, username, password string) {
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return "", "", ""
	}

	// Format: ip:port:user:pass
	if !strings.Contains(rawProxy, "://") && strings.Count(rawProxy, ":") == 3 {
		parts := strings.Split(rawProxy, ":")
		serverFlag = fmt.Sprintf("http://%s:%s", parts[0], parts[1])
		username = parts[2]
		password = parts[3]
		return
	}

	// Format: scheme://...
	if strings.Contains(rawProxy, "://") {
		u, err := url.Parse(rawProxy)
		if err == nil && u.Host != "" {
			serverFlag = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
			if u.User != nil {
				username = u.User.Username()
				password, _ = u.User.Password()
			}
			return
		}
	}

	// Format: ip:port
	if !strings.HasPrefix(rawProxy, "http://") && !strings.HasPrefix(rawProxy, "socks5://") {
		serverFlag = "http://" + rawProxy
	} else {
		serverFlag = rawProxy
	}
	return
}

// OpenBrowser launches a dedicated Chrome instance pointing to a saved account's profile directory.
func (m *LoginManager) OpenBrowser(profileDir, targetURL, proxy string) error {
	chromePath, err := FindChromeExecutable()
	if err != nil {
		return err
	}
	absProfileDir, err := filepath.Abs(profileDir)
	if err != nil {
		absProfileDir = profileDir
	}
	_ = os.MkdirAll(absProfileDir, 0755)

	if targetURL == "" {
		targetURL = "https://flow.google.com"
	}
	args := []string{
		fmt.Sprintf("--user-data-dir=%s", absProfileDir),
		"--new-window",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--profile-directory=Default",
		"--window-size=1200,850",
	}

	if proxy != "" {
		serverFlag, _, _ := FormatChromeProxyFlag(proxy)
		if serverFlag != "" {
			args = append(args, fmt.Sprintf("--proxy-server=%s", serverFlag))
		}
	}

	args = append(args, targetURL)
	cmd := exec.Command(chromePath, args...)
	return cmd.Start()
}

