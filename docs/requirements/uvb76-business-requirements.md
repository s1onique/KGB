# UVB-76 Business Requirements

This document outlines the business requirements driving the UVB-76 control plane implementation.

## Overview

UVB-76 is a lightweight anti-censorship control plane component that actively scrapes tovarisch leaf daemons. It runs on trusted infrastructure (e.g., home routers, small servers) and provides an admin interface for monitoring infrastructure health.

## Business Requirements

### BR-UVB-001: Active Tovarisch Scraping

**Description:** UVB-76 must actively poll configured tovarisch endpoints at configurable intervals.

**Acceptance Criteria:**
- Periodic HTTP GET requests to `{base_url}/status`
- Configurable scrape interval (default: 30 seconds)
- Configurable timeout (default: 5000ms)
- Store latest snapshot per target in memory

**Implementation:** `scraper/scraper.go`

---

### BR-UVB-002: HTTPS Basic Auth Admin Interface

**Description:** UVB-76 must provide a secure admin interface over HTTPS with Basic Authentication.

**Acceptance Criteria:**
- HTTPS-only (no plain HTTP in production)
- Basic Auth middleware on all admin endpoints
- API endpoints:
  - `GET /api/v1/healthz` - public health check
  - `GET /api/v1/targets` - list all configured targets
  - `GET /api/v1/targets/{id}/snapshot` - latest snapshot for target
- Embedded admin HTML page at `/`

**Implementation:** `server/server.go`, `auth/middleware.go`

---

### BR-UVB-007: Constrained Router/Server Runtime

**Description:** UVB-76 must run on constrained infrastructure (e.g., linux/arm64 routers).

**Acceptance Criteria:**
- Pure Go implementation (no CGO dependencies)
- Cross-compile to linux/arm64
- Bounded memory usage (no database, no unbounded maps)
- Small binary footprint

**Implementation:** `Makefile`, module structure

---

### BR-UVB-008: Fail-Closed Admin/Security Boundaries

**Description:** UVB-76 must fail securely when misconfigured.

**Acceptance Criteria:**
- Refuse to start without TLS cert/key (unless `-dev` flag)
- Refuse to start without valid auth configuration
- Refuse to start with invalid password hash format
- Reject all requests without valid Basic Auth credentials
- Return 401 Unauthorized for missing/bad credentials

**Implementation:** `config/config.go`, `main.go`, `auth/middleware.go`

---

## Non-Goals

The following are explicitly out of scope for the initial UVB-76 implementation:

1. **No dynamic reconfiguration commands** - config is loaded from file only
2. **No persistent database** - all state is in-memory
3. **No ICMP/raw socket ping** - HTTP scraping only
4. **No full graph UI** - minimal admin page with target list
5. **No event detection beyond reachable/unreachable** - basic state only

---

## Privacy Constraints

UVB-76 observes **infrastructure health**, not people.

**Allowed:**
- Node identity (node_id)
- Transport state (reachable/unreachable)
- Health check results (ok/warn/error)
- Configuration version
- Clock skew

**Forbidden:**
- Browsing history
- Visited domains
- Destination IP flow logs
- Message contents
- Per-user behavioral timelines

---

## Password Hash Format

UVB-76 uses `sha256:<salt>:<hex>` format for stored passwords.

- **salt**: 16 bytes, hex-encoded (32 hex characters)
- **hash**: SHA-256(salt + password), hex-encoded (64 hex characters)

Example: `sha256:aabbccddeeff00112233445566778899:abc123def456...`

---

## Configuration Schema

```json
{
  "listen": {
    "addr": ":8443",
    "tls_cert_file": "/etc/uvb76/cert.pem",
    "tls_key_file": "/etc/uvb76/key.pem"
  },
  "auth": {
    "username": "admin",
    "password_sha256": "sha256:<salt>:<hex>"
  },
  "scrape": {
    "interval_seconds": 30,
    "timeout_milliseconds": 5000
  },
  "targets": [
    {
      "id": "router-1",
      "name": "Home Router",
      "base_url": "https://192.168.1.1:8080",
      "enabled": true
    }
  ]
}
```

---

## Future Considerations

1. **Persistent storage** - SQLite for historical snapshots
2. **Multiple UVB-76 instances** - peer synchronization
3. **Alerting** - webhook/Slack notifications on state changes
4. **ICMP ping** - network reachability checks
5. **gRPC transport** - for tovarisch communication
