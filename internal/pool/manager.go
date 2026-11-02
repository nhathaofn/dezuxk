package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dezuxk/internal/domain/providers"
)

// Manager specifies the thread-safe account pool lifecycle and leasing operations.
type Manager interface {
	LeaseAccount(ctx context.Context, providerID string) (*providers.Account, func(), error)
	MarkAccountRateLimited(ctx context.Context, accountID string, duration time.Duration) error
	MarkAccountSignedOut(ctx context.Context, accountID string) error
	GetPoolStats(ctx context.Context, providerID string) (PoolStats, error)
}

// Options configures behavior of PoolManager.
type Options struct {
	MaxInFlight int
	WaitTimeout time.Duration
	Clock       Clock
}

// Option modifies PoolManager configuration.
type Option func(*Options)

// WithMaxInFlight sets maximum concurrent leases per account (default 1 for exclusive lease).
func WithMaxInFlight(n int) Option {
	return func(o *Options) {
		o.MaxInFlight = n
	}
}

// WithWaitTimeout sets default duration to wait in FIFO queue before returning ErrPoolExhausted.
func WithWaitTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.WaitTimeout = d
	}
}

// WithClock provides custom clock implementation for testing.
func WithClock(c Clock) Option {
	return func(o *Options) {
		o.Clock = c
	}
}

// PoolManager coordinates thread-safe leasing, fair rotation, and status transitions.
type PoolManager struct {
	mu              sync.RWMutex
	repo            providers.Repository
	pools           map[string]*providerSubPool
	accountProvider map[string]string // accountID -> providerID index
	clock           Clock
	opts            Options
}

// NewManager creates a new initialized PoolManager.
func NewManager(repo providers.Repository, opts ...Option) *PoolManager {
	options := Options{
		MaxInFlight: 1,
		WaitTimeout: 0,
		Clock:       realClock{},
	}
	for _, opt := range opts {
		opt(&options)
	}
	if options.Clock == nil {
		options.Clock = realClock{}
	}
	if options.MaxInFlight <= 0 {
		options.MaxInFlight = 1
	}

	return &PoolManager{
		repo:            repo,
		pools:           make(map[string]*providerSubPool),
		accountProvider: make(map[string]string),
		clock:           options.Clock,
		opts:            options,
	}
}

// AddAccount registers an account in the pool under its provider.
func (m *PoolManager) AddAccount(acc *providers.Account) {
	if acc == nil {
		return
	}

	m.mu.Lock()
	sp, exists := m.pools[acc.ProviderID]
	if !exists {
		sp = newProviderSubPool(acc.ProviderID, m.clock, m.opts.MaxInFlight, m.onLazyRecover)
		m.pools[acc.ProviderID] = sp
	}
	m.accountProvider[acc.ID] = acc.ProviderID
	m.mu.Unlock()

	sp.mu.Lock()
	sp.addAccountLocked(acc)
	sp.mu.Unlock()
}

// RemoveAccount removes an account from the pool.
func (m *PoolManager) RemoveAccount(providerID, accountID string) {
	m.mu.Lock()
	sp, exists := m.pools[providerID]
	delete(m.accountProvider, accountID)
	m.mu.Unlock()

	if exists {
		sp.mu.Lock()
		sp.removeAccountLocked(accountID)
		sp.mu.Unlock()
	}
}

// GetAccount retrieves a cloned copy of an account from the pool.
func (m *PoolManager) GetAccount(providerID, accountID string) (*providers.Account, bool) {
	m.mu.RLock()
	sp, exists := m.pools[providerID]
	m.mu.RUnlock()

	if !exists {
		return nil, false
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()
	pa, ok := sp.byID[accountID]
	if !ok {
		return nil, false
	}
	return cloneAccount(pa.account), true
}

// LoadFromRepository hydrates the pool from SQLite persistence and applies cold-start recovery.
func (m *PoolManager) LoadFromRepository(ctx context.Context) (int, error) {
	if m.repo == nil {
		return 0, nil
	}

	accounts, err := m.repo.ListAccounts(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("pool: load from repository: %w", err)
	}

	now := m.clock.Now()
	loaded := 0

	for i := range accounts {
		acc := accounts[i]

		// Cold start recovery check for rate-limited accounts
		if acc.Status == providers.AccountStatusRateLimited {
			if acc.PausedUntil == nil || !now.Before(*acc.PausedUntil) {
				// Expired during downtime: heal to ready
				acc.Status = providers.AccountStatusReady
				acc.PausedUntil = nil
				_ = m.repo.UpdateAccountStatus(ctx, acc.ID, providers.AccountStatusReady, nil)
			}
		}

		m.AddAccount(&acc)
		loaded++

		// If still in future cooldown, schedule recovery timer
		if acc.Status == providers.AccountStatusRateLimited && acc.PausedUntil != nil && acc.PausedUntil.After(now) {
			rem := acc.PausedUntil.Sub(now)
			m.scheduleFutureRateLimit(acc.ProviderID, acc.ID, rem, *acc.PausedUntil)
		}
	}

	return loaded, nil
}

func (m *PoolManager) scheduleFutureRateLimit(providerID, accountID string, duration time.Duration, expiry time.Time) {
	m.mu.RLock()
	sp, ok := m.pools[providerID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()
	pa, ok := sp.byID[accountID]
	if !ok || pa.account.Status != providers.AccountStatusRateLimited {
		return
	}
	pa.cooldownExpiry = expiry
	pa.cooldownTimer = sp.clock.AfterFunc(duration, func() {
		sp.recoverAccount(accountID, expiry, m.onAccountRecovered)
	})
}

// LeaseAccount acquires an available account for the requested provider.
// Returns a deep copy of the account and an idempotent release callback.
func (m *PoolManager) LeaseAccount(ctx context.Context, providerID string) (*providers.Account, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	m.mu.RLock()
	sp, exists := m.pools[providerID]
	m.mu.RUnlock()

	if !exists {
		return nil, nil, ErrProviderNotFound
	}

	sp.mu.Lock()
	acc, err := sp.selectAvailableAccountLocked(m.clock.Now())
	if err == nil {
		releaseFunc := sp.makeReleaseFunc(acc)
		cloned := cloneAccount(acc.account)
		sp.mu.Unlock()
		return cloned, releaseFunc, nil
	}

	// If pool is exhausted and wait timeout is configured, queue as waiter
	if (err == ErrPoolExhausted || isPoolExhausted(err)) && m.opts.WaitTimeout > 0 {
		w, cancel := sp.enqueueWaiterLocked()
		sp.mu.Unlock()
		defer cancel()

		select {
		case <-ctx.Done():
			cancel()
			return nil, nil, ctx.Err()
		case <-time.After(m.opts.WaitTimeout):
			cancel()
			return nil, nil, err
		case leased := <-w.ch:
			if err := ctx.Err(); err != nil {
				cancel()
				sp.release(leased)
				return nil, nil, err
			}
			return cloneAccount(leased.account), sp.makeReleaseFunc(leased), nil
		case werr := <-w.errCh:
			cancel()
			return nil, nil, werr
		}
	}

	sp.mu.Unlock()
	return nil, nil, err
}

// LeaseAccountWait acquires an available account, waiting up to waitTimeout if currently busy.
func (m *PoolManager) LeaseAccountWait(ctx context.Context, providerID string, waitTimeout time.Duration) (*providers.Account, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	m.mu.RLock()
	sp, exists := m.pools[providerID]
	m.mu.RUnlock()

	if !exists {
		return nil, nil, ErrProviderNotFound
	}

	sp.mu.Lock()
	acc, err := sp.selectAvailableAccountLocked(m.clock.Now())
	if err == nil {
		releaseFunc := sp.makeReleaseFunc(acc)
		cloned := cloneAccount(acc.account)
		sp.mu.Unlock()
		return cloned, releaseFunc, nil
	}

	if (err == ErrPoolExhausted || isPoolExhausted(err)) && waitTimeout > 0 {
		w, cancel := sp.enqueueWaiterLocked()
		sp.mu.Unlock()
		defer cancel()

		select {
		case <-ctx.Done():
			cancel()
			return nil, nil, ctx.Err()
		case <-time.After(waitTimeout):
			cancel()
			return nil, nil, err
		case leased := <-w.ch:
			if err := ctx.Err(); err != nil {
				cancel()
				sp.release(leased)
				return nil, nil, err
			}
			return cloneAccount(leased.account), sp.makeReleaseFunc(leased), nil
		case werr := <-w.errCh:
			cancel()
			return nil, nil, werr
		}
	}

	sp.mu.Unlock()
	return nil, nil, err
}

func isPoolExhausted(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*PoolExhaustedError)
	return ok
}

// MarkAccountRateLimited transitions an account to rate-limited status with cooldown.
// SQLite persistence update is dispatched outside the in-memory pool lock.
func (m *PoolManager) MarkAccountRateLimited(ctx context.Context, accountID string, duration time.Duration) error {
	m.mu.RLock()
	providerID, ok := m.accountProvider[accountID]
	if !ok {
		m.mu.RUnlock()
		return ErrAccountNotFound
	}
	sp := m.pools[providerID]
	m.mu.RUnlock()

	sp.mu.Lock()
	pausedUntil, err := sp.markRateLimitedLocked(accountID, duration, m.onAccountRecovered)
	sp.mu.Unlock()
	if err != nil {
		return err
	}

	if m.repo != nil {
		if dbErr := m.repo.UpdateAccountStatus(ctx, accountID, providers.AccountStatusRateLimited, pausedUntil); dbErr != nil {
			return fmt.Errorf("pool: failed to persist rate_limited status for %s: %w", accountID, dbErr)
		}
	}
	return nil
}

// MarkAccountSignedOut transitions an account to signed-out status and cancels timers.
// SQLite persistence update is dispatched outside the in-memory pool lock.
func (m *PoolManager) MarkAccountSignedOut(ctx context.Context, accountID string) error {
	m.mu.RLock()
	providerID, ok := m.accountProvider[accountID]
	if !ok {
		m.mu.RUnlock()
		return ErrAccountNotFound
	}
	sp := m.pools[providerID]
	m.mu.RUnlock()

	sp.mu.Lock()
	err := sp.markSignedOutLocked(accountID)
	sp.mu.Unlock()
	if err != nil {
		return err
	}

	if m.repo != nil {
		if dbErr := m.repo.UpdateAccountStatus(ctx, accountID, providers.AccountStatusSignedOut, nil); dbErr != nil {
			return fmt.Errorf("pool: failed to persist signed_out status for %s: %w", accountID, dbErr)
		}
	}
	return nil
}

func (m *PoolManager) onAccountRecovered(providerID, accountID string) {
	if m.repo != nil {
		_ = m.repo.UpdateAccountStatus(context.Background(), accountID, providers.AccountStatusReady, nil)
	}
}

func (m *PoolManager) onLazyRecover(providerID, accountID string) {
	if m.repo != nil {
		go func() {
			_ = m.repo.UpdateAccountStatus(context.Background(), accountID, providers.AccountStatusReady, nil)
		}()
	}
}

// GetPoolStats reports point-in-time statistics for a specific provider or aggregated across all.
func (m *PoolManager) GetPoolStats(ctx context.Context, providerID string) (PoolStats, error) {
	now := m.clock.Now()
	if providerID != "" {
		m.mu.RLock()
		sp, exists := m.pools[providerID]
		m.mu.RUnlock()
		if !exists {
			return PoolStats{ProviderID: providerID, Timestamp: now}, ErrProviderNotFound
		}
		sp.mu.Lock()
		defer sp.mu.Unlock()
		return sp.getStatsLocked(now), nil
	}

	m.mu.RLock()
	pools := make([]*providerSubPool, 0, len(m.pools))
	for _, sp := range m.pools {
		pools = append(pools, sp)
	}
	m.mu.RUnlock()

	agg := PoolStats{
		ProviderID: "all",
		Timestamp:  now,
	}
	for _, sp := range pools {
		sp.mu.Lock()
		st := sp.getStatsLocked(now)
		sp.mu.Unlock()

		agg.Total += st.Total
		agg.Ready += st.Ready
		agg.Leased += st.Leased
		agg.RateLimited += st.RateLimited
		agg.SignedOut += st.SignedOut
		agg.Error += st.Error
	}
	return agg, nil
}

// Close stops all active timers and releases resources.
func (m *PoolManager) Close() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sp := range m.pools {
		sp.mu.Lock()
		sp.stopAllTimersLocked()
		sp.mu.Unlock()
	}
}
