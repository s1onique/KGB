#!/usr/bin/env bash
# build_opkg_package.sh - Build Entware/opkg package for UVB-76
#
# Creates a proper opkg package (.ipk) containing:
#   /opt/bin/uvb76            - main binary
#   /opt/etc/uvb76/uvb76.json.example - example config
#   /opt/etc/init.d/S76uvb76 - Entware init script
#   /opt/var/log/uvb76/      - log directory
#
# The package uses Entware-compatible outer gzip tar containing debian-binary, data.tar.gz, and control.tar.gz.
set -euo pipefail

# Defaults
PACKAGE_NAME="${PACKAGE_NAME:-uvb76}"
VERSION="${VERSION:-}"
REVISION="${REVISION:-1}"
OPKG_ARCH="${OPKG_ARCH:-aarch64-3.10}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-arm64}"
GOARM="${GOARM:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/dist/opkg}"
# Ensure OUTPUT_DIR is absolute
if [[ "${OUTPUT_DIR}" != /* ]]; then
    OUTPUT_DIR="${ROOT_DIR}/${OUTPUT_DIR}"
fi

# Normalize version - strip leading 'v' if present
if [ -z "${VERSION}" ]; then
    if [ -d "${ROOT_DIR}/.git" ]; then
        VERSION=$(git -C "${ROOT_DIR}" describe --tags --always 2>/dev/null | sed 's/^v//')
    else
        VERSION="0.0.0-dev"
    fi
else
    VERSION="${VERSION#v}"
fi

# Compute derived names
PKG_FILENAME="${PACKAGE_NAME}_${VERSION}-${REVISION}_${OPKG_ARCH}.ipk"
PKG_PATH="${OUTPUT_DIR}/${PKG_FILENAME}"

echo "=== Building opkg package ==="
echo "  Package:    ${PACKAGE_NAME}"
echo "  Version:    ${VERSION}"
echo "  Revision:   ${REVISION}"
echo "  Arch:       ${OPKG_ARCH}"
echo "  Target:     ${GOOS}/${GOARCH}${GOARM:+/}${GOARM}"
echo "  Output:     ${PKG_PATH}"

# Create output directory
mkdir -p "${OUTPUT_DIR}"

# Create temporary package root
PKGROOT=$(mktemp -d)
trap 'rm -rf "${PKGROOT}"' EXIT

# Create package directory structure
mkdir -p "${PKGROOT}/opt/bin"
mkdir -p "${PKGROOT}/opt/etc/uvb76"
mkdir -p "${PKGROOT}/opt/etc/init.d"
mkdir -p "${PKGROOT}/opt/var/log/uvb76"
mkdir -p "${PKGROOT}/CONTROL"

# --- Run tests first (on CI host architecture) ---
echo ""
echo "=== Running tests ==="
cd "${ROOT_DIR}/uvb76"
# Do not inherit GOOS/GOARCH here: go test executes test binaries on the CI host.
if ! env -u GOOS -u GOARCH -u GOARM -u CGO_ENABLED go test -v ./... 2>&1; then
    echo "ERROR: Tests failed" >&2
    exit 1
fi
cd "${ROOT_DIR}"

# --- Build static Go binary (for target architecture) ---
echo ""
echo "=== Building binary ==="
cd "${ROOT_DIR}/uvb76"

build_env=(
    "CGO_ENABLED=0"
    "GOOS=${GOOS}"
    "GOARCH=${GOARCH}"
)
if [ -n "${GOARM}" ]; then
    build_env+=("GOARM=${GOARM}")
fi

env "${build_env[@]}" go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "${PKGROOT}/opt/bin/${PACKAGE_NAME}" .

cd "${ROOT_DIR}"

# Verify binary was created
if [ ! -s "${PKGROOT}/opt/bin/${PACKAGE_NAME}" ]; then
    echo "ERROR: Binary build failed or produced empty file" >&2
    exit 1
fi
chmod +x "${PKGROOT}/opt/bin/${PACKAGE_NAME}"

# --- Copy init script ---
echo ""
echo "=== Including init script ==="
if [ -f "${ROOT_DIR}/packaging/entware/S76${PACKAGE_NAME}" ]; then
    cp "${ROOT_DIR}/packaging/entware/S76${PACKAGE_NAME}" "${PKGROOT}/opt/etc/init.d/"
    chmod +x "${PKGROOT}/opt/etc/init.d/S76${PACKAGE_NAME}"
else
    echo "WARNING: Init script not found at packaging/entware/S76${PACKAGE_NAME}" >&2
fi

# --- Copy example config ---
echo ""
echo "=== Including example config ==="
SOURCE_CONFIG=""
if [ -f "${ROOT_DIR}/uvb76/uvb76.example.json" ]; then
    SOURCE_CONFIG="${ROOT_DIR}/uvb76/uvb76.example.json"
elif [ -f "${ROOT_DIR}/packaging/entware/${PACKAGE_NAME}.json.example" ]; then
    SOURCE_CONFIG="${ROOT_DIR}/packaging/entware/${PACKAGE_NAME}.json.example"
else
    echo "ERROR: Example config not found" >&2
    exit 1
fi

# Install as uvb76.json.example (canonical name per package contract)
install -m 0644 "${SOURCE_CONFIG}" "${PKGROOT}/opt/etc/uvb76/uvb76.json.example"

# --- Create CONTROL/control ---
echo ""
echo "=== Creating control metadata ==="
cat > "${PKGROOT}/CONTROL/control" << CONTROL_EOF
Package: ${PACKAGE_NAME}
Version: ${VERSION}-${REVISION}
Architecture: ${OPKG_ARCH}
Maintainer: KGB Project <kgb@example.com>
Description: UVB-76 - KGB Control Plane Station
Section: net
Priority: optional
CONTROL_EOF

# --- Create CONTROL/conffiles ---
# Mark the example config as a conffile so opkg preserves existing user configs
if [ -f "${PKGROOT}/opt/etc/uvb76/uvb76.json.example" ]; then
    echo "/opt/etc/uvb76/uvb76.json.example" > "${PKGROOT}/CONTROL/conffiles"
fi

# --- Create CONTROL/postinst ---
echo ""
echo "=== Creating postinst script ==="
cat > "${PKGROOT}/CONTROL/postinst" << 'POSTINST_EOF'
#!/bin/sh
# postinst for uvb76 - runs after package installation
set -e

case "$1" in
    configure)
        # Create required directories
        mkdir -p /opt/etc/uvb76
        mkdir -p /opt/var/log/uvb76
        
        # Copy example config only if no config exists
        if [ ! -f /opt/etc/uvb76/uvb76.json ] && [ -f /opt/etc/uvb76/uvb76.json.example ]; then
            cp /opt/etc/uvb76/uvb76.json.example /opt/etc/uvb76/uvb76.json
            echo "Created /opt/etc/uvb76/uvb76.json from example."
            echo "Please edit /opt/etc/uvb76/uvb76.json to configure UVB-76."
        fi
        
        # Ensure binaries are executable
        chmod +x /opt/bin/uvb76 2>/dev/null || true
        chmod +x /opt/etc/init.d/S76uvb76 2>/dev/null || true
        
        echo ""
        echo "=== UVB-76 Installation Complete ==="
        echo ""
        echo "To start the service:"
        echo "  /opt/etc/init.d/S76uvb76 start"
        echo ""
        echo "To check status:"
        echo "  /opt/etc/init.d/S76uvb76 check"
        echo ""
        echo "To stop the service:"
        echo "  /opt/etc/init.d/S76uvb76 stop"
        echo ""
        echo "Configuration: /opt/etc/uvb76/uvb76.json"
        echo "Logs:          /opt/var/log/uvb76/"
        echo ""
        ;;
    abort-upgrade|abort-remove|abort-deconfigure)
        exit 0
        ;;
    *)
        echo "postinst called with unknown argument \`$1'" >&2
        exit 1
        ;;
esac

exit 0
POSTINST_EOF
chmod +x "${PKGROOT}/CONTROL/postinst"

# --- Create CONTROL/prerm ---
echo ""
echo "=== Creating prerm script ==="
cat > "${PKGROOT}/CONTROL/prerm" << 'PRERM_EOF'
#!/bin/sh
# prerm for uvb76 - runs before package removal
# Best-effort service stop - never fails package removal
set -e

case "$1" in
    remove|upgrade|deconfigure)
        # Try to stop the service, but don't fail if it's not running
        if [ -x /opt/etc/init.d/S76uvb76 ]; then
            /opt/etc/init.d/S76uvb76 stop 2>/dev/null || true
        fi
        ;;
    failed-upgrade)
        exit 0
        ;;
    *)
        echo "prerm called with unknown argument \`$1'" >&2
        exit 1
        ;;
esac

exit 0
PRERM_EOF
chmod +x "${PKGROOT}/CONTROL/prerm"

# --- Create data.tar.gz ---
echo ""
echo "=== Creating data.tar.gz ==="
cd "${PKGROOT}"
# Tar from parent dir so opt/ prefix is preserved
tar -czf "${PKGROOT}/data.tar.gz" opt/

# --- Create control.tar.gz ---
echo ""
echo "=== Creating control.tar.gz ==="
cd "${PKGROOT}"
# Canonical opkg layout: root-level files inside control.tar.gz
# (./control, ./postinst, ./prerm, ./conffiles)
# Build from CONTROL directory but use .. prefix for root-level paths
tar -czf control.tar.gz -C CONTROL .

# --- Create debian-binary ---
echo ""
echo "=== Creating debian-binary ==="
echo "2.0" > "${PKGROOT}/debian-binary"

# --- Assemble final .ipk with gzip tar (Entware-compatible format) ---
echo ""
echo "=== Assembling .ipk package ==="
# Use absolute path for output since we've cd'd to PKGROOT
mkdir -p "${OUTPUT_DIR}"
AR_OUT="${OUTPUT_DIR}/$(basename "${PKG_PATH}")"
rm -f "${AR_OUT}"
# Entware ipkg-build creates outer gzip tar, matching OpenEmbedded/Yocto style
# Use portable tar syntax (works on both Linux and macOS)
(
    cd "${PKGROOT}"
    tar cf - ./debian-binary ./data.tar.gz ./control.tar.gz | gzip -n - > "${AR_OUT}"
)

# Verify package was created
if [ ! -s "${PKG_PATH}" ]; then
    echo "ERROR: Package creation failed" >&2
    exit 1
fi

# --- Generate SHA256 checksum ---
echo ""
echo "=== Generating SHA256 checksum ==="
sha256sum "${PKG_PATH}" > "${PKG_PATH}.sha256"

echo ""
echo "=== Package created successfully ==="
echo "  Package: ${PKG_PATH}"
echo "  Size:    $(du -h "${PKG_PATH}" | cut -f1)"
echo "  SHA256:  $(cut -d' ' -f1 "${PKG_PATH}.sha256")"
echo ""
echo "Verifying package structure..."
if command -v "${ROOT_DIR}/scripts/verify_opkg_package.sh" >/dev/null 2>&1; then
    "${ROOT_DIR}/scripts/verify_opkg_package.sh" "${PKG_PATH}"
fi
