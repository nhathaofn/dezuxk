// Package providers defines domain models and repository contracts for dynamic reverse providers.
package providers

import (
	"time"
)

// AccountStatus represents the operational readiness of an account.
type AccountStatus string

const (
	AccountStatusUnverified  AccountStatus = "unverified"
	AccountStatusReady       AccountStatus = "ready"
	AccountStatusSignedOut   AccountStatus = "signed_out"
	AccountStatusRateLimited AccountStatus = "rate_limited"
	AccountStatusError       AccountStatus = "error"
)

// Cookie represents a single browser cookie.
type Cookie struct {
	Name     string     `json:"name"`
	Value    string     `json:"value"`
	Domain   string     `json:"domain"`
	Path     string     `json:"path"`
	Expires  float64    `json:"expires,omitempty"`
	HTTPOnly bool       `json:"httpOnly,omitempty"`
	Secure   bool       `json:"secure,omitempty"`
	SameSite string     `json:"sameSite,omitempty"`
	Zone     CookieZone `json:"zone,omitempty"`
}

// AccountSecrets stores sensitive session material (cookies, tokens).
type AccountSecrets struct {
	AccountID  string            `json:"account_id"`
	Cookies    []Cookie          `json:"cookies"`
	Tokens     map[string]string `json:"tokens"`
	CookieHash string            `json:"cookie_hash"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// QuotaUsage tracks credit/quota information.
type QuotaUsage struct {
	Feature   string    `json:"feature"`
	Remaining int64     `json:"remaining"`
	Total     int64     `json:"total"`
	ResetAt   time.Time `json:"reset_at,omitempty"`
}

// Account represents a user session for a provider.
type Account struct {
	ID            string         `json:"id"`
	ProviderID    string         `json:"provider_id"`
	Label         string         `json:"label"`
	Identity      string         `json:"identity"`
	Status        AccountStatus  `json:"status"`
	ProfileDir    string         `json:"profile_dir"`
	ProxyID       string         `json:"proxy_id,omitempty"`
	PausedUntil   *time.Time     `json:"paused_until,omitempty"`
	Usage         []QuotaUsage   `json:"usage,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	LastCheckedAt *time.Time     `json:"last_checked_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
