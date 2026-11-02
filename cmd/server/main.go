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
	"dezuxk/internal/pool"
	"dezuxk/internal/provider/manifest"
	"dezuxk/internal/provider/registry"
	"dezuxk/internal/proxy"
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

	// Context for initialization and shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Resolve master encryption key dynamically (Zero-Hardcode)
	keyResolver := config.NewKeyResolver()
	keyRes, err := keyResolver.Resolve(ctx, cfg, cfg.ResolverFlags)
	if err != nil {
		logger.Error("failed to resolve master encryption key", "error", err)
		os.Exit(1)
	}
	cfg.Security.MasterKey = keyRes.Key

	logger.Info("master key resolved", "source", keyRes.Source)
	if keyRes.FilePath != "" {
		logger.Info("master key file loaded", "path", keyRes.FilePath)
	}

	// 3. Initialize persistence storage
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

	// 4. Initialize security cipher
	cipher, err := security.NewAdaptiveCipher(cfg.Security.MasterKey, cfg.Security.UseDPAPI)
	if err != nil {
		logger.Error("failed to initialize security cipher", "error", err)
		os.Exit(1)
	}

	// 5. Initialize repository
	repo := sqlite.NewProvidersRepository(db, cipher)

	// 6. Initialize Dynamic Provider Manifest Loader & Registry
	validator := manifest.NewValidator()
	loader := manifest.NewLoader(validator)
	reg := registry.NewRegistry(repo)

	// Hydrate registry with any existing providers in persistence
	if _, err := reg.LoadFromRepository(ctx); err != nil {
		logger.Warn("could not load existing providers from repository", "error", err)
	}

	// Resolve manifests directory from configuration
	manifestsDir := cfg.GetManifestsDir()

	// Load manifests from filesystem directory, falling back to embedded manifests
	provs, err := loader.LoadDirectory(manifestsDir)
	if err != nil || len(provs) == 0 {
		logger.Info("no filesystem manifests found; loading embedded fallback manifests",
			"dir", manifestsDir,
			"err", err,
		)
		provs, err = loader.LoadEmbedded()
		if err != nil {
			logger.Error("failed to load embedded provider manifests", "error", err)
			os.Exit(1)
		}
	}

	// Register loaded manifests into registry and synchronize to persistence
	if err := reg.RegisterBatch(ctx, provs); err != nil {
		logger.Error("failed to register provider manifests into registry", "error", err)
		os.Exit(1)
	}

	logger.Info("dynamic provider manifests loaded and registered", "count", len(provs))
	for _, p := range provs {
		logger.Info("active provider registered", "id", p.ID, "name", p.Name, "base_url", p.BaseURL)
	}

	// 6b. Initialize Account Pool Manager (Milestone 3)
	poolMgr := pool.NewManager(repo)
	if loaded, err := poolMgr.LoadFromRepository(ctx); err != nil {
		logger.Warn("could not load existing accounts into pool", "error", err)
	} else {
		logger.Info("account pool hydrated", "accounts_loaded", loaded)
	}
	defer poolMgr.Close()

	// 6c. Initialize Dynamic Reverse Proxy Engine (Milestone 4)
	proxyEngine := proxy.NewEngine(repo, reg, poolMgr)

	// 7. Build HTTP server with registry, pool, and proxy injected
	srv := server.NewServer(cfg, repo,
		server.WithRegistry(reg),
		server.WithPool(poolMgr),
		server.WithProxy(proxyEngine),
	)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 8. Graceful shutdown handling
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
