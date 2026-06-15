#!/usr/bin/env bash
# verify_git_history_safety_policy.sh
# Verifies the git history safety policy is properly configured.
#
# Checks:
#   1. Doctrine doc exists
#   2. Cline rule exists
#   3. Hook script exists and is executable
#   4. Hook self-test passes
#   5. No forbidden push patterns outside allowlisted files
#
# Usage:
#   ./scripts/verify_git_history_safety_policy.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
FAILED=0

log_fail() {
    echo "[FAIL] $*" >&2
    FAILED=1
}

log_pass() {
    echo "[PASS] $*"
}

echo "=== Git History Safety Policy Verification ==="
echo ""

cd "$REPO_ROOT"

# --- Check 1: Doctrine doc exists ---
if [ -s "docs/doctrine/git-history-safety.md" ]; then
    log_pass "Doctrine doc exists and is non-empty"
else
    log_fail "Doctrine doc missing or empty: docs/doctrine/git-history-safety.md"
fi

# --- Check 2: Cline rule exists ---
if [ -s ".clinerules/10-git-history-safety.md" ]; then
    log_pass "Cline rule exists and is non-empty"
else
    log_fail "Cline rule missing or empty: .clinerules/10-git-history-safety.md"
fi

# --- Check 3: Hook script exists and is executable ---
if [ -f "scripts/git_no_history_rewrite_pre_push.sh" ] && [ -x "scripts/git_no_history_rewrite_pre_push.sh" ]; then
    log_pass "Hook script exists and is executable"
else
    log_fail "Hook script missing or not executable: scripts/git_no_history_rewrite_pre_push.sh"
fi

# --- Check 4: Hook self-test passes ---
echo ""
echo "Running hook self-test..."
if ./scripts/git_no_history_rewrite_pre_push.sh --self-test >/dev/null 2>&1; then
    log_pass "Hook self-test passed"
else
    log_fail "Hook self-test failed"
fi

# --- Check 5: No forbidden push patterns outside allowlisted files ---
echo ""
echo "Scanning for forbidden push patterns..."

# Allowlisted files that document the policy (these are expected to contain examples)
ALLOWLIST=(
    "docs/doctrine/git-history-safety.md"
    ".clinerules/10-git-history-safety.md"
    "scripts/verify_git_history_safety_policy.sh"
    "scripts/git_no_history_rewrite_pre_push.sh"
)

FORBIDDEN_PATTERNS=(
    'git push --force'
    'git push -f'
    'git push --force-with-lease'
    'git push --force-if-includes'
    'git push --mirror'
    'git push --delete'
    'git push --no-verify'
    'git push origin :'
    'git push origin \+'
    'git push.* \+refs/'
    'git push.* \+refs/heads/'
)

# Scan tracked AND untracked files
ALL_FILES="$({
    git ls-files
    git ls-files --others --exclude-standard
} | sort -u)"

FOUND_VIOLATIONS=0
for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
    # Search all files
    while IFS= read -r f; do
        [ -n "$f" ] || continue
        [ -f "$f" ] || continue

        # Check if allowlisted
        ALLOWED=0
        for allowed in "${ALLOWLIST[@]}"; do
            if [ "$f" = "$allowed" ]; then
                ALLOWED=1
                break
            fi
        done
        [ "$ALLOWED" -eq 1 ] && continue

        if grep -q -E "$pattern" "$f" 2>/dev/null; then
            echo "  [FAIL] Forbidden pattern '$pattern' found in: $f" >&2
            FOUND_VIOLATIONS=1
        fi
    done <<EOF
$ALL_FILES
EOF
done

if [ "$FOUND_VIOLATIONS" -eq 0 ]; then
    log_pass "No forbidden push patterns outside allowlisted files"
else
    log_fail "Forbidden push patterns found in non-allowlisted files"
fi

# --- Summary ---
echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "=== Verification PASSED ==="
    exit 0
else
    echo "=== Verification FAILED ===" >&2
    exit 1
fi
