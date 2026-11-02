package pool_test

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

func TestFairness_StrictRoundRobinOrder(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "fairness-rr"
	mgr.AddAccount(createTestAccount("acc-A", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-B", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-C", providerID, providers.AccountStatusReady))

	expectedOrder := []string{
		"acc-A", "acc-B", "acc-C",
		"acc-A", "acc-B", "acc-C",
		"acc-A", "acc-B", "acc-C",
	}

	for i, expected := range expectedOrder {
		acc, release, err := mgr.LeaseAccount(ctx, providerID)
		if err != nil {
			t.Fatalf("lease %d failed: %v", i, err)
		}
		if acc.ID != expected {
			t.Fatalf("lease %d: expected %s, got %s", i, expected, acc.ID)
		}
		release()
	}
}

func TestFairness_UniformConcurrentDistribution(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "fairness-dist"
	mgr.AddAccount(createTestAccount("acc-1", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-2", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-3", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-4", providerID, providers.AccountStatusReady))

	var countsMu sync.Mutex
	counts := make(map[string]int)

	numWorkers := 40
	leasesPerWorker := 10
	totalLeases := numWorkers * leasesPerWorker // 400

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < leasesPerWorker; i++ {
				for {
					acc, release, err := mgr.LeaseAccount(ctx, providerID)
					if err != nil {
						if errors.Is(err, pool.ErrPoolExhausted) {
							time.Sleep(time.Duration(1+rand.Intn(2)) * time.Millisecond)
							continue
						}
						t.Errorf("unexpected lease error: %v", err)
						return
					}

					countsMu.Lock()
					counts[acc.ID]++
					countsMu.Unlock()

					time.Sleep(time.Duration(rand.Intn(2)) * time.Millisecond)
					release()
					break
				}
			}
		}()
	}

	wg.Wait()

	totalCounted := 0
	for id, count := range counts {
		totalCounted += count
		// With 4 accounts and 400 leases, expected mean is 100.
		// Allow reasonable variance between 60 and 140 to prevent flaky runs under OS scheduler variance.
		if count < 60 || count > 140 {
			t.Errorf("Account %s received skewed lease count: %d (expected ~100)", id, count)
		}
	}

	if totalCounted != totalLeases {
		t.Fatalf("total counted leases %d != expected %d", totalCounted, totalLeases)
	}
}

func TestFairness_MidFlightAccountAddition(t *testing.T) {
	ctx := context.Background()
	mgr := pool.NewManager(nil)
	defer mgr.Close()

	providerID := "fairness-add"
	mgr.AddAccount(createTestAccount("acc-A", providerID, providers.AccountStatusReady))
	mgr.AddAccount(createTestAccount("acc-B", providerID, providers.AccountStatusReady))

	// Round 1: acc-A, acc-B
	a1, relA1, _ := mgr.LeaseAccount(ctx, providerID)
	if a1.ID != "acc-A" {
		t.Fatalf("expected acc-A, got %s", a1.ID)
	}
	relA1()

	b1, relB1, _ := mgr.LeaseAccount(ctx, providerID)
	if b1.ID != "acc-B" {
		t.Fatalf("expected acc-B, got %s", b1.ID)
	}
	relB1()

	// Mid-flight addition of acc-C
	mgr.AddAccount(createTestAccount("acc-C", providerID, providers.AccountStatusReady))

	// Next leases should cycle through all 3
	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		acc, release, err := mgr.LeaseAccount(ctx, providerID)
		if err != nil {
			t.Fatalf("lease %d failed: %v", i, err)
		}
		seen[acc.ID] = true
		release()
	}

	if !seen["acc-A"] || !seen["acc-B"] || !seen["acc-C"] {
		t.Fatalf("expected all 3 accounts to be visited after dynamic addition, seen: %+v", seen)
	}
}
