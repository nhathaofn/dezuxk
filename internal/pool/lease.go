package pool

import (
	"time"

	"dezuxk/internal/domain/providers"
)

// poolAccount wraps domain Account with leasing and lifecycle metadata.
type poolAccount struct {
	account        *providers.Account
	inFlight       int
	leasedAt       time.Time
	releasedAt     time.Time
	leaseCount     uint64
	cooldownTimer  Timer
	cooldownExpiry time.Time
}

// cloneAccount creates a deep copy of a providers.Account to prevent callers
// from mutating internal pool state.
func cloneAccount(src *providers.Account) *providers.Account {
	if src == nil {
		return nil
	}
	dst := *src
	if src.PausedUntil != nil {
		t := *src.PausedUntil
		dst.PausedUntil = &t
	}
	if src.LastCheckedAt != nil {
		t := *src.LastCheckedAt
		dst.LastCheckedAt = &t
	}
	if src.Usage != nil {
		dst.Usage = make([]providers.QuotaUsage, len(src.Usage))
		copy(dst.Usage, src.Usage)
	}
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}
	return &dst
}
