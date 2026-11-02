package pool_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
	"dezuxk/internal/security"
	"dezuxk/internal/storage/sqlite"
)

func setupTestDB(t *testing.T) (providers.Repository, func()) {
	t.Helper()
	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_pool.db")

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

	// Register provider first for foreign key integrity
	prov := &providers.Provider{
		ID:        "sync-provider",
		Name:      "Sync Provider",
		BaseURL:   "https://api.example.com",
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

func TestPersistence_StatusChangeSyncToSQLite(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	acc := createTestAccount("acc-sync-1", "sync-provider", providers.AccountStatusReady)
	if err := repo.SaveAccount(ctx, acc); err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	mgr := pool.NewManager(repo)
	defer mgr.Close()

	loaded, err := mgr.LoadFromRepository(ctx)
	if err != nil || loaded != 1 {
		t.Fatalf("LoadFromRepository failed: loaded=%d, err=%v", loaded, err)
	}

	// 1. Mark Rate Limited
	err = mgr.MarkAccountRateLimited(ctx, "acc-sync-1", 15*time.Minute)
	if err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}

	dbAcc, err := repo.GetAccount(ctx, "acc-sync-1")
	if err != nil {
		t.Fatalf("repo.GetAccount failed: %v", err)
	}
	if dbAcc.Status != providers.AccountStatusRateLimited {
		t.Fatalf("expected SQLite status 'rate_limited', got '%s'", dbAcc.Status)
	}
	if dbAcc.PausedUntil == nil {
		t.Fatalf("expected SQLite paused_until to be non-nil")
	}

	// 2. Mark Signed Out
	err = mgr.MarkAccountSignedOut(ctx, "acc-sync-1")
	if err != nil {
		t.Fatalf("MarkAccountSignedOut failed: %v", err)
	}

	dbAcc, err = repo.GetAccount(ctx, "acc-sync-1")
	if err != nil {
		t.Fatalf("repo.GetAccount failed: %v", err)
	}
	if dbAcc.Status != providers.AccountStatusSignedOut {
		t.Fatalf("expected SQLite status 'signed_out', got '%s'", dbAcc.Status)
	}
	if dbAcc.PausedUntil != nil {
		t.Fatalf("expected SQLite paused_until to be nil after sign-out, got: %v", dbAcc.PausedUntil)
	}
}

func TestPersistence_RestartHydrationStateRecovery(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC()
	pastExpiry := now.Add(-1 * time.Hour)
	futureExpiry := now.Add(2 * time.Hour)

	// Seed 5 accounts with different states:
	// 2 ready
	acc1 := createTestAccount("acc-r1", "sync-provider", providers.AccountStatusReady)
	acc2 := createTestAccount("acc-r2", "sync-provider", providers.AccountStatusReady)
	// 1 expired rate-limited (should heal to ready)
	acc3 := createTestAccount("acc-rl-expired", "sync-provider", providers.AccountStatusRateLimited)
	acc3.PausedUntil = &pastExpiry
	// 1 future rate-limited (should stay rate-limited)
	acc4 := createTestAccount("acc-rl-future", "sync-provider", providers.AccountStatusRateLimited)
	acc4.PausedUntil = &futureExpiry
	// 1 signed-out
	acc5 := createTestAccount("acc-so", "sync-provider", providers.AccountStatusSignedOut)

	for _, a := range []*providers.Account{acc1, acc2, acc3, acc4, acc5} {
		if err := repo.SaveAccount(ctx, a); err != nil {
			t.Fatalf("SaveAccount %s failed: %v", a.ID, err)
		}
	}

	// Cold start simulation
	mgr := pool.NewManager(repo)
	defer mgr.Close()

	loaded, err := mgr.LoadFromRepository(ctx)
	if err != nil {
		t.Fatalf("LoadFromRepository failed: %v", err)
	}
	if loaded != 5 {
		t.Fatalf("expected 5 accounts loaded, got %d", loaded)
	}

	stats, err := mgr.GetPoolStats(ctx, "sync-provider")
	if err != nil {
		t.Fatalf("GetPoolStats failed: %v", err)
	}

	// 2 initially ready + 1 healed expired = 3 Ready!
	// 1 future rate-limited = 1 RateLimited
	// 1 signed-out = 1 SignedOut
	// Total = 5
	if stats.Ready != 3 || stats.RateLimited != 1 || stats.SignedOut != 1 || stats.Total != 5 {
		t.Fatalf("unexpected hydrated stats: %+v (expected Ready: 3, RateLimited: 1, SignedOut: 1, Total: 5)", stats)
	}
	if !stats.InvariantValid() {
		t.Fatalf("stats invariant violation: %+v", stats)
	}

	// Check SQLite: acc-rl-expired should have been updated to 'ready'
	dbAcc3, err := repo.GetAccount(ctx, "acc-rl-expired")
	if err != nil {
		t.Fatalf("GetAccount acc-rl-expired failed: %v", err)
	}
	if dbAcc3.Status != providers.AccountStatusReady {
		t.Fatalf("expected SQLite status for expired account to be healed to 'ready', got: %s", dbAcc3.Status)
	}
}

func TestPersistence_TimerRecoverySyncsToSQLite(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	acc := createTestAccount("acc-timer-db", "sync-provider", providers.AccountStatusReady)
	if err := repo.SaveAccount(ctx, acc); err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	mgr := pool.NewManager(repo)
	defer mgr.Close()

	_, _ = mgr.LoadFromRepository(ctx)

	// Rate limit for 40ms
	err := mgr.MarkAccountRateLimited(ctx, "acc-timer-db", 40*time.Millisecond)
	if err != nil {
		t.Fatalf("MarkAccountRateLimited failed: %v", err)
	}

	// Verify DB is rate-limited
	dbAcc, err := repo.GetAccount(ctx, "acc-timer-db")
	if err != nil || dbAcc.Status != providers.AccountStatusRateLimited {
		t.Fatalf("expected SQLite status rate_limited, got: %s", dbAcc.Status)
	}

	// Wait 80ms for timer to recover and trigger DB sync
	time.Sleep(80 * time.Millisecond)

	// Verify DB transitioned back to ready
	dbAcc, err = repo.GetAccount(ctx, "acc-timer-db")
	if err != nil {
		t.Fatalf("repo.GetAccount failed: %v", err)
	}
	if dbAcc.Status != providers.AccountStatusReady {
		t.Fatalf("expected SQLite status to auto-recover to 'ready', got: %s", dbAcc.Status)
	}
}
