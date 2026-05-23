#!/usr/bin/env bash
# Verify that the Debian package includes the systemd unit with correct content.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/../dist"

# Find the most recent .deb file
DEB_PATH=$(ls -t "${DIST_DIR}"/*.deb 2>/dev/null | head -1)

if [[ -z "${DEB_PATH}" ]]; then
    echo "[verify-deb-systemd] No .deb found in ${DIST_DIR}. Run 'make package-deb' first."
    exit 1
fi

echo "[verify-deb-systemd] Inspecting: ${DEB_PATH}"

# Extract to temp directory for inspection
TMP_DIR=$(mktemp -d)
trap "rm -rf '${TMP_DIR}'" EXIT

dpkg-deb -x "${DEB_PATH}" "${TMP_DIR}"

# Verify package contains required files
echo "[verify-deb-systemd] Checking package contents..."

# Check binary exists
if ! dpkg-deb -c "${DEB_PATH}" | grep -q 'usr/bin/tovarisch$'; then
    echo "[verify-deb-systemd] FAIL: usr/bin/tovarisch not found in package"
    exit 1
fi
echo "[verify-deb-systemd] OK: usr/bin/tovarisch found"

# Check service file exists
if ! dpkg-deb -c "${DEB_PATH}" | grep -q 'tovarisch\.service$'; then
    echo "[verify-deb-systemd] FAIL: tovarisch.service not found in package"
    exit 1
fi
echo "[verify-deb-systemd] OK: tovarisch.service found"

SERVICE_FILE="${TMP_DIR}/lib/systemd/system/tovarisch.service"
if [[ ! -f "${SERVICE_FILE}" ]]; then
    SERVICE_FILE="${TMP_DIR}/usr/lib/systemd/system/tovarisch.service"
fi

if [[ ! -f "${SERVICE_FILE}" ]]; then
    echo "[verify-deb-systemd] FAIL: service file not extracted"
    exit 1
fi

echo "[verify-deb-systemd] Checking service file hardening options..."

# Verify service contains required hardening
check_required() {
    local pattern="$1"
    local description="$2"
    if grep -qF "${pattern}" "${SERVICE_FILE}"; then
        echo "[verify-deb-systemd] OK: ${description}"
    else
        echo "[verify-deb-systemd] FAIL: ${description} not found"
        exit 1
    fi
}

check_required 'ExecStart=/usr/bin/tovarisch serve' 'ExecStart command'
check_required 'NoNewPrivileges=true' 'NoNewPrivileges'
check_required 'PrivateTmp=true' 'PrivateTmp'
check_required 'ProtectSystem=strict' 'ProtectSystem'
check_required 'CapabilityBoundingSet=' 'CapabilityBoundingSet (empty)'
check_required 'AmbientCapabilities=' 'AmbientCapabilities (empty)'

echo "[verify-deb-systemd] Checking ACT 1 intentional exclusions..."

# Verify ACT 1 intentionally does NOT contain User/Group
check_absent() {
    local pattern="$1"
    local description="$2"
    if grep -q "^${pattern}" "${SERVICE_FILE}"; then
        echo "[verify-deb-systemd] FAIL: ${description} should NOT be present in ACT 1"
        exit 1
    else
        echo "[verify-deb-systemd] OK: ${description} absent (deferred to ACT 2)"
    fi
}

check_absent 'User=' 'User=tovarisch'
check_absent 'Group=' 'Group=tovarisch'

echo "[verify-deb-systemd] === All checks passed ==="
