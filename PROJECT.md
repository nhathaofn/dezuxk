# Project: dezuxk Architecture & Zero-Hardcode Modernization

## Architecture
- **Layered Architecture adhering to Single Responsibility Principle (SRP)**:
  - `cmd/server/`: Main application entry point for service bootstrapping, CLI flag parsing, and dependency injection only.
  - `internal/config/`: Configuration loading with dynamic `KeyResolver` (CLI flag -> ENV -> Config JSON -> Key file -> Persistent `<DataDir>/.master.key` generated via `crypto/rand`).
  - `internal/domain/providers/`: Pure domain interfaces and entities (`Provider`, `Account`, `ProbeRules`, `Registry`, `Repository`).
  - `internal/provider/manifest/`: Declarative manifest loader (file, directory, embedded FS) and schema validator.
  - `internal/provider/registry/`: Concurrent in-memory provider registry synchronized with SQLite persistence.
  - `internal/pool/`: Account pool manager and scheduler (thread-safe leasing, round-robin/fair rotation, health status tracking).
  - `internal/proxy/`: Dynamic reverse proxy engine, upstream dispatcher, cookie/token injector, transport router, and multi-account retry/failover loop.
  - `internal/server/`: HTTP presentation layer partitioned into DTOs (`internal/server/dto`), handlers (`internal/server/handlers`), middleware, and routing.
  - `internal/storage/sqlite/`: Relational persistence layer for providers, accounts, and encrypted credentials.
  - `internal/security/`: Cryptographic primitives (AES-GCM, DPAPI, secure token generation).

## Feature Inventory
Every feature from the Survey phase appears here with its assigned milestone:
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Dynamic Master Key Resolution | Eliminate hardcoded secret in config; dynamic resolution via CLI, ENV, file, or auto-generated `<DataDir>/.master.key` | M1 | survey (R3) |
| 2 | Dynamic Config Layering | Centralize flags, env vars, JSON config, manifests directory config without hardcoded secrets | M1 | survey (R3, R4) |
| 3 | Declarative Provider Manifest Schema | External JSON provider manifest (`manifests/flow.json`) and embedded fallback | M2 | survey (R1) |
| 4 | Manifest Loader & Validator | Directory scanning, JSON unmarshaling, embedded FS fallback, and strict field validation | M2 | survey (R1) |
| 5 | Dynamic Provider Registry | In-memory thread-safe registry with automatic SQLite synchronization | M2 | survey (R1) |
| 6 | Elimination of Hardcoded Seeding in Entrypoint | Remove `seedDefaultProviders` from `cmd/server/main.go`; wire dynamic manifest loader | M2 | survey (R1) |
| 7 | Account Pool Manager & Scheduler | Dedicated pool package for thread-safe leasing, fair rotation, and status transitions | M3 | survey (R4) |
| 8 | Upstream Credential Injection | Decrypt cookies and tokens from pool accounts and inject into upstream requests | M4 | survey (R2) |
| 9 | Reverse Proxy Dispatcher & Failover | Dynamic endpoint dispatching, upstream request execution, and retry/failover loop | M4 | survey (R2) |
| 10 | Elimination of Mock & Hardcoded Handler Logic | Remove mock PNG, mock URLs, static version string, and hardcoded provider/aspect-ratio fallbacks | M5 | survey (R2) |
| 11 | Handlers & DTO Modular Refactoring | Separate handlers and DTOs into dedicated packages following SRP | M5 | survey (R4) |
| 12 | Opaque-Box E2E Test Suite (Tiers 1-4) | Comprehensive requirement-driven test suite covering all features, boundaries, interactions, and scenarios | E2E Track / M6 | survey (E2E) |
| 13 | Adversarial Coverage Hardening (Tier 5) | White-box adversarial testing, coverage audit, and edge-case hardening | M6 | survey (Tier 5) |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Dynamic Master Key & Config Resolution | Eliminate hardcoded master key fallback; implement dynamic `KeyResolver` in `internal/config/key_resolver.go`; update config tests | none | DONE |
| M2 | Dynamic Provider Manifest Loader & Registry | Domain `Registry` contract; `internal/provider/manifest` (loader + validator); `internal/provider/registry`; `manifests/flow.json`; remove `seedDefaultProviders` from `main.go` | M1 | DONE |
| M3 | Account Pool Manager & Scheduler | Dedicated `internal/pool` package for thread-safe account leasing, rotation fairness, and status updates | M1 | DONE |
| M4 | Upstream Reverse Proxy & Failover Engine | Dedicated `internal/proxy` package for credential injection (cookies + tokens), dynamic endpoint dispatching, and multi-account retry/failover | M2, M3 | DONE |
| M5 | Server Handlers Refactoring & De-Mocking | Refactor `internal/server` (split DTOs, handlers), remove mock PNG / URLs / static fallbacks, delegate to `proxy.Engine`, wire up in `main.go` | M4 | DONE |
| M6 | Final Verification: 100% E2E Test Pass & Hardening | Phase 1: Pass 100% of Tiers 1-4 E2E tests. Phase 2: Adversarial coverage hardening (Tier 5) | M5, TEST_READY | DONE |

## Interface Contracts

### `config.KeyResolver`
- Input: `Config`, `DataDir`, CLI flags, ENV
- Output: `string` (resolved 32-byte master key or auto-generated hex key)
- Guarantees: Never returns a hardcoded secret fallback; creates `.master.key` in `DataDir` with restricted permissions if no key configured.

### `providers.Registry`
```go
type Registry interface {
    Get(id string) (*Provider, bool)
    List() []*Provider
    ListActive() []*Provider
    Has(id string) bool
    Register(ctx context.Context, p *Provider) error
    RegisterBatch(ctx context.Context, ps []*Provider) error
    DefaultProvider() (*Provider, bool)
}
```

### `pool.Manager`
```go
type Manager interface {
    LeaseAccount(ctx context.Context, providerID string) (*providers.Account, func(), error)
    MarkAccountRateLimited(ctx context.Context, accountID string, duration time.Duration) error
    MarkAccountSignedOut(ctx context.Context, accountID string) error
    GetPoolStats(ctx context.Context, providerID string) (PoolStats, error)
}
```

### `proxy.Engine`
```go
type Engine interface {
    GenerateImage(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}
```

## Code Layout
- `cmd/server/main.go`: Bootstrap and wiring only
- `manifests/flow.json`: External provider manifest
- `internal/config/`: Config, flags, and `key_resolver.go`
- `internal/domain/providers/`: Domain entities and interfaces
- `internal/provider/manifest/`: Manifest loader and validator
- `internal/provider/registry/`: In-memory registry with DB synchronization
- `internal/pool/`: Account pool scheduler and leasing manager
- `internal/proxy/`: Reverse proxy engine, upstream dispatching, credential injection, retry/failover
- `internal/server/`: HTTP router, middleware
- `internal/server/dto/`: Request/response DTO structures
- `internal/server/handlers/`: Focused HTTP handlers (health, images, etc.)
- `internal/storage/sqlite/`: SQLite database repository
- `internal/security/`: Adaptive cipher, AES-GCM, DPAPI, secure token generator
- `tests/e2e/`: E2E test harness, runner, and test suites (Tiers 1-4)
