package pool

import "time"

// PoolStats provides a thread-safe point-in-time snapshot of account states for a provider.
type PoolStats struct {
	ProviderID  string    `json:"provider_id"`
	Total       int       `json:"total"`        // Total accounts tracked for this provider
	Ready       int       `json:"ready"`        // Accounts idle and immediately available for leasing
	Leased      int       `json:"leased"`       // Accounts currently in-flight
	RateLimited int       `json:"rate_limited"` // Accounts in cooldown backoff
	SignedOut   int       `json:"signed_out"`   // Accounts signed out / evicted
	Error       int       `json:"error"`        // Accounts in error state
	Timestamp   time.Time `json:"timestamp"`    // UTC timestamp when stats were captured
}

// InvariantValid returns true if Total equals the sum of all state partitions.
func (s PoolStats) InvariantValid() bool {
	return s.Total == s.Ready+s.Leased+s.RateLimited+s.SignedOut+s.Error
}
