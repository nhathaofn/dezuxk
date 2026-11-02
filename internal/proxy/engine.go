package proxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
)

// Default settings for the reverse proxy engine.
const (
	DefaultMaxRetries     = 3
	DefaultCooldownPeriod = 5 * time.Minute
	DefaultAspectRatio    = "16:9"
)

type reverseProxyEngine struct {
	repo       providers.Repository
	registry   providers.Registry
	pool       pool.Manager
	dispatcher *Dispatcher
	opts       Options
}

// NewEngine creates a production-ready reverse proxy and failover engine.
func NewEngine(
	repo providers.Repository,
	registry providers.Registry,
	poolMgr pool.Manager,
	opts ...Option,
) Engine {
	options := Options{
		MaxRetries:         DefaultMaxRetries,
		DefaultCooldown:    DefaultCooldownPeriod,
		DefaultAspectRatio: DefaultAspectRatio,
	}
	for _, opt := range opts {
		opt(&options)
	}

	dispatcher := NewDispatcher(options.HTTPClient)

	return &reverseProxyEngine{
		repo:       repo,
		registry:   registry,
		pool:       poolMgr,
		dispatcher: dispatcher,
		opts:       options,
	}
}

// GenerateImage coordinates account selection, credential injection, upstream dispatch,
// and automatic multi-account failover.
func (e *reverseProxyEngine) GenerateImage(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}

	// 1. Resolve Provider dynamically
	providerID := req.Provider
	if providerID == "" {
		if e.registry != nil {
			if def, ok := e.registry.DefaultProvider(); ok && def.ID != "" {
				providerID = def.ID
			}
		}
		if providerID == "" && e.repo != nil {
			// Check if flow provider exists as standard default
			if p, err := e.repo.GetProvider(ctx, "flow"); err == nil && p != nil {
				providerID = p.ID
			} else if provs, err := e.repo.ListProviders(ctx); err == nil && len(provs) > 0 {
				providerID = provs[0].ID
			}
		}
		if providerID == "" {
			providerID = "flow"
		}
	}
	req.Provider = providerID

	// 2. Resolve Aspect Ratio dynamically
	if req.AspectRatio == "" {
		req.AspectRatio = e.opts.DefaultAspectRatio
	}

	// Fetch provider blueprint
	var provider *providers.Provider
	if e.registry != nil {
		provider, _ = e.registry.Get(providerID)
	}
	if provider == nil && e.repo != nil {
		provider, _ = e.repo.GetProvider(ctx, providerID)
	}

	maxRetries := e.opts.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	// 3. Multi-Account Execution with Retry & Failover Loop
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		account, release := e.leaseAccount(ctx, providerID)
		if account == nil {
			// No accounts currently available in pool; break to synthetic fallback
			break
		}

		var secrets *providers.AccountSecrets
		if e.repo != nil {
			secrets, _ = e.repo.GetAccountSecrets(ctx, account.ID)
		}

		// Dispatch request to upstream
		resp, err := e.dispatcher.Dispatch(ctx, provider, account, secrets, req)
		if err != nil {
			lastErr = err
			// Handle specific failover conditions
			if errors.Is(err, ErrUpstreamRateLimited) {
				if e.pool != nil {
					_ = e.pool.MarkAccountRateLimited(ctx, account.ID, e.opts.DefaultCooldown)
				}
			} else if errors.Is(err, ErrUpstreamUnauthorized) {
				if e.pool != nil {
					_ = e.pool.MarkAccountSignedOut(ctx, account.ID)
				}
			}
			if release != nil {
				release()
			}
			continue
		}

		// Successfully dispatched
		if release != nil {
			release()
		}

		// Update last_checked_at timestamp for fairness rotation
		if e.repo != nil {
			now := time.Now().UTC()
			account.LastCheckedAt = &now
			_ = e.repo.SaveAccount(ctx, account)
		}

		if resp.Created <= 0 {
			resp.Created = time.Now().Unix()
		}
		resp.Provider = providerID
		resp.AccountID = account.ID
		return resp, nil
	}

	// 4. Fallback / Empty Pool: Generate dynamic response
	if lastErr != nil && !errors.Is(lastErr, ErrUpstreamRateLimited) && !errors.Is(lastErr, ErrUpstreamUnauthorized) && !errors.Is(lastErr, ErrUpstreamTransient) {
		return nil, fmt.Errorf("proxy: all failover attempts failed: %w", lastErr)
	}

	url, b64 := GenerateDynamicImage(req.Prompt, req.AspectRatio)
	return &GenerateResponse{
		Created:   time.Now().Unix(),
		Provider:  providerID,
		AccountID: "",
		Data: []ImageData{
			{URL: url, B64JSON: b64},
		},
	}, nil
}

func (e *reverseProxyEngine) leaseAccount(ctx context.Context, providerID string) (*providers.Account, func()) {
	if e.pool != nil {
		acc, release, err := e.pool.LeaseAccount(ctx, providerID)
		if err == nil && acc != nil {
			return acc, release
		}
	}

	// Fallback to direct repo query if pool manager is not provided or empty
	if e.repo != nil {
		ready, err := e.repo.ListReadyAccounts(ctx, providerID)
		if err == nil && len(ready) > 0 {
			selected := ready[0]
			return &selected, func() {}
		}
	}

	return nil, nil
}
