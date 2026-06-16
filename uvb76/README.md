# UVB-76 Control Plane

UVB-76 is the Go-based control tower component of KGB that actively scrapes tovarisch leaf daemons.

## Features

- **Active Scraping**: Periodically polls tovarisch endpoints at configurable intervals
- **HTTPS + Basic Auth**: Secure admin interface with fail-closed design
- **Constrained Runtime**: Runs on linux/arm64 routers (CGO-free)
- **Bounded State**: In-memory only, no database, one snapshot per target
- **Minimal Admin UI**: Embedded HTML page for viewing targets and status

## Quick Start

```bash
# Build for current platform
make build

# Build for Linux ARM64 (ASUSWRT-Merlin)
make build-linux-arm64

# Run tests
make test

# Run (requires config file with TLS certs)
./uvb76 -config uvb76.json

# Run in dev mode (no TLS required)
./uvb76 -dev -config uvb76.json
```

## Configuration

Copy `uvb76.example.json` to `uvb76.json` and configure:

```json
{
  "listen": {
    "addr": ":8443",
    "tls_cert_file": "/opt/etc/uvb76/cert.pem",
    "tls_key_file": "/opt/etc/uvb76/key.pem"
  },
  "auth": {
    "username": "admin",
    "password_sha256": "sha256:GENERATED_HASH"
  },
  "scrape": {
    "interval_seconds": 30,
    "timeout_milliseconds": 5000
  },
  "targets": [
    {
      "id": "home-router",
      "name": "Home Router",
      "base_url": "https://192.168.1.1:9080",
      "enabled": true
    }
  ]
}
```

### Generate Password Hash

Before starting UVB-76, you must generate a password hash:

```bash
cd uvb76
go run ./tools/genhash
# Enter password when prompted
```

The output will be a hash in the format `sha256:<32-char-hex-salt>:<64-char-hex-hash>`.
Replace `sha256:GENERATED_HASH` in your config with this value.

**Important**: UVB-76 will refuse to start without a valid password hash.

## API Endpoints

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /api/v1/healthz` | No | Health check |
| `GET /api/v1/targets` | Yes | List configured targets |
| `GET /api/v1/targets/{id}/snapshot` | Yes | Latest snapshot for target |
| `GET /` | Yes | Admin HTML page |

## Generate Password Hash

```go
package main

import (
    "fmt"
    "github.com/s1onique/KGB/uvb76/config"
)

func main() {
    salt, _ := config.GenerateSalt()
    hash, _ := config.HashPassword("your-password", salt)
    fmt.Println(hash)
}
```

## Generate TLS Certificates

```bash
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes -subj "/CN=uvb76"
```

## Architecture

```
uvb76/
├── main.go          # Process boundary, signal handling
├── config/          # Config loading, validation, password hashing
├── auth/            # Basic Auth HTTP middleware
├── server/          # HTTPS server, routes, embedded admin UI
├── state/           # Bounded in-memory state manager
└── scraper/         # Periodic tovarisch scrape client
```

## Security

- **Fail-closed**: Refuses to start without TLS cert/key (unless `-dev`)
- **Fail-closed**: Refuses to start without valid auth configuration
- **Constant-time comparison**: Prevents timing attacks on password verification
- **HTTPS-only**: No plain HTTP in production

## Requirements

- Go 1.21+
- gorilla/mux (included via go.mod)

## See Also

- [docs/requirements/uvb76-business-requirements.md](../../docs/requirements/uvb76-business-requirements.md)
- [docs/architecture/uvb76-runtime.md](../../docs/architecture/uvb76-runtime.md)
- [docs/runbooks/uvb76-asuswrt-merlin.md](../../docs/runbooks/uvb76-asuswrt-merlin.md)
