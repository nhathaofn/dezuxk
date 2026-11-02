package pool_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

func TestCooldown_RealTimeShortDuration(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "cooldown-realtime"
	mgr.AddAccount(createTestAccount("acc-1", providerID, providers.AccountStatusReady))

	// Lease and mark rate-limited for 40ms
	acc, release, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("initial lease failed: %v", err)
	}
	err = mgr.MarkAccountRateLimited(ctx, acc.ID, 40*time.Millisecond)
	if err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}
	release()

	// Immediate lease should fail with ErrPoolExhausted
	_, _, err = mgr.LeaseAccount(ctx, providerID)
	if !errors.Is(err, pool.ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted during active cooldown, got: %v", err)
	}

	stats, _ := mgr.GetPoolStats(ctx, providerID)
	if stats.RateLimited != 1 || stats.Ready != 0 {
		t.Fatalf("expected RateLimited: 1, Ready: 0; got %+v", stats)
	}

	// Sleep 60ms to exceed the 40ms timer
	time.Sleep(60 * time.Millisecond)

	// Lease should now succeed and account should be ready
	recovered, release2, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("lease failed after cooldown expired: %v", err)
	}
	if recovered.ID != "acc-1" {
		t.Fatalf("expected acc-1, got %s", recovered.ID)
	}
	release2()

	stats, _ = mgr.GetPoolStats(ctx, providerID)
	if stats.Ready != 1 || stats.RateLimited != 0 {
		t.Fatalf("expected Ready: 1, RateLimited: 0; got %+v", stats)
	}
}

func TestCooldown_InjectableClockFastDeterminism(t *testing.T) {
	ctx := context.Background()
	clock := newMockClock(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	mgr := pool.NewManager(nil, pool.WithClock(clock))
	defer mgr.Close()

	providerID := "cooldown-mock"
	mgr.AddAccount(createTestAccount("acc-mock", providerID, providers.AccountStatusReady))

	// Mark rate-limited for 24 hours
	err := mgr.MarkAccountRateLimited(ctx, "acc-mock", 24*time.Hour)
	if err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}

	// Lease should fail immediately
	_, _, err = mgr.LeaseAccount(ctx, providerID)
	if !errors.Is(err, pool.ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted, got: %v", err)
	}

	// Advance clock by 24 hours + 1 second instantly
	clock.Advance(24*time.Hour + 1*time.Second)

	// Lease should succeed immediately without any wall-clock wait!
	recovered, release, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("lease failed after clock advance: %v", err)
	}
	if recovered.ID != "acc-mock" {
		t.Fatalf("expected acc-mock, got %s", recovered.ID)
	}
	release()

	stats, _ := mgr.GetPoolStats(ctx, providerID)
	if stats.Ready != 1 || stats.RateLimited != 0 {
		t.Fatalf("expected Ready: 1, RateLimited: 0; got %+v", stats)
	}
}

func TestCooldown_CascadingMultiAccountRecovery(t *testing.T) {
	ctx := context.Background()
	clock := newMockClock(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	mgr := pool.NewManager(nil, pool.WithClock(clock))
	defer mgr.Close()

	providerID := "cascading-prov"
	mgr.AddAccount(createTestAccount("acc-1h", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-2h", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-3h", providerID, providers.AccountStatusReady))

	// Rate limit all 3 accounts with staggered durations
	_ = mgr.MarkAccountRateLimited(ctx, "acc-1h", 1*time.Hour)
	_ = mgr.MarkAccountRateLimited(ctx, "acc-2h", 2*time.Hour)
	_ = mgr.MarkAccountRateLimited(ctx, "acc-3h", 3*time.Hour)

	stats, _ := mgr.GetPoolStats(ctx, providerID)
	if stats.RateLimited != 3 || stats.Ready != 0 {
		t.Fatalf("expected RateLimited: 3, Ready: 0; got %+v", stats)
	}

	// Advance 1h + 1s: acc-1h recovers
	clock.Advance(1*time.Hour + 1*time.Second)
	stats, _ = mgr.GetPoolStats(ctx, providerID)
	if stats.Ready != 1 || stats.RateLimited != 2 {
		t.Fatalf("after 1h: expected Ready: 1, RateLimited: 2; got %+v", stats)
	}

	// Advance another 1h: acc-2h recovers
	clock.Advance(1 * time.Hour)
	stats, _ = mgr.GetPoolStats(ctx, providerID)
	if stats.Ready != 2 || stats.RateLimited != 1 {
		t.Fatalf("after 2h: expected Ready: 2, RateLimited: 1; got %+v", stats)
	}

	// Advance another 1h: acc-3h recovers
	clock.Advance(1 * time.Hour)
	stats, _ = mgr.GetPoolStats(ctx, providerID)
	if stats.Ready != 3 || stats.RateLimited != 0 {
		t.Fatalf("after 3h: expected Ready: 3, RateLimited: 0; got %+v", stats)
	}
}

func TestCooldown_SignOutCancelsPendingTimer(t *testing.T) {
	ctx := context.Background()
	clock := newMockClock(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	mgr := pool.NewManager(nil, pool.WithClock(clock))
	defer mgr.Close()

	providerID := "so-cancel"
	mgr.AddAccount(createTestAccount("acc-so-cancel", providerID, providers.AccountStatusReady))

	// Mark rate-limited for 2 hours
	_ = mgr.MarkAccountRateLimited(ctx, "acc-so-cancel", 2*time.Hour)

	// Immediately mark signed out
	err := mgr.MarkAccountSignedOut(ctx, "acc-so-cancel")
	if err != nil {
		t.Fatalf("MarkAccountSignedOut failed: %v", err)
	}

	stats, _ := mgr.GetPoolStats(ctx, providerID)
	if stats.SignedOut != 1 || stats.RateLimited != 0 {
		t.Fatalf("expected SignedOut: 1, RateLimited: 0; got %+v", stats)
	}

	// Advance clock past the original 2 hour cooldown
	clock.Advance(3 * time.Hour)

	// Status must remain signed_out and NOT resurrect to ready
	stats, _ = mgr.GetPoolStats(ctx, providerID)
	if stats.SignedOut != 1 || stats.Ready != 0 {
		t.Fatalf("account resurrected! expected SignedOut: 1, Ready: 0; got %+v", stats)
	}

	_, _, err = mgr.LeaseAccount(ctx, providerID)
	if !errors.Is(err, pool.ErrNoAccountsAvailable) {
		t.Fatalf("expected ErrNoAccountsAvailable, got: %v", err)
	}
}
