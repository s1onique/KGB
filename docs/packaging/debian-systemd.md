# Debian systemd Packaging

## Overview

The `tovarisch` Debian package includes a systemd unit file for service management and creates a dedicated non-privileged system user.

## Package Contents

| Path | Description |
|------|-------------|
| `usr/bin/tovarisch` | Main binary |
| `lib/systemd/system/tovarisch.service` | systemd unit file |
| `etc/kgb/tovarisch.conf` | Sample configuration (Debian conffile) |
| `DEBIAN/conffiles` | Lists conffiles for dpkg preservation |
| `DEBIAN/postinst` | Post-installation script (creates user, directories, daemon-reload) |
| `DEBIAN/prerm` | Pre-removal script (stops service) |
| `DEBIAN/postrm` | Post-removal script (daemon-reload) |

## Sample Configuration

The package installs a sample configuration file at `/etc/kgb/tovarisch.conf` as a **Debian conffile**. This means:

- dpkg will prompt the operator if the packaged version differs from the local file during upgrades
- Local edits are preserved across package upgrades
- The sample includes commented-out options for server listening and BFD configuration

### Configuration Format

```ini
[server]
# listen = "127.0.0.1:8317"

# [bfd]
# enabled = true
# local_addr = "192.0.2.10"
# peer_addr = "192.0.2.1"
# interval_ms = 800
# multiplier = 3
```

**BFD is disabled by default.** The BFD section is fully commented out and must be explicitly enabled to activate.

Operators can edit `/etc/kgb/tovarisch.conf` to customize their deployment. The file will not be overwritten by package upgrades without operator confirmation.

## Service Configuration

The service is configured to:
- Run as non-privileged `tovarisch` user and group
- Listen on localhost only (`127.0.0.1:8317`) by default
- Restart automatically on failure (5 second delay)
- Run with baseline security hardening

### Hardening Features

- `User=tovarisch` and `Group=tovarisch` — Non-privileged execution
- `NoNewPrivileges=true` — Prevents privilege escalation
- `PrivateTmp=true` — Isolates /tmp
- `ProtectSystem=strict` — Read-only /usr, /boot, /etc
- `ProtectHome=true` — Hide /home directories
- `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` — Grants CAP_NET_BIND_SERVICE to bind privileged BGP TCP/179 as non-root
- `AmbientCapabilities=CAP_NET_BIND_SERVICE` — Allows persistent binding of port 179 for BGP passive listener
- `LockPersonality=true` — Prevent personality changes
- `MemoryDenyWriteExecute=true` — Deny memory write+execute
- `RestrictRealtime=true` — Disable realtime scheduling
- `RestrictSUIDSGID=true` — Block SUID/SGID execution
- `SystemCallArchitectures=native` — Native syscalls only

## Installation

### Install the package

```bash
sudo dpkg -i tovarisch_<version>_amd64.deb
```

The `postinst` script automatically:
1. Creates the `tovarisch` system group (if missing)
2. Creates the `tovarisch` system user (if missing)
3. Creates required directories with correct ownership:
   - `/var/lib/tovarisch` (state)
   - `/var/cache/tovarisch` (cache)
   - `/var/log/tovarisch` (logs)
4. Runs `systemctl daemon-reload`

The package does **NOT** auto-enable or auto-start the service. This is intentional to avoid surprising listeners, even localhost-only ones.

### Enable and start the service

```bash
sudo systemctl enable --now tovarisch
```

### Check service status

```bash
systemctl status tovarisch
journalctl -u tovarisch
curl http://127.0.0.1:8317/status
```

## Debian Maintainer Scripts

### postinst

Creates dedicated system user/group and directories idempotently. Runs `systemctl daemon-reload` but does NOT enable or start the service.

### prerm

Stops the service during package removal, deconfiguration, or upgrade. Gracefully ignores failures if systemctl is unavailable.

### postrm

Runs `systemctl daemon-reload` after package removal. Does NOT delete:
- The `tovarisch` user
- The `tovarisch` group
- Any state, cache, or log directories

## Building the Package

```bash
make package-deb          # Build the .deb package (macOS or Linux)
make verify-deb-systemd   # Static verification of package contents (Linux only)
make deb-gate            # Full package verification: build + verify (Linux only)
```

### Linux CI Verification

`make deb-gate` is **Linux-only** because it requires `dpkg-deb` to inspect
Debian package contents. macOS does not ship `dpkg-deb`, so the verification
step cannot run locally on macOS development machines.

**GitHub Actions CI** runs `make verify-deb-systemd` in the `build-deb` job after
building the versioned `.deb` package and **before** smoke install / artifact upload:

```yaml
- name: Verify deb package (Linux only)
  run: make verify-deb-systemd
```

This ensures that broken packages are never published as release artifacts.
Failed verification blocks the workflow, preventing upload and release steps.

### Local Development on macOS

macOS developers can still build the package and run local gate checks:

```bash
make gate                  # Run local hygiene, build, test gates (macOS OK)
make package-deb           # Build the .deb (uses cross-compilation)
# make deb-gate fails on macOS - requires Linux CI
```

The local `make gate` remains portable and does not include Debian verification.
Linux CI owns that contract.

## Design Decisions

### No auto-enable on install

Security/local-first leaf daemon should not automatically start listening, even on localhost. Operators explicitly enable the service after reviewing the configuration.

### Why systemd capabilities not file setcap?

Using systemd's `CapabilityBoundingSet` and `AmbientCapabilities` is preferred over `setcap` on the binary for several reasons:

1. **Package upgrade safety**: File capabilities can be lost during package upgrades if the preinst/postinst scripts fail or are skipped. systemd capabilities are declarative in the unit file and survive upgrades.

2. **Principle of least privilege**: `setcap` grants the capability to anyone who executes the binary. systemd capabilities are scoped to the service process only, and only when the service is started via systemd.

3. **Auditability**: systemd unit files are declarative and version-controlled. File capabilities require separate tooling (`getcap`, `setcap`) and are harder to audit across systems.

4. **Consistency**: All service capabilities are defined in one place (the unit file), making security posture review straightforward.

### No state deletion on remove

Preserving state directories and user account on ordinary remove allows:
- Configuration persistence across package upgrades
- Service state retention for debugging
- Clean migration path if service is reinstalled

Purge behavior can be added explicitly if needed and documented.

## Future Work

- Add optional purge cleanup (explicit operator action)
- Add smoke tests in postinst
- Consider multi-architecture package variants
