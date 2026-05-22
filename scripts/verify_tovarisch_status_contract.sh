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
    jq -e '.version == "0.1.1"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: version == 0.1.1"
    jq -e '.node_id == "local-dev"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: node_id == local-dev"
    jq -e '.status == "warn" or .status == "ok"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: status is valid"
    jq -e '.checks[0].name == "process"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: first check is process"
    jq -e '.checks[0].status == "ok"' docs/contracts/examples/tovarisch-status-v0.json >/dev/null && echo "  OK: first check status is ok"

    echo "[status-contract] Fixture fields verified"

    # Check if Zig is available to compare CLI output
    if command -v zig >/dev/null 2>&1; then
        echo "[status-contract] Zig available — comparing CLI output with fixture"

        # Get CLI output and normalize both through jq -c
        cli_output=$(cd tovarisch && zig build run -- status --json 2>/dev/null | jq -c . 2>/dev/null)
        fixture_normalized=$(jq -c . docs/contracts/examples/tovarisch-status-v0.json 2>/dev/null)

        if [[ -z "$cli_output" ]]; then
            echo "[status-contract] FAIL: could not get CLI output" >&2
            exit 1
        fi

        echo "[status-contract] CLI output: $cli_output"
        echo "[status-contract] Fixture normalized: $fixture_normalized"

        if [[ "$cli_output" != "$fixture_normalized" ]]; then
            echo "[status-contract] FAIL: CLI output differs from fixture" >&2
            echo "[status-contract] CLI output: $cli_output" >&2
            echo "[status-contract] Fixture:    $fixture_normalized" >&2
            exit 1
        fi

        echo "[status-contract] CLI output matches fixture"
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
    grep -Eq '"version"[[:space:]]*:[[:space:]]*"0.1.1"' docs/contracts/examples/tovarisch-status-v0.json || {
        echo "[status-contract] FAIL: fixture missing version field" >&2
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
