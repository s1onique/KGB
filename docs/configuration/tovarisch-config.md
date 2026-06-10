# tovarisch Configuration Reference

This document describes the `tovarisch.conf` INI configuration file format.

## File Location

Default location: `/etc/kgb/tovarisch.conf`

## Sections

### `[server]`

HTTP server configuration for the status/heartbeat endpoint.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `listen` | string | `"127.0.0.1:8317"` | Listen address and port |

### `[bfd]`

Bidirectional Forwarding Detection (BFD) for multihop path monitoring.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable BFD session |
| `local_addr` | string | (required) | Local BFD address |
| `peer_addr` | string | (required) | Remote BFD peer address |
| `interval_ms` | u32 | `800` | Transmit interval in milliseconds |
| `multiplier` | u8 | `3` | Detection time multiplier |

### `[wg]`

WireGuard local server configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable WireGuard config generation |
| `interface` | string | `"wg-kgb0"` | WireGuard interface name |
| `address` | string | `"10.77.0.1/24"` | Interface address in CIDR notation |
| `listen_port` | u16 | `51820` | UDP listen port (1..65535) |
| `output_dir` | string | `"/var/lib/kgb/wireguard"` | Output directory for generated configs |
| `private_key_file` | string | (required) | Path to server private key file |

## Example Configuration

```ini
[server]
listen = "127.0.0.1:8317"

[bfd]
enabled = false
local_addr = "192.0.2.10"
peer_addr = "192.0.2.1"
interval_ms = 800
multiplier = 3

[wg]
enabled = false
interface = wg-kgb0
address = 10.77.0.1/24
listen_port = 51820
output_dir = /var/lib/kgb/wireguard
private_key_file = /etc/kgb/wireguard/server.key
```

## Command-Line Usage

Generate WireGuard config from config file:

```bash
tovarisch wg generate --config /etc/kgb/tovarisch.conf
