# Runbook: UVB-76 on ASUSWRT-Merlin

This runbook describes how to deploy and run UVB-76 on ASUSWRT-Merlin routers (ARMv8/arm64).

## Prerequisites

- ASUS router running ASUSWRT-Merlin (>= 386.x)
- Entware-ng installed
- SSH access to router
- USB storage (recommended for swap)

## Installation Steps

### 1. Prepare Environment

SSH into your router:

```bash
ssh admin@router.local
```

Ensure Entware is up to date:

```bash
opkg update
opkg upgrade
```

### 2. Install Go (cross-compile approach)

Since Go cannot run natively on the router efficiently, cross-compile UVB-76 on a development machine:

```bash
# On your development machine
cd KGB/uvb76
make build-linux-arm64
scp uvb76-linux-arm64 admin@router.local:/tmp/uvb76
```

### 3. Generate TLS Certificates

Generate self-signed certificates for HTTPS:

```bash
# On your development machine
mkdir -p /tmp/uvb76-certs
cd /tmp/uvb76-certs

# Generate private key
openssl genrsa -out uvb76.key 2048

# Generate certificate
openssl req -new -x509 -key uvb76.key -out uvb76.crt -days 365 \
  -subj "/CN=uvb76/O=KGB"

# Copy to router
scp uvb76.crt uvb76.key admin@router.local:/tmp/uvb76/
```

### 4. Generate Password Hash

On your development machine:

```go
// Create tools/genhash/main.go
package main

import (
    "bufio"
    "fmt"
    "os"
    "github.com/s1onique/KGB/uvb76/config"
)

func main() {
    fmt.Print("Enter password: ")
    reader := bufio.NewReader(os.Stdin)
    password, _ := reader.ReadString('\n')
    password = password[:len(password)-1] // Remove newline
    
    salt, _ := config.GenerateSalt()
    hash, _ := config.HashPassword(password, salt)
    fmt.Println("\nPassword hash:")
    fmt.Println(hash)
}
```

```bash
cd KGB/uvb76/tools/genhash
go run main.go
# Enter your password
# Copy the generated hash
```

### 5. Create Configuration

Create `uvb76.json` on your router:

```json
{
  "listen": {
    "addr": ":8443",
    "tls_cert_file": "/tmp/uvb76/uvb76.crt",
    "tls_key_file": "/tmp/uvb76/uvb76.key"
  },
  "auth": {
    "username": "admin",
    "password_sha256": "sha256:YOUR_GENERATED_HASH_HERE"
  },
  "scrape": {
    "interval_seconds": 30,
    "timeout_milliseconds": 5000
  },
  "targets": [
    {
      "id": "router-tovarisch",
      "name": "ASUS Router Tovarisch",
      "base_url": "https://127.0.0.1:9080",
      "enabled": true
    }
  ]
}
```

### 6. Run UVB-76

Test run:

```bash
cd /tmp/uvb76
chmod +x uvb76
./uvb76 -config uvb76.json
```

Background run with logging:

```bash
cd /tmp/uvb76
nohup ./uvb76 -config uvb76.json > uvb76.log 2>&1 &
echo $! > uvb76.pid
```

### 7. Set Up Startup Script (Post-Scripts)

Create `/jffs/scripts/post-mount` to start UVB-76 on router boot:

```bash
#!/bin/sh
sleep 5
if [ -x /tmp/uvb76/uvb76 ]; then
    /tmp/uvb76/uvb76 -config /tmp/uvb76/uvb76.json &> /tmp/uvb76/uvb76.log &
    logger "UVB-76 started"
fi
```

Make executable:

```bash
chmod +x /jffs/scripts/post-mount
```

## Monitoring

### Check Status

```bash
# Check if running
ps | grep uvb76

# Check logs
cat /tmp/uvb76/uvb76.log
```

### API Health Check

```bash
curl -k https://localhost:8443/api/v1/healthz
```

### Get Targets (with auth)

```bash
curl -k -u admin:password https://localhost:8443/api/v1/targets
```

### Get Snapshot

```bash
curl -k -u admin:password https://localhost:8443/api/v1/targets/router-tovarisch/snapshot
```

## Troubleshooting

### UVB-76 Won't Start

1. Check config validity:
   ```bash
   /tmp/uvb76/uvb76 -config /tmp/uvb76/uvb76.json 2>&1
   ```

2. Verify TLS certs exist:
   ```bash
   ls -la /tmp/uvb76/uvb76.crt /tmp/uvb76/uvb76.key
   ```

### Connection Refused

1. Check if listening:
   ```bash
   netstat -ln | grep 8443
   ```

2. Check if process running:
   ```bash
   ps | grep uvb76
   ```

### Authentication Failing

1. Verify password hash format:
   ```bash
   cat /tmp/uvb76/uvb76.json | grep password_sha256
   ```
   Should be `sha256:<32-char-salt>:<64-char-hash>`

2. Regenerate password if needed

### Tovarisch Not Scraping

1. Verify tovarisch is running:
   ```bash
   curl http://localhost:9080/status
   ```

2. Check network connectivity:
   ```bash
   wget -O- --timeout=5 https://localhost:9080/status
   ```

## Security Notes

1. **Use strong passwords** - Generate random passwords, don't use "admin"
2. **Rotate TLS certs** - Regenerate annually
3. **Restrict access** - Consider firewall rules:
   ```bash
   iptables -A INPUT -p tcp --dport 8443 -s 192.168.1.0/24 -j ACCEPT
   iptables -A INPUT -p tcp --dport 8443 -j DROP
   ```
4. **Monitor logs** - Check for unauthorized access attempts

## Performance Considerations

- UVB-76 uses ~5-10MB RAM
- CPU usage is minimal (polling every 30s by default)
- Increase `interval_seconds` if many targets to reduce load
- Consider using USB storage for swap if RAM is constrained

## Updating

1. Stop current instance:
   ```bash
   kill $(cat /tmp/uvb76/uvb76.pid)
   ```

2. Deploy new binary:
   ```bash
   scp uvb76-linux-arm64 admin@router.local:/tmp/uvb76/uvb76
   ```

3. Restart:
   ```bash
   cd /tmp/uvb76
   nohup ./uvb76 -config uvb76.json > uvb76.log 2>&1 &
   echo $! > uvb76.pid
   ```
