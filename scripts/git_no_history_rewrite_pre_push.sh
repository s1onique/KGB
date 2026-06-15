#!/usr/bin/env bash
# git_no_history_rewrite_pre_push.sh
# Pre-push hook to prevent force pushes, branch deletions, tag rewrites, and tag deletions.
#
# This hook inspects actual ref updates rather than parsing command-line flags,
# catching --force, --force-with-lease, +refspec, aliases, and UI/tool variations.
#
# Usage:
#   ./git_no_history_rewrite_pre_push.sh           # Run as pre-push hook
#   ./git_no_history_rewrite_pre_push.sh --self-test  # Run self-test suite

set -eu

# The all-zeroes SHA representing "no object" (new ref creation or deletion)
ZERO_SHA="0000000000000000000000000000000000000000"

# --- Main Hook Logic ---
check_ref_update() {
    local local_ref="$1"
    local local_oid="$2"
    local remote_ref="$3"
    local remote_oid="$4"

    # Only process heads and tags
    case "$remote_ref" in
        refs/heads/*|refs/tags/*) ;;
        *) return 0 ;;
    esac

    # Detect deletion attempts (local_oid is all zeroes)
    if [ "$local_oid" = "$ZERO_SHA" ]; then
        echo "ERROR: deleting remote refs is forbidden: $remote_ref" >&2
        return 1
    fi

    # Skip new branch/tag creation (remote_oid is all zeroes)
    if [ "$remote_oid" = "$ZERO_SHA" ]; then
        return 0
    fi

    case "$remote_ref" in
        refs/heads/*)
            # Check for non-fast-forward using merge-base
            if ! git merge-base --is-ancestor "$remote_oid" "$local_oid"; then
                echo "ERROR: non-fast-forward push is forbidden: $remote_ref" >&2
                echo "Use a new branch or open a PR. Do not force-push." >&2
                return 1
            fi
            ;;
        refs/tags/*)
            # Reject tag rewrites (local_oid differs from remote_oid)
            if [ "$remote_oid" != "$local_oid" ]; then
                echo "ERROR: tag rewrite is forbidden: $remote_ref" >&2
                return 1
            fi
            ;;
    esac

    return 0
}

# --- Self-Test Mode ---
run_self_test() {
    # Resolve the script's own path (not $0 which may be relative)
    SELF_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

    # Capture exit code for final reporting
    TEST_PASS=0
    TEST_FAIL=0

    report_test() {
        local result="$1"
        local description="$2"
        if [ "$result" -eq 0 ]; then
            echo "[PASS] $description"
            TEST_PASS=$((TEST_PASS + 1))
        else
            echo "[FAIL] $description"
            TEST_FAIL=$((TEST_FAIL + 1))
        fi
    }

    echo "=== Git History Rewrite Pre-Push Hook Self-Test ==="
    echo ""

    # --- Test 1: Normal new branch creation should succeed (remote_oid is zero) ---
    result=0
    check_ref_update "refs/heads/feature" "abc123" "refs/heads/feature" "$ZERO_SHA" 2>/dev/null || result=$?
    report_test $result "New branch creation allowed (remote_oid=zero)"

    # --- Test 2: Normal tag creation should succeed (remote_oid is zero) ---
    result=0
    check_ref_update "refs/tags/v1.0" "abc123" "refs/tags/v1.0" "$ZERO_SHA" 2>/dev/null || result=$?
    report_test $result "New tag creation allowed (remote_oid=zero)"

    # --- Test 3: Non-ref updates should be skipped ---
    result=0
    check_ref_update "refs/pull/1/head" "abc123" "refs/pull/1/head" "def456" 2>/dev/null || result=$?
    report_test $result "Non-head/tag refs skipped"

    # --- Test 4: Fast-forward push should succeed ---
    # Create temp bare repo and do a real fast-forward push
    TEST_DIR=$(mktemp -d)
    ORIGIN_DIR="$TEST_DIR/origin.git"
    LOCAL_DIR="$TEST_DIR/local"
    mkdir -p "$ORIGIN_DIR"
    git init --bare -q "$ORIGIN_DIR" 2>/dev/null
    mkdir -p "$LOCAL_DIR"
    cd "$LOCAL_DIR"
    git init -q 2>/dev/null
    git config user.email "test@example.com"
    git config user.name "Test User"
    echo "initial" > file.txt
    git add file.txt
    git commit -q -m "initial"
    git remote add origin "$ORIGIN_DIR"
    # Use </dev/null to prevent blocking on stdin from pre-push hook
    git push -q origin main </dev/null 2>&1 || true

    LOCAL_OID=$(git rev-parse HEAD)
    REMOTE_OID=$LOCAL_OID

    # In the temp repo, these SHAs exist and same commit is fast-forward
    result=0
    check_ref_update "refs/heads/main" "$LOCAL_OID" "refs/heads/main" "$REMOTE_OID" 2>/dev/null || result=$?
    report_test $result "Fast-forward push allowed"

    cd "$REPO_ROOT"
    rm -rf "$TEST_DIR"

    # --- Test 5: Non-fast-forward push should fail ---
    result=0
    check_ref_update "refs/heads/main" "abc123" "refs/heads/main" "def456" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Non-fast-forward push rejected"

    # --- Test 6: Branch deletion should fail ---
    result=0
    check_ref_update "refs/heads/feature" "$ZERO_SHA" "refs/heads/feature" "abc123" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Branch deletion rejected"

    # --- Test 7: Tag rewrite should fail ---
    result=0
    check_ref_update "refs/tags/v1.0" "abc123" "refs/tags/v1.0" "def456" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Tag rewrite rejected"

    # --- Test 8: Tag deletion should fail ---
    result=0
    check_ref_update "refs/tags/v1.0" "$ZERO_SHA" "refs/tags/v1.0" "abc123" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Tag deletion rejected"

    # --- Test 9: Stdin entrypoint - branch deletion via piping stdin ---
    # This tests the actual hook entrypoint (not check_ref_update directly)
    result=0
    echo "refs/heads/test $ZERO_SHA refs/heads/test abc123def456" | "$SELF_PATH" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Stdin entrypoint: branch deletion rejected"

    # --- Test 10: Stdin entrypoint - non-fast-forward via piping stdin ---
    result=0
    echo "refs/heads/main abc123 refs/heads/main def456" | "$SELF_PATH" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Stdin entrypoint: non-fast-forward rejected"

    # --- Test 11: Stdin entrypoint - tag rewrite via piping stdin ---
    result=0
    echo "refs/tags/v1.0 abc123 refs/tags/v1.0 def456" | "$SELF_PATH" 2>/dev/null || result=$?
    report_test $([ $result -ne 0 ] && echo 0 || echo 1) "Stdin entrypoint: tag rewrite rejected"

    # --- Test 12: Stdin entrypoint - new branch allowed ---
    result=0
    echo "refs/heads/newfeature abc123 refs/heads/newfeature $ZERO_SHA" | "$SELF_PATH" 2>/dev/null || result=$?
    report_test $result "Stdin entrypoint: new branch allowed"

    # --- Summary ---
    echo ""
    echo "=== Self-Test Summary ==="
    echo "Passed: $TEST_PASS"
    echo "Failed: $TEST_FAIL"

    if [ "$TEST_FAIL" -eq 0 ]; then
        echo "All tests passed."
        return 0
    else
        echo "Some tests failed."
        return 1
    fi
}

# --- Entry Point ---
if [ "$#" -eq 1 ] && [ "$1" = "--self-test" ]; then
    REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    run_self_test
else
    # Read pre-push stdin: local_ref local_oid remote_ref remote_oid
    # NOTE: Do NOT use IFS= - Git sends space-separated values, we need default word splitting
    while read -r local_ref local_oid remote_ref remote_oid
    do
        check_ref_update "$local_ref" "$local_oid" "$remote_ref" "$remote_oid" || exit 1
    done
    exit 0
fi
