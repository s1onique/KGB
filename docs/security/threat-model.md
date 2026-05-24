# KGB Threat Model

## Components

### tovarisch
- Zig leaf daemon
- Runs on constrained machines (tiny VPS, family-side nodes)
- Pulls desired-state, supervises tunnels, runs health probes
- Exposes local HTTP diagnostics
- Holds local config and last-known-good fallback

### UVB-76
- Go control tower
- Runs on trusted/home infrastructure
- Issues signed desired-state
- Receives signed reports from tovarisch
- Provides dashboard and federation

### kgbctl
- Operator CLI
- Runs on operator machine
- Holds signing keys
- Authorizes desired-state changes

### Config Source
- YAML config file on local disk
- May be served over network in future
- Contains tunnel endpoints, probe targets, UVB-76 URLs

### Release Artifacts
- .deb packages
- Compiled Zig binaries
- Scripts and systemd units

### Local OS/Kernel
- Linux kernel on target machines
- Systemd service manager
- Network interfaces and tunnels

### Network Interfaces/Tunnels
- WireGuard, AmneziaWG, or other transport backends
- Probe endpoints
- UVB-76 control connections

## Assets

### Leaf Identity
- Unique node identifier
- Used to authenticate reports to UVB-76
- **Current state**: no formal identity yet (deferred)

### Config Integrity
- Config file must not be tampered with
- Affects tunnel endpoints, probe targets, UVB-76 URLs
- **Current state**: no signed config bundles (deferred)

### Operator Intent
- Desired-state authored by kgbctl
- Must reach tovarisch without modification
- **Current state**: no signing verification yet

### Tunnel State
- Active tunnel status (connected, handshake age, peer)
- Sensitive to surveillance if unencrypted transport fails
- **Current state**: transport backend handles this

### Status/Metrics Data
- Health probe results, tunnel health, system state
- Contains infrastructure facts, not human behavior
- **Current state**: local JSON endpoint

### Logs
- Structured logs from tovarisch
- May contain error messages, config state, probe results
- **Current state**: stdout/JSON, no redaction yet

### Release Artifacts
- .deb packages downloaded from release URL
- Could be tampered with in transit
- **Current state**: no release signing (deferred)

### Signing Keys
- Key material for signing reports and desired-state
- Extremely high value targets
- **Current state**: deferred, manual bootstrapping planned

### Local Host Resources
- CPU, memory, disk, network interfaces
- tovarisch must not consume excessive resources
- **Current state**: bounded, not actively throttled

## Trust Boundaries

```
Operator Machine          Trusted Infrastructure         Constrained Leaf
+----------------+        +------------------------+       +------------------+
|   kgbctl       |=======>|      UVB-76             |======>|    tovarisch     |
|   signing key  | HTTPS |   signed desired-state  | HTTPS |   reports        |
+----------------+        +------------------------+       +------------------+
                                  |
                                  v
                         +------------------------+
                         |   config source        |
                         |   (local YAML file)    |
                         +------------------------+
```

**Key trust boundaries:**
- Operator machine to UVB-76: HTTPS, operator auth
- UVB-76 to tovarisch: HTTPS with future certificate pinning
- Config source to tovarisch: local file read, no network trust
- Release URL to tovarisch: future signed releases

## Abuse Case Table

| ID | Abuse Case | Affected Components | Severity | Status |
|---|---|---|---|---|
| AC-01 | Config tampering: attacker modifies config file | tovarisch, config source | High | Deferred |
| AC-02 | Fake desired-state: attacker injects malicious desired-state | tovarisch, operator intent | High | Deferred |
| AC-03 | Release tampering: attacker replaces .deb with malicious version | release artifacts, tovarisch | Critical | Deferred |
| AC-04 | Report interception: attacker reads reports in transit | UVB-76, tovarisch | Medium | HTTPS mitigates |
| AC-05 | Status disclosure: local HTTP endpoint exposes sensitive data | tovarisch, logs | Low | Local-only by default |
| AC-06 | Resource exhaustion: tovarisch consumes excessive memory/disk | local host resources | Medium | Bounded, not enforced |
| AC-07 | Tunnel takeover: attacker hijacks WireGuard/transport session | tunnel state | High | Transport backend responsibility |
| AC-08 | Signing key theft: attacker steals operator signing key | kgbctl, operator intent | Critical | Deferred |
| AC-09 | UVB-76 impersonation: attacker runs fake UVB-76 | operator, tovarisch | High | HTTPS + future pinning |
| AC-10 | Probe target manipulation: attacker changes probe target config | tovarisch | Medium | Config integrity deferred |
| AC-11 | Public bind exposure: tovarisch mistakenly bound to public IP | tovarisch HTTP listener | High | `classifyServeBindHost` blocks without flag |
| AC-12 | Metrics topology disclosure: network peer learns interface names/counts | tovarisch `/metrics.json` | Medium | Loopback default, private bind only |
| AC-13 | Status enumeration: network peer learns node identity and runtime state | tovarisch `/status`, `/status.json` | Low | Loopback default, no auth yet |
| AC-14 | Error leak: unknown routes expose internal state via error messages | tovarisch all routes | Low | `handleNotFound` is silent |

## Attack Surface Inventory

See [tovarisch-attack-surface.md](./tovarisch-attack-surface.md) for the complete HTTP route and listener attack surface inventory.

**Quick reference** (read this before reading Zig):

| Route | Sensitivity | Bind Scope | Key Risk |
|-------|-------------|------------|----------|
| `/healthz` | Low | Any | None |
| `/status`, `/status.json` | Medium | Private only | Node identity disclosure |
| `/metrics.json` | Medium/High | Private only | Topology and counter disclosure |
| Unknown paths | Low | Any | Internal leak via error messages |

**Listener defaults**:
- Default bind: `127.0.0.1:8317` (loopback only)
- Private bind: allowed without flag
- Public bind: requires `--listen-all-public-dangerous`

**Public bind misuse** is tracked in AC-11 above.

## Controls Table

| Control | Phase | Status | Notes |
|---|---|---|---|
| HTTPS for release download transport | Now | Implemented | HTTPS for release downloads |
| Minimal network listeners | Now | Implemented | tovarisch listens on localhost by default |
| No default credentials | Now | Implemented | No embedded passwords |
| Config file permissions | Now | Implemented | Operator controls file ownership |
| Local-only status endpoint | Now | Implemented | Status on localhost, not 0.0.0.0 |
| Structured logging | Now | Implemented | JSON logs, machine-parseable |
| Node identity bootstrapping | Near-term | Deferred | ACT 2 |
| Signed desired-state | Near-term | Deferred | ACT 2 |
| Signed config bundles | Near-term | Deferred | ACT 5 |
| Signed release artifacts | Near-term | Deferred | ACT 4 |
| Release signing (cosign/sigstore) | Identity/Integrity | Deferred | ACT 4 |
| SBOM generation | Identity/Integrity | Deferred | Future |
| Memory limits (systemd) | Identity/Integrity | Implemented | Unit file with limits |
| Privilege separation (capabilities) | Identity/Integrity | Deferred | Future ACT |
| Fuzzing for parsers | Assurance | Deferred | Future ACT |
| Penetration testing | Assurance | Deferred | Future ACT |
| Formal threat model review | Assurance | This doc | ACT 1 |

## Deferred Controls (Not Forgotten)

The following controls are intentionally deferred and tracked:

| Control | Why Deferred | Tracking Epic |
|---|---|---|
| Node identity | Requires bootstrap ceremony design | KGB security doctrine |
| Signed config bundles | Requires identity first | KGB security doctrine |
| Release signing | Requires signing key infrastructure | KGB security doctrine |
| Fuzzing | Requires fuzzing infrastructure setup | Future epic |
| Privilege separation | Requires minimal permissions audit | Future epic |
| Penetration testing | Requires running system with real network | Future epic |

## Threat Modeling Assumptions

1. Operator machine is trusted (compromised operator machine = game over)
2. UVB-76 runs on trusted infrastructure (physical security assumed)
3. tovarisch runs on semi-trusted machines (could be shared VPS)
4. Network is hostile (assume interception, manipulation)
5. Release distribution is trust-minimized (HTTPS only for now)
6. Config source is local (operator responsibility)

## Review Triggers

This threat model must be updated when:

- New component added
- New network listener added
- New config input added
- New file parser added
- New route/tunnel/kernel interaction
- Public bind behavior added
- Auth/identity/signing changes
- Packaging/release changes

See `security-review-ceremony.md` for review process.
