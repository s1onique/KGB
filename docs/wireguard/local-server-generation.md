# WireGuard Local Server Generation

This document describes the WireGuard server config generation feature of `tovarisch`.

## Overview

The `[wg]` section in `tovarisch.conf` configures WireGuard local server config generation. When enabled, `tovarisch wg generate` produces a WireGuard server config file with strict permissions.

**This ACT only generates files.** It does not:
- Start `wg-quick` or `wg`
- Mutate routing
- Open firewall rules
- Supervise the WireGuard process

## Generated File

Location: `{output_dir}/{interface}.conf`

Example: `/var/lib/kgb/wireguard/wg-kgb0.conf`

### Content Format

```ini
[Interface]
Address = 10.77.0.1/24
ListenPort = 51820
PrivateKey = <loaded from private_key_file>
```

### File Permissions

- Config file: `0600` (owner read/write only)
- Parent directory: `0700` (owner read/write/execute only)

## Security Notes

### Private Key Handling

- Private keys are read from the path specified in `private_key_file`
- Private keys are **never logged** or printed to output
- Only safe summary fields are printed (interface, address, port, output path)

### VLESS-Only Exposure Warning

Generated WireGuard config is **not sufficient by itself** to enforce VLESS-only exposure. WireGuard binds to UDP ports, not HTTP-style hosts.

Enforcing VLESS-only exposure requires additional runtime/firewall measures:

1. **Firewall rules** (e.g., iptables) to restrict incoming UDP traffic
2. **Network namespace binding** to constrain WireGuard to specific interfaces
3. **Userspace tunnel/proxy** design where VLESS is the only externally reachable listener
4. **Deployment bundle** where the VLESS side handles external connectivity

This ACT documents the constraint but does not implement runtime enforcement.

## Configuration

See [`docs/configuration/tovarisch-config.md`](../configuration/tovarisch-config.md) for the full `[wg]` configuration reference.

## Usage

### Generate WireGuard Config

```bash
tovarisch wg generate --config /etc/kgb/tovarisch.conf
```

### Output

On success:
```
WireGuard config generated:
  interface: wg-kgb0
  address: 10.77.0.1/24
  listen_port: 51820
  output: /var/lib/kgb/wireguard/wg-kgb0.conf
```

When disabled:
```
WireGuard config generation is disabled in [wg] section.
```

### Error Cases

| Error | Cause |
|-------|-------|
| `error: config file not found` | Config file path is invalid |
| `error: invalid [wg] config` | Config parsing failed |
| `error: private key file not found` | Private key path is invalid |
| `error: invalid private key` | Key is not 44 base64 characters |
| `error: failed to create output directory` | Cannot create output_dir |
| `error: failed to write config file` | Cannot write to output path |

## Follow-up ACTs

### [ ] ACT: Add VLESS tunnel-side WireGuard UDP forwarding

Acceptance criteria would cover generating the VLESS/Xray/sing-box side so WireGuard UDP reaches the local server only through the tunnel.

### [ ] ACT: Enforce WireGuard VLESS-only exposure

Acceptance criteria would cover firewall/network namespace/systemd hardening so the WireGuard UDP port is not reachable directly.

### [ ] ACT: Add WireGuard peer config generation

Acceptance criteria would cover `[wg.peer.*]` sections and client config generation.
