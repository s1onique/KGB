#!/usr/bin/env bash
set -euo pipefail

echo "[gate] checking LLM-friendliness"
./scripts/check_llm_friendliness.sh

echo "[gate] checking final newlines"

missing_newline=0
while IFS= read -r f; do
    if [[ -f "$f" && -s "$f" ]]; then
        last_char=$(tail -c1 "$f" | tr -d '\n')
        if [[ -n "$last_char" ]]; then
            echo "[gate] missing final newline: $f" >&2
            missing_newline=1
        fi
    fi
done < <(git ls-files)

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

echo "[gate] checking coverage ledger mentions all public commands"

grep -q '`--help`' docs/coverage/tovarisch-coverage.md
grep -q '`--version`' docs/coverage/tovarisch-coverage.md
grep -q '`check`' docs/coverage/tovarisch-coverage.md
grep -q '`status --json`' docs/coverage/tovarisch-coverage.md

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
echo "[gate:coverage] checking coverage status"

# TODO: When Zig coverage backend matures, wire in real coverage collection.
# For now, we use test execution as the coverage signal.
# See docs/doctrine/day-0-code-coverage.md for philosophy.

if command -v zig >/dev/null 2>&1; then
  if (cd tovarisch && zig build test 2>&1); then
    echo "[INFO] coverage: Zig tests passed — current coverage proxy"
    echo "[INFO] coverage: Zig coverage backend not configured yet"
    echo "[INFO] coverage: See docs/doctrine/day-0-code-coverage.md"
  else
    echo "[gate] FAIL: Zig tests failed" >&2
    exit 1
  fi
else
  echo "[INFO] coverage: Zig not installed; using test-as-signal proxy"
  echo "[INFO] coverage: See docs/doctrine/day-0-code-coverage.md"
fi

echo ""
echo "[gate] checking status contract"

./scripts/verify_tovarisch_status_contract.sh

echo ""
echo "[gate] PASS"
