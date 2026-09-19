package chrome

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dezuxk/internal/db"
	"dezuxk/internal/models"
)

// KeepAliveWorker periodically refreshes Google account sessions in the background.
type KeepAliveWorker struct {
	database    *db.DB
	ticker      *time.Ticker
	stopChan    chan struct{}
	isRunning   bool
	refreshFunc func(*models.GoogleAccount) error
	mu          sync.Mutex
	processMu   sync.Mutex
	processes   map[int]struct{}
}

// NewKeepAliveWorker creates a new worker instance.
func NewKeepAliveWorker(database *db.DB) *KeepAliveWorker {
	return &KeepAliveWorker{
		database:  database,
		stopChan:  make(chan struct{}),
		processes: make(map[int]struct{}),
	}
}

// SetRefreshFunc lets the owning service provide the shared per-account lock.
// Background refreshes must not race with an interactive browser or manual refresh.
func (w *KeepAliveWorker) SetRefreshFunc(fn func(*models.GoogleAccount) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.refreshFunc = fn
}

// Start begins the periodic refresh loop (checks every 20 minutes, refreshes accounts older than 3 hours).
func (w *KeepAliveWorker) Start() {
	w.mu.Lock()
	if w.isRunning {
		w.mu.Unlock()
		return
	}
	w.isRunning = true
	stopChan := make(chan struct{})
	w.stopChan = stopChan
	w.ticker = time.NewTicker(20 * time.Minute)
	ticker := w.ticker
	w.mu.Unlock()

	log.Println("[KeepAliveWorker] Started background session refresh worker.")

	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				w.checkAndRefreshAll()
			}
		}
	}()
}

// Stop halts the worker gracefully.
func (w *KeepAliveWorker) Stop() {
	w.mu.Lock()
	if w.isRunning {
		w.isRunning = false
		if w.ticker != nil {
			w.ticker.Stop()
		}
		close(w.stopChan)
	}
	w.mu.Unlock()

	w.stopManagedProcesses()
	log.Println("[KeepAliveWorker] Stopped.")
}

func (w *KeepAliveWorker) startManagedProcess(cmd *exec.Cmd) error {
	if err := startBackgroundCommand(cmd); err != nil {
		return err
	}
	if cmd.Process == nil {
		return fmt.Errorf("Chrome process was not created")
	}
	w.processMu.Lock()
	w.processes[cmd.Process.Pid] = struct{}{}
	w.processMu.Unlock()
	return nil
}

func (w *KeepAliveWorker) unregisterManagedProcess(pid int) {
	if pid <= 0 {
		return
	}
	w.processMu.Lock()
	delete(w.processes, pid)
	w.processMu.Unlock()
}

func (w *KeepAliveWorker) stopManagedProcesses() {
	w.processMu.Lock()
	pids := make([]int, 0, len(w.processes))
	for pid := range w.processes {
		pids = append(pids, pid)
	}
	w.processes = make(map[int]struct{})
	w.processMu.Unlock()

	for _, pid := range pids {
		KillProcessTree(pid)
	}
}

func (w *KeepAliveWorker) checkAndRefreshAll() {
	if w.database == nil {
		return
	}

	accounts, err := w.database.ListGoogleAccounts()
	if err != nil {
		log.Printf("[KeepAliveWorker] Error listing accounts: %v\n", err)
		return
	}

	now := time.Now().UTC()
	for _, acc := range accounts {
		if acc.Status != "ACTIVE" {
			continue
		}

		// Check age since last refresh
		needsRefresh := true
		if acc.LastRefreshAt != "" {
			t, err := time.Parse("2006-01-02 15:04:05", acc.LastRefreshAt)
			if err == nil && now.Sub(t) < 3*time.Hour {
				needsRefresh = false
			}
		}

		if needsRefresh {
			log.Printf("[KeepAliveWorker] Refreshing session for %s (%s)...\n", acc.Name, acc.Email)
			w.mu.Lock()
			refreshFunc := w.refreshFunc
			w.mu.Unlock()
			var refreshErr error
			if refreshFunc != nil {
				refreshErr = refreshFunc(acc)
			} else {
				refreshErr = w.RefreshAccount(acc)
			}
			if refreshErr != nil {
				log.Printf("[KeepAliveWorker] Failed refreshing %s: %v\n", acc.Email, refreshErr)
			} else {
				log.Printf("[KeepAliveWorker] Successfully refreshed %s.\n", acc.Email)
			}
		}
	}
}

// RefreshAccount executes a silent headless session refresh on an existing account profile.
func (w *KeepAliveWorker) RefreshAccount(acc *models.GoogleAccount) error {
	if acc == nil || acc.ProfileDir == "" {
		return fmt.Errorf("invalid account profile")
	}

	chromePath, err := FindChromeExecutable()
	if err != nil {
		return err
	}

	port, err := findFreePort()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	targetURL := "https://flow.google.com"
	if strings.Contains(strings.ToLower(acc.Services), "gemini") && !strings.Contains(strings.ToLower(acc.Services), "flow") {
		targetURL = "https://gemini.google.com/app"
	}

	// Launch in headless mode with existing profile
	args := []string{
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--user-data-dir=%s", acc.ProfileDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-gpu",
		"--disable-blink-features=AutomationControlled",
		"--window-size=1280,800",
	}

	var proxyUser, proxyPass string
	if acc.Proxy != "" {
		serverFlag, u, p := FormatChromeProxyFlag(acc.Proxy)
		if serverFlag != "" {
			args = append(args, fmt.Sprintf("--proxy-server=%s", serverFlag))
			proxyUser = u
			proxyPass = p
		}
	}

	// Never delete Chromium lock files: an interactive Chrome process may own the
	// profile. Let the caller retry after the profile is released instead.
	for _, lockName := range []string{"lockfile", "SingletonLock", "SingletonCookie", "SingletonSocket"} {
		if _, statErr := os.Stat(filepath.Join(acc.ProfileDir, lockName)); statErr == nil {
			return fmt.Errorf("profile đang được Chrome sử dụng (%s)", lockName)
		}
	}

	args = append(args, targetURL)

	cmd := exec.Command(chromePath, args...)
	if err := w.startManagedProcess(cmd); err != nil {
		return fmt.Errorf("failed to start headless chrome: %w", err)
	}

	defer func() {
		if cmd.Process != nil {
			KillProcessTree(cmd.Process.Pid)
			w.unregisterManagedProcess(cmd.Process.Pid)
		}
	}()

	// Wait for debug port
	var target *CDPTarget
	startWait := time.Now()
	for {
		if time.Since(startWait) > 15*time.Second {
			return fmt.Errorf("timeout waiting for headless chrome debug port")
		}

		targets, err := QueryCDPTargets(port)
		if err == nil && len(targets) > 0 {
			for _, t := range targets {
				if t.Type == "page" && t.WebSocketDebuggerURL != "" {
					target = &t
					break
				}
			}
			if target != nil {
				break
			}
		}
		time.Sleep(400 * time.Millisecond)
	}

	client, err := ConnectCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return fmt.Errorf("failed to connect CDP to headless instance: %w", err)
	}
	defer client.Close()

	// Enable proxy authentication via CDP if credentials exist
	if proxyUser != "" && proxyPass != "" {
		_ = client.EnableProxyAuth(ctx, proxyUser, proxyPass)
	}
	if err := ensureGoogleSessionCookies(ctx, client, acc.Cookies); err != nil {
		return fmt.Errorf("failed to seed Google session: %w", err)
	}

	// Let Chrome interact silently for 5 seconds to rotate __Secure-1PSIDTS
	time.Sleep(5 * time.Second)
	// Warm both Google product origins so one managed profile keeps a complete
	// shared session even when the account was originally added from one side.
	hydratedSnlm0e, hydrateErr := hydrateGoogleServiceSession(ctx, client)
	if hydrateErr != nil {
		log.Printf("[KeepAliveWorker] Warning: could not warm both Google services for %s: %v\n", acc.Email, hydrateErr)
	}

	// Fetch updated cookies
	cookies, err := client.GetCookies(ctx)
	if err != nil {
		return fmt.Errorf("failed to read cookies: %w", err)
	}

	cookieHeader, hasPSID, _ := FilterGoogleCookies(cookies)
	if !hasPSID {
		_ = w.database.UpdateGoogleAccountStatus(acc.ID, "EXPIRED", "Session expired: __Secure-1PSID cookie not present")
		return fmt.Errorf("session expired on Google")
	}

	// Fetch fresh SNlM0e token if this is a Gemini target
	snlm0e := hydratedSnlm0e
	if strings.Contains(targetURL, "gemini.google.com") {
		if refreshedToken, tokenErr := client.EvaluateJS(ctx, `(function() {
			if (window.WIZ_global_data && window.WIZ_global_data.SNlM0e) {
				return window.WIZ_global_data.SNlM0e;
			}
			var match = document.documentElement.innerHTML.match(/"SNlM0e":"([^"]+)"/);
			if (match && match[1]) return match[1];
			return "";
		})()`); tokenErr == nil && refreshedToken != "" {
			snlm0e = refreshedToken
		}
	}

	// Persist session material only. Live Flow/Gemini quota is fetched on demand
	// and is intentionally not written to SQLite.
	_, dynTier := ExtractAccountCreditsAndTier(ctx, client)
	if dynTier != "" {
		log.Printf("[KeepAliveWorker] Live plan detected for %s: %s\n", acc.Email, dynTier)
	}
	if err := w.database.UpdateGoogleAccountSessionData(acc.ID, cookieHeader, snlm0e, -1, dynTier); err != nil {
		return fmt.Errorf("failed to persist refreshed cookies: %w", err)
	}

	_ = client.CloseBrowser(ctx)
	return nil
}

// ExtractAccountCreditsAndTier executes JavaScript in the current page session to probe for live credits and plan tier.
func ExtractAccountCreditsAndTier(ctx context.Context, client *CDPClient) (int, string) {
	if client == nil {
		return -1, ""
	}

	jsExpr := `(async function() {
		var result = { credits: -1, tier: "" };

		function scanTextForCredits(root) {
			try {
				var walker = document.createTreeWalker(root || document.body, NodeFilter.SHOW_TEXT, null, false);
				var node;
				while (node = walker.nextNode()) {
					var txt = (node.nodeValue || "").trim();
					if (!txt || (node.parentElement && node.parentElement.matches("style, script, noscript"))) continue;
				var m = txt.match(/([\d.,]+)\s*(?:Google\s*Flow\s*)?(?:credits?|t\xEDn\s*d\u1EE5ng)/i) ||
				        txt.match(/(?:credits?|t\xEDn\s*d\u1EE5ng)\s*:\s*([\d.,]+)/i);
					if (m && m[1]) {
				var val = parseInt(m[1].replace(/[.,]/g, ""), 10);
						if (!isNaN(val) && val >= 0 && val < 10000000) {
							return val;
						}
					}
				}
			} catch (e) {}
			return -1;
		}

		// 1. Initial scan in current DOM
		var initCredits = scanTextForCredits(document.body);
		if (initCredits >= 0) {
			result.credits = initCredits;
		}

		// Check for PRO badge in header or nav
		try {
			var headerTexts = Array.from(document.querySelectorAll("header, nav, [role=banner], [class*='header']"))
				.map(el => el.innerText || "").join(" ");
			if (/\bPRO\b/.test(headerTexts)) {
				result.tier = "PRO";
			}
		} catch (e) {}

		// 2. Open Google Flow account avatar drawer to reveal live credits panel
		try {
			var avatar = document.querySelector("[aria-label*='Google Account'], [aria-label*='T\xE0i kho\u1EA3n Google'], [aria-label*='Google membership'], button.gb_d, img.gb_k");
			if (!avatar) {
				var allBtns = Array.from(document.querySelectorAll("button, [role=button]"));
				avatar = allBtns.find(b => b.querySelector("img[src*='googleusercontent']"));
			}

			if (avatar) {
				avatar.click();
				await new Promise(r => setTimeout(r, 1200));

				var drawerCredits = scanTextForCredits(document.body);
				if (drawerCredits >= 0) {
					result.credits = drawerCredits;
				}

				var bodyText = document.body.innerText || "";
				if (bodyText.includes("Manage subscription") || /\bPRO\b/.test(bodyText)) {
					result.tier = "PRO";
				}
			}
		} catch (e) {}

		// 3. Fallback for labs.google session endpoint
		if (result.credits < 0) {
			try {
				var sessionRes = await fetch("https://labs.google/fx/api/auth/session", { credentials: "include" });
				if (sessionRes.ok) {
					var sessionData = await sessionRes.json();
					if (sessionData && sessionData.access_token) {
						var credRes = await fetch("https://aisandbox-pa.googleapis.com/v1/credits", {
							headers: { "Authorization": "Bearer " + sessionData.access_token }
						});
						if (credRes.ok) {
							var credData = await credRes.json();
							if (typeof credData.credits === "number") {
								result.credits = credData.credits;
							} else if (typeof credData.subscriptionCredits === "number") {
								result.credits = credData.subscriptionCredits;
							}
							if (credData.userPaygateTier) {
								result.tier = String(credData.userPaygateTier).toUpperCase();
							} else if (credData.serviceTier) {
								result.tier = String(credData.serviceTier).toUpperCase();
							}
						}
					}
				}
			} catch (e) {}
		}

		// 4. Fallback for __NEXT_DATA__
		if (result.credits < 0) {
			try {
				if (window.__NEXT_DATA__ && window.__NEXT_DATA__.props) {
					var propsStr = JSON.stringify(window.__NEXT_DATA__.props);
					var cm = propsStr.match(/"credits":\s*(\d+)/i) || propsStr.match(/"subscriptionCredits":\s*(\d+)/i);
					if (cm && cm[1]) {
						result.credits = parseInt(cm[1], 10);
					}
					var tm = propsStr.match(/"(?:userPaygateTier|serviceTier|tier)":\s*"([^"]+)"/i);
					if (tm && tm[1]) {
						result.tier = tm[1].toUpperCase();
					}
				}
			} catch (e) {}
		}

		return JSON.stringify(result);
	})()`

	evalCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	valStr, err := client.EvaluateJS(evalCtx, jsExpr)
	if err != nil || valStr == "" {
		return -1, ""
	}

	var parsed struct {
		Credits int    `json:"credits"`
		Tier    string `json:"tier"`
	}
	if err := json.Unmarshal([]byte(valStr), &parsed); err != nil {
		return -1, ""
	}

	return parsed.Credits, parsed.Tier
}
