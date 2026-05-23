# Debian systemd Packaging

## Overview

The `tovarisch` Debian package includes a systemd unit file for service management and creates a dedicated non-privileged system user.

## Package Contents

| Path | Description |
|------|-------------|
| `usr/bin/tovarisch` | Main binary |
| `lib/systemd/system/tovarisch.service` | systemd unit file |
| `DEBIAN/postinst` | Post-installation script (creates user, directories, daemon-reload) |
| `DEBIAN/prerm` | Pre-removal script (stops service) |
| `DEBIAN/postrm` | Post-removal script (daemon-reload) |

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
- `CapabilityBoundingSet=` — No capabilities granted
- `AmbientCapabilities=` — No ambient capabilities
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
make package-deb          # Build the .deb package
make verify-deb-systemd   # Static verification of package contents
make deb-gate            # Full package verification (Linux only)
```

## Design Decisions

### No auto-enable on install

Security/local-first leaf daemon should not automatically start listening, even on localhost. Operators explicitly enable the service after reviewing the configuration.

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
