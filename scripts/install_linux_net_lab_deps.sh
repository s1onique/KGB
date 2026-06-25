#!/usr/bin/env bash
# install_linux_net_lab_deps.sh — Prepare Linux network lab dependencies
#
# Installs iproute2 to provide the `ip` command for privileged network labs.
# This script is idempotent: running multiple times is safe.
#
# Usage:
#   ./scripts/install_linux_net_lab_deps.sh
#
# Exit codes:
#   0 - Dependencies satisfied or installed successfully
#   1 - Failed to install dependencies

set -euo pipefail

# Check if already satisfied
if command -v ip >/dev/null 2>&1; then
    echo "=== Linux Network Lab Dependencies ==="
    echo "iproute2 already installed (ip command found)"
    ip -Version 2>&1 | head -1
    exit 0
fi

echo "=== Installing Linux Network Lab Dependencies ==="

# Detect package manager and install iproute2
if command -v apt-get >/dev/null 2>&1; then
    echo "Using apt-get..."
    sudo apt-get update
    sudo apt-get install -y --no-install-recommends iproute2
elif command -v dnf >/dev/null 2>&1; then
    echo "Using dnf..."
    sudo dnf install -y iproute
elif command -v yum >/dev/null 2>&1; then
    echo "Using yum..."
    sudo yum install -y iproute
elif command -v pacman >/dev/null 2>&1; then
    echo "Using pacman..."
    sudo pacman -S --noconfirm iproute2
elif command -v apk >/dev/null 2>&1; then
    echo "Using apk..."
    sudo apk add --no-cache iproute2
else
    echo "ERROR: No supported package manager found (apt-get, dnf, yum, pacman, apk)"
    exit 1
fi

# Verify installation
if command -v ip >/dev/null 2>&1; then
    echo "=== Installation Complete ==="
    echo "ip command verified:"
    ip -Version 2>&1 | head -1
else
    echo "ERROR: ip command not found after installation"
    exit 1
fi
