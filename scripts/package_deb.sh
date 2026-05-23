#!/usr/bin/env bash
set -euo pipefail

APP_NAME="tovarisch"
VERSION="${VERSION:-0.1.1}"
ARCH="${ARCH:-amd64}"
TARGET="${TARGET:-x86_64-linux-gnu}"
DIST_DIR="${DIST_DIR:-dist}"
PKG_ROOT="${DIST_DIR}/pkg/${APP_NAME}_${VERSION}_${ARCH}"
DEB_PATH="${DIST_DIR}/${APP_NAME}_${VERSION}_${ARCH}.deb"

rm -rf "${PKG_ROOT}" "${DEB_PATH}"
mkdir -p \
  "${PKG_ROOT}/usr/bin" \
  "${PKG_ROOT}/DEBIAN"

zig build -Dtarget="${TARGET}" -Doptimize=ReleaseSafe

install -m 0755 "zig-out/bin/${APP_NAME}" "${PKG_ROOT}/usr/bin/${APP_NAME}"

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
