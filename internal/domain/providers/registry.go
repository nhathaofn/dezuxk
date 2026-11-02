// Package providers defines domain models and contracts for dynamic upstream providers.
package providers

import (
	"context"
	"errors"
)

var (
	// ErrNilProvider is returned when attempting to register a nil provider pointer.
	ErrNilProvider = errors.New("providers: provider cannot be nil")

	// ErrEmptyProviderID is returned when attempting to register a provider with an empty ID.
	ErrEmptyProviderID = errors.New("providers: provider ID cannot be empty")

	// ErrProviderNotFound is returned when a requested provider is not found.
	ErrProviderNotFound = errors.New("providers: provider not found")
)

// Registry specifies operations for an in-memory, thread-safe dynamic provider registry.
type Registry interface {
	// Get retrieves a provider by ID from memory.
	// Returns (nil, false) if not found.
	Get(id string) (*Provider, bool)

	// List returns all registered providers from memory sorted deterministically by ID.
	List() []*Provider

	// ListActive returns all providers with Status == ProviderStatusActive,
	// sorted deterministically by ID.
	ListActive() []*Provider

	// Has returns true if a provider with the given ID exists in the registry.
	Has(id string) bool

	// Register validates, persists to storage, and caches the provider in memory.
	// Persistence to storage occurs before memory cache update to guarantee relational integrity.
	Register(ctx context.Context, p *Provider) error

	// RegisterBatch persists and registers multiple providers in a single operation.
	// All providers are validated and persisted before memory cache update.
	RegisterBatch(ctx context.Context, ps []*Provider) error

	// DefaultProvider returns the primary provider to use for generation requests.
	// If an explicit default ID is configured and active, it is returned.
	// Otherwise, it returns the first active provider in deterministic order.
	// Returns (nil, false) if no active providers are available.
	DefaultProvider() (*Provider, bool)
}
