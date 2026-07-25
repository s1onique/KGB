#!/usr/bin/env bash
# Build one exact canary image and atomically replace canonical v2 metadata.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

TESTED_COMMIT="$(git rev-parse HEAD)"
TESTED_TREE="$(git rev-parse HEAD^{tree})"
CANARY_SUBTREE="$(git rev-parse 'HEAD:tovarisch/labs/memory/cmd/canary')"
IMAGE_REF="${TOVARISCH_CANARY_IMAGE_REF:-kgb-tovarisch-canary:latest}"
METADATA_OUTPUT="${TOVARISCH_CANARY_METADATA_OUTPUT:-tovarisch/labs/memory/canary-image-build.json}"

BUILD_DIR="$(mktemp -d)"
cleanup() { rm -rf "$BUILD_DIR"; }
trap cleanup EXIT
cp tovarisch/labs/memory/Dockerfile.canary "$BUILD_DIR/Dockerfile"

(
  cd tovarisch/labs/memory
  CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o "$BUILD_DIR/canary" ./cmd/canary
)

BUILDKIT_METADATA=""
if docker buildx version >/dev/null 2>&1; then
  BUILDKIT_METADATA="$BUILD_DIR/buildkit-metadata.json"
  docker buildx build --load --metadata-file "$BUILDKIT_METADATA" \
    --label "org.opencontainers.image.revision=$TESTED_COMMIT" \
    --label "kgb.dev/source-tree=$TESTED_TREE" \
    --label "kgb.dev/canary-source-tree=$CANARY_SUBTREE" \
    --label "kgb.dev/canary-binary-sha256=$(sha256sum "$BUILD_DIR/canary" | awk '{print $1}')" \
    -f "$BUILD_DIR/Dockerfile" -t "$IMAGE_REF" "$BUILD_DIR"
else
  docker build \
    --label "org.opencontainers.image.revision=$TESTED_COMMIT" \
    --label "kgb.dev/source-tree=$TESTED_TREE" \
    --label "kgb.dev/canary-source-tree=$CANARY_SUBTREE" \
    --label "kgb.dev/canary-binary-sha256=$(sha256sum "$BUILD_DIR/canary" | awk '{print $1}')" \
    -f "$BUILD_DIR/Dockerfile" -t "$IMAGE_REF" "$BUILD_DIR"
fi

args=(
  --source-commit "$TESTED_COMMIT"
  --source-tree "$TESTED_TREE"
  --canary-source-tree "$CANARY_SUBTREE"
  --canary-binary "$BUILD_DIR/canary"
  --output "$METADATA_OUTPUT"
)
if [[ -n "$BUILDKIT_METADATA" ]]; then
  args+=(--buildkit-metadata "$BUILDKIT_METADATA")
fi
.factory/bin/extract-image-metadata "${args[@]}" "$IMAGE_REF"

printf 'source_commit=%s\nsource_tree=%s\ncanary_source_tree=%s\nrequested_reference=%s\nmetadata=%s\n' \
  "$TESTED_COMMIT" "$TESTED_TREE" "$CANARY_SUBTREE" "$IMAGE_REF" "$METADATA_OUTPUT"
