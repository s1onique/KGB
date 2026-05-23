#!/usr/bin/env bash
set -euo pipefail

# Parse --hygiene-only flag
HYGIENE_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --hygiene-only) HYGIENE_ONLY=1 ;;
  esac
done

if [[ "$HYGIENE_ONLY" -eq 1 ]]; then
  echo "[gate:hygiene] starting hygiene-only gate"
fi

echo "[gate] checking LLM-friendliness"
./scripts/check_llm_friendliness.sh

echo "[gate] checking final newlines"

missing_newline=0

# Scan both tracked and untracked/new files to catch files that escape locally but fail in CI
# when the patch is materialized. This mirrors LLM-friendliness behavior.
files="$(
  {
    git ls-files
    git ls-files --others --exclude-standard
  } | sort -u
)"

while IFS= read -r f; do
    [ -n "$f" ] || continue
    [ -f "$f" ] || continue

    # Skip binary and generated file types
    case "$f" in
      .git/*|zig-cache/*|zig-out/*|.zig-cache/*|coverage/*|kcov-output/*)
        continue
        ;;
      *.png|*.jpg|*.jpeg|*.gif|*.ico|*.pdf|*.zip|*.gz|*.tar|*.tgz|*.wasm)
        continue
        ;;
    esac

    # Empty files are okay
    [ -s "$f" ] || continue

    # Check if last byte is a newline
    if [[ "$(tail -c 1 "$f" | wc -l | tr -d ' ')" != "1" ]]; then
        echo "[gate] missing final newline: $f" >&2
        missing_newline=1
    fi
done <<EOF
$files
EOF

if [[ "$missing_newline" -eq 1 ]]; then
    echo "[gate] fix: append newline to files above" >&2
    exit 1
fi

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
  docs/tooling/scripts-inventory.md
  docs/coverage/tovarisch-coverage.md
  docs/contracts/tovarisch-status-v0.md
  docs/contracts/examples/tovarisch-status-v0.json
  scripts/verify_tovarisch_status_contract.sh
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

# Use git grep to search tracked file contents (not filenames) with timeout
# Excludes policy files that document the rule (they will mention the forbidden term)
status=0
timeout 15s git grep -n -i -E 'kgb-agent|KGB agent' -- . \
  ':(exclude)scripts/quality_gate.sh' \
  ':(exclude).clinerules/' || status=$?

if [[ "$status" -eq 1 ]]; then
  :  # git grep returns 1 when no matches found — this is success
elif [[ "$status" -eq 124 ]]; then
  echo "[gate] FAIL: forbidden generic naming check timed out" >&2
  exit 1
else
  echo "[gate] FAIL: forbidden generic naming check: found 'kgb-agent' or 'KGB agent' in tracked file contents" >&2
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

echo "[gate] checking forbidden Zig dev toolchain pin"

if grep -R "0\.16\.0-dev\.732" .github scripts Makefile 2>/dev/null; then
  echo "[gate] FAIL: forbidden Zig dev toolchain pin found" >&2
  exit 2
fi

echo "[gate] checking coverage ledger mentions all public commands"

grep -q '`--help`' docs/coverage/tovarisch-coverage.md
grep -q '`--version`' docs/coverage/tovarisch-coverage.md
grep -q '`check`' docs/coverage/tovarisch-coverage.md
grep -q '`status --json`' docs/coverage/tovarisch-coverage.md

if [[ "$HYGIENE_ONLY" -eq 1 ]]; then
  echo ""
  echo "[gate:hygiene] hygiene-only gate PASS"
  exit 0
fi

if command -v zig >/dev/null 2>&1; then
  (
    cd tovarisch
    zig fmt --check build.zig src/main.zig src/cli.zig src/status.zig
    zig build
    zig build test
    zig build run -- --version >/dev/null
    zig build run -- check >/dev/null
    zig build run -- status --json | ../scripts/verify_status_json.sh
  )
else
  echo "[gate] zig not installed; skipping Zig build/test"
fi

echo ""
echo "[gate:coverage] running real line coverage gate"

# Real coverage gate using kcov (fails if missing unless ALLOW_MISSING_KCOV=1)
# The gate also checks behavior coverage ledger exists and mentions all public commands
if ! ./scripts/coverage_gate.sh; then
  echo "[gate] FAIL: coverage gate failed" >&2
  exit 1
fi

echo ""
echo "[gate] checking status contract"

./scripts/verify_tovarisch_status_contract.sh

echo "[gate] checking structured logs"

./scripts/verify_structured_logs.sh

echo ""
echo "[gate] PASS"
