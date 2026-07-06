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

echo "[gate] checking memory ownership hygiene in status/request paths"
./scripts/check_memory_ownership.sh

echo "[gate] checking memory ownership hygiene sentinel self-test"
./scripts/check_memory_ownership.sh --self-test

echo "[gate] checking allocation pattern gate (HULK18: enforcing)"
bash scripts/check_allocation_patterns.sh

echo "[gate] checking allocation pattern gate self-test (HULK18)"
bash scripts/check_allocation_patterns.sh --self-test

echo "[gate] checking Zig memory copy safety hygiene"
./scripts/check_zig_memory_copy_safety.py

echo "[gate] checking Zig memory copy safety sentinel self-test"
./scripts/check_zig_memory_copy_safety.py --self-test

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
      *.png|*.jpg|*.jpeg|*.gif|*.ico|*.pdf|*.zip|*.gz|*.tar|*.tgz|*.wasm|uvb76/uvb76-latency-crash-*|*/uvb76-latency-crash-*)
        continue
        ;;
      *node_modules*)
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
  docs/doctrine/ai-native-code-discipline-axioms.md
  docs/doctrine/manifesto_axiom_coverage.csv
  docs/doctrine/git-history-safety.md
  docs/doctrine/shell-containment.md
  docs/doctrine/native-owned-critical-paths.md
  docs/doctrine/embedded-memory-frugality.md
  docs/generated/shell_inventory.csv
  docs/tooling/cli-composition-inventory.csv
  docs/architecture/overview.md
  docs/architecture/naming.md
  docs/architecture/components.md
  docs/epics/kgb-repo-indoctrination.md
  docs/epics/bootstrap-zig-tovarisch-leaf-service.md
  docs/tooling/zig-0.16-field-manual.md
  docs/tooling/zig-memory-copy-safety.md
  docs/tooling/cline-context.md
  docs/tooling/scripts-inventory.md
  docs/coverage/tovarisch-coverage.md
  docs/contracts/tovarisch-status-v0.md
  docs/contracts/examples/tovarisch-status-v0.json
  scripts/verify_tovarisch_status_contract.py
  scripts/verify_manifesto_axiom_coverage.py
  scripts/verify_repo_local_memory.py
  scripts/verify_cold_resume_checkpoints.py
  docs/tooling/cold-resume-checkpoints.md
  docs/reference_allowlists/cold_resume_checkpoint_legacy_allowlist.csv
  docs/memory/budgets/tovarisch-memory-budget.yaml
  docs/memory/budgets/uvb76-memory-budget.yaml
  scripts/verify_memory_budgets.py
  scripts/verify_memory_lab_artifact.py
  AGENTS.md
  .clinerules/00-bootstrap.md
  .clinerules/10-kgb-doctrine.md
  .clinerules/10-git-history-safety.md
  .clinerules/20-zig-016.md
  .clinerules/30-karpathy.md
  .clinerules/30-verification.md
  scripts/git_no_history_rewrite_pre_push.sh
  scripts/install_git_safety_hooks.sh
  scripts/verify_git_history_safety_policy.sh
  scripts/verify_github_no_force_push_ruleset.sh
  scripts/verify_workflow_release_safety.sh
  scripts/verify_workflow_release_safety.py
  docs/architecture/tovarisch-effect-boundary-register.md
  docs/architecture/tovarisch-total-parser-register.md
  docs/architecture/tovarisch-state-transition-register.md
  scripts/verify_state_transition_register.py
  tests/test_verify_state_transition_register.py
  scripts/verify_no_doconly_regression_tests.py
  tests/test_verify_no_doconly_regression_tests.py
  docs/tooling/memory-ownership-inventory.csv
  scripts/verify_memory_ownership_inventory.py
  tests/test_verify_memory_ownership_inventory.py
  scripts/tovarisch_status_rss_canary.py
  tests/test_tovarisch_status_rss_canary.py
)

for path in "${required[@]}"; do
  if [[ ! -s "$path" ]]; then
    echo "[gate] missing or empty: $path" >&2
    exit 1
  fi
done

echo "[gate] checking manifesto axiom coverage matrix"
python3 scripts/verify_manifesto_axiom_coverage.py

echo "[gate] checking manifesto axiom coverage matrix self-test"
python3 scripts/verify_manifesto_axiom_coverage.py --self-test

echo "[gate] checking AXIOM-1 repo-local memory"
python3 scripts/verify_repo_local_memory.py

echo "[gate] checking AXIOM-1 repo-local memory self-test"
python3 scripts/verify_repo_local_memory.py --self-test

echo "[gate] checking AXIOM-2 cold-resume checkpoints"
python3 scripts/verify_cold_resume_checkpoints.py

echo "[gate] checking AXIOM-2 cold-resume checkpoints self-test"
python3 scripts/verify_cold_resume_checkpoints.py --self-test

echo "[gate] checking UVB-76 capture helpers self-test"
bash scripts/verify_uvb76_capture_helpers.sh --self-test
python3 scripts/verify_uvb76_capture_helpers.py --self-test

echo "[gate] checking UVB-76 diag packet contract self-test"
bash scripts/verify_uvb76_diag_packet_contract.sh --self-test

echo "[gate] checking UVB-76 polling (nested Go module)"
(cd uvb76/cmd/uvb76-capture-netns-polling && go build -o uvb76-capture-netns-polling . && go test ./...)

echo "[gate] checking frontend test hygiene"

python3 scripts/verify_frontend_test_hygiene.py

echo "[gate] running frontend tests via bounded wrapper"

if [[ -d "uvb76/web/node_modules" ]]; then
  # Use --kill-stale to clean up any stale processes from previous runs
  ./scripts/run_frontend_tests.sh --kill-stale
else
  echo "[gate] frontend dependencies not installed; skipping frontend tests"
fi

echo "[gate] checking forbidden generic naming"

# Use git grep to search tracked file contents (not filenames) with timeout
# Excludes policy files that document the rule (they will mention the forbidden term)
status=0
timeout 15s git grep -n -i -E 'kgb-agent|KGB agent' -- . \
  ':(exclude)scripts/quality_gate.sh' \
  ':(exclude)scripts/audit_repo_health.sh' \
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

echo "[gate] checking linux_read.zig statx migration (HULK13 regression)"

# Reject reintroduction of stale fstat/Stat API in linux_read.zig
# ACT-HULK13R5-ZIG016-LINUX-READ-STATX-FD-FIX migrated to statx(AT_EMPTY_PATH)
if git grep -n 'std\.os\.linux\.Stat\b' -- tovarisch/src/net/linux_read.zig 2>/dev/null; then
  echo "[gate] FAIL: found std.os.linux.Stat in linux_read.zig — use linux.Statx with AT_EMPTY_PATH" >&2
  exit 1
fi

if git grep -n 'std\.os\.linux\.fstat' -- tovarisch/src/net/linux_read.zig 2>/dev/null; then
  echo "[gate] FAIL: found std.os.linux.fstat in linux_read.zig — use linux.statx with AT_EMPTY_PATH" >&2
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
grep -qi 'git history safety' .clinerules/10-git-history-safety.md

echo "[gate] checking git history safety policy"

./scripts/verify_git_history_safety_policy.sh

echo "[gate] checking split test inventory drift"

./scripts/verify_split_test_inventory.sh

echo "[gate] checking shell containment"

python3 scripts/verify_shell_containment.py

echo "[gate] checking shell containment self-test"

python3 scripts/verify_shell_containment.py --self-test

echo "[gate] checking functional core / effect boundaries"

python3 scripts/verify_effect_boundaries.py

echo "[gate] checking functional core / effect boundary self-test"

python3 scripts/verify_effect_boundaries.py --self-test

echo "[gate] checking total parser / no-panic external input discipline (HULK21)"

python3 scripts/verify_total_parsers.py

echo "[gate] checking total parser verifier self-test (HULK21)"

python3 scripts/verify_total_parsers.py --self-test

echo "[gate] checking state transition register (HULK26)"

python3 scripts/verify_state_transition_register.py

echo "[gate] checking state transition register self-test (HULK26)"

python3 tests/test_verify_state_transition_register.py

echo "[gate] checking doc-only regression tests (HULK29R-ZIG016-MEMOWN03)"

python3 scripts/verify_no_doconly_regression_tests.py

echo "[gate] checking doc-only regression tests self-test (HULK29R-ZIG016-MEMOWN03)"

python3 tests/test_verify_no_doconly_regression_tests.py

echo "[gate] checking memory ownership inventory (HULK29R-ZIG016-MEMOWN04)"

python3 scripts/verify_memory_ownership_inventory.py

echo "[gate] checking memory ownership inventory self-test (HULK29R-ZIG016-MEMOWN04)"

python3 tests/test_verify_memory_ownership_inventory.py

echo "[gate] checking tovarisch status RSS canary self-test (HULK29R-ZIG016-MEMOWN05)"

python3 tests/test_tovarisch_status_rss_canary.py

echo "[gate] checking shell inventory consistency"

python3 scripts/verify_shell_containment.py --check-inventory

echo "[gate] checking CLI composition inventory"

python3 scripts/verify_cli_composition_inventory.py

echo "[gate] checking CLI composition inventory self-test"

python3 scripts/verify_cli_composition_inventory.py --self-test

echo "[gate] checking workflow release safety (no release-in-build)"

./scripts/verify_workflow_release_safety.sh

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

python3 scripts/verify_tovarisch_status_contract.py

echo "[gate] checking structured logs"

./scripts/verify_structured_logs.sh

echo "[gate] checking plaintext logging policy"

./scripts/verify_plaintext_logs.sh

echo "[gate] checking memory budgets schema"

python3 scripts/verify_memory_budgets.py

echo "[gate] checking memory budgets schema self-test"

python3 scripts/verify_memory_budgets.py --self-test

echo "[gate] checking memory lab artifacts schema"

python3 scripts/verify_memory_lab_artifact.py

echo "[gate] checking memory lab artifacts schema self-test"

python3 scripts/verify_memory_lab_artifact.py --self-test

echo "[gate] checking memory attribution matrix verifier self-test"

python3 scripts/verify_memory_attribution_matrix.py --self-test

echo "[gate] checking memory attribution matrix workflow shape self-test"

python3 scripts/verify_memory_matrix_workflow_shape.py --self-test

echo "[gate] checking memory attribution matrix workflow shape"

python3 scripts/verify_memory_matrix_workflow_shape.py \
  --workflow .github/workflows/tovarisch-idle-memory-attribution-matrix.yml

echo "[gate] checking memory allocation ownership hygiene"

bash scripts/check_memory_ownership.sh

echo ""
echo "[gate] PASS"
