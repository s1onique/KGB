#!/usr/bin/env bash
# Verify that the Debian package includes systemd unit and maintainer scripts.
# Static verification only - no root, no install, no service start.
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

# Extract to temp directories for inspection
TMP_DIR=$(mktemp -d)
trap "rm -rf '${TMP_DIR}'" EXIT

DATA_DIR="${TMP_DIR}/data"
CONTROL_DIR="${TMP_DIR}/control"
mkdir -p "${DATA_DIR}" "${CONTROL_DIR}"

# Extract data archive (payload files)
dpkg-deb -x "${DEB_PATH}" "${DATA_DIR}"

# Extract control archive (maintainer scripts)
dpkg-deb -e "${DEB_PATH}" "${CONTROL_DIR}"

echo "[verify-deb-systemd] Checking package data contents..."

# Check binary exists in data archive
if ! dpkg-deb -c "${DEB_PATH}" | grep -q 'usr/bin/tovarisch$'; then
    echo "[verify-deb-systemd] FAIL: usr/bin/tovarisch not found in package"
    exit 1
fi
echo "[verify-deb-systemd] OK: usr/bin/tovarisch found"

# Check service file exists in data archive
if ! dpkg-deb -c "${DEB_PATH}" | grep -q 'tovarisch\.service$'; then
    echo "[verify-deb-systemd] FAIL: tovarisch.service not found in package"
    exit 1
fi
echo "[verify-deb-systemd] OK: tovarisch.service found"

# Locate service file in data archive
SERVICE_FILE="${DATA_DIR}/lib/systemd/system/tovarisch.service"
if [[ ! -f "${SERVICE_FILE}" ]]; then
    SERVICE_FILE="${DATA_DIR}/usr/lib/systemd/system/tovarisch.service"
fi

if [[ ! -f "${SERVICE_FILE}" ]]; then
    echo "[verify-deb-systemd] FAIL: service file not extracted"
    exit 1
fi

echo "[verify-deb-systemd] Checking service file..."

# Verify service contains required configuration
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
check_required 'User=tovarisch' 'User=tovarisch'
check_required 'Group=tovarisch' 'Group=tovarisch'
check_required 'NoNewPrivileges=true' 'NoNewPrivileges'
check_required 'PrivateTmp=true' 'PrivateTmp'
check_required 'ProtectSystem=strict' 'ProtectSystem'
check_required 'CapabilityBoundingSet=' 'CapabilityBoundingSet (empty)'
check_required 'AmbientCapabilities=' 'AmbientCapabilities (empty)'

echo "[verify-deb-systemd] Checking control archive (maintainer scripts)..."

# Verify maintainer scripts exist and are executable in control archive
for script in postinst prerm postrm; do
    SCRIPT_PATH="${CONTROL_DIR}/${script}"
    if [[ ! -f "${SCRIPT_PATH}" ]]; then
        echo "[verify-deb-systemd] FAIL: control/${script} not found in control archive"
        exit 1
    fi
    echo "[verify-deb-systemd] OK: control/${script} found"

    if [[ ! -x "${SCRIPT_PATH}" ]]; then
        echo "[verify-deb-systemd] FAIL: control/${script} is not executable"
        exit 1
    fi
    echo "[verify-deb-systemd] OK: control/${script} is executable"
done

echo "[verify-deb-systemd] Checking postinst content..."

# Verify postinst contains user/group creation logic
POSTINST="${CONTROL_DIR}/postinst"
check_file_contains() {
    local pattern="$1"
    local description="$2"
    if grep -q "${pattern}" "${POSTINST}"; then
        echo "[verify-deb-systemd] OK: postinst ${description}"
    else
        echo "[verify-deb-systemd] FAIL: postinst ${description} not found"
        exit 1
    fi
}

check_file_contains 'getent group tovarisch' 'getent group tovarisch'
check_file_contains 'getent passwd tovarisch' 'getent passwd tovarisch'
check_file_contains 'addgroup --system tovarisch' 'addgroup --system tovarisch'
check_file_contains 'adduser --system' 'adduser --system'
check_file_contains 'systemctl daemon-reload' 'systemctl daemon-reload'

# Verify postinst does NOT auto-enable or auto-start
if grep -q 'systemctl enable' "${POSTINST}"; then
    echo "[verify-deb-systemd] FAIL: postinst should NOT contain 'systemctl enable'"
    exit 1
fi
echo "[verify-deb-systemd] OK: postinst does not contain 'systemctl enable'"

if grep -q 'systemctl start' "${POSTINST}"; then
    echo "[verify-deb-systemd] FAIL: postinst should NOT contain 'systemctl start'"
    exit 1
fi
echo "[verify-deb-systemd] OK: postinst does not contain 'systemctl start'"

echo "[verify-deb-systemd] Checking postrm content..."

# Verify postrm contains daemon-reload
POSTRM="${CONTROL_DIR}/postrm"
if ! grep -q 'systemctl daemon-reload' "${POSTRM}"; then
    echo "[verify-deb-systemd] FAIL: postrm should contain 'systemctl daemon-reload'"
    exit 1
fi
echo "[verify-deb-systemd] OK: postrm contains 'systemctl daemon-reload'"

echo "[verify-deb-systemd] Checking sample config as conffile..."

# Check config file exists in data archive
if ! dpkg-deb -c "${DEB_PATH}" | grep -q '/etc/kgb/tovarisch\.conf$'; then
    echo "[verify-deb-systemd] FAIL: /etc/kgb/tovarisch.conf not found in package"
    exit 1
fi
echo "[verify-deb-systemd] OK: /etc/kgb/tovarisch.conf found in package"

# Check conffile metadata
if ! dpkg-deb -I "${DEB_PATH}" conffiles | grep -q '^/etc/kgb/tovarisch\.conf$'; then
    echo "[verify-deb-systemd] FAIL: /etc/kgb/tovarisch.conf not in conffiles metadata"
    exit 1
fi
echo "[verify-deb-systemd] OK: /etc/kgb/tovarisch.conf marked as conffile"

echo "[verify-deb-systemd] === All checks passed ==="
exit 0
