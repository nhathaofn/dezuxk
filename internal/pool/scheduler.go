package pool

import (
	"time"

	"dezuxk/internal/domain/providers"
)

// selectAvailableAccountLocked searches for the next available ready account
// using circular round-robin rotation starting from rrCursor.
// It also checks for and executes lazy cooldown recovery.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) selectAvailableAccountLocked(now time.Time) (*poolAccount, error) {
	n := len(sp.accounts)
	if n == 0 {
		return nil, ErrNoAccountsAvailable
	}

	var (
		totalCount       = n
		readyCount       = 0
		leasedCount      = 0
		rateLimitedCount = 0
		signedOutCount   = 0
		errorCount       = 0
		earliestReset    *time.Time
	)

	// Pass 1: Lazy recovery check and state partition accounting
	for _, pa := range sp.accounts {
		// Lazy cooldown recovery check
		if pa.account.Status == providers.AccountStatusRateLimited &&
			pa.account.PausedUntil != nil &&
			!now.Before(*pa.account.PausedUntil) {
			pa.account.Status = providers.AccountStatusReady
			pa.account.PausedUntil = nil
			if pa.cooldownTimer != nil {
				pa.cooldownTimer.Stop()
				pa.cooldownTimer = nil
			}
			if sp.onLazyRecover != nil {
				sp.onLazyRecover(sp.providerID, pa.account.ID)
			}
		}

		switch pa.account.Status {
		case providers.AccountStatusReady:
			if pa.inFlight >= sp.maxInFlight {
				leasedCount++
			} else {
				readyCount++
			}
		case providers.AccountStatusRateLimited:
			rateLimitedCount++
			if pa.account.PausedUntil != nil {
				if earliestReset == nil || pa.account.PausedUntil.Before(*earliestReset) {
					earliestReset = pa.account.PausedUntil
				}
			}
		case providers.AccountStatusSignedOut:
			signedOutCount++
		case providers.AccountStatusError, providers.AccountStatusUnverified:
			errorCount++
		default:
			errorCount++
		}
	}

	// Pass 2: Fair circular scan starting at rrCursor
	for i := 0; i < n; i++ {
		idx := (sp.rrCursor + i) % n
		candidate := sp.accounts[idx]

		if candidate.account.Status != providers.AccountStatusReady {
			continue
		}
		if candidate.inFlight >= sp.maxInFlight {
			continue
		}

		// Account is available for lease
		candidate.inFlight++
		candidate.leasedAt = now
		candidate.leaseCount++
		checkedAt := now
		candidate.account.LastCheckedAt = &checkedAt

		// Advance cursor to next slot
		sp.rrCursor = (idx + 1) % n
		return candidate, nil
	}

	// No accounts could be leased. Determine appropriate sentinel error.
	if readyCount == 0 && leasedCount == 0 && rateLimitedCount == 0 {
		return nil, ErrNoAccountsAvailable
	}

	return nil, &PoolExhaustedError{
		ProviderID:      sp.providerID,
		TotalAccounts:   totalCount,
		LeasedAccounts:  leasedCount,
		RateLimited:     rateLimitedCount,
		EarliestResetAt: earliestReset,
	}
}
