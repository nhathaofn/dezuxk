package pool_test

import (
	"sync"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

// mockTimer implements pool.Timer for deterministic unit testing.
type mockTimer struct {
	clock   *mockClock
	expiry  time.Time
	fn      func()
	stopped bool
}

func (mt *mockTimer) Stop() bool {
	mt.clock.mu.Lock()
	defer mt.clock.mu.Unlock()
	if mt.stopped {
		return false
	}
	mt.stopped = true
	return true
}

// mockClock implements pool.Clock for fast, instant duration advancement.
type mockClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*mockTimer
}

func newMockClock(start time.Time) *mockClock {
	return &mockClock{
		now:    start,
		timers: make([]*mockTimer, 0),
	}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) AfterFunc(d time.Duration, f func()) pool.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &mockTimer{
		clock:  c,
		expiry: c.now.Add(d),
		fn:     f,
	}
	c.timers = append(c.timers, t)
	return t
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	current := c.now

	var toFire []func()
	var remaining []*mockTimer
	for _, t := range c.timers {
		if !t.stopped && !current.Before(t.expiry) {
			t.stopped = true
			toFire = append(toFire, t.fn)
		} else if !t.stopped {
			remaining = append(remaining, t)
		}
	}
	c.timers = remaining
	c.mu.Unlock()

	for _, fn := range toFire {
		fn()
	}
}

// createTestAccount creates a domain account for testing.
func createTestAccount(id, providerID string, status providers.AccountStatus) *providers.Account {
	now := time.Now().UTC()
	return &providers.Account{
		ID:         id,
		ProviderID: providerID,
		Label:      "Test Account " + id,
		Status:     status,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata: map[string]any{
			"env": "test",
		},
		Usage: []providers.QuotaUsage{
			{Feature: "image_gen", Remaining: 100, Total: 100},
		},
	}
}
