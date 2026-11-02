package registry

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
	"dezuxk/internal/storage/sqlite"
)

func createTestRepo(t *testing.T) (*sqlite.ProvidersRepository, *sqlite.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "adv_registry.db")

	ctx := context.Background()
	db, err := sqlite.Open(ctx, sqlite.Options{
		Path: dbPath,
	})
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}

	cipher, err := security.NewAdaptiveCipher("test-master-key-32-bytes-long!!", false)
	if err != nil {
		t.Fatalf("NewAdaptiveCipher failed: %v", err)
	}

	repo := sqlite.NewProvidersRepository(db, cipher)
	return repo, db
}

func newMockProvider(id string, active bool) *providers.Provider {
	status := providers.ProviderStatusActive
	if !active {
		status = providers.ProviderStatusDisabled
	}
	return &providers.Provider{
		ID:             id,
		Name:           "AdvProvider " + id,
		BaseURL:        "https://" + id + ".example.com",
		AuthDomain:     "." + id + ".example.com",
		AllowedDomains: []string{"." + id + ".example.com", "sub." + id + ".example.com"},
		ProbeRules: providers.ProbeRules{
			IdentityExpressions: []string{"[data-identity]", "meta[name='id']"},
			DiscoveryKeywords:   []string{"GenImages", "RPC"},
			TokenExtractors: map[string]string{
				"csrf": `csrf="([^"]+)"`,
			},
		},
		SecurityRules: map[string]any{
			"anti_bot": true,
			"score":    99.5,
		},
		Status:    status,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

// TestAdversarial_UltraHighConcurrency tests 100 concurrent workers pounding the registry
// with mixed reads, writes, batch updates, unregisters, default switches, and hydration.
func TestAdversarial_UltraHighConcurrency(t *testing.T) {
	repo, db := createTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	reg := NewRegistry(repo)

	// Pre-seed 20 providers
	for i := 0; i < 20; i++ {
		p := newMockProvider(fmt.Sprintf("seed-%02d", i), i%2 == 0)
		if err := reg.Register(ctx, p); err != nil {
			t.Fatalf("pre-seed failed: %v", err)
		}
	}

	const (
		numWorkers    = 80
		opsPerWorker  = 100
		totalOpsTarget = numWorkers * opsPerWorker
	)

	var (
		wg          sync.WaitGroup
		successOps  atomic.Int64
		readCount   atomic.Int64
		writeCount  atomic.Int64
		errCount    atomic.Int64
	)

	startSignal := make(chan struct{})

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		workerID := w

		go func(wid int) {
			defer wg.Done()
			<-startSignal

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(wid*1000)))

			for i := 0; i < opsPerWorker; i++ {
				op := rng.Intn(10)
				keyIdx := rng.Intn(30)
				pID := fmt.Sprintf("p-%02d", keyIdx)

				switch op {
				case 0, 1: // Read single
					p, ok := reg.Get(pID)
					if ok && p != nil {
						// Immediately attempt to mutate returned provider to stress immutability under concurrency
						p.AllowedDomains[0] = "mutated-concurrent.com"
					}
					readCount.Add(1)
					successOps.Add(1)

				case 2: // Has
					_ = reg.Has(pID)
					readCount.Add(1)
					successOps.Add(1)

				case 3: // List
					all := reg.List()
					for _, item := range all {
						if len(item.AllowedDomains) > 0 {
							item.AllowedDomains[0] = "corrupt.com"
						}
					}
					readCount.Add(1)
					successOps.Add(1)

				case 4: // ListActive
					active := reg.ListActive()
					for _, item := range active {
						if item.Status != providers.ProviderStatusActive {
							t.Errorf("ListActive returned inactive provider: %s", item.ID)
						}
					}
					readCount.Add(1)
					successOps.Add(1)

				case 5: // DefaultProvider
					_, _ = reg.DefaultProvider()
					readCount.Add(1)
					successOps.Add(1)

				case 6: // Register
					prov := newMockProvider(pID, rng.Intn(2) == 0)
					if err := reg.Register(ctx, prov); err != nil {
						errCount.Add(1)
					} else {
						writeCount.Add(1)
						successOps.Add(1)
					}

				case 7: // Batch Register
					batch := []*providers.Provider{
						newMockProvider(fmt.Sprintf("batch-%d-1", wid), true),
						newMockProvider(fmt.Sprintf("batch-%d-2", wid), false),
					}
					if err := reg.RegisterBatch(ctx, batch); err != nil {
						errCount.Add(1)
					} else {
						writeCount.Add(2)
						successOps.Add(1)
					}

				case 8: // SetDefault
					reg.SetDefault(pID)
					writeCount.Add(1)
					successOps.Add(1)

				case 9: // Unregister
					if err := reg.Unregister(ctx, pID); err != nil {
						errCount.Add(1)
					} else {
						writeCount.Add(1)
						successOps.Add(1)
					}
				}
			}
		}(workerID)
	}

	// Release all workers simultaneously
	close(startSignal)
	wg.Wait()

	t.Logf("Adversarial Concurrency Stress: Completed %d total operations (Reads=%d, Writes=%d, Errs=%d)",
		successOps.Load(), readCount.Load(), writeCount.Load(), errCount.Load())

	// Sanity checks on registry state after storm
	all := reg.List()
	if len(all) == 0 {
		t.Errorf("expected registry to contain surviving providers after stress test, got 0")
	}

	// Verify no cache corruption occurred during mutation attempts
	for _, p := range all {
		for _, d := range p.AllowedDomains {
			if d == "mutated-concurrent.com" || d == "corrupt.com" {
				t.Fatalf("Cache corruption detected! Provider %s contains leaked domain mutation %q", p.ID, d)
			}
		}
	}
}

// TestAdversarial_DefensiveCloningExhaustive tests deep cloning of all nested slices and maps
// to guarantee that no pointer or reference leaks outside the registry.
func TestAdversarial_DefensiveCloningExhaustive(t *testing.T) {
	reg := New(nil)
	ctx := context.Background()

	original := newMockProvider("clone-test", true)
	original.AllowedDomains = []string{"dom1.com", "dom2.com"}
	original.ProbeRules.IdentityExpressions = []string{"expr1", "expr2"}
	original.ProbeRules.DiscoveryKeywords = []string{"kw1", "kw2"}
	original.ProbeRules.TokenExtractors = map[string]string{"k1": "v1", "k2": "v2"}
	original.SecurityRules = map[string]any{"sec1": "val1", "count": 42}

	// 1. Pre-registration mutation test:
	// Register the provider, then immediately mutate the local original variable
	if err := reg.Register(ctx, original); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	original.AllowedDomains[0] = "MODIFIED_AFTER_REG"
	original.ProbeRules.IdentityExpressions[0] = "MODIFIED_AFTER_REG"
	original.ProbeRules.DiscoveryKeywords[0] = "MODIFIED_AFTER_REG"
	original.ProbeRules.TokenExtractors["k1"] = "MODIFIED_AFTER_REG"
	original.SecurityRules["sec1"] = "MODIFIED_AFTER_REG"

	// Fetch from registry: it should remain pristine!
	fetched1, ok := reg.Get("clone-test")
	if !ok || fetched1 == nil {
		t.Fatal("expected provider in registry")
	}
	if fetched1.AllowedDomains[0] == "MODIFIED_AFTER_REG" {
		t.Errorf("Pre-registration mutation leaked into registry AllowedDomains!")
	}
	if fetched1.ProbeRules.IdentityExpressions[0] == "MODIFIED_AFTER_REG" {
		t.Errorf("Pre-registration mutation leaked into registry IdentityExpressions!")
	}
	if fetched1.ProbeRules.DiscoveryKeywords[0] == "MODIFIED_AFTER_REG" {
		t.Errorf("Pre-registration mutation leaked into registry DiscoveryKeywords!")
	}
	if fetched1.ProbeRules.TokenExtractors["k1"] == "MODIFIED_AFTER_REG" {
		t.Errorf("Pre-registration mutation leaked into registry TokenExtractors!")
	}
	if fetched1.SecurityRules["sec1"] == "MODIFIED_AFTER_REG" {
		t.Errorf("Pre-registration mutation leaked into registry SecurityRules!")
	}

	// 2. Post-Get mutation test:
	fetched1.AllowedDomains[0] = "MODIFIED_ON_GET"
	fetched1.AllowedDomains = append(fetched1.AllowedDomains, "APPENDED_DOM")
	fetched1.ProbeRules.IdentityExpressions[0] = "MODIFIED_ON_GET"
	fetched1.ProbeRules.DiscoveryKeywords[0] = "MODIFIED_ON_GET"
	fetched1.ProbeRules.TokenExtractors["k1"] = "MODIFIED_ON_GET"
	fetched1.ProbeRules.TokenExtractors["new_k"] = "NEW_V"
	fetched1.SecurityRules["sec1"] = "MODIFIED_ON_GET"

	// Fetch again:
	fetched2, ok := reg.Get("clone-test")
	if !ok || fetched2 == nil {
		t.Fatal("expected provider in registry")
	}
	if fetched2.AllowedDomains[0] != "dom1.com" || len(fetched2.AllowedDomains) != 2 {
		t.Errorf("Post-Get mutation corrupted registry cache: AllowedDomains=%v", fetched2.AllowedDomains)
	}
	if fetched2.ProbeRules.IdentityExpressions[0] != "expr1" {
		t.Errorf("Post-Get mutation corrupted registry cache: IdentityExpressions=%v", fetched2.ProbeRules.IdentityExpressions)
	}
	if fetched2.ProbeRules.DiscoveryKeywords[0] != "kw1" {
		t.Errorf("Post-Get mutation corrupted registry cache: DiscoveryKeywords=%v", fetched2.ProbeRules.DiscoveryKeywords)
	}
	if fetched2.ProbeRules.TokenExtractors["k1"] != "v1" || len(fetched2.ProbeRules.TokenExtractors) != 2 {
		t.Errorf("Post-Get mutation corrupted registry cache: TokenExtractors=%v", fetched2.ProbeRules.TokenExtractors)
	}
	if fetched2.SecurityRules["sec1"] != "val1" {
		t.Errorf("Post-Get mutation corrupted registry cache: SecurityRules=%v", fetched2.SecurityRules)
	}

	// 3. DefaultProvider mutation test:
	def, ok := reg.DefaultProvider()
	if !ok || def == nil {
		t.Fatal("expected DefaultProvider")
	}
	def.AllowedDomains[0] = "MODIFIED_ON_DEFAULT"
	def.ProbeRules.TokenExtractors["k1"] = "MODIFIED_ON_DEFAULT"

	fetched3, _ := reg.Get("clone-test")
	if fetched3.AllowedDomains[0] == "MODIFIED_ON_DEFAULT" || fetched3.ProbeRules.TokenExtractors["k1"] == "MODIFIED_ON_DEFAULT" {
		t.Errorf("DefaultProvider mutation corrupted registry cache!")
	}

	// 4. List and ListActive mutation test:
	listItems := reg.List()
	for _, it := range listItems {
		it.AllowedDomains[0] = "MODIFIED_ON_LIST"
	}
	fetched4, _ := reg.Get("clone-test")
	if fetched4.AllowedDomains[0] == "MODIFIED_ON_LIST" {
		t.Errorf("List mutation corrupted registry cache!")
	}
}

// TestAdversarial_NilAndEmptyEdgeCases tests handling of nils, blanks, and boundary cases.
func TestAdversarial_NilAndEmptyEdgeCases(t *testing.T) {
	ctx := context.Background()
	reg := New(nil)

	// Register nil
	if err := reg.Register(ctx, nil); err == nil {
		t.Errorf("expected error registering nil provider")
	}

	// Register empty ID
	if err := reg.Register(ctx, &providers.Provider{ID: ""}); err == nil {
		t.Errorf("expected error registering provider with empty ID")
	}

	// RegisterBatch with nil list
	if err := reg.RegisterBatch(ctx, nil); err != nil {
		t.Errorf("RegisterBatch with nil list should succeed as no-op, got %v", err)
	}

	// RegisterBatch with empty list
	if err := reg.RegisterBatch(ctx, []*providers.Provider{}); err != nil {
		t.Errorf("RegisterBatch with empty list should succeed as no-op, got %v", err)
	}

	// RegisterBatch containing nil item among valid items
	batchWithNil := []*providers.Provider{
		newMockProvider("good-1", true),
		nil,
		newMockProvider("good-2", true),
	}
	if err := reg.RegisterBatch(ctx, batchWithNil); err == nil {
		t.Errorf("expected error registering batch with nil item")
	}
	// Verify atomic rollback / rejection: good-1 should NOT be in cache
	if reg.Has("good-1") || reg.Has("good-2") {
		t.Errorf("batch with invalid item partially committed to cache!")
	}

	// Unregister empty ID
	if err := reg.Unregister(ctx, ""); err == nil {
		t.Errorf("expected error unregistering empty ID")
	}

	// cloneProvider nil check
	if cloneProvider(nil) != nil {
		t.Errorf("cloneProvider(nil) should return nil")
	}
}

// TestAdversarial_RegisterBatch_DuplicateIDsInSameBatch tests behavior when batch contains duplicate IDs.
func TestAdversarial_RegisterBatch_DuplicateIDsInSameBatch(t *testing.T) {
	repo, db := createTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	reg := NewRegistry(repo)

	p1 := newMockProvider("dup-id", true)
	p1.Name = "Initial Duplicate"
	p2 := newMockProvider("dup-id", true)
	p2.Name = "Overwriting Duplicate"

	batch := []*providers.Provider{p1, p2}
	if err := reg.RegisterBatch(ctx, batch); err != nil {
		t.Fatalf("RegisterBatch failed: %v", err)
	}

	got, ok := reg.Get("dup-id")
	if !ok || got == nil {
		t.Fatal("expected provider in registry")
	}
	if got.Name != "Overwriting Duplicate" {
		t.Errorf("expected last provider to overwrite previous, got %q", got.Name)
	}
}
