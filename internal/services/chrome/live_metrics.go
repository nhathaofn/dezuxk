package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"dezuxk/internal/models"
)

// CollectLiveMetrics opens the account's managed Chrome profile, reads Flow
// and Gemini quota pages, and returns an in-memory snapshot. It deliberately
// does not update the database: cookies/profile state remain persistent, while
// quota is fetched on demand and expires with this response.
func (w *KeepAliveWorker) CollectLiveMetrics(acc *models.GoogleAccount) (*models.LiveAccountMetrics, error) {
	if acc == nil || strings.TrimSpace(acc.ProfileDir) == "" {
		return nil, errors.New("tài khoản chưa có profile Chrome được quản lý")
	}

	for _, lockName := range []string{"lockfile", "SingletonLock", "SingletonCookie", "SingletonSocket"} {
		if _, err := os.Stat(filepath.Join(acc.ProfileDir, lockName)); err == nil {
			return nil, fmt.Errorf("profile đang được Chrome sử dụng (%s)", lockName)
		}
	}

	chromePath, err := FindChromeExecutable()
	if err != nil {
		return nil, err
	}
	port, err := findFreePort()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	args := []string{
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--user-data-dir=%s", acc.ProfileDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-gpu",
		"--disable-blink-features=AutomationControlled",
		"--window-size=1440,1000",
		"https://flow.google.com/",
	}

	var proxyUser, proxyPass string
	if strings.TrimSpace(acc.Proxy) != "" {
		serverFlag, user, pass := FormatChromeProxyFlag(acc.Proxy)
		if serverFlag != "" {
			args = append(args[:len(args)-1], fmt.Sprintf("--proxy-server=%s", serverFlag), args[len(args)-1])
			proxyUser, proxyPass = user, pass
		}
	}

	cmd := exec.Command(chromePath, args...)
	if err := w.startManagedProcess(cmd); err != nil {
		return nil, fmt.Errorf("không thể khởi chạy Chrome headless để đọc quota: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			KillProcessTree(cmd.Process.Pid)
			w.unregisterManagedProcess(cmd.Process.Pid)
		}
	}()

	target, err := waitForLiveCDPTarget(ctx, port)
	if err != nil {
		return nil, err
	}
	client, err := ConnectCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối Chrome CDP để đọc quota: %w", err)
	}
	defer client.Close()

	if proxyUser != "" || proxyPass != "" {
		_ = client.EnableProxyAuth(ctx, proxyUser, proxyPass)
	}

	if err := ensureGoogleSessionCookies(ctx, client, acc.Cookies); err != nil {
		return nil, fmt.Errorf("không thể xác thực profile Google: %w", err)
	}

	metrics := &models.LiveAccountMetrics{
		AccountID:   acc.ID,
		Status:      "ERROR",
		RetrievedAt: time.Now().UTC(),
	}
	var serviceErrors []string

	if err := navigateLivePage(ctx, client, "https://flow.google.com/", []string{"flow.google.com", "labs.google"}); err != nil {
		serviceErrors = append(serviceErrors, "Flow: "+err.Error())
	} else if flow, extractErr := ExtractFlowQuota(ctx, client); extractErr != nil {
		serviceErrors = append(serviceErrors, "Flow: "+extractErr.Error())
	} else {
		metrics.Flow = flow
	}

	if err := navigateLivePage(ctx, client, "https://gemini.google.com/usage", []string{"gemini.google.com"}); err != nil {
		serviceErrors = append(serviceErrors, "Gemini: "+err.Error())
	} else if gemini, extractErr := ExtractGeminiQuota(ctx, client); extractErr != nil {
		serviceErrors = append(serviceErrors, "Gemini: "+extractErr.Error())
	} else {
		metrics.Gemini = gemini
	}

	available := 0
	if metrics.Flow.Available {
		available++
	}
	if metrics.Gemini.Available {
		available++
	}
	switch {
	case available == 2:
		metrics.Status = "LIVE"
	case available == 1:
		metrics.Status = "PARTIAL"
	default:
		metrics.Status = "ERROR"
	}
	if len(serviceErrors) > 0 {
		metrics.Error = strings.Join(serviceErrors, "; ")
	}

	_ = client.CloseBrowser(ctx)
	return metrics, nil
}

func waitForLiveCDPTarget(ctx context.Context, port int) (*CDPTarget, error) {
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		targets, err := QueryCDPTargets(port)
		if err == nil {
			for i := range targets {
				if targets[i].Type == "page" && targets[i].WebSocketDebuggerURL != "" {
					return &targets[i], nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return nil, errors.New("timeout chờ Chrome CDP để đọc quota")
}

func navigateLivePage(ctx context.Context, client *CDPClient, pageURL string, allowedHosts []string) error {
	if _, err := client.Call(ctx, "Page.navigate", map[string]interface{}{"url": pageURL}); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, err := client.EvaluateJS(ctx, `location.hostname + "|" + document.readyState`)
		if err == nil {
			parts := strings.SplitN(state, "|", 2)
			if len(parts) == 2 && parts[1] == "complete" && liveHostAllowed(parts[0], allowedHosts) {
				time.Sleep(2500 * time.Millisecond)
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return fmt.Errorf("trang %s chưa tải xong", pageURL)
}

func liveHostAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if host == candidate || strings.HasSuffix(host, "."+candidate) {
			return true
		}
	}
	return false
}

type flowQuotaProbeResult struct {
	Tier           string `json:"tier"`
	TotalCredits   *int   `json:"totalCredits"`
	DailyCredits   *int   `json:"dailyCredits"`
	MonthlyCredits *int   `json:"monthlyCredits"`
	DailyResetAt   string `json:"dailyResetAt"`
	MonthlyResetAt string `json:"monthlyResetAt"`
	Message        string `json:"message"`
}

// ExtractFlowQuota reads the rendered Flow account drawer. The page's public
// UI is the source of truth; it may return a combined balance only.
func ExtractFlowQuota(ctx context.Context, client *CDPClient) (models.LiveFlowQuota, error) {
	if client == nil {
		return models.LiveFlowQuota{}, errors.New("CDP client không hợp lệ")
	}
	evalCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	value, err := client.EvaluateJS(evalCtx, flowQuotaProbeJS)
	if err != nil {
		return models.LiveFlowQuota{}, err
	}
	var raw flowQuotaProbeResult
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return models.LiveFlowQuota{}, fmt.Errorf("phản hồi Flow không hợp lệ: %w", err)
	}

	return flowQuotaFromProbe(raw)
}

func flowQuotaFromProbe(raw flowQuotaProbeResult) (models.LiveFlowQuota, error) {
	quota := models.LiveFlowQuota{Tier: strings.ToUpper(strings.TrimSpace(raw.Tier)), Message: strings.TrimSpace(raw.Message)}
	if raw.TotalCredits != nil && *raw.TotalCredits >= 0 {
		quota.TotalCredits = *raw.TotalCredits
		quota.HasTotalCredits = true
	}
	if raw.DailyCredits != nil && *raw.DailyCredits >= 0 {
		quota.DailyCredits = *raw.DailyCredits
		quota.HasDailyCredits = true
	}
	if raw.MonthlyCredits != nil && *raw.MonthlyCredits >= 0 {
		quota.MonthlyCredits = *raw.MonthlyCredits
		quota.HasMonthlyCredits = true
	}
	quota.DailyResetAt = strings.TrimSpace(raw.DailyResetAt)
	quota.MonthlyResetAt = strings.TrimSpace(raw.MonthlyResetAt)
	quota.Available = quota.HasTotalCredits || quota.HasDailyCredits || quota.HasMonthlyCredits
	if !quota.Available {
		return quota, errors.New("Flow không trả về số credit trong phiên hiện tại")
	}
	if quota.Message == "" && (!quota.HasDailyCredits || !quota.HasMonthlyCredits) {
		quota.Message = "Google Flow hiện chỉ cung cấp số credit tổng hợp trong phiên này"
	}
	return quota, nil
}

type geminiQuotaProbeResult struct {
	Tier               string `json:"tier"`
	CurrentUsedPercent *int   `json:"currentUsedPercent"`
	WeeklyUsedPercent  *int   `json:"weeklyUsedPercent"`
	CurrentResetAt     string `json:"currentResetAt"`
	WeeklyResetAt      string `json:"weeklyResetAt"`
	Message            string `json:"message"`
}

// ExtractGeminiQuota reads Gemini's first-party /usage page. Gemini exposes
// percentage windows and reset times, not a single credit integer.
func ExtractGeminiQuota(ctx context.Context, client *CDPClient) (models.LiveGeminiQuota, error) {
	if client == nil {
		return models.LiveGeminiQuota{}, errors.New("CDP client không hợp lệ")
	}
	evalCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	value, err := client.EvaluateJS(evalCtx, geminiQuotaProbeJS)
	if err != nil {
		return models.LiveGeminiQuota{}, err
	}
	var raw geminiQuotaProbeResult
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return models.LiveGeminiQuota{}, fmt.Errorf("phản hồi Gemini không hợp lệ: %w", err)
	}

	return geminiQuotaFromProbe(raw)
}

func geminiQuotaFromProbe(raw geminiQuotaProbeResult) (models.LiveGeminiQuota, error) {
	quota := models.LiveGeminiQuota{
		Tier:           strings.ToUpper(strings.TrimSpace(raw.Tier)),
		CurrentResetAt: strings.TrimSpace(raw.CurrentResetAt),
		WeeklyResetAt:  strings.TrimSpace(raw.WeeklyResetAt),
		Message:        strings.TrimSpace(raw.Message),
	}
	if raw.CurrentUsedPercent != nil && *raw.CurrentUsedPercent >= 0 && *raw.CurrentUsedPercent <= 100 {
		quota.CurrentUsedPercent = *raw.CurrentUsedPercent
		quota.HasCurrentUsedPercent = true
		quota.CurrentRemainingPercent = 100 - *raw.CurrentUsedPercent
		quota.HasCurrentRemaining = true
	}
	if raw.WeeklyUsedPercent != nil && *raw.WeeklyUsedPercent >= 0 && *raw.WeeklyUsedPercent <= 100 {
		quota.WeeklyUsedPercent = *raw.WeeklyUsedPercent
		quota.HasWeeklyUsedPercent = true
		quota.WeeklyRemainingPercent = 100 - *raw.WeeklyUsedPercent
		quota.HasWeeklyRemaining = true
	}
	quota.Available = quota.HasCurrentUsedPercent || quota.HasWeeklyUsedPercent || quota.CurrentResetAt != "" || quota.WeeklyResetAt != ""
	if !quota.Available {
		return quota, errors.New("Gemini không trả về hạn mức trong phiên hiện tại")
	}
	return quota, nil
}

const flowQuotaProbeJS = `(async function() {
  function clean(value) {
    return String(value || "").replace(/\s+/g, " ").trim().slice(0, 500);
  }
  function parseNumber(raw) {
    var value = String(raw || "").replace(/\s/g, "").replace(/[^0-9.,-]/g, "");
    if (!value) return -1;
    if (value.indexOf(".") >= 0 && value.indexOf(",") >= 0) {
      value = value.replace(/\./g, "").replace(",", ".");
    } else {
      value = value.replace(/[.,]/g, "");
    }
    var parsed = Number(value);
    return Number.isFinite(parsed) && parsed >= 0 ? Math.round(parsed) : -1;
  }
  function valueFromText(text) {
    var match = String(text || "").match(/([0-9][0-9.,\s]*)\s*(?:google\s*flow\s*)?(?:credits?|tín\s*dụng)/i);
    return match ? parseNumber(match[1]) : -1;
  }
  function labelledValue(label) {
    var nodes = Array.from(document.querySelectorAll(".credits-row, .credits-info-wrapper, [class*='daily'], [class*='monthly'], [class*='refresh']"));
    for (var i = 0; i < nodes.length; i++) {
      var text = clean(nodes[i].innerText || nodes[i].textContent || "");
      if (label.test(text) && /credit|tín\s*dụng/i.test(text)) {
        var value = valueFromText(text);
        if (value >= 0) return value;
      }
    }
    return -1;
  }
  function labelledReset(label) {
    var nodes = Array.from(document.querySelectorAll("[class*='daily'], [class*='monthly'], [class*='refresh'], .credits-row, .credits-info-wrapper"));
    for (var i = 0; i < nodes.length; i++) {
      var text = clean(nodes[i].innerText || nodes[i].textContent || "");
      if (label.test(text) && /reset|refresh|làm mới|đặt lại/i.test(text)) return text;
    }
    return "";
  }
  var avatar = document.querySelector("[aria-label*='Google Account'], [aria-label*='Tài khoản Google'], button.gb_d, img.gb_k");
  if (!avatar) {
    avatar = Array.from(document.querySelectorAll("button, [role=button]")).find(function(el) { return el.querySelector("img[src*='googleusercontent']"); });
  }
  if (avatar) {
    try { avatar.click(); await new Promise(function(resolve) { setTimeout(resolve, 900); }); } catch (e) {}
  }
  var total = -1;
  var countNodes = Array.from(document.querySelectorAll(".credits-count"));
  for (var i = 0; i < countNodes.length && total < 0; i++) total = valueFromText(countNodes[i].innerText || countNodes[i].textContent || "");
  if (total < 0) total = valueFromText(document.body && document.body.innerText || "");
  var body = clean(document.body && document.body.innerText || "");
  var tierMatch = body.match(/\b(ULTRA|PRO|PLUS|FREE)\b/i);
  var daily = labelledValue(/daily|hằng\s*ngày|hàng\s*ngày|mỗi\s*ngày|ngày/i);
  var monthly = labelledValue(/monthly|hằng\s*tháng|hàng\s*tháng|mỗi\s*tháng|tháng/i);
  return JSON.stringify({
    tier: tierMatch ? tierMatch[1].toUpperCase() : "",
    totalCredits: total,
    dailyCredits: daily,
    monthlyCredits: monthly,
    dailyResetAt: labelledReset(/daily|hằng\s*ngày|hàng\s*ngày|mỗi\s*ngày|ngày/i),
    monthlyResetAt: labelledReset(/monthly|hằng\s*tháng|hàng\s*tháng|mỗi\s*tháng|tháng/i),
    message: daily < 0 || monthly < 0 ? "Google Flow hiện chỉ cung cấp số credit tổng hợp trong phiên này" : ""
  });
})()`

const geminiQuotaProbeJS = `(function() {
  function clean(value) { return String(value || "").replace(/\s+/g, " ").trim().slice(0, 500); }
  function percentFrom(root) {
    if (!root) return -1;
    var match = clean(root.innerText || root.textContent || "").match(/(?:^|\s)(\d{1,3})\s*%/);
    if (!match) return -1;
    var value = Number(match[1]);
    return Number.isFinite(value) && value >= 0 && value <= 100 ? value : -1;
  }
  function resetFrom(root) {
    if (!root) return "";
    var lines = String(root.innerText || root.textContent || "").split(/\n+/).map(clean).filter(Boolean);
    for (var i = lines.length - 1; i >= 0; i--) {
      if (!/\d{1,3}\s*%/.test(lines[i])) return lines[i];
    }
    return "";
  }
  function leafPercentages() {
    var values = [];
    var nodes = Array.from(document.querySelectorAll("p, span, div"));
    for (var i = 0; i < nodes.length; i++) {
      if (nodes[i].children.length > 0) continue;
      var value = percentFrom(nodes[i]);
      if (value >= 0) values.push(value);
    }
    return values;
  }

  var body = String(document.body && document.body.innerText || "");
  var currentRoot = document.querySelector("[class*='gxu-currently']");
  var weeklyRoot = document.querySelector("[class*='gxu-weekly']");
  var current = percentFrom(currentRoot);
  var weekly = percentFrom(weeklyRoot);
  var fallback = leafPercentages();
  if (current < 0 && fallback.length > 0) current = fallback[0];
  if (weekly < 0 && fallback.length > 1) weekly = fallback[1];

  var tierMatch = body.match(/\b(ULTRA|PRO|PLUS|FREE)\b/i);
  return JSON.stringify({
    tier: tierMatch ? tierMatch[1].toUpperCase() : "",
    currentUsedPercent: current,
    weeklyUsedPercent: weekly,
    currentResetAt: resetFrom(currentRoot),
    weeklyResetAt: resetFrom(weeklyRoot),
    message: current < 0 && weekly < 0 ? "Gemini không cung cấp hạn mức trong trang hiện tại" : ""
  });
})()`
