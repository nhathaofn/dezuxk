package chrome

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"dezuxk/internal/db"
	"dezuxk/internal/models"
)

// KeepAliveWorker periodically refreshes Google account sessions in the background.
type KeepAliveWorker struct {
	database  *db.DB
	ticker    *time.Ticker
	stopChan  chan struct{}
	isRunning bool
	mu        sync.Mutex
}

// NewKeepAliveWorker creates a new worker instance.
func NewKeepAliveWorker(database *db.DB) *KeepAliveWorker {
	return &KeepAliveWorker{
		database: database,
		stopChan: make(chan struct{}),
	}
}

// Start begins the periodic refresh loop (checks every 20 minutes, refreshes accounts older than 3 hours).
func (w *KeepAliveWorker) Start() {
	w.mu.Lock()
	if w.isRunning {
		w.mu.Unlock()
		return
	}
	w.isRunning = true
	w.ticker = time.NewTicker(20 * time.Minute)
	w.mu.Unlock()

	log.Println("[KeepAliveWorker] Started background session refresh worker.")

	go func() {
		for {
			select {
			case <-w.stopChan:
				return
			case <-w.ticker.C:
				w.checkAndRefreshAll()
			}
		}
	}()
}

// Stop halts the worker gracefully.
func (w *KeepAliveWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.isRunning {
		return
	}
	w.isRunning = false
	if w.ticker != nil {
		w.ticker.Stop()
	}
	close(w.stopChan)
	log.Println("[KeepAliveWorker] Stopped.")
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
			if err := w.RefreshAccount(acc); err != nil {
				log.Printf("[KeepAliveWorker] Failed refreshing %s: %v\n", acc.Email, err)
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
	}

	if acc.Proxy != "" {
		serverFlag, _, _ := FormatChromeProxyFlag(acc.Proxy)
		if serverFlag != "" {
			args = append(args, fmt.Sprintf("--proxy-server=%s", serverFlag))
		}
	}

	args = append(args, targetURL)

	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start headless chrome: %w", err)
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// Wait for debug port
	var target *CDPTarget
	startWait := time.Now()
	for {
		if time.Since(startWait) > 12*time.Second {
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

	// Let Chrome interact silently for 5 seconds to rotate __Secure-1PSIDTS
	time.Sleep(5 * time.Second)

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

	// Fetch fresh SNlM0e token
	snlm0e, _ := client.EvaluateJS(ctx, `(function() {
		if (window.WIZ_global_data && window.WIZ_global_data.SNlM0e) {
			return window.WIZ_global_data.SNlM0e;
		}
		var match = document.documentElement.innerHTML.match(/"SNlM0e":"([^"]+)"/);
		if (match && match[1]) return match[1];
		return "";
	})()`)

	// Persist fresh tokens
	if err := w.database.UpdateGoogleAccountCookies(acc.ID, cookieHeader, snlm0e); err != nil {
		return fmt.Errorf("failed to persist refreshed cookies: %w", err)
	}

	_ = client.CloseBrowser(ctx)
	return nil
}
