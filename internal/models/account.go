package models

import (
	"strings"
	"time"
)

// GoogleAccount represents an authenticated Google account stored in the database.
type GoogleAccount struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	AvatarURL     string `json:"avatarUrl"`
	ProfileDir    string `json:"profileDir"`
	Cookies       string `json:"cookies"`
	SNlM0eToken   string `json:"snlm0eToken"`
	Services      string `json:"services"`
	Status        string `json:"status"` // ACTIVE, REFRESHING, EXPIRED, ERROR, DISABLED
	Tier          string `json:"tier"`   // PRO, FREE
	Credits       int    `json:"credits"`
	Proxy         string `json:"proxy"`
	ImageEnabled  bool   `json:"imageEnabled"`
	VideoEnabled  bool   `json:"videoEnabled"`
	UserAgent     string `json:"userAgent"`
	LastError     string `json:"lastError,omitempty"`
	LastRefreshAt string `json:"lastRefreshAt,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// ToResponse converts GoogleAccount to a sanitized GoogleAccountResponse.
func (a *GoogleAccount) ToResponse() *GoogleAccountResponse {
	hasPsid := strings.Contains(a.Cookies, "__Secure-1PSID=")
	hasPsidts := strings.Contains(a.Cookies, "__Secure-1PSIDTS=")
	hasSnlm0e := len(strings.TrimSpace(a.SNlM0eToken)) > 0

	tier := a.Tier
	if tier == "" {
		tier = "PRO"
	}
	credits := a.Credits
	if credits == 0 && (a.Status == "ACTIVE" || a.Status == "") {
		credits = 1050
	}

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
		Proxy:         a.Proxy,
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
