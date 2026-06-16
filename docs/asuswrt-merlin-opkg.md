# UVB-76 on AsusWRT-Merlin with Entware

This guide covers installing and running UVB-76 on AsusWRT-Merlin routers using Entware and the opkg package system.

## Prerequisites

### Entware Installation

Entware must be installed on your router before proceeding.

1. SSH into your router as admin
2. Check if Entware is installed:

```bash
ls /opt/bin/opkg
opkg print-architecture
```

3. If not installed, follow the [Entware installation guide](https://github.com/RMerl/asuswrt-merlin/wiki/Entware)

### Check Router Architecture

Different router models have different architectures:

```bash
# Check CPU architecture
uname -m

# Check opkg architecture
opkg print-architecture
```

Common architectures for AsusWRT-Merlin:

| Router Series | `uname -m` | opkg Architecture |
|--------------|------------|-------------------|
| ARMv8 (RT-AX86U, etc.) | `aarch64` | `aarch64-3.10` |
| ARMv7 (RT-AC87U, etc.) | `armv7l` | `armv7hf` |
| MIPS (RT-N66U, etc.) | `mips` | `mipsel` |

UVB-76 currently ships for **aarch64-3.10** (ARMv8). Other architectures may be added in future releases.

## Downloading the Package

1. Go to the [GitHub Releases](https://github.com/s1onique/KGB/releases) page
2. Find the latest release tagged with `v*.*.*`
3. Download the following files:
   - `uvb76_<version>-1_aarch64-3.10.ipk` - the package
   - `uvb76_<version>-1_aarch64-3.10.ipk.sha256` - the checksum (optional but recommended)

4. (Optional) Verify the download:

```bash
# Calculate SHA256 of downloaded file
sha256sum uvb76_<version>-1_aarch64-3.10.ipk

# Compare with the sha256 file
cat uvb76_<version>-1_aarch64-3.10.ipk.sha256
```

## Installing

### 1. Transfer to Router

```bash
# From your development machine
scp uvb76_<version>-1_aarch64-3.10.ipk admin@router.local:/tmp/
```

### 2. Install Package

SSH into your router:

```bash
ssh admin@router.local
```

Install the package:

```bash
opkg install /tmp/uvb76_<version>-1_aarch64-3.10.ipk
```

The installation will:
- Create `/opt/bin/uvb76` (the binary)
- Create `/opt/etc/init.d/S76uvb76` (the init script)
- Create `/opt/etc/uvb76/` directory
- Install `/opt/etc/uvb76/uvb76.json.example` (example config)
- Copy example config to `/opt/etc/uvb76/uvb76.json` (if not already present)
- Create `/opt/var/log/uvb76/` (log directory)

### 3. Configure

Edit `/opt/etc/uvb76/uvb76.json`:

```json
{
  "listen": {
    "addr": ":8443",
    "tls_cert_file": "/opt/etc/uvb76/cert.pem",
    "tls_key_file": "/opt/etc/uvb76/key.pem"
  },
  "auth": {
    "username": "admin",
    "password_sha256": "sha256:YOUR_SALT:YOUR_HASH"
  },
  "scrape": {
    "interval_seconds": 30,
    "timeout_milliseconds": 5000
  },
  "latency": {
    "enabled": true,
    "interval_seconds": 30,
    "timeout_milliseconds": 5000,
    "histogram_buckets_ms": [5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000],
    "recent_samples_max": 100
  },
  "targets": [
    {
      "id": "my-tovarisch",
      "name": "My Tovarisch",
      "base_url": "https://localhost:9080",
      "enabled": true
    }
  ]
}
```

#### Generate Password Hash

On your development machine:

```bash
cd KGB/uvb76
go run ./tools/genhash
```

Follow the prompts to generate a password hash. Copy the generated `sha256:...` value into your config.

#### Generate TLS Certificates

```bash
# SSH to router
ssh admin@router.local

# Generate certificates
cd /opt/etc/uvb76
openssl genrsa -out key.pem 2048
openssl req -new -x509 -key key.pem -out cert.pem -days 365 \
  -subj "/CN=uvb76/O=KGB"
```

## Service Management

### Start the Service

```bash
/opt/etc/init.d/S76uvb76 start
```

### Check Status

```bash
/opt/etc/init.d/S76uvb76 check
```

### Stop the Service

```bash
/opt/etc/init.d/S76uvb76 stop
```

### Restart the Service

```bash
/opt/etc/init.d/S76uvb76 restart
```

### Automatic Startup

The service is configured to start automatically on router boot by default (via Entware's `rc.unslung`).

To disable auto-start:

```bash
sed -i 's/ENABLED=yes/ENABLED=no/' /opt/etc/init.d/S76uvb76
```

## Viewing Logs

```bash
# View full log
cat /opt/var/log/uvb76/uvb76.log

# Follow log in real-time
tail -f /opt/var/log/uvb76/uvb76.log
```

## Troubleshooting

### "opkg install" fails with "Unknown package"

Ensure you have the correct architecture for your router:

```bash
opkg print-architecture
```

This package is built for `aarch64-3.10`. If your router shows a different architecture, the package will not install.

### Service Does Not Start

1. Check if config is valid:
   ```bash
   /opt/bin/uvb76 -config /opt/etc/uvb76/uvb76.json
   ```
   Look for error messages about missing fields or invalid JSON.

2. Check file permissions:
   ```bash
   ls -la /opt/bin/uvb76
   ls -la /opt/etc/init.d/S76uvb76
   ```
   The binary and init script must be executable.

3. Make executable if needed:
   ```bash
   chmod +x /opt/bin/uvb76
   chmod +x /opt/etc/init.d/S76uvb76
   ```

4. Check logs:
   ```bash
   cat /opt/var/log/uvb76/uvb76.log
   ```

### Config Parse Failure

Common issues:
- **Trailing commas** - JSON doesn't allow trailing commas
- **Missing quotes** - Object keys must be in double quotes
- **Invalid paths** - TLS cert/key paths must exist
- **Bad password hash** - Must be format `sha256:<32-char-salt>:<64-char-hash>`

### Wrong Architecture

If `opkg install` fails with architecture errors:

1. Verify your architecture:
   ```bash
   uname -m
   opkg print-architecture
   ```

2. Download the correct package for your architecture (if available)

3. If no package exists for your architecture, file a GitHub Issue

### Entware Not Found

If commands fail with "not found":

```bash
# Verify Entware is installed
ls /opt/bin/opkg

# If missing, install Entware
# See: https://github.com/RMerl/asuswrt-merlin/wiki/Entware
```

### Service Hangs or Crashes

1. Check system resources:
   ```bash
   free
   df -h
   ```

2. Check if another process is using the same port:
   ```bash
   netstat -ln | grep 8443
   ```

3. Try running in foreground to see errors:
   ```bash
   /opt/bin/uvb76 -config /opt/etc/uvb76/uvb76.json
   ```

## Uninstalling

```bash
opkg remove uvb76
```

This will:
- Stop the service (best-effort)
- Remove the binary and init script
- Remove example config
- **Preserve** your `/opt/etc/uvb76/uvb76.json` if it exists

To remove all files including config:

```bash
rm -rf /opt/etc/uvb76
rm -rf /opt/var/log/uvb76
```

## Updating

To update to a new version:

1. Stop the current service:
   ```bash
   /opt/etc/init.d/S76uvb76 stop
   ```

2. Download and install the new package:
   ```bash
   opkg install /tmp/uvb76_<new-version>-1_aarch64-3.10.ipk
   ```

3. Start the service:
   ```bash
   /opt/etc/init.d/S76uvb76 start
   ```

Your config file will be preserved between updates.

## File Reference

| Path | Description |
|------|-------------|
| `/opt/bin/uvb76` | UVB-76 binary |
| `/opt/etc/uvb76/uvb76.json` | Runtime configuration |
| `/opt/etc/uvb76/uvb76.json.example` | Example configuration |
| `/opt/etc/init.d/S76uvb76` | Entware init script |
| `/opt/var/log/uvb76/uvb76.log` | Service log file |

## Security Considerations

1. **Use strong passwords** - Generate random passwords, don't use defaults
2. **Restrict access** - Consider firewall rules:
   ```bash
   iptables -A INPUT -p tcp --dport 8443 -s 192.168.1.0/24 -j ACCEPT
   iptables -A INPUT -p tcp --dport 8443 -j DROP
   ```
3. **Rotate TLS certificates** - Regenerate annually
4. **Monitor logs** - Check for unauthorized access attempts

## Getting Help

For issues, please open a GitHub Issue with:
- Router model and firmware version
- Output of `uname -m`
- Output of `opkg print-architecture`
- Relevant log excerpts from `/opt/var/log/uvb76/uvb76.log`
- Steps to reproduce the issue
