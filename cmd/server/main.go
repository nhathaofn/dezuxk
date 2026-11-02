// Package main provides the entry point for the standalone dezuxk Gateway service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
	"dezuxk/internal/server"
	"dezuxk/internal/storage/sqlite"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 1. Load configuration with zero-hardcode precedence
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("starting dezuxk gateway service",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"db", cfg.Storage.DBFile,
		"dpapi", cfg.Security.UseDPAPI,
	)

	// 2. Initialize persistence storage
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPath := cfg.Storage.DBFile
	if !filepath.IsAbs(dbPath) && cfg.Storage.DataDir != "." && cfg.Storage.DataDir != "" {
		dbPath = filepath.Join(cfg.Storage.DataDir, dbPath)
	}

	db, err := sqlite.Open(ctx, sqlite.Options{
		Path:   dbPath,
		Logger: logger,
	})
	if err != nil {
		logger.Error("failed to open sqlite database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 3. Initialize security cipher
	cipher, err := security.NewAdaptiveCipher(cfg.Security.MasterKey, cfg.Security.UseDPAPI)
	if err != nil {
		logger.Error("failed to initialize security cipher", "error", err)
		os.Exit(1)
	}

	// 4. Initialize repository
	repo := sqlite.NewProvidersRepository(db, cipher)

	// Seed default provider 'flow' if not already present
	seedDefaultProviders(ctx, repo, logger)

	// 5. Build HTTP server
	srv := server.NewServer(cfg, repo)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 6. Graceful shutdown handling
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("gateway server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-stopChan
	logger.Info("shutting down gateway server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced server shutdown", "error", err)
	}

	logger.Info("dezuxk gateway stopped")
}

func seedDefaultProviders(ctx context.Context, repo providers.Repository, logger *slog.Logger) {
	existing, err := repo.GetProvider(ctx, "flow")
	if err != nil || existing != nil {
		return
	}

	p := &providers.Provider{
		ID:             "flow",
		Name:           "Google Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com", "flow.google.com"},
		ProbeRules: providers.ProbeRules{
			IdentityExpressions: []string{`[data-email]`, `meta[name="user-email"]`},
			DiscoveryKeywords:   []string{"BatchGenerateImages", "FlowService"},
		},
		SecurityRules: map[string]any{"anti_bot": true},
		Status:        providers.ProviderStatusActive,
	}

	if err := repo.SaveProvider(ctx, p); err != nil {
		logger.Warn("could not seed default flow provider", "error", err)
	} else {
		logger.Info("seeded default flow provider manifest")
	}
}
