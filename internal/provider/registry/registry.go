// Package registry provides a thread-safe in-memory provider cache backed by persistent repository storage.
package registry

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"dezuxk/internal/domain/providers"
)

// Ensure Registry satisfies providers.Registry at compile time.
var _ providers.Registry = (*Registry)(nil)

// Registry provides a concurrent-safe in-memory cache of providers backed by SQLite.
type Registry struct {
	mu        sync.RWMutex
	repo      providers.Repository
	items     map[string]*providers.Provider
	defaultID string
}

// Option configures the Registry instance.
type Option func(*Registry)

// WithDefaultProvider sets the preferred default provider ID.
func WithDefaultProvider(id string) Option {
	return func(r *Registry) {
		r.defaultID = id
	}
}

// WithInitialProviders pre-seeds the registry with an initial set of providers.
func WithInitialProviders(ps ...*providers.Provider) Option {
	return func(r *Registry) {
		for _, p := range ps {
			if p != nil && p.ID != "" {
				r.items[p.ID] = cloneProvider(p)
			}
		}
	}
}

// New creates a new thread-safe Registry instance.
// repo is optional (can be nil for pure in-memory operation, e.g. in tests).
func New(repo providers.Repository, opts ...Option) *Registry {
	r := &Registry{
		repo:  repo,
		items: make(map[string]*providers.Provider),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// NewRegistry is an alias for New to support both naming styles.
func NewRegistry(repo providers.Repository, opts ...Option) *Registry {
	return New(repo, opts...)
}

// LoadFromRepository hydrates the in-memory cache by reading all existing providers
// from the persistent database repository.
func (r *Registry) LoadFromRepository(ctx context.Context) (int, error) {
	if r.repo == nil {
		return 0, nil
	}

	dbProviders, err := r.repo.ListProviders(ctx)
	if err != nil {
		return 0, fmt.Errorf("registry: failed to load providers from repository: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	loaded := 0
	for i := range dbProviders {
		p := dbProviders[i]
		r.items[p.ID] = cloneProvider(&p)
		loaded++
	}

	return loaded, nil
}

// Get retrieves a provider by ID from the in-memory cache.
func (r *Registry) Get(id string) (*providers.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.items[id]
	if !ok {
		return nil, false
	}
	return cloneProvider(p), true
}

// Has checks if a provider exists in the in-memory cache.
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.items[id]
	return ok
}

// List returns all registered providers from memory, sorted deterministically by ID.
func (r *Registry) List() []*providers.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*providers.Provider, 0, len(r.items))
	for _, p := range r.items {
		result = append(result, cloneProvider(p))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListActive returns all active providers from memory, sorted deterministically by ID.
func (r *Registry) ListActive() []*providers.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*providers.Provider, 0, len(r.items))
	for _, p := range r.items {
		if p.Status == providers.ProviderStatusActive {
			result = append(result, cloneProvider(p))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// DefaultProvider returns the configured default provider, or the first active provider alphabetically.
func (r *Registry) DefaultProvider() (*providers.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. If explicit defaultID is set and active, return it
	if r.defaultID != "" {
		if p, ok := r.items[r.defaultID]; ok && p.Status == providers.ProviderStatusActive {
			return cloneProvider(p), true
		}
	}

	// 2. Otherwise find the first active provider sorted deterministically by ID
	var active []*providers.Provider
	for _, p := range r.items {
		if p.Status == providers.ProviderStatusActive {
			active = append(active, p)
		}
	}
	if len(active) == 0 {
		return nil, false
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].ID < active[j].ID
	})
	return cloneProvider(active[0]), true
}

// Register persists the provider to the backing repository and updates the memory cache.
// Persistence to storage occurs before memory cache update to guarantee relational integrity.
func (r *Registry) Register(ctx context.Context, p *providers.Provider) error {
	if p == nil {
		return providers.ErrNilProvider
	}
	if p.ID == "" {
		return providers.ErrEmptyProviderID
	}

	// Persist to database repository first
	if r.repo != nil {
		if err := r.repo.SaveProvider(ctx, p); err != nil {
			return fmt.Errorf("registry: failed to persist provider %s: %w", p.ID, err)
		}
	}

	// Update in-memory cache under write lock
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[p.ID] = cloneProvider(p)
	return nil
}

// RegisterBatch validates and persists multiple providers, then updates the cache.
func (r *Registry) RegisterBatch(ctx context.Context, ps []*providers.Provider) error {
	if len(ps) == 0 {
		return nil
	}

	// Step 1: Pre-validation of all items
	for i, p := range ps {
		if p == nil {
			return fmt.Errorf("%w at index %d", providers.ErrNilProvider, i)
		}
		if p.ID == "" {
			return fmt.Errorf("%w at index %d", providers.ErrEmptyProviderID, i)
		}
	}

	// Step 2: Persist all items to database repository
	if r.repo != nil {
		for _, p := range ps {
			if err := r.repo.SaveProvider(ctx, p); err != nil {
				return fmt.Errorf("registry: failed to persist provider %s in batch: %w", p.ID, err)
			}
		}
	}

	// Step 3: Batch update memory cache under single write lock
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range ps {
		r.items[p.ID] = cloneProvider(p)
	}

	return nil
}

// Unregister deletes a provider from the persistent repository and the memory cache.
func (r *Registry) Unregister(ctx context.Context, id string) error {
	if id == "" {
		return providers.ErrEmptyProviderID
	}

	if r.repo != nil {
		if err := r.repo.DeleteProvider(ctx, id); err != nil {
			return fmt.Errorf("registry: failed to delete provider %s: %w", id, err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.items, id)
	return nil
}

// SetDefault updates the default provider ID.
func (r *Registry) SetDefault(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultID = id
}

// cloneProvider creates a complete deep copy of a Provider struct and its nested slices and maps.
func cloneProvider(src *providers.Provider) *providers.Provider {
	if src == nil {
		return nil
	}
	dst := *src

	// Deep copy AllowedDomains slice
	if src.AllowedDomains != nil {
		dst.AllowedDomains = make([]string, len(src.AllowedDomains))
		copy(dst.AllowedDomains, src.AllowedDomains)
	}

	// Deep copy ProbeRules slices and maps
	dst.ProbeRules = src.ProbeRules
	if src.ProbeRules.IdentityExpressions != nil {
		dst.ProbeRules.IdentityExpressions = make([]string, len(src.ProbeRules.IdentityExpressions))
		copy(dst.ProbeRules.IdentityExpressions, src.ProbeRules.IdentityExpressions)
	}
	if src.ProbeRules.DiscoveryKeywords != nil {
		dst.ProbeRules.DiscoveryKeywords = make([]string, len(src.ProbeRules.DiscoveryKeywords))
		copy(dst.ProbeRules.DiscoveryKeywords, src.ProbeRules.DiscoveryKeywords)
	}
	if src.ProbeRules.TokenExtractors != nil {
		dst.ProbeRules.TokenExtractors = make(map[string]string, len(src.ProbeRules.TokenExtractors))
		for k, v := range src.ProbeRules.TokenExtractors {
			dst.ProbeRules.TokenExtractors[k] = v
		}
	}

	// Deep copy SecurityRules map
	if src.SecurityRules != nil {
		dst.SecurityRules = make(map[string]any, len(src.SecurityRules))
		for k, v := range src.SecurityRules {
			dst.SecurityRules[k] = v
		}
	}

	return &dst
}
