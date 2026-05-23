# Debian systemd Packaging

## Overview

The `tovarisch` Debian package includes a systemd unit file for service management.

## Package Contents

| Path | Description |
|------|-------------|
| `usr/bin/tovarisch` | Main binary |
| `lib/systemd/system/tovarisch.service` | systemd unit file |

## Service Configuration

The service is configured to:
- Listen on localhost only (`127.0.0.1:8317`) by default
- Restart automatically on failure (5 second delay)
- Run with baseline security hardening

### Hardening Features

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

## Installation Steps

After installing the `.deb` package:

```bash
sudo dpkg -i tovarisch_<version>_amd64.deb
sudo systemctl daemon-reload
sudo systemctl start tovarisch
systemctl status tovarisch
curl http://127.0.0.1:8317/status
```

## ACT 1 Status

ACT 1 adds the systemd unit to the package without:
- Automatic service enablement on install
- Non-privileged `tovarisch` user creation
- Maintainer scripts (postinst, prerm, etc.)

The service currently runs as root. ACT 2 will add:
- `User=tovarisch` to the unit file
- Proper Debian maintainer scripts

## Future Work (ACT 2)

- Add `tovarisch` system user
- Add maintainer scripts for user creation and daemon-reload
- Do NOT auto-enable or auto-start the service
- Operator explicitly enables with: `sudo systemctl enable --now tovarisch`

## Building the Package

```bash
make package-deb
make verify-deb-systemd  # Optional verification
make deb-gate            # Full package verification
