#!/usr/bin/env bash
# scripts/build_tovarisch_canary_image.sh — Build the
# kgb-tovarisch-canary:latest image with immutable OCI + kgb.dev
# provenance labels.
#
# CORRECTION02 §7 binds the canary image to the tested source
# tree via:
#   - org.opencontainers.image.revision (OCI)
#   - kgb.dev/source-tree (repository tree)
#   - kgb.dev/canary-source-tree (canary subtree)
#   - kgb.dev/canary-binary-sha256 (binary hash)
#
# Build the canary binary outside Docker (so the build context
# only contains the binary + a minimal Dockerfile) and assemble
# the image from a tmpfs build dir to avoid leaking the binary
# into the working tree.

set -euo pipefail

# Resolve repo root (parent of scripts/).
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Capture provenance.
TESTED_COMMIT="$(git rev-parse HEAD)"
TESTED_TREE="$(git rev-parse HEAD^{tree})"
CANARY_SUBTREE="$(git rev-parse "HEAD:tovarisch/labs/memory/cmd/canary")"

echo "TESTED_COMMIT=$TESTED_COMMIT"
echo "TESTED_TREE=$TESTED_TREE"
echo "CANARY_SUBTREE=$CANARY_SUBTREE"

# Build the canary binary into a tmp build dir.
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT

cp tovarisch/labs/memory/Dockerfile.canary "$BUILD_DIR/Dockerfile"

(
  cd tovarisch/labs/memory
  CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o "$BUILD_DIR/canary" ./cmd/canary
)

CANARY_SHA256="$(sha256sum "$BUILD_DIR/canary" | awk '{print $1}')"
echo "CANARY_SHA256=$CANARY_SHA256"

# Build the distroless image.
docker build \
  --label "org.opencontainers.image.revision=$TESTED_COMMIT" \
  --label "kgb.dev/source-tree=$TESTED_TREE" \
  --label "kgb.dev/canary-source-tree=$CANARY_SUBTREE" \
  --label "kgb.dev/canary-binary-sha256=$CANARY_SHA256" \
  -f "$BUILD_DIR/Dockerfile" \
  -t kgb-tovarisch-canary:latest \
  "$BUILD_DIR"

echo "=== canary image built: kgb-tovarisch-canary:latest ==="
docker inspect kgb-tovarisch-canary:latest --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' || true
docker inspect kgb-tovarisch-canary:latest --format '{{ index .Config.Labels "kgb.dev/canary-binary-sha256" }}' || true
