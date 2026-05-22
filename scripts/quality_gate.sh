#!/usr/bin/env bash
set -euo pipefail

echo "[gate] checking LLM-friendliness"
./scripts/check_llm_friendliness.sh

echo "[gate] checking required docs"

required=(
  README.md
  docs/doctrine/factory.md
  docs/doctrine/kgb.md
  docs/doctrine/privacy.md
  docs/doctrine/tiny-leafs.md
  docs/doctrine/metrics.md
  docs/architecture/overview.md
  docs/architecture/naming.md
  docs/architecture/components.md
  docs/epics/kgb-repo-indoctrination.md
  docs/epics/bootstrap-zig-tovarisch-leaf-service.md
  docs/tooling/zig-0.16-field-manual.md
  docs/tooling/cline-context.md
  AGENTS.md
  .clinerules/00-bootstrap.md
  .clinerules/10-kgb-doctrine.md
  .clinerules/20-zig-016.md
  .clinerules/30-verification.md
)

for path in "${required[@]}"; do
  if [[ ! -s "$path" ]]; then
    echo "[gate] missing or empty: $path" >&2
    exit 1
  fi
done

echo "[gate] checking forbidden generic naming"

if grep -RIn --exclude-dir=.git --exclude='quality_gate.sh' --exclude-dir=.clinerules 'kgb-agent\|KGB agent' .; then
  echo "[gate] avoid kgb-agent naming; use tovarisch" >&2
  exit 1
fi

echo "[gate] checking privacy doctrine exists"

grep -RIn 'browsing history\|visited domains\|destination IP' docs/doctrine/privacy.md >/dev/null

echo "[gate] checking tovarisch Zig package"

if [[ ! -s "tovarisch/build.zig" || ! -s "tovarisch/src/main.zig" ]]; then
  echo "[gate] missing tovarisch Zig package" >&2
  exit 1
fi

echo "[gate] checking Zig 0.16 field manual content"

grep -q 'std.process.Init' docs/tooling/zig-0.16-field-manual.md
grep -q 'std.Io' docs/tooling/zig-0.16-field-manual.md
grep -qi 'do not downgrade' docs/tooling/zig-0.16-field-manual.md

echo "[gate] checking AGENTS.md content"

grep -q 'Zig Learning Protocol' AGENTS.md
grep -qi 'Do not downgrade' AGENTS.md
grep -q 'KGB observes infrastructure health, not people' AGENTS.md

echo "[gate] checking .clinerules content"

grep -q 'AGENTS.md' .clinerules/00-bootstrap.md
grep -q 'docs/tooling/zig-0.16-field-manual.md' .clinerules/20-zig-016.md
grep -q 'make gate' .clinerules/30-verification.md

if command -v zig >/dev/null 2>&1; then
  (
    cd tovarisch
    zig fmt --check build.zig src/main.zig src/cli.zig src/status.zig
    zig build
    zig build test
    zig build run -- --version >/dev/null
    zig build run -- check >/dev/null
    zig build run -- status --json | grep -q '"service":"tovarisch"'
  )
else
  echo "[gate] zig not installed; skipping Zig build/test"
fi

echo "[gate] PASS"