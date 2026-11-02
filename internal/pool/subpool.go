package pool

import (
	"sync"
	"time"

	"dezuxk/internal/domain/providers"
)

// leaseWaiter enables non-polling, context-cancellable FIFO waiting when pool is busy.
type leaseWaiter struct {
	ch    chan *poolAccount
	errCh chan error
	done  bool
}

// providerSubPool manages accounts and concurrency locks for a single provider.
type providerSubPool struct {
	mu            sync.Mutex
	providerID    string
	accounts      []*poolAccount
	byID          map[string]*poolAccount
	rrCursor      int
	waiters       []*leaseWaiter
	maxInFlight   int
	clock         Clock
	onLazyRecover func(providerID, accountID string)
}

func newProviderSubPool(providerID string, clock Clock, maxInFlight int, onLazyRecover func(string, string)) *providerSubPool {
	if maxInFlight <= 0 {
		maxInFlight = 1
	}
	if clock == nil {
		clock = realClock{}
	}
	return &providerSubPool{
		providerID:    providerID,
		accounts:      make([]*poolAccount, 0),
		byID:          make(map[string]*poolAccount),
		maxInFlight:   maxInFlight,
		clock:         clock,
		onLazyRecover: onLazyRecover,
	}
}

// addAccountLocked adds or updates an account in the subpool.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) addAccountLocked(acc *providers.Account) *poolAccount {
	if existing, ok := sp.byID[acc.ID]; ok {
		existing.account = acc
		return existing
	}

	pa := &poolAccount{
		account: acc,
	}
	sp.accounts = append(sp.accounts, pa)
	sp.byID[acc.ID] = pa
	return pa
}

// removeAccountLocked removes an account from the subpool.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) removeAccountLocked(id string) {
	pa, ok := sp.byID[id]
	if !ok {
		return
	}
	if pa.cooldownTimer != nil {
		pa.cooldownTimer.Stop()
		pa.cooldownTimer = nil
	}
	delete(sp.byID, id)

	for i, a := range sp.accounts {
		if a.account.ID == id {
			sp.accounts = append(sp.accounts[:i], sp.accounts[i+1:]...)
			break
		}
	}
	if len(sp.accounts) == 0 {
		sp.rrCursor = 0
	} else if sp.rrCursor >= len(sp.accounts) {
		sp.rrCursor = 0
	}
}

// getStatsLocked captures the point-in-time state of the subpool.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) getStatsLocked(now time.Time) PoolStats {
	stats := PoolStats{
		ProviderID: sp.providerID,
		Total:      len(sp.accounts),
		Timestamp:  now,
	}

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
			if pa.inFlight > 0 {
				stats.Leased++
			} else {
				stats.Ready++
			}
		case providers.AccountStatusRateLimited:
			stats.RateLimited++
		case providers.AccountStatusSignedOut:
			stats.SignedOut++
		case providers.AccountStatusError, providers.AccountStatusUnverified:
			stats.Error++
		default:
			stats.Error++
		}
	}

	return stats
}

// enqueueWaiterLocked registers a waiter in the FIFO queue and returns an idempotent cancellation func.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) enqueueWaiterLocked() (*leaseWaiter, func()) {
	w := &leaseWaiter{
		ch:    make(chan *poolAccount, 1),
		errCh: make(chan error, 1),
	}
	sp.waiters = append(sp.waiters, w)
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			sp.mu.Lock()
			defer sp.mu.Unlock()
			w.done = true
			for i, waiter := range sp.waiters {
				if waiter == w {
					sp.waiters = append(sp.waiters[:i], sp.waiters[i+1:]...)
					break
				}
			}
			// Drain and release account if one was handed over right before or during cancellation
			select {
			case leased := <-w.ch:
				sp.releaseLocked(leased)
			default:
			}
		})
	}
	return w, cancel
}

// releaseLocked returns a leased account to the pool or hands it over to the next waiter.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) releaseLocked(acc *poolAccount) {
	if acc == nil || acc.account == nil {
		return
	}

	now := sp.clock.Now()
	acc.releasedAt = now

	// If account is still in pool and ready, and there are waiters in queue, hand over directly
	if current, exists := sp.byID[acc.account.ID]; exists && current == acc && acc.account.Status == providers.AccountStatusReady {
		for len(sp.waiters) > 0 {
			w := sp.waiters[0]
			sp.waiters = sp.waiters[1:]
			if !w.done {
				w.done = true
				acc.leasedAt = now
				acc.leaseCount++
				checkedAt := now
				acc.account.LastCheckedAt = &checkedAt
				w.ch <- acc
				return
			}
		}
	}

	// No waiting caller claimed it (or account was removed/not ready): decrement inFlight
	if acc.inFlight > 0 {
		acc.inFlight--
	}
}

// release returns a leased account to the pool or hands it over to the next waiter.
func (sp *providerSubPool) release(acc *poolAccount) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.releaseLocked(acc)
}

// makeReleaseFunc returns an idempotent release callback wrapped with sync.Once.
func (sp *providerSubPool) makeReleaseFunc(acc *poolAccount) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			sp.release(acc)
		})
	}
}

// markRateLimitedLocked puts an account into rate-limited state with a cooldown timer.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) markRateLimitedLocked(accountID string, duration time.Duration, onTimerRecover func(providerID, accountID string)) (*time.Time, error) {
	pa, ok := sp.byID[accountID]
	if !ok {
		return nil, ErrAccountNotFound
	}

	if pa.cooldownTimer != nil {
		pa.cooldownTimer.Stop()
		pa.cooldownTimer = nil
	}

	pausedUntil := sp.clock.Now().Add(duration)
	pa.account.Status = providers.AccountStatusRateLimited
	pa.account.PausedUntil = &pausedUntil
	pa.cooldownExpiry = pausedUntil

	pa.cooldownTimer = sp.clock.AfterFunc(duration, func() {
		sp.recoverAccount(accountID, pausedUntil, onTimerRecover)
	})

	return &pausedUntil, nil
}

// recoverAccount is invoked when cooldown timer expires to transition account back to ready.
func (sp *providerSubPool) recoverAccount(accountID string, expectedExpiry time.Time, onTimerRecover func(providerID, accountID string)) {
	sp.mu.Lock()
	pa, ok := sp.byID[accountID]
	if !ok || pa.account.Status != providers.AccountStatusRateLimited {
		sp.mu.Unlock()
		return
	}

	now := sp.clock.Now()
	if now.Before(expectedExpiry) {
		sp.mu.Unlock()
		return
	}

	pa.account.Status = providers.AccountStatusReady
	pa.account.PausedUntil = nil
	pa.cooldownTimer = nil

	// Check if any waiter can take this newly ready account without violating concurrency limits
	if pa.inFlight < sp.maxInFlight {
		for len(sp.waiters) > 0 {
			w := sp.waiters[0]
			sp.waiters = sp.waiters[1:]
			if !w.done {
				w.done = true
				pa.inFlight++
				pa.leasedAt = now
				pa.leaseCount++
				checkedAt := now
				pa.account.LastCheckedAt = &checkedAt
				w.ch <- pa
				break
			}
		}
	}
	sp.mu.Unlock()

	if onTimerRecover != nil {
		onTimerRecover(sp.providerID, accountID)
	}
}

// markSignedOutLocked flags an account as signed-out and cancels any active timer.
// MUST be called while holding sp.mu.
func (sp *providerSubPool) markSignedOutLocked(accountID string) error {
	pa, ok := sp.byID[accountID]
	if !ok {
		return ErrAccountNotFound
	}
	if pa.cooldownTimer != nil {
		pa.cooldownTimer.Stop()
		pa.cooldownTimer = nil
	}
	pa.account.Status = providers.AccountStatusSignedOut
	pa.account.PausedUntil = nil
	return nil
}

// stopAllTimersLocked cancels any pending timers in the subpool.
func (sp *providerSubPool) stopAllTimersLocked() {
	for _, pa := range sp.accounts {
		if pa.cooldownTimer != nil {
			pa.cooldownTimer.Stop()
			pa.cooldownTimer = nil
		}
	}
}
