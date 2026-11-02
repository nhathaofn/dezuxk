package pool

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNoAccountsAvailable indicates that no operational accounts are registered or ready
	// for the requested provider (e.g. pool is empty, or all are signed_out/unverified/error).
	ErrNoAccountsAvailable = errors.New("pool: no ready accounts available for provider")

	// ErrPoolExhausted indicates that accounts exist and are operational, but all are
	// currently busy (leased in-flight or in rate-limit cooldown).
	ErrPoolExhausted = errors.New("pool: account pool exhausted (all accounts leased or rate-limited)")

	// ErrAccountNotFound indicates that the specified account ID does not exist in the pool.
	ErrAccountNotFound = errors.New("pool: account not found")

	// ErrProviderNotFound indicates that the specified provider ID has no configured pool.
	ErrProviderNotFound = errors.New("pool: provider not found")

	// ErrInvalidAccountStatus indicates an invalid or unsupported status transition.
	ErrInvalidAccountStatus = errors.New("pool: invalid account status transition")
)

// PoolExhaustedError wraps ErrPoolExhausted with diagnostic timing information for proxy Retry-After.
type PoolExhaustedError struct {
	ProviderID      string
	TotalAccounts   int
	LeasedAccounts  int
	RateLimited     int
	EarliestResetAt *time.Time
}

func (e *PoolExhaustedError) Error() string {
	if e.EarliestResetAt != nil {
		return fmt.Sprintf("pool: provider %s exhausted (total: %d, leased: %d, rate_limited: %d, earliest reset: %s)",
			e.ProviderID, e.TotalAccounts, e.LeasedAccounts, e.RateLimited, e.EarliestResetAt.UTC().Format(time.RFC3339))
	}
	return fmt.Sprintf("pool: provider %s exhausted (total: %d, leased: %d, rate_limited: %d)",
		e.ProviderID, e.TotalAccounts, e.LeasedAccounts, e.RateLimited)
}

func (e *PoolExhaustedError) Is(target error) bool {
	return target == ErrPoolExhausted
}

func (e *PoolExhaustedError) Unwrap() error {
	return ErrPoolExhausted
}
