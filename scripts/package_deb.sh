#!/usr/bin/env bash
set -euo pipefail

APP_NAME="tovarisch"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOVARISCH_DIR="${SCRIPT_DIR}/../tovarisch"
VERSION="${VERSION:-0.1.1}"
ARCH="${ARCH:-amd64}"
TARGET="${TARGET:-x86_64-linux-gnu}"
DIST_DIR="${SCRIPT_DIR}/../dist"
PKG_ROOT="${DIST_DIR}/pkg/${APP_NAME}_${VERSION}_${ARCH}"
DEB_PATH="${DIST_DIR}/${APP_NAME}_${VERSION}_${ARCH}.deb"

rm -rf "${PKG_ROOT}" "${DEB_PATH}"
mkdir -p \
  "${PKG_ROOT}/usr/bin" \
  "${PKG_ROOT}/lib/systemd/system" \
  "${PKG_ROOT}/DEBIAN"

# Change to tovarisch/ directory so zig build finds build.zig
cd "${TOVARISCH_DIR}"
zig build -Dtarget="${TARGET}" -Doptimize=ReleaseSafe

install -m 0755 "zig-out/bin/${APP_NAME}" "${PKG_ROOT}/usr/bin/${APP_NAME}"
install -m 0644 "${SCRIPT_DIR}/../packaging/systemd/${APP_NAME}.service" "${PKG_ROOT}/lib/systemd/system/${APP_NAME}.service"

cat > "${PKG_ROOT}/DEBIAN/control" <<EOF
Package: ${APP_NAME}
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Maintainer: KGB Project <noreply@example.invalid>
Description: Local-first KGB leaf service
 Rudimentary package for the tovarisch leaf service.
EOF

dpkg-deb --build --root-owner-group "${PKG_ROOT}" "${DEB_PATH}"

dpkg-deb --info "${DEB_PATH}"
dpkg-deb --contents "${DEB_PATH}"

echo "[package] built ${DEB_PATH}"
