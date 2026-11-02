package pool_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

func TestManager_BasicLeaseAndRelease(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	acc := createTestAccount("acc-1", "flow", providers.AccountStatusReady)
	mgr.AddAccount(acc)

	stats, err := mgr.GetPoolStats(ctx, "flow")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.Ready != 1 || stats.Total != 1 || stats.Leased != 0 {
		t.Fatalf("expected Ready: 1, Leased: 0; got Ready: %d, Leased: %d", stats.Ready, stats.Leased)
	}

	leased, release, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("LeaseAccount failed: %v", err)
	}
	if leased.ID != "acc-1" {
		t.Fatalf("expected leased account ID 'acc-1', got %s", leased.ID)
	}

	// Verify stats while leased
	stats, err = mgr.GetPoolStats(ctx, "flow")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.Ready != 0 || stats.Leased != 1 {
		t.Fatalf("expected Ready: 0, Leased: 1; got Ready: %d, Leased: %d", stats.Ready, stats.Leased)
	}

	// Release
	release()

	// Verify stats after release
	stats, err = mgr.GetPoolStats(ctx, "flow")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.Ready != 1 || stats.Leased != 0 {
		t.Fatalf("expected Ready: 1, Leased: 0; got Ready: %d, Leased: %d", stats.Ready, stats.Leased)
	}
}

func TestManager_DeepCopyCloning(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	acc := createTestAccount("acc-clone", "flow", providers.AccountStatusReady)
	mgr.AddAccount(acc)

	leased, release, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("LeaseAccount failed: %v", err)
	}

	// Caller mutates leased struct
	leased.Metadata["env"] = "corrupted"
	leased.Usage[0].Remaining = 0
	release()

	// Verify internal state is uncorrupted
	stored, ok := mgr.GetAccount("flow", "acc-clone")
	if !ok {
		t.Fatalf("GetAccount failed to find account")
	}
	if stored.Metadata["env"] != "test" {
		t.Fatalf("expected Metadata[env] to remain 'test', got %v", stored.Metadata["env"])
	}
	if stored.Usage[0].Remaining != 100 {
		t.Fatalf("expected Usage[0].Remaining to remain 100, got %d", stored.Usage[0].Remaining)
	}
}

func TestManager_Errors_ProviderAndAccountNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	// 1. Unknown provider lease
	_, _, err := mgr.LeaseAccount(ctx, "nonexistent-provider")
	if !errors.Is(err, pool.ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got: %v", err)
	}

	// 2. Unknown account rate limited
	err = mgr.MarkAccountRateLimited(ctx, "nonexistent-acc", 10*time.Minute)
	if !errors.Is(err, pool.ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got: %v", err)
	}

	// 3. Unknown account signed out
	err = mgr.MarkAccountSignedOut(ctx, "nonexistent-acc")
	if !errors.Is(err, pool.ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got: %v", err)
	}
}

func TestManager_PoolExhausted(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	mgr.AddAccount(createTestAccount("acc-1", "flow", providers.AccountStatusReady))

	_, release, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("first lease failed: %v", err)
	}
	defer release()

	// Second lease should return ErrPoolExhausted
	_, _, err = mgr.LeaseAccount(ctx, "flow")
	if err == nil {
		t.Fatalf("expected error on second lease, got nil")
	}
	if !errors.Is(err, pool.ErrPoolExhausted) {
		t.Fatalf("expected errors.Is(err, ErrPoolExhausted), got: %v", err)
	}

	var exhaustedErr *pool.PoolExhaustedError
	if !errors.As(err, &exhaustedErr) {
		t.Fatalf("expected *pool.PoolExhaustedError type, got %T", err)
	}
	if exhaustedErr.ProviderID != "flow" || exhaustedErr.TotalAccounts != 1 || exhaustedErr.LeasedAccounts != 1 {
		t.Fatalf("unexpected PoolExhaustedError content: %+v", exhaustedErr)
	}
}

func TestManager_SignedOutEviction(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	mgr.AddAccount(createTestAccount("acc-so", "flow", providers.AccountStatusReady))

	err := mgr.MarkAccountSignedOut(ctx, "acc-so")
	if err != nil {
		t.Fatalf("MarkAccountSignedOut failed: %v", err)
	}

	// Leased account should now return ErrNoAccountsAvailable
	_, _, err = mgr.LeaseAccount(ctx, "flow")
	if !errors.Is(err, pool.ErrNoAccountsAvailable) {
		t.Fatalf("expected ErrNoAccountsAvailable, got: %v", err)
	}

	stats, err := mgr.GetPoolStats(ctx, "flow")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.SignedOut != 1 || stats.Ready != 0 || stats.Total != 1 {
		t.Fatalf("expected SignedOut: 1, Ready: 0, Total: 1; got %+v", stats)
	}
}

func TestManager_StatsInvariantAllStates(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	// Register 5 accounts
	mgr.AddAccount(createTestAccount("acc-1", "flow", providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-2", "flow", providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-3", "flow", providers.AccountStatusRateLimited))
	mgr.AddAccount(createTestAccount("acc-4", "flow", providers.AccountStatusSignedOut))
	mgr.AddAccount(createTestAccount("acc-5", "flow", providers.AccountStatusError))

	// Lease acc-1
	_, release1, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("lease failed: %v", err)
	}
	defer release1()

	stats, err := mgr.GetPoolStats(ctx, "flow")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}

	if !stats.InvariantValid() {
		t.Fatalf("PoolStats invariant violation! Total: %d != Ready(%d)+Leased(%d)+RateLimited(%d)+SignedOut(%d)+Error(%d)",
			stats.Total, stats.Ready, stats.Leased, stats.RateLimited, stats.SignedOut, stats.Error)
	}
	if stats.Total != 5 || stats.Leased != 1 || stats.Ready != 1 || stats.RateLimited != 1 || stats.SignedOut != 1 || stats.Error != 1 {
		t.Fatalf("unexpected stats distribution: %+v", stats)
	}

	// Also verify aggregate stats across all providers
	allStats, err := mgr.GetPoolStats(ctx, "")
	if err != nil {
		t.Fatalf("GetPoolStats('') failed: %v", err)
	}
	if !allStats.InvariantValid() || allStats.Total != 5 {
		t.Fatalf("aggregate stats invariant violation: %+v", allStats)
	}
}

func TestManager_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mgr := pool.NewManager(nil)
	defer mgr.Close()
	mgr.AddAccount(createTestAccount("acc-1", "flow", providers.AccountStatusReady))

	_, _, err := mgr.LeaseAccount(ctx, "flow")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestManager_WaiterQueueOnExhaustion(t *testing.T) {
	ctx := context.Background()
	// Manager configured with wait timeout of 500ms
	mgr := pool.NewManager(nil, pool.WithWaitTimeout(500*time.Millisecond))
	defer mgr.Close()

	mgr.AddAccount(createTestAccount("acc-waiter", "flow", providers.AccountStatusReady))

	// Goroutine 1 leases the account
	leased1, release1, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("first lease failed: %v", err)
	}
	if leased1.ID != "acc-waiter" {
		t.Fatalf("expected acc-waiter, got %s", leased1.ID)
	}

	receivedCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Goroutine 2 attempts to lease with wait queue
	go func() {
		leased2, release2, err := mgr.LeaseAccount(ctx, "flow")
		if err != nil {
			errCh <- err
			return
		}
		defer release2()
		receivedCh <- leased2.ID
	}()

	// Sleep 30ms then release from Goroutine 1
	time.Sleep(30 * time.Millisecond)
	release1()

	select {
	case id := <-receivedCh:
		if id != "acc-waiter" {
			t.Fatalf("expected waiter to receive acc-waiter, got %s", id)
		}
	case err := <-errCh:
		t.Fatalf("waiter failed to acquire lease: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for waiter to acquire lease")
	}
}
