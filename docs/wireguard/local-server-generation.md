# WireGuard Config Generation

This document describes the WireGuard server and client config generation features of `tovarisch`.

## Overview

The `[wg]` section in `tovarisch.conf` configures WireGuard config generation. When enabled, `tovarisch wg generate` produces:

1. **Server config** (`{output_dir}/{interface}.conf`) with `[Peer]` blocks for each enabled peer
2. **Client config** files for each enabled peer (written to paths specified in `[wg.peer.*]` sections)

**This ACT only generates files.** It does not:
- Start `wg-quick` or `wg`
- Mutate routing
- Open firewall rules
- Supervise the WireGuard process

## Generated Files

### Server Config

Location: `{output_dir}/{interface}.conf`

Example: `/var/lib/kgb/wireguard/wg-kgb0.conf`

#### Content Format

```ini
[Interface]
Address = 10.149.149.1/24
ListenPort = 51821
PrivateKey = <server private key>
SaveConfig = false

[Peer]
# phone
PublicKey = <phone public key>
AllowedIPs = 10.149.149.10/32

[Peer]
# laptop
PublicKey = <laptop public key>
AllowedIPs = 10.149.149.11/32
```

**Note:** Server config contains only peer **public keys**, never peer private keys.

### Client Config

Location: Specified by `client_output_file` in each `[wg.peer.<name>]` section.

Example: `/var/lib/kgb/wireguard/clients/phone.conf`

#### Content Format

```ini
[Interface]
Address = 10.149.149.10/32
PrivateKey = <phone private key>

[Peer]
PublicKey = <server public key>
AllowedIPs = 10.149.149.0/24
Endpoint = 127.0.0.1:51821
PersistentKeepalive = 25
```

**Note:** Client config contains the **peer's private key** and **server's public key**.

### File Permissions

- Config files: `0600` (owner read/write only)
- Parent directories: `0700` (owner read/write/execute only)

## Security Notes

### Private Key Handling

- Private keys are read from paths specified in config files
- Private keys are **never logged** or printed to output
- Only safe summary fields are printed (interface, address, port, output path)
- Server config contains only public keys
- Client config contains peer private key (needed for client) but no server private key

### Key Validation

Keys must be exactly 44 base64 characters (WireGuard format):
- Valid characters: A-Z, a-z, 0-9, +, /
- Invalid keys cause generation to fail with appropriate error

### VLESS-Only Exposure Warning

Generated WireGuard config is **not sufficient by itself** to enforce VLESS-only exposure. WireGuard binds to UDP ports, not HTTP-style hosts.

Enforcing VLESS-only exposure requires additional runtime/firewall measures:

1. **Firewall rules** (e.g., iptables) to restrict incoming UDP traffic
2. **Network namespace binding** to constrain WireGuard to specific interfaces
3. **Userspace tunnel/proxy** design where VLESS is the only externally reachable listener
4. **Deployment bundle** where the VLESS side handles external connectivity

This ACT documents the constraint but does not implement runtime enforcement.

## Configuration

See [`docs/configuration/tovarisch-config.md`](../configuration/tovarisch-config.md) for the full configuration reference including `[wg.peer.<name>]` sections.

### Quick Example

```ini
[wg]
enabled = true
interface = wg-kgb0
address = 10.149.149.1/24
listen_port = 51821
output_dir = /var/lib/kgb/wireguard
private_key_file = /etc/kgb/wireguard/server.key
public_key_file = /etc/kgb/wireguard/server.pub
client_allowed_ips = 10.149.149.0/24

[wg.peer.phone]
enabled = true
address = 10.149.149.10/32
private_key_file = /etc/kgb/wireguard/peers/phone.key
public_key_file = /etc/kgb/wireguard/peers/phone.pub
allowed_ips = 10.149.149.10/32
endpoint = 127.0.0.1:51821
persistent_keepalive = 25
client_output_file = /var/lib/kgb/wireguard/clients/phone.conf
```

## Usage

### Generate WireGuard Configs

```bash
tovarisch wg generate --config /etc/kgb/tovarisch.conf
```

### Output

On success:
```
Server config generated: /var/lib/kgb/wireguard/wg-kgb0.conf
  Interface: wg-kgb0
  Address: 10.149.149.1/24
  ListenPort: 51821
  Peers: 2
Client config generated: /var/lib/kgb/wireguard/clients/phone.conf
  Peer: phone
Client config generated: /var/lib/kgb/wireguard/clients/laptop.conf
  Peer: laptop
```

When disabled:
```
error: WireGuard config generation is disabled
Set enabled = true in [wg] section
```

### Error Cases

| Error | Cause |
|-------|-------|
| `error: config file not found` | Config file path is invalid |
| `error: invalid [wg] config` | Config parsing failed |
| `error: private key file not found` | Private key path is invalid |
| `error: invalid private key` | Key is not 44 base64 characters |
| `error: public key file not found` | Public key path is invalid |
| `error: invalid public key` | Public key is not 44 base64 characters |
| `error: failed to create output directory` | Cannot create output_dir |
| `error: failed to write config file` | Cannot write to output path |

## Follow-up ACTs

### [ ] ACT: Add VLESS tunnel-side WireGuard UDP forwarding

Acceptance criteria would cover generating the VLESS/Xray/sing-box side so WireGuard UDP reaches the local server only through the tunnel.

### [ ] ACT: Enforce WireGuard VLESS-only exposure

Acceptance criteria would cover firewall/network namespace/systemd hardening so the WireGuard UDP port is not reachable directly.

### [x] ACT: Add WireGuard peer config generation

Peer configuration is now supported via `[wg.peer.<name>]` sections with client config generation.
