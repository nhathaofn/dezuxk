package pool_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
	"dezuxk/internal/security"
	"dezuxk/internal/storage/sqlite"
)

// setupChallengeDB initializes an isolated SQLite database with migrations and schema.
func setupChallengeDB(t *testing.T, provID string) (providers.Repository, func()) {
	t.Helper()
	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "challenge_pool.db")

	db, err := sqlite.Open(ctx, sqlite.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}

	cipher, err := security.NewAdaptiveCipher("0123456789abcdef0123456789abcdef", false)
	if err != nil {
		db.Close()
		t.Fatalf("NewAdaptiveCipher failed: %v", err)
	}

	repo := sqlite.NewProvidersRepository(db, cipher)

	// Save provider to satisfy foreign keys
	prov := &providers.Provider{
		ID:        provID,
		Name:      "Challenge Provider " + provID,
		BaseURL:   "https://api.challenge.example.com",
		Status:    providers.ProviderStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.SaveProvider(ctx, prov); err != nil {
		db.Close()
		t.Fatalf("SaveProvider failed: %v", err)
	}

	cleanup := func() {
		db.Close()
	}
	return repo, cleanup
}

// =========================================================================
// Challenge 1: Cooldown Expiry while Account is Leased & Concurrency Limits
// =========================================================================

// TestChallenge_CooldownRecoveryWhileLeased_MaxInFlightConstraint tests whether
// an account that recovers from rate-limit while still leased violates MaxInFlight
// by handing over to a waiting lease caller.
func TestChallenge_CooldownRecoveryWhileLeased_MaxInFlightConstraint(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupChallengeDB(t, "p-inflight")
	defer cleanup()

	acc := createTestAccount("acc-inf-1", "p-inflight", providers.AccountStatusReady)
	if err := repo.SaveAccount(ctx, acc); err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	// Create manager with MaxInFlight = 1
	mgr := pool.NewManager(repo, pool.WithMaxInFlight(1))
	defer mgr.Close()
	_, _ = mgr.LoadFromRepository(ctx)

	// 1. Goroutine 1 leases acc-inf-1
	leased1, release1, err := mgr.LeaseAccount(ctx, "p-inflight")
	if err != nil {
		t.Fatalf("initial lease failed: %v", err)
	}
	if leased1.ID != "acc-inf-1" {
		t.Fatalf("unexpected account: %s", leased1.ID)
	}

	// 2. While leased1 is still held, mark account rate-limited for 40ms
	err = mgr.MarkAccountRateLimited(ctx, leased1.ID, 40*time.Millisecond)
	if err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}

	// 3. Goroutine 2 attempts to lease with wait timeout 200ms
	type leaseResult struct {
		acc     *providers.Account
		release func()
		err     error
	}
	ch2 := make(chan leaseResult, 1)

	go func() {
		a, rel, lerr := mgr.LeaseAccountWait(ctx, "p-inflight", 200*time.Millisecond)
		ch2 <- leaseResult{acc: a, release: rel, err: lerr}
	}()

	// 4. Wait 80ms for the 40ms cooldown timer to expire
	time.Sleep(80 * time.Millisecond)

	// Check if Goroutine 2 acquired the lease while Goroutine 1 has NOT released it
	select {
	case res := <-ch2:
		if res.err != nil {
			t.Logf("Goroutine 2 did not acquire account while Goroutine 1 holds it (err: %v)", res.err)
		} else {
			// Goroutine 2 got the lease!
			// If MaxInFlight == 1, this means BOTH Goroutine 1 and Goroutine 2 held the account simultaneously!
			t.Errorf("CONCURRENCY VIOLATION: Goroutine 2 acquired account %s while Goroutine 1 still holds the lease (MaxInFlight=1)", res.acc.ID)
			res.release()
		}
	default:
		// Goroutine 2 correctly blocked and did not double-lease
		t.Logf("PASS: Goroutine 2 did not double-acquire account while Goroutine 1 still holds the lease")
	}

	// Now Goroutine 1 finally releases
	release1()

	// If Goroutine 2 hadn't received it yet, it should receive it now upon release1
	select {
	case res := <-ch2:
		if res.err != nil {
			t.Fatalf("Goroutine 2 failed to lease after release: %v", res.err)
		}
		t.Logf("Goroutine 2 successfully acquired account after Goroutine 1 released")
		res.release()
	case <-time.After(150 * time.Millisecond):
		// Goroutine 2 may have timed out if not handed over
	}
}

// =========================================================================
// Challenge 2: Multi-Rate-Limit Overwrite / Stale Timer Overwrite
// =========================================================================

// TestChallenge_RateLimitOverwrite_StaleTimerDoesNotRecoverPrematurely tests that
// if an account is marked rate-limited for 40ms, and then updated to 1 hour,
// the original 40ms timer DOES NOT prematurely reset the account to ready.
func TestChallenge_RateLimitOverwrite_StaleTimerDoesNotRecoverPrematurely(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupChallengeDB(t, "p-overwrite")
	defer cleanup()

	acc := createTestAccount("acc-ow-1", "p-overwrite", providers.AccountStatusReady)
	if err := repo.SaveAccount(ctx, acc); err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	mgr := pool.NewManager(repo)
	defer mgr.Close()
	_, _ = mgr.LoadFromRepository(ctx)

	// 1. Mark rate-limited for 40ms
	err := mgr.MarkAccountRateLimited(ctx, "acc-ow-1", 40*time.Millisecond)
	if err != nil {
		t.Fatalf("first MarkAccountRateLimited failed: %v", err)
	}

	// 2. At 15ms, update rate-limit to 2 hours
	time.Sleep(15 * time.Millisecond)
	err = mgr.MarkAccountRateLimited(ctx, "acc-ow-1", 2*time.Hour)
	if err != nil {
		t.Fatalf("second MarkAccountRateLimited failed: %v", err)
	}

	// 3. Sleep 70ms (exceeding original 40ms timer)
	time.Sleep(70 * time.Millisecond)

	// In-memory status check
	stats, err := mgr.GetPoolStats(ctx, "p-overwrite")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.Ready != 0 || stats.RateLimited != 1 {
		t.Errorf("BUG FOUND: Account prematurely recovered to Ready! Stats: %+v (expected RateLimited: 1, Ready: 0)", stats)
	}

	// SQLite status check
	dbAcc, err := repo.GetAccount(ctx, "acc-ow-1")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Status != providers.AccountStatusRateLimited {
		t.Errorf("BUG FOUND: SQLite status was prematurely reset to '%s' (expected 'rate_limited')", dbAcc.Status)
	}
	if dbAcc.PausedUntil == nil || time.Until(*dbAcc.PausedUntil) < 1*time.Hour {
		t.Errorf("BUG FOUND: SQLite PausedUntil was overwritten or cleared: %v", dbAcc.PausedUntil)
	}

	// Attempting to lease should still fail with ErrPoolExhausted
	_, _, err = mgr.LeaseAccount(ctx, "p-overwrite")
	if !errors.Is(err, pool.ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
}

// =========================================================================
// Challenge 3: Immediate Eviction Under Active Load & Contention
// =========================================================================

// TestChallenge_Eviction_SignOutCancelsActiveTimerAndPersists tests that
// marking an account signed-out while in rate-limited cooldown cancels timers,
// purges from active rotation, and syncs to SQLite.
func TestChallenge_Eviction_SignOutCancelsActiveTimerAndPersists(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupChallengeDB(t, "p-evict")
	defer cleanup()

	acc := createTestAccount("acc-evict-1", "p-evict", providers.AccountStatusReady)
	if err := repo.SaveAccount(ctx, acc); err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	mgr := pool.NewManager(repo)
	defer mgr.Close()
	_, _ = mgr.LoadFromRepository(ctx)

	// Rate-limit for 40ms
	if err := mgr.MarkAccountRateLimited(ctx, "acc-evict-1", 40*time.Millisecond); err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}

	// Immediately sign out
	if err := mgr.MarkAccountSignedOut(ctx, "acc-evict-1"); err != nil {
		t.Fatalf("MarkAccountSignedOut failed: %v", err)
	}

	// Verify SQLite has signed_out and nil paused_until
	dbAcc, err := repo.GetAccount(ctx, "acc-evict-1")
	if err != nil {
		t.Fatalf("repo.GetAccount failed: %v", err)
	}
	if dbAcc.Status != providers.AccountStatusSignedOut {
		t.Fatalf("expected SQLite status 'signed_out', got '%s'", dbAcc.Status)
	}
	if dbAcc.PausedUntil != nil {
		t.Fatalf("expected SQLite paused_until to be nil, got %v", dbAcc.PausedUntil)
	}

	// Sleep 70ms to allow original cooldown timer to fire if not canceled
	time.Sleep(70 * time.Millisecond)

	// Verify it did NOT resurrect
	stats, _ := mgr.GetPoolStats(ctx, "p-evict")
	if stats.SignedOut != 1 || stats.Ready != 0 || stats.RateLimited != 0 {
		t.Errorf("BUG FOUND: Account resurrected after sign-out! Stats: %+v", stats)
	}

	dbAcc, _ = repo.GetAccount(ctx, "acc-evict-1")
	if dbAcc.Status != providers.AccountStatusSignedOut {
		t.Errorf("BUG FOUND: SQLite status resurrected to '%s'", dbAcc.Status)
	}

	// Lease should fail with ErrNoAccountsAvailable
	_, _, err = mgr.LeaseAccount(ctx, "p-evict")
	if !errors.Is(err, pool.ErrNoAccountsAvailable) {
		t.Errorf("expected ErrNoAccountsAvailable, got: %v", err)
	}
}

// =========================================================================
// Challenge 4: Cold-Start Hydration Matrix & Boundary Recovery
// =========================================================================

// TestChallenge_ColdStartHydration_MixedStateMatrix tests cold-start hydration
// across 7 diverse account states including future/past/corrupted rate-limits.
func TestChallenge_ColdStartHydration_MixedStateMatrix(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupChallengeDB(t, "p-matrix")
	defer cleanup()

	now := time.Now().UTC()
	past := now.Add(-30 * time.Minute)
	futureShort := now.Add(50 * time.Millisecond)
	futureLong := now.Add(2 * time.Hour)

	// Seed 7 accounts with distinct status profiles
	accounts := []*providers.Account{
		// 1. Ready account
		createTestAccount("m-ready", "p-matrix", providers.AccountStatusReady),
		// 2. Expired rate-limited -> should heal to ready
		func() *providers.Account {
			a := createTestAccount("m-rl-past", "p-matrix", providers.AccountStatusRateLimited)
			a.PausedUntil = &past
			return a
		}(),
		// 3. Corrupted rate-limited (nil PausedUntil) -> should heal to ready
		createTestAccount("m-rl-corrupted", "p-matrix", providers.AccountStatusRateLimited),
		// 4. Future short rate-limited -> should stay rate-limited then auto-recover
		func() *providers.Account {
			a := createTestAccount("m-rl-short", "p-matrix", providers.AccountStatusRateLimited)
			a.PausedUntil = &futureShort
			return a
		}(),
		// 5. Future long rate-limited -> should stay rate-limited
		func() *providers.Account {
			a := createTestAccount("m-rl-long", "p-matrix", providers.AccountStatusRateLimited)
			a.PausedUntil = &futureLong
			return a
		}(),
		// 6. Signed out
		createTestAccount("m-signedout", "p-matrix", providers.AccountStatusSignedOut),
		// 7. Error / unverified
		createTestAccount("m-error", "p-matrix", providers.AccountStatusError),
	}

	for _, a := range accounts {
		if err := repo.SaveAccount(ctx, a); err != nil {
			t.Fatalf("SaveAccount %s failed: %v", a.ID, err)
		}
	}

	// Cold-start hydration
	mgr := pool.NewManager(repo)
	defer mgr.Close()

	loaded, err := mgr.LoadFromRepository(ctx)
	if err != nil {
		t.Fatalf("LoadFromRepository failed: %v", err)
	}
	if loaded != 7 {
		t.Fatalf("expected 7 accounts loaded, got %d", loaded)
	}

	// Snapshot check immediately after hydration
	stats, err := mgr.GetPoolStats(ctx, "p-matrix")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}

	// Expected partition:
	// Ready: 3 (m-ready + m-rl-past healed + m-rl-corrupted healed)
	// RateLimited: 2 (m-rl-short + m-rl-long)
	// SignedOut: 1 (m-signedout)
	// Error: 1 (m-error)
	// Total: 7
	if stats.Ready != 3 {
		t.Errorf("expected Ready: 3, got: %d", stats.Ready)
	}
	if stats.RateLimited != 2 {
		t.Errorf("expected RateLimited: 2, got: %d", stats.RateLimited)
	}
	if stats.SignedOut != 1 {
		t.Errorf("expected SignedOut: 1, got: %d", stats.SignedOut)
	}
	if stats.Error != 1 {
		t.Errorf("expected Error: 1, got: %d", stats.Error)
	}
	if stats.Total != 7 {
		t.Errorf("expected Total: 7, got: %d", stats.Total)
	}
	if !stats.InvariantValid() {
		t.Errorf("PoolStats invariant failed: %+v", stats)
	}

	// Check that m-rl-past and m-rl-corrupted were healed in SQLite as well
	dbPast, _ := repo.GetAccount(ctx, "m-rl-past")
	if dbPast.Status != providers.AccountStatusReady {
		t.Errorf("SQLite m-rl-past was not healed to ready: %s", dbPast.Status)
	}
	dbCorrupt, _ := repo.GetAccount(ctx, "m-rl-corrupted")
	if dbCorrupt.Status != providers.AccountStatusReady {
		t.Errorf("SQLite m-rl-corrupted was not healed to ready: %s", dbCorrupt.Status)
	}

	// Wait 80ms for m-rl-short to expire and auto-recover
	time.Sleep(80 * time.Millisecond)

	statsAfter, _ := mgr.GetPoolStats(ctx, "p-matrix")
	if statsAfter.Ready != 4 || statsAfter.RateLimited != 1 {
		t.Errorf("after short cooldown: expected Ready: 4, RateLimited: 1; got %+v", statsAfter)
	}
	if !statsAfter.InvariantValid() {
		t.Errorf("PoolStats invariant failed after auto-recovery: %+v", statsAfter)
	}

	// Check SQLite for m-rl-short
	dbShort, _ := repo.GetAccount(ctx, "m-rl-short")
	if dbShort.Status != providers.AccountStatusReady {
		t.Errorf("SQLite m-rl-short was not auto-recovered to ready: %s", dbShort.Status)
	}
}

// =========================================================================
// Challenge 5: High Churn State Transitions & Invariant Stress Harness
// =========================================================================

// TestChallenge_HighChurn_InvariantAndConcurrencyHarness bombards the pool with
// 30 concurrent workers performing rapid leases, releases, rate-limiting, and
// status checks to stress-test invariants and mutual exclusion.
func TestChallenge_HighChurn_InvariantAndConcurrencyHarness(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupChallengeDB(t, "p-churn")
	defer cleanup()

	// Seed 8 accounts
	const numAccounts = 8
	for i := 0; i < numAccounts; i++ {
		id := "acc-churn-" + string(rune('A'+i))
		acc := createTestAccount(id, "p-churn", providers.AccountStatusReady)
		if err := repo.SaveAccount(ctx, acc); err != nil {
			t.Fatalf("SaveAccount failed: %v", err)
		}
	}

	mgr := pool.NewManager(repo, pool.WithMaxInFlight(1))
	defer mgr.Close()
	_, err := mgr.LoadFromRepository(ctx)
	if err != nil {
		t.Fatalf("LoadFromRepository failed: %v", err)
	}

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Track active leases per account to verify mutual exclusion
	activeLeases := sync.Map{} // accountID -> *atomic.Int32
	for i := 0; i < numAccounts; i++ {
		id := "acc-churn-" + string(rune('A'+i))
		activeLeases.Store(id, new(atomic.Int32))
	}

	var (
		totalLeases       atomic.Uint64
		invariantFailures atomic.Uint64
		doubleLeaseCount  atomic.Uint64
	)

	// 1. Worker goroutines leasing, releasing, and occasionally rate-limiting
	const numWorkers = 25
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}

				acc, release, err := mgr.LeaseAccount(ctx, "p-churn")
				if err != nil {
					// Pool might be busy or exhausted; back off briefly
					time.Sleep(1 * time.Millisecond)
					continue
				}

				totalLeases.Add(1)

				// Verify mutual exclusion
				val, _ := activeLeases.Load(acc.ID)
				cnt := val.(*atomic.Int32).Add(1)
				if cnt > 1 {
					doubleLeaseCount.Add(1)
				}

				// Simulate brief work
				time.Sleep(500 * time.Microsecond)

				// 10% chance to rate-limit this account
				if workerID%5 == 0 {
					_ = mgr.MarkAccountRateLimited(ctx, acc.ID, 10*time.Millisecond)
				}

				val.(*atomic.Int32).Add(-1)
				release()

				// Idempotency check: release again should not panic or double-decrement
				release()
			}
		}(w)
	}

	// 2. Observer goroutine continuously asserting PoolStats invariant
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
			}

			stats, err := mgr.GetPoolStats(ctx, "p-churn")
			if err == nil {
				if !stats.InvariantValid() {
					invariantFailures.Add(1)
				}
				if stats.Total != numAccounts {
					invariantFailures.Add(1)
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// Run stress harness for 1.2 seconds
	time.Sleep(1200 * time.Millisecond)
	close(stopCh)
	wg.Wait()

	t.Logf("High Churn Test Finished: total leases=%d, double leases=%d, invariant failures=%d",
		totalLeases.Load(), doubleLeaseCount.Load(), invariantFailures.Load())

	if invariantFailures.Load() > 0 {
		t.Errorf("PoolStats invariant was violated %d times during churn!", invariantFailures.Load())
	}
	if doubleLeaseCount.Load() > 0 {
		t.Errorf("CONCURRENCY FAILURE: Detected %d instances of double leasing (more than 1 active lease on same account)!", doubleLeaseCount.Load())
	}

	// Final verification after settling: sleep for cooldowns to finish
	time.Sleep(50 * time.Millisecond)
	finalStats, err := mgr.GetPoolStats(ctx, "p-churn")
	if err != nil {
		t.Fatalf("final GetPoolStats failed: %v", err)
	}
	if !finalStats.InvariantValid() {
		t.Errorf("final PoolStats invariant failed: %+v", finalStats)
	}
	if finalStats.Total != numAccounts {
		t.Errorf("expected total %d, got %d", numAccounts, finalStats.Total)
	}
}
