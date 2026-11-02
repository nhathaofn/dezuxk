package registry_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/registry"
	"dezuxk/internal/security"
	"dezuxk/internal/storage/sqlite"
)

func setupTestRepo(t *testing.T) (*sqlite.ProvidersRepository, *sqlite.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_registry.db")

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

func sampleProvider(id string, active bool) *providers.Provider {
	status := providers.ProviderStatusActive
	if !active {
		status = providers.ProviderStatusDisabled
	}
	return &providers.Provider{
		ID:             id,
		Name:           "Provider " + id,
		BaseURL:        "https://" + id + ".example.com",
		AuthDomain:     "." + id + ".example.com",
		AllowedDomains: []string{"." + id + ".example.com"},
		ProbeRules: providers.ProbeRules{
			IdentityExpressions: []string{"[data-user]"},
			DiscoveryKeywords:   []string{"ServiceDiscovery"},
			TokenExtractors: map[string]string{
				"token": "regex",
			},
		},
		SecurityRules: map[string]any{"anti_bot": true},
		Status:        status,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	reg := registry.NewRegistry(repo)

	p := sampleProvider("flow", true)
	if err := reg.Register(ctx, p); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 1. Get from memory
	got, ok := reg.Get("flow")
	if !ok || got == nil {
		t.Fatalf("expected provider 'flow' in registry, got ok=%v, got=%+v", ok, got)
	}
	if got.ID != "flow" || got.Name != p.Name {
		t.Errorf("got unexpected provider: %+v", got)
	}

	// 2. Has check
	if !reg.Has("flow") {
		t.Error("expected Has('flow') to be true")
	}
	if reg.Has("nonexistent") {
		t.Error("expected Has('nonexistent') to be false")
	}

	// 3. Verify SQLite persistence sync
	persisted, err := repo.GetProvider(ctx, "flow")
	if err != nil || persisted == nil {
		t.Fatalf("expected provider to be persisted to DB: err=%v, persisted=%+v", err, persisted)
	}
	if persisted.ID != "flow" {
		t.Errorf("persisted provider has wrong ID: %q", persisted.ID)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	reg := registry.NewRegistry(repo)

	p := sampleProvider("flow", true)
	_ = reg.Register(ctx, p)

	if !reg.Has("flow") {
		t.Fatal("expected provider to exist before unregister")
	}

	if err := reg.Unregister(ctx, "flow"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	if reg.Has("flow") {
		t.Error("expected provider to be removed from memory cache")
	}

	// Verify DB deletion
	persisted, err := repo.GetProvider(ctx, "flow")
	if err != nil {
		t.Fatalf("GetProvider returned error: %v", err)
	}
	if persisted != nil {
		t.Error("expected provider to be deleted from database")
	}
}

func TestRegistry_ListAndListActive(t *testing.T) {
	reg := registry.New(nil)
	ctx := context.Background()

	// Insert in non-alphabetical order
	_ = reg.Register(ctx, sampleProvider("zebra", true))
	_ = reg.Register(ctx, sampleProvider("alpha", false)) // disabled
	_ = reg.Register(ctx, sampleProvider("beta", true))

	// Test List() - all sorted by ID ASC
	all := reg.List()
	if len(all) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(all))
	}
	if all[0].ID != "alpha" || all[1].ID != "beta" || all[2].ID != "zebra" {
		t.Errorf("expected sorted [alpha, beta, zebra], got [%s, %s, %s]", all[0].ID, all[1].ID, all[2].ID)
	}

	// Test ListActive() - only active sorted by ID ASC
	active := reg.ListActive()
	if len(active) != 2 {
		t.Fatalf("expected 2 active providers, got %d", len(active))
	}
	if active[0].ID != "beta" || active[1].ID != "zebra" {
		t.Errorf("expected sorted active [beta, zebra], got [%s, %s]", active[0].ID, active[1].ID)
	}
}

func TestRegistry_DefaultProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("empty registry", func(t *testing.T) {
		reg := registry.New(nil)
		p, ok := reg.DefaultProvider()
		if ok || p != nil {
			t.Errorf("expected (nil, false), got (%+v, %v)", p, ok)
		}
	})

	t.Run("first active alphabetically", func(t *testing.T) {
		reg := registry.New(nil)
		_ = reg.Register(ctx, sampleProvider("zebra", true))
		_ = reg.Register(ctx, sampleProvider("apple", false)) // disabled
		_ = reg.Register(ctx, sampleProvider("banana", true))

		p, ok := reg.DefaultProvider()
		if !ok || p == nil {
			t.Fatalf("expected default provider, got ok=%v", ok)
		}
		if p.ID != "banana" {
			t.Errorf("expected first active 'banana', got %q", p.ID)
		}
	})

	t.Run("explicit configured default active", func(t *testing.T) {
		reg := registry.New(nil, registry.WithDefaultProvider("zebra"))
		_ = reg.Register(ctx, sampleProvider("apple", true))
		_ = reg.Register(ctx, sampleProvider("zebra", true))

		p, ok := reg.DefaultProvider()
		if !ok || p == nil {
			t.Fatalf("expected default provider, got ok=%v", ok)
		}
		if p.ID != "zebra" {
			t.Errorf("expected configured default 'zebra', got %q", p.ID)
		}
	})

	t.Run("configured default disabled falls back to active", func(t *testing.T) {
		reg := registry.New(nil, registry.WithDefaultProvider("zebra"))
		_ = reg.Register(ctx, sampleProvider("apple", true))
		_ = reg.Register(ctx, sampleProvider("zebra", false)) // disabled!

		p, ok := reg.DefaultProvider()
		if !ok || p == nil {
			t.Fatalf("expected fallback default provider, got ok=%v", ok)
		}
		if p.ID != "apple" {
			t.Errorf("expected fallback to first active 'apple', got %q", p.ID)
		}
	})

	t.Run("only disabled providers returns nil false", func(t *testing.T) {
		reg := registry.New(nil)
		_ = reg.Register(ctx, sampleProvider("apple", false))
		_ = reg.Register(ctx, sampleProvider("banana", false))

		p, ok := reg.DefaultProvider()
		if ok || p != nil {
			t.Errorf("expected (nil, false), got (%+v, %v)", p, ok)
		}
	})
}

func TestRegistry_RegisterBatch(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()
	reg := registry.NewRegistry(repo)

	batch := []*providers.Provider{
		sampleProvider("p1", true),
		sampleProvider("p2", true),
		sampleProvider("p3", false),
	}

	if err := reg.RegisterBatch(ctx, batch); err != nil {
		t.Fatalf("RegisterBatch failed: %v", err)
	}

	for _, p := range batch {
		if !reg.Has(p.ID) {
			t.Errorf("expected provider %q in memory", p.ID)
		}
		persisted, err := repo.GetProvider(ctx, p.ID)
		if err != nil || persisted == nil {
			t.Errorf("expected provider %q persisted in DB", p.ID)
		}
	}

	// Test validation failure in batch
	badBatch := []*providers.Provider{
		sampleProvider("valid", true),
		nil,
	}
	if err := reg.RegisterBatch(ctx, badBatch); err == nil {
		t.Fatal("expected error registering batch with nil item")
	}
}

func TestRegistry_LoadFromRepository(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer db.Close()

	ctx := context.Background()

	// Seed directly into SQLite repo
	p1 := sampleProvider("direct1", true)
	p2 := sampleProvider("direct2", true)
	if err := repo.SaveProvider(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveProvider(ctx, p2); err != nil {
		t.Fatal(err)
	}

	// Create fresh registry and hydrate
	reg := registry.NewRegistry(repo)
	if reg.Has("direct1") || reg.Has("direct2") {
		t.Fatal("new registry should be empty before LoadFromRepository")
	}

	loaded, err := reg.LoadFromRepository(ctx)
	if err != nil {
		t.Fatalf("LoadFromRepository failed: %v", err)
	}
	if loaded != 2 {
		t.Fatalf("expected 2 loaded providers, got %d", loaded)
	}

	if !reg.Has("direct1") || !reg.Has("direct2") {
		t.Error("expected loaded providers in registry cache")
	}
}

func TestRegistry_DefensiveCopy_Immutability(t *testing.T) {
	reg := registry.New(nil)
	ctx := context.Background()

	orig := sampleProvider("test-defensive", true)
	orig.AllowedDomains = []string{"original.com"}
	orig.ProbeRules.IdentityExpressions = []string{"orig-expr"}
	orig.ProbeRules.TokenExtractors = map[string]string{"orig-key": "orig-val"}
	orig.SecurityRules = map[string]any{"orig-sec": true}

	_ = reg.Register(ctx, orig)

	// Caller retrieves and mutates returned object
	got, ok := reg.Get("test-defensive")
	if !ok {
		t.Fatal("provider not found")
	}

	got.AllowedDomains[0] = "MUTATED.com"
	got.ProbeRules.IdentityExpressions[0] = "MUTATED-expr"
	got.ProbeRules.TokenExtractors["orig-key"] = "MUTATED-val"
	got.SecurityRules["orig-sec"] = false

	// Retrieve again from registry and verify immutability
	fresh, ok := reg.Get("test-defensive")
	if !ok {
		t.Fatal("provider not found")
	}

	if fresh.AllowedDomains[0] != "original.com" {
		t.Errorf("cache was corrupted! AllowedDomains=%v", fresh.AllowedDomains)
	}
	if fresh.ProbeRules.IdentityExpressions[0] != "orig-expr" {
		t.Errorf("cache was corrupted! IdentityExpressions=%v", fresh.ProbeRules.IdentityExpressions)
	}
	if fresh.ProbeRules.TokenExtractors["orig-key"] != "orig-val" {
		t.Errorf("cache was corrupted! TokenExtractors=%v", fresh.ProbeRules.TokenExtractors)
	}
	if fresh.SecurityRules["orig-sec"] != true {
		t.Errorf("cache was corrupted! SecurityRules=%v", fresh.SecurityRules)
	}
}

type failingRepo struct {
	providers.Repository
}

func (f *failingRepo) SaveProvider(ctx context.Context, p *providers.Provider) error {
	return errors.New("simulated database disk error")
}

func TestRegistry_PersistenceFailure_AtomicRollback(t *testing.T) {
	ctx := context.Background()
	reg := registry.NewRegistry(&failingRepo{})

	p := sampleProvider("ghost-prov", true)
	err := reg.Register(ctx, p)
	if err == nil {
		t.Fatal("expected error from failing repository, got nil")
	}

	// Verify in-memory cache did NOT add the ghost provider
	if reg.Has("ghost-prov") {
		t.Fatal("ghost provider found in memory after persistence failure! Persistence-first violated.")
	}
}

func TestRegistry_ConcurrentStress(t *testing.T) {
	reg := registry.New(nil)
	ctx := context.Background()

	// Pre-seed some items
	for i := 0; i < 5; i++ {
		_ = reg.Register(ctx, sampleProvider(fmt.Sprintf("initial-%d", i), true))
	}

	var wg sync.WaitGroup
	workers := 40
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Mix reads and writes
				if workerID%4 == 0 {
					pID := fmt.Sprintf("dynamic-w%d-%d", workerID, i%5)
					_ = reg.Register(ctx, sampleProvider(pID, true))
				} else if workerID%4 == 1 {
					_, _ = reg.Get("initial-0")
					_ = reg.Has("initial-1")
				} else if workerID%4 == 2 {
					_ = reg.List()
					_ = reg.ListActive()
				} else {
					_, _ = reg.DefaultProvider()
				}
			}
		}()
	}

	wg.Wait()

	// Verify integrity after stress run
	if !reg.Has("initial-0") {
		t.Error("expected initial provider to survive concurrent operations")
	}
}
