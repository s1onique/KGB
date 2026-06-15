#!/usr/bin/env bash
# verify_github_no_force_push_ruleset.sh
# Verifies GitHub repository has active ruleset blocking force pushes.
#
# Uses GitHub REST API rules/branches endpoint which returns ALL active rules
# applying to a branch (excludes evaluate/disabled rulesets).
#
# Fails if:
#   - GitHub auth is unavailable (GITHUB_REPOSITORY not set in CI)
#   - non_fast_forward rule is not active
#
# In local development, skips with explicit message if GitHub auth unavailable.
#
# Usage:
#   ./scripts/verify_github_no_force_push_ruleset.sh

set -euo pipefail

REPO="${GITHUB_REPOSITORY:-}"
GH_CLI_AVAILABLE=0

# Check if gh CLI is available
if command -v gh >/dev/null 2>&1; then
    GH_CLI_AVAILABLE=1
fi

# Check if we have GitHub context
if [ -z "$REPO" ]; then
    if [ "$GH_CLI_AVAILABLE" -eq 1 ] && gh auth status >/dev/null 2>&1; then
        REPO=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || echo "")
    fi
fi

if [ -z "$REPO" ]; then
    echo "SKIP: GitHub repository context not available (GITHUB_REPOSITORY not set, not in CI)"
    echo "     To verify, run this script in GitHub Actions or set GITHUB_REPOSITORY"
    exit 0
fi

echo "=== GitHub Force-Push Ruleset Verification ==="
echo "Repository: $REPO"
echo ""

FAILED=0

log_fail() {
    echo "[FAIL] $*" >&2
    FAILED=1
}

log_pass() {
    echo "[PASS] $*"
}

log_info() {
    echo "[INFO] $*"
}

# Get default branch
DEFAULT_BRANCH=$(gh repo view "$REPO" --json defaultBranchRef --jq '.defaultBranchRef.name' 2>/dev/null || echo "main")
log_info "Default branch: $DEFAULT_BRANCH"

# --- Primary check: Use rules/branches endpoint ---
# This returns ALL active rulesets affecting this branch, not just ones targeting it by name
echo ""
echo "Checking branch protection rules (legacy)..."
PROTECTION_JSON=$(gh api "repos/${REPO}/branches/${DEFAULT_BRANCH}/protection" 2>/dev/null || echo "{}")

if echo "$PROTECTION_JSON" | jq -e '.allow_force_pushes == false' >/dev/null 2>&1; then
    log_pass "Branch protection blocks force pushes"
elif echo "$PROTECTION_JSON" | jq -e '.allow_force_pushes == null' >/dev/null 2>&1; then
    # null means using default (usually block for protected branches)
    log_pass "Branch protection uses default (typically blocks force pushes)"
elif echo "$PROTECTION_JSON" | jq -e 'length > 0' >/dev/null 2>&1; then
    # Has protection but allows force pushes
    log_fail "Branch protection allows force pushes"
else
    log_info "No legacy branch protection configured"
fi

# --- Primary check: Use repos/{repo}/rules/branches/{branch} endpoint ---
# This is the authoritative endpoint for getting all rules affecting a branch
echo ""
echo "Checking active rules for branch..."

RULES_JSON=$(gh api "repos/${REPO}/rules/branches/${DEFAULT_BRANCH}" 2>/dev/null || echo "[]")

if [ -z "$RULES_JSON" ] || [ "$RULES_JSON" = "[]" ]; then
    log_fail "No rules found for branch (force pushes may be allowed)"
else
    # Check for non_fast_forward rule
    NON_FF_COUNT=$(echo "$RULES_JSON" | jq '[.[] | select(.type == "non_fast_forward")] | length' 2>/dev/null || echo "0")
    if [ "$NON_FF_COUNT" -gt 0 ]; then
        log_pass "Found $NON_FF_COUNT active non_fast_forward rule(s)"
    else
        log_fail "No non_fast_forward rule found (force pushes are allowed)"
    fi

    # Check for deletion protection (informational)
    DELETION_COUNT=$(echo "$RULES_JSON" | jq '[.[] | select(.type == "deletion")] | length' 2>/dev/null || echo "0")
    if [ "$DELETION_COUNT" -gt 0 ]; then
        log_info "Found $DELETION_COUNT deletion protection rule(s)"
    else
        log_info "No explicit deletion protection (may depend on branch protection)"
    fi

    # Log all active rules for visibility
    ALL_RULES=$(echo "$RULES_JSON" | jq -r '.[].type' 2>/dev/null | sort -u | tr '\n' ' ')
    log_info "Active rule types: $ALL_RULES"
fi

# --- Summary ---
echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "=== GitHub Ruleset Verification PASSED ==="
    exit 0
else
    echo "=== GitHub Ruleset Verification FAILED ===" >&2
    exit 1
fi
