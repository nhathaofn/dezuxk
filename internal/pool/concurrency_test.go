package pool_test

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

func TestConcurrency_FiftyGoroutinesLeaseAndRelease(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "concurrency-prov"
	mgr.AddAccount(createTestAccount("acc-1", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-2", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-3", providerID, providers.AccountStatusReady))

	var inFlightLeases sync.Map // accountID -> goroutineID
	var activeCount atomic.Int32
	var totalSuccessfulLeases atomic.Int32
	var maxObservedInFlight atomic.Int32

	var wg sync.WaitGroup
	numGoroutines := 50
	leasesPerGoroutine := 10

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < leasesPerGoroutine; i++ {
				for {
					acc, release, err := mgr.LeaseAccount(ctx, providerID)
					if err != nil {
						if errors.Is(err, pool.ErrPoolExhausted) {
							// Contention backoff
							time.Sleep(time.Duration(1+rand.Intn(3)) * time.Millisecond)
							continue
						}
						t.Errorf("goroutine %d unexpected error: %v", gid, err)
						return
					}

					// Verify mutual exclusion: account MUST NOT already be leased
					if existing, loaded := inFlightLeases.LoadOrStore(acc.ID, gid); loaded {
						t.Errorf("FATAL: Account %s leased concurrently to goroutine %d while held by %v!", acc.ID, gid, existing)
					}

					current := activeCount.Add(1)
					for {
						prevMax := maxObservedInFlight.Load()
						if current <= prevMax || maxObservedInFlight.CompareAndSwap(prevMax, current) {
							break
						}
					}
					if current > 3 {
						t.Errorf("FATAL: Active leases count %d exceeded total accounts 3!", current)
					}

					// Simulate fast workload
					time.Sleep(time.Duration(1+rand.Intn(2)) * time.Millisecond)

					inFlightLeases.Delete(acc.ID)
					activeCount.Add(-1)
					release()
					totalSuccessfulLeases.Add(1)
					break
				}
			}
		}(g)
	}

	wg.Wait()

	expectedLeases := int32(numGoroutines * leasesPerGoroutine)
	if totalSuccessfulLeases.Load() != expectedLeases {
		t.Fatalf("expected %d total successful leases, got %d", expectedLeases, totalSuccessfulLeases.Load())
	}

	stats, err := mgr.GetPoolStats(ctx, providerID)
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if stats.Leased != 0 || stats.Ready != 3 {
		t.Fatalf("expected Leased: 0, Ready: 3 after all released; got Leased: %d, Ready: %d", stats.Leased, stats.Ready)
	}
	if !stats.InvariantValid() {
		t.Fatalf("PoolStats invariant violation: %+v", stats)
	}
}

func TestConcurrency_ReleaseCallbackIdempotencyUnderRace(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	mgr.AddAccount(createTestAccount("acc-race-release", "flow", providers.AccountStatusReady))

	// Acquire single lease
	acc, release, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("lease failed: %v", err)
	}
	if acc.ID != "acc-race-release" {
		t.Fatalf("expected acc-race-release, got %s", acc.ID)
	}

	stats, _ := mgr.GetPoolStats(ctx, "flow")
	if stats.Leased != 1 || stats.Ready != 0 {
		t.Fatalf("expected Leased: 1, Ready: 0; got %+v", stats)
	}

	// Hammer release() concurrently with 25 goroutines
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release()
		}()
	}
	wg.Wait()

	// Verify stats: exactly 1 ready, 0 leased (release executed only once!)
	stats, _ = mgr.GetPoolStats(ctx, "flow")
	if stats.Leased != 0 || stats.Ready != 1 {
		t.Fatalf("expected Leased: 0, Ready: 1; got %+v", stats)
	}

	// Assert account can be leased again without issue
	acc2, release2, err := mgr.LeaseAccount(ctx, "flow")
	if err != nil {
		t.Fatalf("subsequent lease failed: %v", err)
	}
	if acc2.ID != "acc-race-release" {
		t.Fatalf("expected acc-race-release, got %s", acc2.ID)
	}
	release2()
}

func TestConcurrency_SimultaneousLeaseAndStatusEviction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "eviction-test"
	for i := 1; i <= 6; i++ {
		id := "acc-" + string(rune('0'+i))
		mgr.AddAccount(createTestAccount(id, providerID, providers.AccountStatusReady))
	}

	var wg sync.WaitGroup
	stopSignal := make(chan struct{})

	// 20 worker goroutines continuously leasing and releasing
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopSignal:
					return
				case <-ctx.Done():
					return
				default:
					_, release, err := mgr.LeaseAccount(ctx, providerID)
					if err != nil {
						// ErrPoolExhausted or ErrNoAccountsAvailable are expected as accounts get evicted
						time.Sleep(1 * time.Millisecond)
						continue
					}
					time.Sleep(time.Duration(rand.Intn(2)) * time.Millisecond)
					release()
				}
			}
		}()
	}

	// Evictor goroutine marks accounts rate-limited and signed-out
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 6; i++ {
			time.Sleep(15 * time.Millisecond)
			id := "acc-" + string(rune('0'+i))
			if i%2 == 0 {
				_ = mgr.MarkAccountRateLimited(ctx, id, 100*time.Millisecond)
			} else {
				_ = mgr.MarkAccountSignedOut(ctx, id)
			}
		}
	}()

	time.Sleep(150 * time.Millisecond)
	close(stopSignal)
	wg.Wait()

	stats, err := mgr.GetPoolStats(ctx, providerID)
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}
	if !stats.InvariantValid() {
		t.Fatalf("PoolStats invariant violation: %+v", stats)
	}
}
