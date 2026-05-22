# [Open] Epic: Make tovarisch a Day-0 webservice

## Goal

Make `tovarisch` a **local-first webservice daemon** from Day 0, listening on private interfaces and reporting tunnel/interface metrics.

Canonical runtime:

```bash
tovarisch serve
```

CLI becomes a control/debug surface, but the daemon is the primary runtime.

## Doctrine

`tovarisch` is a **leaf-local observability and control service** for exactly the host/network properties KGB cares about.

Not Netdata.
Not Prometheus-node-exporter.
Not generic host monitoring.
A narrow, opinionated field daemon.

## Non-goals

- No Prometheus text format yet.
- No TLS yet.
- No OAuth/OIDC.
- No WebSocket yet.
- No Kubernetes assumptions.
- No containers by default.

## Default listen behavior

```
Default: listen on all private, non-loopback interface addresses.
Do not listen on public IPs by default.
Do not bind wildcard 0.0.0.0 blindly.
```

Only addresses classified as private / local / ULA should be included.

Suggested defaults:

```
port: 8317
bind_mode: private_interfaces
allow_loopback: true
allow_public: false
```

Explicit overrides:

```bash
tovarisch serve --listen 127.0.0.1:8317
tovarisch serve --listen-private
tovarisch serve --listen-all-public-dangerous
```

## Security stance

v0: read-only endpoints only, no mutations.

| Endpoint        | Safe by default? | Notes |
|-----------------|-----------------|-------|
| `GET /healthz`  | yes | plain ok/error |
| `GET /status.json` | yes, but redacted | service + checks + summarized tunnels |
| `GET /metrics.json` | maybe | may reveal topology |
| `GET /debug/*` | no | loopback only or disabled |

## Architecture

```
tovarisch/src/
  main.zig
  cli.zig
  status.zig
  http/
    server.zig      # TCP listener, minimal HTTP parser
    routes.zig      # path → handler mapping
    response.zig    # JSON response helpers
  net/
    interfaces.zig  # enumerate network interfaces
    private_ip.zig  # classify private vs public
    linux_stats.zig # read /sys/class/net/*/statistics
    tunnel_detect.zig # detect wireguard/tun interfaces
  metrics/
    model.zig       # metrics data structures
    collect.zig     # collect interface/tunnel metrics
```

## Implementation philosophy

Primitive HTTP server:

```
accept TCP
read HTTP request line
support GET only
route by path
write HTTP/1.1 JSON response
close connection
```

No chunking.
No keepalive.
No TLS.
No framework.

## ACT Board

| ACT | Scope | Status |
|-----|-------|--------|
| ACT 1 | Define HTTP service contract and private-bind doctrine | ✅ done |
| ACT 2 | Add `tovarisch serve` with minimal HTTP `/healthz` | ✅ done |
| ACT 2b | Fix CI build failure: explicit libc for HTTP sockets | ✅ done |
| ACT 3 | Enumerate private interface addresses and bind only those by default | open |
| ACT 4 | Add `/status.json` over HTTP using existing status model | open |
| ACT 5 | Add private interface traffic collector from Linux sysfs | open |
| ACT 6 | Add tunnel interface detection and up/down summary | open |
| ACT 7 | Add `/metrics.json` contract, fixture, and verifier | open |
| ACT 8 | Wire webservice checks into `make gate` | open |
| ACT 9 | Split `cli.zig` for LLM-friendliness | open |

## ACT 1: Define HTTP service contract and private-bind doctrine

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-001 | Create epic doc `docs/epics/tovarisch-webservice-day0.md` | open |
| webservice-002 | Create HTTP contract doc `docs/contracts/tovarisch-http-v0.md` | open |
| webservice-003 | Define endpoints: /healthz, /status.json, /metrics.json | open |
| webservice-004 | Define bind behavior: private interfaces only by default | open |
| webservice-005 | Document security stance: read-only v0 | open |
| webservice-006 | Run `make gate` | open |

### Acceptance

- [ ] Epic doc with ACT board, HTTP contract, endpoints, bind behavior, security stance
- [ ] Default port 8317, private-interfaces-only bind mode
- [ ] `make gate` passes

## ACT 2: Add `tovarisch serve` with minimal HTTP `/healthz`

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-007 | Create `tovarisch/src/http/` module structure | ✅ done |
| webservice-008 | Implement `http/server.zig` - TCP listener | ✅ done |
| webservice-009 | Implement `http/routes.zig` - GET routing | ✅ done |
| webservice-010 | Implement `http/response.zig` - JSON helpers | ✅ done |
| webservice-011 | Add `serve` command to `cli.zig` | ✅ done |
| webservice-012 | Implement `GET /healthz` handler | ✅ done |
| webservice-013 | Add `serve` tests | ✅ done |
| webservice-014 | Run `make gate`, build, test | ✅ done |

### Acceptance

- [x] `tovarisch serve` starts HTTP server, `GET /healthz` returns `{"status":"ok"}`
- [x] Server binds loopback (127.0.0.1:8317), build/test/gate pass

## ACT 2b: Fix CI build failure - explicit libc for HTTP sockets

### Problem

Cross-compilation to `arm-linux-musleabihf` failed with: `dependency on libc must be explicitly specified`.

### Key Insight

Zig 0.16 module-style build creates separate root modules. Module options do NOT automatically apply across artifacts.

### Decision: Explicit libc

Added `.link_libc = true` to both executable and test root modules in `tovarisch/build.zig`:

```zig
const unit_tests = b.addTest(.{
    .root_module = b.createModule(.{
        .root_source_file = b.path("src/test_all.zig"),
        .link_libc = true,
    }),
});
```

**Rule:** `.link_libc` is NOT project-wide. Apply to every `b.createModule()` using `std.c.*` functions.

### Files Changed

- `tovarisch/build.zig` — Added `.link_libc = true` to exe and test modules
- `docs/tooling/zig-0.16-field-manual.md` — Documented socket API limitation

### Verification

- `zig build` ✅ `zig build test` ✅ `zig build -Dtarget=arm-linux-musleabihf` ✅ `make gate` ✅

## ACT 3: Enumerate private interface addresses and bind only those by default

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-015 | Create `net/private_ip.zig` - private IP classification | open |
| webservice-016 | Create `net/interfaces.zig` - enumerate network interfaces | open |
| webservice-017 | Update server to bind to all private interfaces | open |
| webservice-018 | Add `--listen` override for explicit bind address | open |
| webservice-019 | Add tests for private interface enumeration | open |
| webservice-020 | Run `make gate`, `make tovarisch-build`, `make tovarisch-test` | open |

### Acceptance

- [ ] Server binds to all private interface addresses (192.168.x.x, 10.x.x.x, 172.16.x.x, fd00::/8 ULA).
- [ ] Server does NOT bind to public IPs by default.
- [ ] `--listen` flag allows explicit address override.
- [ ] `--listen-private` explicitly enables private interface binding.
- [ ] Tests verify private IP classification.
- [ ] `make gate` passes.

## ACT 4: Add `/status.json` over HTTP using existing status model

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-021 | Add `GET /status.json` route handler | open |
| webservice-022 | Reuse `status.renderPayload()` for JSON output | open |
| webservice-023 | Add HTTP check to status model | open |
| webservice-024 | Add `/status.json` HTTP test | open |
| webservice-025 | Run `make gate`, `make tovarisch-build`, `make tovarisch-test` | open |

### Acceptance

- [ ] `GET /status.json` returns current status JSON.
- [ ] HTTP check is included in status checks (shows "listening on private interfaces").
- [ ] `make tovarisch-test` passes.
- [ ] `make gate` passes.

## ACT 5: Add private interface traffic collector from Linux sysfs

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-026 | Create `net/linux_stats.zig` - read /sys/class/net/*/statistics | open |
| webservice-027 | Implement `getInterfaceStats()` function | open |
| webservice-028 | Filter to private interfaces only | open |
| webservice-029 | Add interface stats to metrics model | open |
| webservice-030 | Add tests for Linux stats collection | open |
| webservice-031 | Run `make gate`, `make tovarisch-build`, `make tovarisch-test` | open |

### Acceptance

- [ ] `linux_stats.zig` reads rx_bytes, tx_bytes, rx_packets, tx_packets from sysfs.
- [ ] Interface stats are available for private interfaces.
- [ ] Non-private interfaces are filtered out by default.
- [ ] Tests verify stats reading.
- [ ] `make gate` passes.

## ACT 6: Add tunnel interface detection and up/down summary

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-032 | Create `net/tunnel_detect.zig` - detect wireguard/tun interfaces | open |
| webservice-033 | Implement tunnel detection by interface name pattern | open |
| webservice-034 | Classify tunnels by kind (wireguard, openvpn, etc.) | open |
| webservice-035 | Add tunnel stats to metrics model | open |
| webservice-036 | Update status checks with tunnel count | open |
| webservice-037 | Add tests for tunnel detection | open |
| webservice-038 | Run `make gate`, `make tovarisch-build`, `make tovarisch-test` | open |

### Acceptance

- [ ] `tunnel_detect.zig` identifies wg*, tun*, tap*, tailscale*, etc.
- [ ] Tunnel status (up/down) is reported.
- [ ] Tunnel traffic stats are available.
- [ ] Status check shows tunnel count summary.
- [ ] Tests verify tunnel detection.
- [ ] `make gate` passes.

## ACT 7: Add `/metrics.json` contract, fixture, and verifier

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-039 | Create `docs/contracts/tovarisch-metrics-v0.md` | open |
| webservice-040 | Create `docs/contracts/examples/tovarisch-metrics-v0.json` | open |
| webservice-041 | Implement `GET /metrics.json` handler | open |
| webservice-042 | Create `scripts/verify_tovarisch_metrics_contract.sh` | open |
| webservice-043 | Wire verifier into quality gate | open |
| webservice-044 | Add `make verify-metrics-contract` target | open |
| webservice-045 | Run `make gate` | open |

### Acceptance

- [ ] `/metrics.json` contract doc defines payload shape.
- [ ] Fixture exists and is valid JSON.
- [ ] `/metrics.json` handler returns metrics JSON.
- [ ] Verification script exists and works.
- [ ] Gate runs verification.
- [ ] `make gate` passes.

## ACT 8: Wire webservice checks into `make gate`

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-046 | Add HTTP endpoint tests to `cli.zig` | open |
| webservice-047 | Add HTTP server tests to test suite | open |
| webservice-048 | Update coverage ledger | open |
| webservice-049 | Update coverage inventory | open |
| webservice-050 | Run `make gate`, `make coverage`, `make tovarisch-test` | open |

### Acceptance

- [ ] `/healthz` endpoint is tested.
- [ ] `/status.json` endpoint is tested.
- [ ] `/metrics.json` endpoint is tested.
- [ ] Coverage ledger updated with web service commands.
- [ ] `make tovarisch-test` passes.
- [ ] `make gate` passes.

## ACT 9: Split `cli.zig` for LLM-friendliness

### Board

| ID | Work Item | Status |
|---|---|---|
| webservice-051 | Create `cli/` subdirectory | ✅ done |
| webservice-052 | Extract usage text to `cli/usage.zig` | ✅ done |
| webservice-053 | Extract argument parsing to `cli/args.zig` | ✅ done |
| webservice-054 | Extract command dispatch to `cli/commands.zig` | ✅ done |
| webservice-055 | Shrink `cli.zig` to thin facade | ✅ done |
| webservice-056 | Run `make gate` | ✅ done |

### Acceptance

- [x] `cli.zig` is a thin public facade used by `main.zig`.
- [x] Serve argument parsing moved to `cli/args.zig`.
- [x] Help/usage text moved to `cli/usage.zig`.
- [x] Existing behavior for `--help`, `-h`, `--version`, `check`, `status --json`, and `serve` is unchanged.
- [x] Deprecated `--listen-all` is rejected.
- [x] `--listen-all-public-dangerous` is visible in help and covered by tests.
- [x] `make gate`, `make tovarisch-test` (76 tests), `make tovarisch-build` all pass.

### Files Changed

- `tovarisch/src/cli.zig` — Thin facade (11 lines, down from 406)
- `tovarisch/src/cli/usage.zig` — Usage text constants and printer
- `tovarisch/src/cli/args.zig` — Argument parsing and `parseServeArgs`
- `tovarisch/src/cli/commands.zig` — Command dispatch and tests

### New Module Structure

```
tovarisch/src/
  cli.zig          # Thin facade (re-exports)
  cli/
    usage.zig      # Usage text + printUsage()
    args.zig       # Argument parsing + parseServeArgs()
    commands.zig  # Command dispatch + run()
```

## Closure Summary

What was accomplished:

- `tovarisch serve` runs as a local-first webservice daemon.
- Server binds to private interfaces by default (no public exposure).
- `/healthz` provides simple liveness check.
- `/status.json` exposes health status over HTTP.
- `/metrics.json` exposes interface/tunnel metrics over HTTP.
- All endpoints are tested and gate-backed.
- No TLS, no OAuth, no framework gravity.
- Primitive HTTP implementation suitable for constrained leaf nodes.
- `cli.zig` split into focused modules for LLM-friendliness.

## Future Work

- Add TLS support (future ACT)
- Add bearer token auth (future ACT)
- Add Prometheus exporter adapter (future ACT)
- Add `/debug/*` endpoints (loopback-only, future ACT)
- Add POST endpoints for control operations (future ACT)
