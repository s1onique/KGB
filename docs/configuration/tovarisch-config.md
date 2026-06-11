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

### `[bgp]`

Border Gateway Protocol (BGP) session for route exchange. Supports both active outbound connections and passive inbound listening.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable BGP session |
| `local_address` | string | (required) | Local IPv4 address for binding and BGP identifier |
| `router_id` | string | (required) | BGP router ID in dotted notation |
| `local_as` | u16 | (required) | Local autonomous system number (1..65535) |
| `peer_address` | string | (required) | Remote BGP peer IPv4 address |
| `peer_port` | u16 | `179` | Remote BGP port |
| `peer_as` | u16 | (required) | Remote autonomous system number (1..65535) |
| `hold_time_seconds` | u16 | `180` | BGP hold time in seconds (0 or >=3) |
| `keepalive_seconds` | u16 | `60` | Keepalive interval in seconds |
| `advertised_prefixes` | string | (required) | Comma-separated list of CIDR prefixes to advertise |
| `advertised_prefix_files` | string | (optional) | Comma-separated list of file paths containing prefixes |
| `same_as` | bool | `false` | AS_PATH is empty (same-AS/iBGP style) |

**Note:** When `local_address` is configured, a passive TCP/179 listener is created automatically, allowing the daemon to accept incoming BGP peer connections in addition to initiating outbound connections. The capability `CAP_NET_BIND_SERVICE` must be granted via systemd to bind port 179 as non-root.

### `[wg]`

WireGuard local server configuration and peer settings.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable WireGuard config generation |
| `interface` | string | `"wg-kgb0"` | WireGuard interface name |
| `address` | string | `"10.77.0.1/24"` | Interface address in CIDR notation |
| `listen_port` | u16 | `51820` | UDP listen port (1..65535) |
| `output_dir` | string | `"/var/lib/kgb/wireguard"` | Output directory for generated configs |
| `private_key_file` | string | (required) | Path to server private key file |
| `public_key_file` | string | (optional) | Path to server public key file (for client config generation) |
| `client_allowed_ips` | string | `"10.149.149.0/24"` | Allowed IPs for clients in generated client configs |

### `[wg.peer.<name>]`

Peer configuration sections. Each named peer gets its own section.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable this peer |
| `address` | string | (required if enabled) | Peer interface address in CIDR notation |
| `private_key_file` | string | (required if enabled) | Path to peer's private key file |
| `public_key_file` | string | (required if enabled) | Path to peer's public key file |
| `allowed_ips` | string | (required if enabled) | Allowed IPs for this peer |
| `endpoint` | string | (optional) | Endpoint address and port |
| `persistent_keepalive` | u16 | (optional) | Keepalive interval in seconds (0..65535) |
| `client_output_file` | string | (required if enabled) | Path to write generated client config |

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

[wg.peer.laptop]
enabled = true
address = 10.149.149.11/32
private_key_file = /etc/kgb/wireguard/peers/laptop.key
public_key_file = /etc/kgb/wireguard/peers/laptop.pub
allowed_ips = 10.149.149.11/32
endpoint = 127.0.0.1:51821
persistent_keepalive = 25
client_output_file = /var/lib/kgb/wireguard/clients/laptop.conf
```

## Command-Line Usage

Generate WireGuard configs from config file:

```bash
tovarisch wg generate --config /etc/kgb/tovarisch.conf
```

### Key Generation

Keys are operator-owned. Generate them manually:

```bash
# Create peers directory
sudo install -d -m 0700 /etc/kgb/wireguard/peers

# Generate a peer key pair
wg genkey | sudo tee /etc/kgb/wireguard/peers/phone.key >/dev/null
sudo chmod 0600 /etc/kgb/wireguard/peers/phone.key
sudo cat /etc/kgb/wireguard/peers/phone.key | wg pubkey | sudo tee /etc/kgb/wireguard/peers/phone.pub >/dev/null
sudo chmod 0644 /etc/kgb/wireguard/peers/phone.pub

# Generate server public key from existing private key
sudo cat /etc/kgb/wireguard/server.key | wg pubkey | sudo tee /etc/kgb/wireguard/server.pub >/dev/null
sudo chmod 0644 /etc/kgb/wireguard/server.pub
