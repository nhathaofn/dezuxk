# Dezuxk — Standalone AI Reverse Gateway & Account Pool Orchestrator

Dezuxk is a high-performance, standalone Go headless server acting as a reverse gateway and account orchestrator for upstream AI generation providers.

## Key Features

- **Pure Go SQLite Persistence**: Uses `modernc.org/sqlite` with automatic embedded migrations (`001_initial_schema.sql`).
- **Unified Dual-Engine Cipher**: Supports Windows DPAPI hardware-bound encryption and cross-platform AES-256-GCM fallback with SHA-256 key derivation and automatic tag format detection.
- **Zero-Hardcode Configuration**: Layered configuration hierarchy (CLI flags > Environment variables > JSON config file > Defaults).
- **Fast HTTP Routing (Chi v5)**:
  - Observability and resilience middlewares: RequestID, RealIP, Logger, Recoverer.
  - Constant-time Bearer token authentication via `crypto/subtle`.
  - `GET /health`: JSON status reporting uptime, version, and account pool readiness.
  - `POST /v1/images/generations`: Account pool rotation and AI generation dispatch.

## Quick Start

### Build

```bash
go build -o server.exe ./cmd/server
```

### Run

```bash
# Using CLI flags
./server.exe --port=8080 --api-key="your-master-api-key"

# Or using environment variables
export DEZUXK_SERVER_PORT=8080
export DEZUXK_SERVER_API_KEY="your-master-api-key"
./server.exe
```

### Run Tests

```bash
go test -v ./...
```
