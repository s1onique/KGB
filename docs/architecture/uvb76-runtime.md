# UVB-76 Runtime Architecture

## Component Overview

UVB-76 is a Go-based control plane service that runs on trusted infrastructure to monitor tovarisch leaf daemons.

```
┌─────────────────────────────────────────────────────────────────────┐
│                           UVB-76                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │
│  │   Config     │  │   Server     │  │   Scraper    │               │
│  │   Loader    │  │   (HTTPS)    │  │   Client     │               │
│  └──────────────┘  └──────────────┘  └──────────────┘               │
│         │                 │                 │                        │
│         │                 │                 │                        │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                    State Manager (in-memory)                  │  │
│  │                 One snapshot per target (bounded)              │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS /status
                                    ▼
                    ┌───────────────────────────────┐
                    │         tovarisch             │
                    │         (Zig leaf)            │
                    └───────────────────────────────┘
```

## Package Structure

```
uvb76/
├── main.go              # Process boundary, signal handling
├── go.mod               # Go module definition
├── Makefile             # Build targets (linux/arm64)
├── config/
│   ├── config.go        # Config loading and validation
│   ├── config_test.go   # Config tests
│   ├── password.go      # Password hashing (sha256:salt:hex)
│   └── password_test.go # Password tests
├── auth/
│   ├── middleware.go    # Basic Auth HTTP middleware
│   └── middleware_test.go
├── server/
│   ├── server.go        # HTTPS server, routes, handlers
│   ├── admin.html       # Embedded admin UI
│   └── server_test.go   # Server tests
├── state/
│   ├── state.go        # Bounded in-memory state manager
│   └── state_test.go   # State tests
└── scraper/
    ├── scraper.go      # Periodic scrape client
    └── scraper_test.go # Scraper tests
```

## Runtime Flow

### Startup

1. Parse flags (`-config`, `-dev`)
2. Load and validate JSON config
3. Create State Manager (empty)
4. Create Scraper Client (not started)
5. Start Scraper (background goroutine)
6. Start HTTPS server (blocks)

### Request Flow

```
Client Request
      │
      ▼
┌─────────────────┐
│  Healthz Check  │ ── public, no auth
│  (GET /healthz) │
└─────────────────┘
      │
      ▼ (if protected endpoint)
┌─────────────────┐
│ Basic Auth MW  │ ── 401 if missing/bad credentials
└─────────────────┘
      │
      ▼
┌─────────────────┐
│  API Handler    │ ── reads from State Manager
│  (GET /targets) │
└─────────────────┘
```

### Scrape Flow

```
Scraper Timer (every N seconds)
      │
      ▼
┌─────────────────────────────────────────┐
│  For each enabled target:              │
│    go scrapeTarget(target)             │
└─────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────┐
│  HTTP GET {base_url}/status            │
│  Timeout: configured milliseconds       │
└─────────────────────────────────────────┘
      │
      ├─── success ──► Parse JSON, store snapshot
      │                              │
      │                              ▼
      │                    State Manager
      │                    (one per target)
      │
      └─── failure ──► Store error snapshot
                              │
                              ▼
                    State Manager
```

## State Bounding

UVB-76 maintains bounded state:

- **Snapshots map**: One entry per target ID
- **Update semantics**: Replace on update (not append)
- **No history**: Only latest snapshot stored
- **Max memory**: O(targets) where targets is the configured count

```go
type Manager struct {
    mu       sync.RWMutex
    snapshots map[string]*TargetSnapshot  // bounded by config
}
```

## Security Boundaries

### Fail-Closed Design

UVB-76 follows fail-closed security principles:

| Condition | Behavior |
|-----------|----------|
| Missing TLS cert/key | Exit with error (unless `-dev`) |
| Missing auth username | Exit with error |
| Missing password hash | Exit with error |
| Invalid password format | Exit with error |
| Missing auth header | Return 401 Unauthorized |
| Invalid credentials | Return 401 Unauthorized |
| Unknown endpoint | Return 404 Not Found |

### Development Mode

The `-dev` flag relaxes TLS requirements for local development:

```bash
./uvb76 -dev -config uvb76.json
```

**WARNING**: Dev mode should never be used in production.

## Cross-Compilation

Target: linux/arm64 (ARM v8)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 go build -o uvb76 .
```

Make target: `make build-linux-arm64`

## Runtime Dependencies

- Go 1.21+
- gorilla/mux (HTTP router)
- No external database
- No system dependencies (CGO-free)

## Memory Profile

Expected memory usage for typical deployment:

- 10 targets: ~5MB RSS
- 50 targets: ~10MB RSS
- 100 targets: ~20MB RSS

Memory is bounded by:
- One snapshot per target (in-memory map)
- 64KB response limit per scrape
- No accumulation of historical data
