#!/usr/bin/env bash
# scripts/build_tovarisch_canary_image.sh — Build the
# kgb-tovarisch-canary:latest image with immutable OCI + kgb.dev
# provenance labels, and write a `canary-image-build.json` file
# containing the pre-build canary binary hash and the resolved
# provenance labels.
#
# CORRECTION03 §3-§5: the pre-build canary binary SHA-256 is
# written to a sidecar JSON the producer reads, and the
# pre-build/extracted/label hashes are compared in the verifier.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

TESTED_COMMIT="$(git rev-parse HEAD)"
TESTED_TREE="$(git rev-parse HEAD^{tree})"
CANARY_SUBTREE="$(git rev-parse "HEAD:tovarisch/labs/memory/cmd/canary")"

echo "TESTED_COMMIT=$TESTED_COMMIT"
echo "TESTED_TREE=$TESTED_TREE"
echo "CANARY_SUBTREE=$CANARY_SUBTREE"

BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT

cp tovarisch/labs/memory/Dockerfile.canary "$BUILD_DIR/Dockerfile"

(
  cd tovarisch/labs/memory
  CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o "$BUILD_DIR/canary" ./cmd/canary
)

PREBUILD_BINARY_SHA256="$(sha256sum "$BUILD_DIR/canary" | awk '{print $1}')"
echo "PREBUILD_BINARY_SHA256=$PREBUILD_BINARY_SHA256"

IMAGE_REF="kgb-tovarisch-canary:latest"
docker build \
  --label "org.opencontainers.image.revision=$TESTED_COMMIT" \
  --label "kgb.dev/source-tree=$TESTED_TREE" \
  --label "kgb.dev/canary-source-tree=$CANARY_SUBTREE" \
  --label "kgb.dev/canary-binary-sha256=$PREBUILD_BINARY_SHA256" \
  -f "$BUILD_DIR/Dockerfile" \
  -t "$IMAGE_REF" \
  "$BUILD_DIR"

# Capture actual image inspect output (RepoDigests, Id, Labels).
INSPECT_JSON="$(docker image inspect "$IMAGE_REF" --format '{{json .}}')"
IMAGE_ID_FROM_INSPECT="$(echo "$INSPECT_JSON" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("Id",""))')"
REPO_DIGESTS_JSON="$(echo "$INSPECT_JSON" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(json.dumps(d.get("RepoDigests",[])))')"

# Write the build metadata sidecar. The producer reads this,
# extracts /app/canary from the image, compares hashes, and
# copies the values into the canonical manifest's
# `subject_image_identity` block.
cat > tovarisch/labs/memory/canary-image-build.json <<EOF
{
  "image_reference": "$IMAGE_REF",
  "image_id": "$IMAGE_ID_FROM_INSPECT",
  "repo_digests": $REPO_DIGESTS_JSON,
  "source_commit_oid": "$TESTED_COMMIT",
  "repository_tree_oid": "$TESTED_TREE",
  "canary_source_subtree_oid": "$CANARY_SUBTREE",
  "prebuild_binary_sha256": "$PREBUILD_BINARY_SHA256"
}
EOF

echo "=== canary image built: $IMAGE_REF ==="
echo "image_id: $IMAGE_ID_FROM_INSPECT"
echo "prebuild_binary_sha256: $PREBUILD_BINARY_SHA256"
echo "build metadata: tovarisch/labs/memory/canary-image-build.json"
