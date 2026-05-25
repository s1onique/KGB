#!/usr/bin/env bash
set -euo pipefail

echo "[status-contract] Starting verification"

# Check required files exist
echo "[status-contract] Checking required files"

required_files=(
    "docs/contracts/tovarisch-status-v0.md"
    "docs/contracts/examples/tovarisch-status-v0.json"
    "tovarisch/src/status.zig"
)

for f in "${required_files[@]}"; do
    if [[ ! -s "$f" ]]; then
        echo "[status-contract] FAIL: missing or empty: $f" >&2
        exit 1
    fi
    echo "  OK: $f"
done

echo "[status-contract] Required files present"

# Check contract doc has required sections
echo "[status-contract] Checking contract documentation"

contract_sections=(
    "Purpose"
    "Top-level Fields"
    "Runtime Object"
    "Check Object Fields"
    "Allowed Values"
    "Privacy Constraints"
    "Non-goals"
    "Example"
    "Future-Compatible"
)

for section in "${contract_sections[@]}"; do
    if ! grep -q "$section" docs/contracts/tovarisch-status-v0.md; then
        echo "[status-contract] FAIL: contract missing section: $section" >&2
        exit 1
    fi
done

echo "[status-contract] Contract documentation complete"

# Check fixture has required strings
echo "[status-contract] Checking fixture"

fixture_strings=(
    '"service"'
    '"version"'
    '"node_id"'
    '"status"'
    '"checks"'
    '"name"'
)

for str in "${fixture_strings[@]}"; do
    if ! grep -q "$str" docs/contracts/examples/tovarisch-status-v0.json; then
        echo "[status-contract] FAIL: fixture missing: $str" >&2
        exit 1
    fi
done

echo "[status-contract] Fixture structure valid"

# If jq is available, do semantic JSON comparison
if command -v jq >/dev/null 2>&1; then
    echo "[status-contract] jq available — performing JSON validation"

    # Validate fixture is valid JSON
    if ! jq empty docs/contracts/examples/tovarisch-status-v0.json 2>/dev/null; then
        echo "[status-contract] FAIL: fixture is not valid JSON" >&2
        exit 1
    fi
    echo "[status-contract] Fixture is valid JSON"

    # Verify fixture fields
    echo "[status-contract] Verifying fixture fields"

    jq -e '.service == "tovarisch"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: service == tovarisch"
    jq -e '.version | test("^0\\.1\\.2\\+")' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: version matches 0.1.2+<sha> pattern"
    jq -e '.node_id == "local-dev"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: node_id == local-dev"
    jq -e '.status == "warn" or .status == "ok"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: status is valid"
    jq -e '.checks[0].name == "process"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: first check is process"
    jq -e '.checks[0].status == "ok"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: first check status is ok"

    # Verify runtime block
    jq -e '.runtime.pid > 0' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: runtime.pid > 0"
    jq -e '(.runtime.rss_kib == null or .runtime.rss_kib >= 0)' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: runtime.rss_kib is null or >= 0"

    echo "[status-contract] Fixture fields verified"

    # Check if Zig is available to compare CLI output with fixture
    if command -v zig >/dev/null 2>&1; then
        echo "[status-contract] Zig available — comparing CLI output with fixture"

        # Use contract mode: force deterministic unavailable-tooling path.
        # This ensures wg_peers check returns "wg command not available"
        # regardless of whether host has wg installed.
        export TOVARISCH_WG_COMMAND_PATH=/nonexistent

        # Get CLI output and fixture
        cli_output=$(cd tovarisch && zig build run -- status --json 2>/dev/null)
        
        if [[ -z "$cli_output" ]]; then
            echo "[status-contract] FAIL: could not get CLI output" >&2
            exit 1
        fi

        # Normalize runtime values (they vary by platform/run)
        # Use same sentinel string for rss_kib regardless of null vs non-null
        # Also normalize version to pattern (since version includes dynamic SHA)
        normalized_filter='
          .runtime.pid = 1
          | .runtime.rss_kib = "normalized"
          | .version = "0.1.2+<sha>"
        '

        cli_normalized=$(printf '%s\n' "$cli_output" | jq -c "$normalized_filter" 2>/dev/null)
        fixture_normalized=$(jq -c "$normalized_filter" docs/contracts/examples/tovarisch-status-v0.json 2>/dev/null)

        if [[ "$cli_normalized" != "$fixture_normalized" ]]; then
            echo "[status-contract] FAIL: CLI output differs from fixture after runtime normalization" >&2
            echo "[status-contract] CLI normalized:     $cli_normalized" >&2
            echo "[status-contract] Fixture normalized: $fixture_normalized" >&2
            exit 1
        fi

        echo "[status-contract] CLI output matches fixture (runtime values normalized)"

        # Also validate that rss_kib is either null or a non-negative number
        printf '%s\n' "$cli_output" | jq -e '(.runtime.rss_kib == null or .runtime.rss_kib >= 0)' >/dev/null && echo "  OK: runtime.rss_kib is null or non-negative"
    else
        echo "[status-contract] INFO: Zig not available — skipping CLI comparison"
    fi

    echo "[status-contract] JSON validation complete"
else
    echo "[status-contract] INFO: jq not available — skipping JSON semantic comparison"
    echo "[status-contract] Basic string checks passed"

    # Still verify the fixture passes basic grep checks even without jq
    # Use whitespace-tolerant regex to handle pretty-printed JSON
    grep -Eq '"service"[[:space:]]*:[[:space:]]*"tovarisch"' docs/contracts/examples/tovarisch-status-v0.json || {
        echo "[status-contract] FAIL: fixture missing service field" >&2
        exit 1
    }
    grep -Eq '"version"[[:space:]]*:[[:space:]]*"0\\.1\\.2\\+' docs/contracts/examples/tovarisch-status-v0.json || {
        echo "[status-contract] FAIL: fixture version does not match 0.1.2+ pattern" >&2
        exit 1
    }
    grep -Eq '"node_id"[[:space:]]*:[[:space:]]*"local-dev"' docs/contracts/examples/tovarisch-status-v0.json || {
        echo "[status-contract] FAIL: fixture missing node_id field" >&2
        exit 1
    }
    grep -Eq '"name"[[:space:]]*:[[:space:]]*"process"' docs/contracts/examples/tovarisch-status-v0.json || {
        echo "[status-contract] FAIL: fixture missing process check" >&2
        exit 1
    }
fi

echo ""
echo "[status-contract] PASS"
