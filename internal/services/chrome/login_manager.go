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
	FinishedAt time.Time
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
	_ = os.MkdirAll(profilesDir, 0700)

	return &LoginManager{
		sessions: make(map[string]*ActiveSession),
		database: database,
		baseDir:  profilesDir,
	}
}

// StartLogin spawns a real Chrome browser on an isolated profile and begins CDP monitoring.
// One Google session is shared by both Flow and Gemini; the service marker is kept
// as metadata for compatibility with existing session status consumers.
func (m *LoginManager) StartLogin(proxy string) (*models.LoginSessionStatus, error) {
	service := "flow,gemini"
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
	if err := os.MkdirAll(absProfileDir, 0700); err != nil {
		return nil, fmt.Errorf("không thể tạo thư mục profile: %w", err)
	}

	port, err := findFreePort()
	if err != nil {
		return nil, fmt.Errorf("không tìm thấy cổng mạng trống: %w", err)
	}

	// Direct Google Sign-In to Flow first. After authentication the same managed
	// profile is warmed against both Flow and Gemini before it is saved.
	targetURL := "https://accounts.google.com/ServiceLogin?continue=https%3A%2F%2Fflow.google.com"

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

	statusCopy := *status
	return &statusCopy, nil
}

// GetStatus returns the current status of a login session.
func (m *LoginManager) GetStatus(sessionID string) (*models.LoginSessionStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, errors.New("phiên đăng nhập không tồn tại hoặc đã hết hạn")
	}

	if !session.FinishedAt.IsZero() && time.Since(session.FinishedAt) > 10*time.Minute {
		delete(m.sessions, sessionID)
		return nil, errors.New("phiên đăng nhập không tồn tại hoặc đã hết hạn")
	}
	statusCopy := *session.Status
	if session.Status.Account != nil {
		accountCopy := *session.Status.Account
		statusCopy.Account = &accountCopy
	}
	return &statusCopy, nil
}

// safeRemoveDirWithRetry repeatedly attempts to delete a directory on Windows until file handles are released.
func safeRemoveDirWithRetry(dir string, maxAttempts int, delay time.Duration) error {
	if dir == "" {
		return nil
	}
	var err error
	for i := 0; i < maxAttempts; i++ {
		if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
			return nil
		}
		err = os.RemoveAll(dir)
		if err == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return err
}

// CancelLogin terminates the login process, closes Chrome, and cleans up temp files if incomplete.
func (m *LoginManager) CancelLogin(sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.Unlock()
		return errors.New("phiên đăng nhập không tồn tại")
	}
	step := sessionStep(session)
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	session.CancelFunc()
	if session.Cmd != nil && session.Cmd.Process != nil {
		KillProcessTree(session.Cmd.Process.Pid)
	}

	// If never completed, cleanly remove empty profile dir using backoff retry
	if step != models.LoginStepCompleted {
		go func(dir string) {
			_ = safeRemoveDirWithRetry(dir, 5, 400*time.Millisecond)
		}(session.ProfileDir)
	}

	return nil
}

// CloseAllSessions forcibly terminates all running login sessions and kills their Chrome processes.
func (m *LoginManager) CloseAllSessions() {
	m.mu.Lock()
	type sessionCleanup struct {
		session *ActiveSession
		step    models.LoginStep
	}
	sessions := make([]sessionCleanup, 0, len(m.sessions))
	for id, s := range m.sessions {
		sessions = append(sessions, sessionCleanup{session: s, step: sessionStep(s)})
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, item := range sessions {
		s := item.session
		if s.CancelFunc != nil {
			s.CancelFunc()
		}
		if s.Cmd != nil && s.Cmd.Process != nil {
			KillProcessTree(s.Cmd.Process.Pid)
		}
		if item.step != models.LoginStepCompleted {
			go func(dir string) {
				_ = safeRemoveDirWithRetry(dir, 4, 400*time.Millisecond)
			}(s.ProfileDir)
		}
	}
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

			isTargetReached := isTargetReached(currentURL, s.Service)

			// Login is ready if:
			// 1. Target service page is reached, OR
			// 2. Has PSID and left sign-in pages AND left intermediate account pages
			loginReady := hasPSID && isTargetReached && !isSignInPage && !isIntermediatePage

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
					m.updateSessionStatus(s.SessionID, models.LoginStepFailed, "Không thể xác định email của tài khoản Google từ trang đã đăng nhập", nil, "missing authenticated Google account identity")
					return
				}
				extractedEmail = strings.ToLower(strings.TrimSpace(extractedEmail))
				if extractedName == "" {
					extractedName = "Google Account"
				}

				// A Google session is shared across both products, but visiting both
				// origins once ensures any service-scoped cookies are materialized in
				// the same managed profile before it is saved.
				if hydratedSnlm0e, hydrateErr := hydrateGoogleServiceSession(ctx, client); hydrateErr == nil {
					if snlm0e == "" {
						snlm0e = hydratedSnlm0e
					}
					if refreshedCookies, cookieErr := client.GetCookies(ctx); cookieErr == nil {
						cookies = refreshedCookies
						cookieHeader, hasPSID, hasPSIDTS = FilterGoogleCookies(cookies)
					}
				} else {
					log.Printf("[LoginManager] Warning: could not warm both Google services: %v\n", hydrateErr)
				}

				// Build GoogleAccount model. The Google session is shared by Flow and
				// Gemini, so one successful login enables both service probes. Quota is
				// read live later and is never persisted in the account record.
				liveCredits, liveTier := ExtractAccountCreditsAndTier(ctx, client)
				if liveTier == "" {
					liveTier = "FREE"
				}
				_ = liveCredits // legacy compatibility; live quota is not stored.

				nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
				account := &models.GoogleAccount{
					ID:            s.AccountID,
					Email:         extractedEmail,
					Name:          extractedName,
					ProfileDir:    s.ProfileDir,
					Cookies:       cookieHeader,
					SNlM0eToken:   snlm0e,
					Services:      "flow,gemini",
					Status:        "ACTIVE",
					Tier:          liveTier,
					Credits:       0,
					Proxy:         s.Proxy,
					ImageEnabled:  true,
					VideoEnabled:  true,
					LastRefreshAt: nowStr,
					CreatedAt:     nowStr,
					UpdatedAt:     nowStr,
				}

				if m.database != nil {
					oldProfileDir := ""
					if existing, lookupErr := m.database.GetGoogleAccountByEmail(extractedEmail); lookupErr == nil && existing != nil {
						oldProfileDir = existing.ProfileDir
						account.ID = existing.ID
					}
					if err := m.database.UpsertGoogleAccount(account); err != nil {
						log.Printf("[LoginManager] Error saving account to DB: %v\n", err)
						m.updateSessionStatus(s.SessionID, models.LoginStepFailed, "Lỗi khi lưu tài khoản vào cơ sở dữ liệu", nil, err.Error())
						return
					}
					if oldProfileDir != "" && oldProfileDir != s.ProfileDir && m.isManagedProfile(oldProfileDir) {
						go func(dir string) { _ = safeRemoveDirWithRetry(dir, 5, 400*time.Millisecond) }(oldProfileDir)
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

func isTargetReached(rawURL, service string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	service = strings.ToLower(strings.TrimSpace(service))
	if strings.Contains(service, "flow") && (host == "flow.google.com" || host == "labs.google") {
		return true
	}
	if strings.Contains(service, "gemini") && host == "gemini.google.com" {
		return true
	}
	return false
}

func hydrateGoogleServiceSession(ctx context.Context, client *CDPClient) (string, error) {
	if client == nil {
		return "", errors.New("CDP client không hợp lệ")
	}
	geminiToken := ""
	// Visit Gemini first so the final page remains Flow, where the legacy plan
	// detector is most reliable, while still capturing Gemini's CSRF token.
	if err := navigateLivePage(ctx, client, "https://gemini.google.com/app", []string{"gemini.google.com"}); err != nil {
		return "", err
	}
	geminiToken, _ = client.EvaluateJS(ctx, `(function() {
		if (window.WIZ_global_data && window.WIZ_global_data.SNlM0e) return window.WIZ_global_data.SNlM0e;
		var match = document.documentElement.innerHTML.match(/"SNlM0e":"([^"]+)"/);
		return match && match[1] ? match[1] : "";
	})()`)
	if err := navigateLivePage(ctx, client, "https://flow.google.com/", []string{"flow.google.com", "labs.google"}); err != nil {
		return geminiToken, err
	}
	return geminiToken, nil
}

func (m *LoginManager) isManagedProfile(dir string) bool {
	root, err := filepath.Abs(m.baseDir)
	if err != nil {
		return false
	}
	candidate, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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
		if step == models.LoginStepCompleted || step == models.LoginStepFailed || step == models.LoginStepCancelled {
			session.FinishedAt = time.Now()
			// Do not retain proxy credentials in the completed-session cache.
			session.Proxy = ""
			session.ProxyUser = ""
			session.ProxyPass = ""
		}
	}
}

func sessionStep(session *ActiveSession) models.LoginStep {
	if session == nil || session.Status == nil {
		return models.LoginStepFailed
	}
	return session.Status.Step
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
	return m.openBrowser(profileDir, targetURL, proxy, "")
}

// OpenBrowserWithCookies opens a saved profile and seeds it with the supplied
// Google session header. This is used only for explicit manual cookie imports.
func (m *LoginManager) OpenBrowserWithCookies(profileDir, targetURL, proxy, cookies string) error {
	return m.openBrowser(profileDir, targetURL, proxy, cookies)
}

func (m *LoginManager) openBrowser(profileDir, targetURL, proxy, cookies string) error {
	chromePath, err := FindChromeExecutable()
	if err != nil {
		return err
	}
	absProfileDir, err := filepath.Abs(profileDir)
	if err != nil {
		absProfileDir = profileDir
	}
	if err := os.MkdirAll(absProfileDir, 0700); err != nil {
		return err
	}
	for _, lockName := range []string{"lockfile", "SingletonLock", "SingletonCookie", "SingletonSocket"} {
		if _, statErr := os.Stat(filepath.Join(absProfileDir, lockName)); statErr == nil {
			return fmt.Errorf("profile đang được Chrome sử dụng (%s)", lockName)
		}
	}

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

	var proxyUser, proxyPass string
	var debugPort int

	if proxy != "" {
		serverFlag, u, p := FormatChromeProxyFlag(proxy)
		if serverFlag != "" {
			args = append(args, fmt.Sprintf("--proxy-server=%s", serverFlag))
			proxyUser = u
			proxyPass = p
		}
	}

	if (proxyUser != "" && proxyPass != "") || strings.TrimSpace(cookies) != "" {
		p, err := findFreePort()
		if err != nil {
			return err
		}
		debugPort = p
		args = append(args, fmt.Sprintf("--remote-debugging-port=%d", debugPort))
	}

	args = append(args, targetURL)
	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		return err
	}

	// Configure proxy authentication and/or seed cookies through the loopback-only CDP port.
	if debugPort > 0 {
		go func(port int, user, pass, cookieHeader, navigateURL string) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()

			startWait := time.Now()
			for {
				if time.Since(startWait) > 15*time.Second {
					return
				}
				targets, err := QueryCDPTargets(port)
				if err == nil && len(targets) > 0 {
					for _, t := range targets {
						if t.Type == "page" && t.WebSocketDebuggerURL != "" {
							client, err := ConnectCDP(ctx, t.WebSocketDebuggerURL)
							if err == nil && client != nil {
								if user != "" && pass != "" {
									_ = client.EnableProxyAuth(ctx, user, pass)
								}
								if strings.TrimSpace(cookieHeader) != "" {
									if err := client.SetGoogleCookieHeader(ctx, cookieHeader); err == nil {
										_, _ = client.EvaluateJS(ctx, "location.reload()")
									}
								}
								_ = navigateURL
								// Keep the connection briefly for the initial proxy challenge/reload.
								time.Sleep(3 * time.Second)
								_ = client.Close()
								return
							}
						}
					}
				}
				time.Sleep(400 * time.Millisecond)
			}
		}(debugPort, proxyUser, proxyPass, cookies, targetURL)
	}

	return nil
}
