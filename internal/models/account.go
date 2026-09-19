package models

import (
	"net/url"
	"strings"
	"time"
)

// GoogleAccount represents an authenticated Google account stored in the database.
type GoogleAccount struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	AvatarURL     string `json:"avatarUrl"`
	ProfileDir    string `json:"-"`
	Cookies       string `json:"-"`
	SNlM0eToken   string `json:"-"`
	Services      string `json:"services"`
	Status        string `json:"status"` // ACTIVE, REFRESHING, EXPIRED, ERROR, DISABLED
	Tier          string `json:"tier"`   // PRO, FREE
	Credits       int    `json:"credits"`
	Proxy         string `json:"-"`
	ImageEnabled  bool   `json:"imageEnabled"`
	VideoEnabled  bool   `json:"videoEnabled"`
	UserAgent     string `json:"userAgent"`
	LastError     string `json:"lastError,omitempty"`
	LastRefreshAt string `json:"lastRefreshAt,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	Password      string `json:"-"`
	RecoveryEmail string `json:"-"`
}

// ToResponse converts GoogleAccount to a sanitized GoogleAccountResponse.
func (a *GoogleAccount) ToResponse() *GoogleAccountResponse {
	hasPsid := strings.Contains(a.Cookies, "__Secure-1PSID=")
	hasPsidts := strings.Contains(a.Cookies, "__Secure-1PSIDTS=")
	hasSnlm0e := len(strings.TrimSpace(a.SNlM0eToken)) > 0

	tier := a.Tier
	if tier == "" {
		tier = "FREE"
	}
	credits := a.Credits

	// No mock cookie preview string
	cookiePreview := ""

	return &GoogleAccountResponse{
		ID:            a.ID,
		Email:         a.Email,
		Name:          a.Name,
		AvatarURL:     a.AvatarURL,
		Status:        a.Status,
		Services:      a.Services,
		Tier:          tier,
		Credits:       credits,
		Proxy:         redactProxy(a.Proxy),
		ImageEnabled:  a.ImageEnabled,
		VideoEnabled:  a.VideoEnabled,
		HasPSID:       hasPsid,
		HasPSIDTS:     hasPsidts,
		HasSnlm0e:     hasSnlm0e,
		CookiePreview: cookiePreview,
		LastError:     a.LastError,
		LastRefreshAt: a.LastRefreshAt,
		CreatedAt:     a.CreatedAt,
	}
}

// redactProxy returns a display-safe proxy address without credentials.
// The full value remains backend-only and is never included in account lists.
func redactProxy(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	parts := strings.Split(raw, ":")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + ":" + parts[1]
	}
	return "configured"
}

// GoogleAccountResponse is the frontend-safe view of a Google account.
type GoogleAccountResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	AvatarURL     string `json:"avatarUrl"`
	Status        string `json:"status"`
	Services      string `json:"services"`
	Tier          string `json:"tier"`
	Credits       int    `json:"credits"`
	Proxy         string `json:"proxy"`
	ImageEnabled  bool   `json:"imageEnabled"`
	VideoEnabled  bool   `json:"videoEnabled"`
	HasPSID       bool   `json:"hasPsid"`
	HasPSIDTS     bool   `json:"hasPsidts"`
	HasSnlm0e     bool   `json:"hasSnlm0e"`
	CookiePreview string `json:"cookiePreview"`
	LastError     string `json:"lastError,omitempty"`
	LastRefreshAt string `json:"lastRefreshAt,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

// LiveFlowQuota is a non-persistent snapshot read directly from the
// authenticated Google Flow session. Google may expose only a combined
// balance for some plans; Has* flags keep that distinction explicit.
type LiveFlowQuota struct {
	Available         bool   `json:"available"`
	Tier              string `json:"tier"`
	TotalCredits      int    `json:"totalCredits"`
	HasTotalCredits   bool   `json:"hasTotalCredits"`
	DailyCredits      int    `json:"dailyCredits"`
	HasDailyCredits   bool   `json:"hasDailyCredits"`
	MonthlyCredits    int    `json:"monthlyCredits"`
	HasMonthlyCredits bool   `json:"hasMonthlyCredits"`
	DailyResetAt      string `json:"dailyResetAt,omitempty"`
	MonthlyResetAt    string `json:"monthlyResetAt,omitempty"`
	Message           string `json:"message,omitempty"`
}

// LiveGeminiQuota is a non-persistent snapshot read from Gemini's usage page.
// Gemini currently exposes percentage-based product limits rather than a
// single credit balance, so current and weekly windows are represented
// independently.
type LiveGeminiQuota struct {
	Available               bool   `json:"available"`
	Tier                    string `json:"tier"`
	CurrentUsedPercent      int    `json:"currentUsedPercent"`
	HasCurrentUsedPercent   bool   `json:"hasCurrentUsedPercent"`
	CurrentRemainingPercent int    `json:"currentRemainingPercent"`
	HasCurrentRemaining     bool   `json:"hasCurrentRemaining"`
	CurrentResetAt          string `json:"currentResetAt,omitempty"`
	WeeklyUsedPercent       int    `json:"weeklyUsedPercent"`
	HasWeeklyUsedPercent    bool   `json:"hasWeeklyUsedPercent"`
	WeeklyRemainingPercent  int    `json:"weeklyRemainingPercent"`
	HasWeeklyRemaining      bool   `json:"hasWeeklyRemaining"`
	WeeklyResetAt           string `json:"weeklyResetAt,omitempty"`
	Message                 string `json:"message,omitempty"`
}

// LiveAccountMetrics contains realtime service data only. It is intentionally
// not part of GoogleAccount and is never written to SQLite.
type LiveAccountMetrics struct {
	AccountID   string          `json:"accountId"`
	Status      string          `json:"status"` // LIVE, PARTIAL, ERROR
	Flow        LiveFlowQuota   `json:"flow"`
	Gemini      LiveGeminiQuota `json:"gemini"`
	RetrievedAt time.Time       `json:"retrievedAt"`
	Error       string          `json:"error,omitempty"`
}

// LoginStep represents the current stage of an interactive Chrome login.
type LoginStep string

const (
	LoginStepInitializing     LoginStep = "INITIALIZING"
	LoginStepWaitingUserLogin LoginStep = "WAITING_USER_LOGIN"
	LoginStepExtracting       LoginStep = "EXTRACTING_COOKIES"
	LoginStepCompleted        LoginStep = "COMPLETED"
	LoginStepFailed           LoginStep = "FAILED"
	LoginStepCancelled        LoginStep = "CANCELLED"
)

// LoginSessionStatus models live feedback for frontend polling or events.
type LoginSessionStatus struct {
	SessionID string                 `json:"sessionId"`
	Step      LoginStep              `json:"step"`
	Message   string                 `json:"message"`
	Account   *GoogleAccountResponse `json:"account,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// AccountTestResult represents the connectivity/health test outcome of an account.
type AccountTestResult struct {
	Success       bool      `json:"success"`
	Message       string    `json:"message"`
	LatencyMs     int64     `json:"latencyMs"`
	TestedService string    `json:"testedService"`
	Timestamp     time.Time `json:"timestamp"`
}

// ManualAccountInput represents input for manual cookie/token account addition.
type ManualAccountInput struct {
	Email       string `json:"email"`
	Cookies     string `json:"cookies"`
	SNlM0eToken string `json:"snlm0eToken"`
	Proxy       string `json:"proxy"`
	Tier        string `json:"tier"`
	Credits     int    `json:"credits"`
	Service     string `json:"service"`
}

// BulkAddInput represents input for adding multiple accounts at once.
type BulkAddInput struct {
	RawList      string `json:"rawList"`
	DefaultProxy string `json:"defaultProxy"`
	SkipExisting bool   `json:"skipExisting"`
	DefaultTier  string `json:"defaultTier"`
	Service      string `json:"service"`
}

// BulkAccountItem models an individual entry parsed from bulk list.
type BulkAccountItem struct {
	Email   string `json:"email"`
	Status  string `json:"status"` // PENDING, ADDED, SKIPPED, ERROR
	Message string `json:"message,omitempty"`
}

// BulkAddResult models the outcome of a bulk add operation.
type BulkAddResult struct {
	TotalParsed  int               `json:"totalParsed"`
	AddedCount   int               `json:"addedCount"`
	SkippedCount int               `json:"skippedCount"`
	FailedCount  int               `json:"failedCount"`
	Items        []BulkAccountItem `json:"items"`
}

// ProxyTestResult models the result of probing a proxy.
type ProxyTestResult struct {
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latencyMs"`
	EgressIP  string `json:"egressIP,omitempty"`
	Message   string `json:"message"`
}

// BackupExportResult models the result of exporting accounts and profiles to a zip.
type BackupExportResult struct {
	Success      bool   `json:"success"`
	FilePath     string `json:"filePath"`
	AccountCount int    `json:"accountCount"`
	ProfileCount int    `json:"profileCount"`
	SizeBytes    int64  `json:"sizeBytes"`
	Message      string `json:"message"`
}

// RestoreResult models the outcome of importing a backup zip or folder.
type RestoreResult struct {
	Success          bool     `json:"success"`
	AccountsRestored int      `json:"accountsRestored"`
	ProfilesRestored int      `json:"profilesRestored"`
	Errors           []string `json:"errors,omitempty"`
	Message          string   `json:"message"`
}

// CachePurgeResult models the outcome of clearing cache from account profile directories.
type CachePurgeResult struct {
	Success         bool   `json:"success"`
	FreedBytes      int64  `json:"freedBytes"`
	ProfilesCleaned int    `json:"profilesCleaned"`
	Message         string `json:"message"`
}
