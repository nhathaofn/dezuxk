package pool_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

// ----------------------------------------------------------------------------
// Challenge 1: 100 Concurrent Goroutines Under Extreme Contention
// ----------------------------------------------------------------------------

func TestChallenge_100GoroutinesExtremeContention(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "prov-contention-100"
	numAccounts := 3
	for i := 1; i <= numAccounts; i++ {
		mgr.AddAccount(createTestAccount(fmt.Sprintf("acc-%d", i), providerID, providers.AccountStatusReady))
	}

	var inFlight sync.Map // accountID -> goroutineID
	var activeLeases atomic.Int32
	var maxActiveLeases atomic.Int32
	var doubleLeaseDetected atomic.Bool
	var totalSuccessful atomic.Int64

	numGoroutines := 100
	leasesPerGoroutine := 20
	targetLeases := int64(numGoroutines * leasesPerGoroutine) // 2000 leases

	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			<-startBarrier

			for i := 0; i < leasesPerGoroutine; i++ {
				for {
					acc, release, err := mgr.LeaseAccount(ctx, providerID)
					if err != nil {
						if errors.Is(err, pool.ErrPoolExhausted) {
							// Realistic backoff
							time.Sleep(time.Duration(200+rand.Intn(500)) * time.Microsecond)
							continue
						}
						t.Errorf("goroutine %d: unexpected lease error: %v", gid, err)
						return
					}

					// Verify mutual exclusion: NO other goroutine may hold acc.ID
					if existing, loaded := inFlight.LoadOrStore(acc.ID, gid); loaded {
						doubleLeaseDetected.Store(true)
						t.Errorf("CRITICAL DOUBLE LEASE: Goroutine %d leased %s while already held by %v", gid, acc.ID, existing)
					}

					cur := activeLeases.Add(1)
					for {
						m := maxActiveLeases.Load()
						if cur <= m || maxActiveLeases.CompareAndSwap(m, cur) {
							break
						}
					}

					if cur > int32(numAccounts) {
						t.Errorf("CRITICAL CAPACITY VIOLATION: Active leases %d exceeded pool capacity %d", cur, numAccounts)
					}

					// Simulate microsecond work
					time.Sleep(time.Duration(100+rand.Intn(200)) * time.Microsecond)

					inFlight.Delete(acc.ID)
					activeLeases.Add(-1)
					release()
					totalSuccessful.Add(1)
					break
				}
			}
		}(g)
	}

	close(startBarrier)
	wg.Wait()

	if doubleLeaseDetected.Load() {
		t.Fatalf("FAILED: Double leasing detected under 100 concurrent goroutines!")
	}
	if totalSuccessful.Load() != targetLeases {
		t.Fatalf("expected %d total leases, got %d", targetLeases, totalSuccessful.Load())
	}

	stats, err := mgr.GetPoolStats(ctx, providerID)
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.Leased != 0 || stats.Ready != numAccounts {
		t.Fatalf("Stats leak after completion: Leased=%d, Ready=%d (expected 0 and %d)", stats.Leased, stats.Ready, numAccounts)
	}
	if !stats.InvariantValid() {
		t.Fatalf("PoolStats invariant violation: %+v", stats)
	}
}

// ----------------------------------------------------------------------------
// Challenge 2: Release Callback Abuse (Hammering, Double Calling, Post-Eviction)
// ----------------------------------------------------------------------------

func TestChallenge_ReleaseCallbackAbuse(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "prov-release-abuse"
	mgr.AddAccount(createTestAccount("acc-abuse", providerID, providers.AccountStatusReady))

	// Sub-test 1: Hammer release with 100 concurrent goroutines
	t.Run("Concurrent100Hammer", func(t *testing.T) {
		acc, release, err := mgr.LeaseAccount(ctx, providerID)
		if err != nil {
			t.Fatalf("lease failed: %v", err)
		}
		if acc.ID != "acc-abuse" {
			t.Fatalf("unexpected account: %s", acc.ID)
		}

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				release()
			}()
		}
		wg.Wait()

		stats, _ := mgr.GetPoolStats(ctx, providerID)
		if stats.Leased != 0 || stats.Ready != 1 {
			t.Fatalf("after 100x release hammer: Leased=%d, Ready=%d (expected 0 and 1)", stats.Leased, stats.Ready)
		}
	})

	// Sub-test 2: Sequential multiple release calls
	t.Run("SequentialMultipleCalls", func(t *testing.T) {
		_, release, err := mgr.LeaseAccount(ctx, providerID)
		if err != nil {
			t.Fatalf("lease failed: %v", err)
		}
		for i := 0; i < 10; i++ {
			release()
		}
		stats, _ := mgr.GetPoolStats(ctx, providerID)
		if stats.Leased != 0 || stats.Ready != 1 {
			t.Fatalf("after 10x sequential release: Leased=%d, Ready=%d", stats.Leased, stats.Ready)
		}
	})

	// Sub-test 3: Calling release after status change to SignedOut
	t.Run("ReleaseAfterSignedOut", func(t *testing.T) {
		_, release, err := mgr.LeaseAccount(ctx, providerID)
		if err != nil {
			t.Fatalf("lease failed: %v", err)
		}

		err = mgr.MarkAccountSignedOut(ctx, "acc-abuse")
		if err != nil {
			t.Fatalf("MarkAccountSignedOut failed: %v", err)
		}

		// Now release the previously leased account
		release()

		stats, _ := mgr.GetPoolStats(ctx, providerID)
		if stats.SignedOut != 1 || stats.Leased != 0 || stats.Ready != 0 {
			t.Fatalf("expected SignedOut: 1, Leased: 0, Ready: 0; got %+v", stats)
		}
		if !stats.InvariantValid() {
			t.Fatalf("invariant broken: %+v", stats)
		}
	})
}

// ----------------------------------------------------------------------------
// Challenge 3: Memory Mutation Attack on Returned Account
// ----------------------------------------------------------------------------

func TestChallenge_MemoryMutationAttack(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "prov-mutation-attack"
	initialTime := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	initialPaused := initialTime.Add(1 * time.Hour)
	initialChecked := initialTime

	acc := &providers.Account{
		ID:            "acc-mutate",
		ProviderID:    providerID,
		Label:         "Original Label",
		Identity:      "Original Identity",
		Status:        providers.AccountStatusReady,
		ProfileDir:    "/original/profile",
		ProxyID:       "proxy-original",
		PausedUntil:   &initialPaused,
		LastCheckedAt: &initialChecked,
		CreatedAt:     initialTime,
		UpdatedAt:     initialTime,
		Metadata: map[string]any{
			"env":       "production",
			"max_reqs":  1000,
			"is_active": true,
		},
		Usage: []providers.QuotaUsage{
			{Feature: "txt2img", Remaining: 50, Total: 100},
			{Feature: "img2img", Remaining: 25, Total: 50},
		},
	}
	mgr.AddAccount(acc)

	leased, release, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("lease failed: %v", err)
	}

	// ATTACK: Malicious or careless consumer mutates EVERY field on returned account
	leased.ID = "CORRUPTED_ID"
	leased.ProviderID = "CORRUPTED_PROV"
	leased.Label = "CORRUPTED_LABEL"
	leased.Identity = "CORRUPTED_IDENTITY"
	leased.Status = providers.AccountStatusSignedOut
	leased.ProfileDir = "/corrupted/dir"
	leased.ProxyID = "CORRUPTED_PROXY"
	*leased.PausedUntil = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	*leased.LastCheckedAt = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)

	// Mutate slice
	leased.Usage[0].Feature = "CORRUPTED_FEATURE"
	leased.Usage[0].Remaining = -999999
	leased.Usage = append(leased.Usage, providers.QuotaUsage{Feature: "EVIL", Remaining: 666})

	// Mutate map
	leased.Metadata["env"] = "COMPROMISED"
	leased.Metadata["injected_attack_key"] = "ATTACK_PAYLOAD"
	delete(leased.Metadata, "max_reqs")

	release()

	// Verify internal account state via GetAccount
	stored, ok := mgr.GetAccount(providerID, "acc-mutate")
	if !ok {
		t.Fatalf("internal account 'acc-mutate' not found after mutation attack!")
	}

	if stored.ID != "acc-mutate" {
		t.Errorf("LEAK: stored.ID mutated: got %s", stored.ID)
	}
	if stored.Label != "Original Label" {
		t.Errorf("LEAK: stored.Label mutated: got %s", stored.Label)
	}
	if stored.Identity != "Original Identity" {
		t.Errorf("LEAK: stored.Identity mutated: got %s", stored.Identity)
	}
	if stored.Status != providers.AccountStatusReady {
		t.Errorf("LEAK: stored.Status mutated: got %s", stored.Status)
	}
	if stored.ProfileDir != "/original/profile" {
		t.Errorf("LEAK: stored.ProfileDir mutated: got %s", stored.ProfileDir)
	}
	if stored.ProxyID != "proxy-original" {
		t.Errorf("LEAK: stored.ProxyID mutated: got %s", stored.ProxyID)
	}
	if stored.PausedUntil == nil || *stored.PausedUntil != initialPaused {
		t.Errorf("LEAK: stored.PausedUntil mutated: got %v", stored.PausedUntil)
	}
	if stored.LastCheckedAt == nil || *stored.LastCheckedAt == time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("LEAK: stored.LastCheckedAt mutated by caller pointer!")
	}

	// Verify Usage slice isolation
	if len(stored.Usage) != 2 {
		t.Errorf("LEAK: stored.Usage length changed: got %d", len(stored.Usage))
	} else {
		if stored.Usage[0].Feature != "txt2img" || stored.Usage[0].Remaining != 50 {
			t.Errorf("LEAK: stored.Usage[0] mutated: %+v", stored.Usage[0])
		}
	}

	// Verify Metadata map isolation
	if stored.Metadata["env"] != "production" {
		t.Errorf("LEAK: stored.Metadata[env] mutated: got %v", stored.Metadata["env"])
	}
	if _, exists := stored.Metadata["injected_attack_key"]; exists {
		t.Errorf("LEAK: stored.Metadata has injected attack key!")
	}
	if stored.Metadata["max_reqs"] != 1000 {
		t.Errorf("LEAK: stored.Metadata[max_reqs] was deleted!")
	}
}

// ----------------------------------------------------------------------------
// Challenge 4: Waiter Queue FIFO Order and Direct Hand-Over
// ----------------------------------------------------------------------------

func TestChallenge_WaiterQueueFIFOOrdering(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "prov-fifo"
	mgr.AddAccount(createTestAccount("acc-solo", providerID, providers.AccountStatusReady))

	// Acquire the single account
	_, releaseInitial, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("initial lease failed: %v", err)
	}

	numWaiters := 5
	arrivalOrder := make([]int, 0, numWaiters)
	var arrivalMu sync.Mutex
	var wg sync.WaitGroup

	// Waiter coordination
	readyToWait := make(chan int)

	for w := 0; w < numWaiters; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			readyToWait <- wid

			_, release, err := mgr.LeaseAccountWait(ctx, providerID, 2*time.Second)
			if err != nil {
				t.Errorf("waiter %d failed: %v", wid, err)
				return
			}
			defer release()

			arrivalMu.Lock()
			arrivalOrder = append(arrivalOrder, wid)
			arrivalMu.Unlock()

			time.Sleep(10 * time.Millisecond)
		}(w)

		// Wait for this waiter to start waiting before launching next to ensure FIFO arrival order
		<-readyToWait
		time.Sleep(5 * time.Millisecond)
	}

	// Release initial lease to trigger first waiter
	releaseInitial()

	wg.Wait()

	arrivalMu.Lock()
	defer arrivalMu.Unlock()

	if len(arrivalOrder) != numWaiters {
		t.Fatalf("expected %d waiters to finish, got %d", numWaiters, len(arrivalOrder))
	}

	for i := 0; i < numWaiters; i++ {
		if arrivalOrder[i] != i {
			t.Fatalf("FIFO violation: waiter order was %v, expected [0, 1, 2, 3, 4]", arrivalOrder)
		}
	}
}

// ----------------------------------------------------------------------------
// Challenge 5: Waiter Queue Context Cancellation / Timeout Account Leak Attack
// ----------------------------------------------------------------------------

func TestChallenge_WaiterCancellationDoesNotLeakAccount(t *testing.T) {
	providerID := "prov-cancel-leak"

	// Run 50 iterations attempting to provoke a race between waiter cancel and release
	for iter := 0; iter < 50; iter++ {
		mgr := pool.NewManager(nil)

		mgr.AddAccount(createTestAccount("acc-cancel", providerID, providers.AccountStatusReady))

		// 1. Lease the account
		_, release1, err := mgr.LeaseAccount(context.Background(), providerID)
		if err != nil {
			t.Fatalf("iter %d: initial lease failed: %v", iter, err)
		}

		// 2. Queue a waiter with short context timeout
		ctxShort, cancelShort := context.WithTimeout(context.Background(), 5*time.Millisecond)

		waiterDone := make(chan error, 1)
		go func() {
			_, releaseW, err := mgr.LeaseAccountWait(ctxShort, providerID, 500*time.Millisecond)
			if err == nil {
				releaseW()
			}
			waiterDone <- err
		}()

		// 3. Sleep roughly the timeout duration, then release concurrently
		time.Sleep(5 * time.Millisecond)
		release1()
		cancelShort()

		<-waiterDone

		// 4. VERIFY: The account MUST NOT BE LEAKED!
		// Either the waiter got it and released it, or the initial release returned it to the pool.
		// In either case, the account MUST be available for leasing now!
		ctxVerify, cancelVerify := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, releaseVerify, err := mgr.LeaseAccount(ctxVerify, providerID)
		cancelVerify()
		if err != nil {
			stats, _ := mgr.GetPoolStats(context.Background(), providerID)
			mgr.Close()
			t.Fatalf("CRITICAL BUG FOUND (iter %d): Account LEAKED after waiter cancellation/timeout! err=%v, stats=%+v", iter, err, stats)
		}
		releaseVerify()
		mgr.Close()
	}
}

// ----------------------------------------------------------------------------
// Challenge 6: Cooldown Recovery While Lease In-Flight (Double Lease Risk)
// ----------------------------------------------------------------------------

func TestChallenge_CooldownExpiryWhileInFlightDoesNotDoubleLease(t *testing.T) {
	ctx := context.Background()
	clock := newMockClock(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	mgr := pool.NewManager(nil, pool.WithClock(clock))
	defer mgr.Close()

	providerID := "prov-cooldown-inflight"
	mgr.AddAccount(createTestAccount("acc-cd-inflight", providerID, providers.AccountStatusReady))

	// 1. Goroutine A leases the account
	leasedA, releaseA, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("lease A failed: %v", err)
	}

	// 2. Upstream returns 429 while Goroutine A is STILL using the account!
	// Pool marks the account rate limited for 1 hour.
	err = mgr.MarkAccountRateLimited(ctx, leasedA.ID, 1*time.Hour)
	if err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}

	// 3. Goroutine B attempts to wait for an account on this provider
	waiterReceived := make(chan string, 1)
	waiterErr := make(chan error, 1)
	go func() {
		accB, releaseB, err := mgr.LeaseAccountWait(ctx, providerID, 10*time.Second)
		if err != nil {
			waiterErr <- err
			return
		}
		defer releaseB()
		waiterReceived <- accB.ID
	}()

	// Small pause to let waiter get queued
	time.Sleep(10 * time.Millisecond)

	// 4. Cooldown timer fires BEFORE Goroutine A has released!
	// (Simulate 1 hour passing)
	clock.Advance(1*time.Hour + 1*time.Second)

	// CHECK: Did Goroutine B receive the account while Goroutine A is STILL HOLDING IT?
	select {
	case id := <-waiterReceived:
		t.Fatalf("CRITICAL DOUBLE LEASE BUG: Waiter acquired account %s via recoverAccount while Goroutine A is STILL HOLDING THE LEASE! (MaxInFlight violated)", id)
	case <-time.After(50 * time.Millisecond):
		// Expected: waiter should NOT receive the account until Goroutine A releases!
	}

	// 5. Now Goroutine A finally releases
	releaseA()

	// 6. NOW the waiter should receive it
	select {
	case id := <-waiterReceived:
		if id != "acc-cd-inflight" {
			t.Fatalf("expected waiter to receive acc-cd-inflight, got %s", id)
		}
	case err := <-waiterErr:
		t.Fatalf("waiter failed: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("waiter timed out waiting for release after cooldown")
	}
}

// ----------------------------------------------------------------------------
// Challenge 7: Removed Account Handover to Waiter (Resurrection Risk)
// ----------------------------------------------------------------------------

func TestChallenge_RemovedAccountNotHandedToWaiter(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "prov-remove-resurrect"
	mgr.AddAccount(createTestAccount("acc-to-remove", providerID, providers.AccountStatusReady))

	// 1. Goroutine A leases the account
	_, releaseA, err := mgr.LeaseAccount(ctx, providerID)
	if err != nil {
		t.Fatalf("lease A failed: %v", err)
	}

	// 2. Waiter B queues for provider
	waiterReceived := make(chan string, 1)
	go func() {
		accB, releaseB, err := mgr.LeaseAccountWait(ctx, providerID, 200*time.Millisecond)
		if err == nil {
			defer releaseB()
			waiterReceived <- accB.ID
		}
	}()

	time.Sleep(10 * time.Millisecond)

	// 3. Admin REMOVES the account from the pool while in-flight
	mgr.RemoveAccount(providerID, "acc-to-remove")

	// 4. Goroutine A finishes and releases
	releaseA()

	// 5. CHECK: Did Waiter B receive the removed/deleted account?
	select {
	case id := <-waiterReceived:
		t.Fatalf("CRITICAL BUG: Waiter received account %s which was REMOVED from the pool!", id)
	case <-time.After(100 * time.Millisecond):
		// Expected: Waiter should NOT receive a removed account!
	}
}
