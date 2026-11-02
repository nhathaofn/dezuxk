package registry_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/registry"
)

// mockFailingRepo allows fine-grained failure injection at the N-th call.
type mockFailingRepo struct {
	providers.Repository
	saveCalls    int
	failAfter    int // fail when saveCalls > failAfter
	failWithErr  error
	savedInDb    map[string]*providers.Provider
	mu           sync.Mutex
}

func newMockFailingRepo(realRepo providers.Repository, failAfter int, err error) *mockFailingRepo {
	return &mockFailingRepo{
		Repository:  realRepo,
		failAfter:   failAfter,
		failWithErr: err,
		savedInDb:   make(map[string]*providers.Provider),
	}
}

func (m *mockFailingRepo) SaveProvider(ctx context.Context, p *providers.Provider) error {
	m.mu.Lock()
	m.saveCalls++
	callNum := m.saveCalls
	m.mu.Unlock()

	if callNum > m.failAfter {
		return m.failWithErr
	}

	if m.Repository != nil {
		if err := m.Repository.SaveProvider(ctx, p); err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.savedInDb[p.ID] = p
	m.mu.Unlock()
	return nil
}

// TestAdversarial_Registry_SingleRegisterPersistenceRollback verifies that when persistence
// fails, the in-memory cache undergoes atomic rollback (remains completely uncorrupted).
func TestAdversarial_Registry_SingleRegisterPersistenceRollback(t *testing.T) {
	ctx := context.Background()
	failErr := errors.New("simulated disk I/O error")
	mockRepo := newMockFailingRepo(nil, 0, failErr) // fails immediately
	reg := registry.NewRegistry(mockRepo)

	p := sampleProvider("test-fail", true)
	err := reg.Register(ctx, p)
	if err == nil {
		t.Fatal("expected persistence error from mock repo, got nil")
	}
	if !errors.Is(err, failErr) {
		t.Errorf("expected wrapped failErr, got: %v", err)
	}

	// In-memory cache MUST NOT have "test-fail"
	if reg.Has("test-fail") {
		t.Fatal("CRITICAL: in-memory cache contains provider after persistence failure! Cache was corrupted.")
	}
	if got, ok := reg.Get("test-fail"); ok || got != nil {
		t.Fatalf("CRITICAL: Get returned provider %+v, ok=%v after failed Register", got, ok)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("CRITICAL: List returned %d items, expected 0", len(reg.List()))
	}
}

// TestAdversarial_Registry_RegisterBatchPartialFailure analyzes batch behavior
// when the N-th provider in a batch fails SQLite persistence.
func TestAdversarial_Registry_RegisterBatchPartialFailure(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	failErr := errors.New("simulated disk quota exceeded")
	// Allow 1st save to succeed, fail on 2nd
	mockRepo := newMockFailingRepo(repo, 1, failErr)
	reg := registry.NewRegistry(mockRepo)

	batch := []*providers.Provider{
		sampleProvider("batch-p1", true),
		sampleProvider("batch-p2", true),
		sampleProvider("batch-p3", true),
	}

	err := reg.RegisterBatch(ctx, batch)
	if err == nil {
		t.Fatal("expected error on batch registration, got nil")
	}

	// 1. In-memory cache check: because Step 3 is after all persistence calls,
	// none of the batch providers should be in memory cache.
	for _, p := range batch {
		if reg.Has(p.ID) {
			t.Errorf("in-memory cache has provider %s despite batch failure", p.ID)
		}
	}

	// 2. Empirical observation of SQLite state:
	// Does SQLite have batch-p1 because it was saved before the error on batch-p2?
	p1InDB, err1 := repo.GetProvider(ctx, "batch-p1")
	if err1 != nil {
		t.Fatalf("GetProvider error: %v", err1)
	}
	t.Logf("Empirical observation: batch-p1 in SQLite DB: %v (present=%v)", p1InDB != nil, p1InDB != nil)

	p2InDB, _ := repo.GetProvider(ctx, "batch-p2")
	t.Logf("Empirical observation: batch-p2 in SQLite DB: %v", p2InDB != nil)

	// Note: RegisterBatch does not wrap multi-provider saves in an SQL transaction.
	// Therefore batch-p1 was persisted to SQLite while batch-p2 was not, and
	// in-memory cache has neither.
}

// TestAdversarial_Registry_LoadFromRepositoryHydration verifies full hydration
// from persistent storage under diverse states.
func TestAdversarial_Registry_LoadFromRepositoryHydration(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()

	// 1. Hydrate from empty DB
	reg := registry.NewRegistry(repo)
	loaded, err := reg.LoadFromRepository(ctx)
	if err != nil {
		t.Fatalf("LoadFromRepository on empty DB failed: %v", err)
	}
	if loaded != 0 {
		t.Errorf("expected 0 loaded from empty DB, got %d", loaded)
	}

	// 2. Populate DB with active and disabled providers
	seedList := []*providers.Provider{
		sampleProvider("p-alpha", true),
		sampleProvider("p-beta", false),
		sampleProvider("p-gamma", true),
	}
	for _, p := range seedList {
		if err := repo.SaveProvider(ctx, p); err != nil {
			t.Fatalf("failed to seed provider %s: %v", p.ID, err)
		}
	}

	// 3. Hydrate into fresh registry
	reg2 := registry.NewRegistry(repo)
	loaded2, err := reg2.LoadFromRepository(ctx)
	if err != nil {
		t.Fatalf("LoadFromRepository failed: %v", err)
	}
	if loaded2 != 3 {
		t.Fatalf("expected 3 loaded providers, got %d", loaded2)
	}

	// Verify all exist in cache
	if !reg2.Has("p-alpha") || !reg2.Has("p-beta") || !reg2.Has("p-gamma") {
		t.Error("expected all seeded providers in hydrated cache")
	}

	// Verify active filtering
	active := reg2.ListActive()
	if len(active) != 2 {
		t.Fatalf("expected 2 active providers, got %d", len(active))
	}
	if active[0].ID != "p-alpha" || active[1].ID != "p-gamma" {
		t.Errorf("unexpected active list: %+v", active)
	}
}

// TestAdversarial_Registry_UpdateExistingProviderIdempotency tests that registering
// an existing provider updates both memory cache and SQLite.
func TestAdversarial_Registry_UpdateExistingProviderIdempotency(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	reg := registry.NewRegistry(repo)

	p1 := sampleProvider("flow", true)
	p1.Name = "Original Flow Name"
	if err := reg.Register(ctx, p1); err != nil {
		t.Fatal(err)
	}

	// Verify initial
	got, _ := reg.Get("flow")
	if got.Name != "Original Flow Name" {
		t.Fatalf("expected initial name, got %q", got.Name)
	}

	// Update provider with same ID
	p1Updated := sampleProvider("flow", true)
	p1Updated.Name = "Updated Flow Name V2"
	if err := reg.Register(ctx, p1Updated); err != nil {
		t.Fatal(err)
	}

	// Verify memory cache updated
	gotUpdated, _ := reg.Get("flow")
	if gotUpdated.Name != "Updated Flow Name V2" {
		t.Fatalf("expected updated name in cache, got %q", gotUpdated.Name)
	}

	// Verify SQLite updated
	persisted, err := repo.GetProvider(ctx, "flow")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Name != "Updated Flow Name V2" {
		t.Fatalf("expected updated name in SQLite, got %q", persisted.Name)
	}

	// Verify total count is still 1 (no duplicate entries)
	if len(reg.List()) != 1 {
		t.Fatalf("expected 1 total provider in cache, got %d", len(reg.List()))
	}
}

// TestAdversarial_Registry_HighContentionStress executes 100 concurrent workers
// performing simultaneous reads, writes, default provider queries, and unregisters.
func TestAdversarial_Registry_HighContentionStress(t *testing.T) {
	reg := registry.New(nil)
	ctx := context.Background()

	// Pre-seed
	for i := 0; i < 10; i++ {
		_ = reg.Register(ctx, sampleProvider(fmt.Sprintf("initial-%02d", i), i%2 == 0))
	}

	var wg sync.WaitGroup
	workers := 50
	iterations := 100

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				op := (workerID + i) % 6
				switch op {
				case 0:
					pID := fmt.Sprintf("worker-%d-item-%d", workerID, i%5)
					_ = reg.Register(ctx, sampleProvider(pID, true))
				case 1:
					pID := fmt.Sprintf("worker-%d-item-%d", (workerID+1)%workers, i%5)
					_ = reg.Unregister(ctx, pID)
				case 2:
					_, _ = reg.Get("initial-00")
					_ = reg.Has("initial-01")
				case 3:
					_ = reg.List()
				case 4:
					_ = reg.ListActive()
				case 5:
					_, _ = reg.DefaultProvider()
				}
			}
		}()
	}

	wg.Wait()

	// Registry must still be fully responsive
	all := reg.List()
	if len(all) == 0 {
		t.Error("expected providers in registry after stress")
	}
}
