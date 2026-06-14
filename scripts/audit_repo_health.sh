#!/bin/bash
# audit_repo_health.sh - Scheduled repo health audit
# Run via: make health-audit
#
# This script performs advisory health checks on the repository.
# Failures here do NOT block development but indicate areas needing attention.
# Calibrate thresholds based on first scheduled runs.

set -euo pipefail

echo "=== KGB Repo Health Audit ==="
echo "Started at: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo ""

AUDIT_FAILED=0

# Track findings
declare -a FINDINGS=()

log_find() {
    local level="$1"
    local msg="$2"
    echo "[${level}] ${msg}"
    FINDINGS+=("[${level}] ${msg}")
    if [[ "${level}" == "FAIL" ]]; then
        AUDIT_FAILED=1
    fi
}

# === Health Check 1: Required Docs Presence ===
echo "--- Check: Required Documentation ---"

REQUIRED_DOCS=(
    "README.md"
    "AGENTS.md"
    "Makefile"
    "docs/doctrine/kgb.md"
    "docs/doctrine/privacy.md"
    "docs/doctrine/tiny-leafs.md"
    "docs/doctrine/ai-native-code-discipline-axioms.md"
    "docs/tooling/zig-0.16-field-manual-rss-leak.md"
)

for doc in "${REQUIRED_DOCS[@]}"; do
    if [[ -f "${doc}" ]]; then
        if [[ -s "${doc}" ]]; then
            log_find "PASS" "${doc} exists and is non-empty"
        else
            log_find "WARN" "${doc} exists but is empty"
        fi
    else
        log_find "FAIL" "${doc} missing"
    fi
done

# === Health Check 2: Agent Rules Presence ===
echo ""
echo "--- Check: Agent Configuration ---"

AGENT_CONFIGS=(
    ".clinerules/00-bootstrap.md"
    ".clinerules/10-kgb-doctrine.md"
    ".clinerules/20-zig-016.md"
    ".clinerules/30-karpathy.md"
    ".clinerules/30-verification.md"
)

for cfg in "${AGENT_CONFIGS[@]}"; do
    if [[ -f "${cfg}" ]]; then
        if [[ -s "${cfg}" ]]; then
            log_find "PASS" "${cfg} exists and is non-empty"
        else
            log_find "WARN" "${cfg} exists but is empty"
        fi
    else
        log_find "FAIL" "${cfg} missing"
    fi
done

# === Health Check 3: Core Source Structure ===
echo ""
echo "--- Check: Core Source Structure ---"

CORE_DIRS=(
    "tovarisch/src"
    "docs/contracts"
    "scripts"
)

for dir in "${CORE_DIRS[@]}"; do
    if [[ -d "${dir}" ]]; then
        FILE_COUNT=$(find "${dir}" -type f | wc -l)
        log_find "PASS" "${dir}/ has ${FILE_COUNT} files"
    else
        log_find "FAIL" "${dir}/ missing"
    fi
done

# === Health Check 4: Zig Package Validity ===
echo ""
echo "--- Check: Zig Package Structure ---"

if [[ -f "tovarisch/build.zig" ]]; then
    log_find "PASS" "tovarisch/build.zig exists"
else
    log_find "FAIL" "tovarisch/build.zig missing"
fi

if [[ -f "tovarisch/build.zig.zon" ]]; then
    log_find "PASS" "tovarisch/build.zig.zon exists"
else
    log_find "FAIL" "tovarisch/build.zig.zon missing"
fi

# === Health Check 5: Forbidden Naming Check ===
echo ""
echo "--- Check: Naming Compliance ---"

FORBIDDEN_PATTERNS=(
    "kgb-agent"
    "kgb_agent"
    "agent daemon"
    "agent-client"
)

NAMING_VIOLATIONS=0
for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
    COUNT=$(grep -r --include='*.md' --include='*.zig' --include='*.sh' -l "${pattern}" . 2>/dev/null | wc -l || true)
    if [[ "${COUNT}" -gt 0 ]]; then
        log_find "WARN" "Found '${pattern}' in ${COUNT} files (should use 'tovarisch' or 'leaf')"
        NAMING_VIOLATIONS=$((NAMING_VIOLATIONS + COUNT))
    fi
done

if [[ "${NAMING_VIOLATIONS}" -eq 0 ]]; then
    log_find "PASS" "No forbidden naming patterns found"
fi

# === Health Check 6: Documentation Freshness (git-history based) ===
echo ""
echo "--- Check: Documentation Freshness (git-history) ---"

# Use git log to determine last meaningful update, not filesystem mtime
# This is reliable in CI where checkout-time mtimes are misleading
KEY_DOCS=(
    "docs/doctrine/kgb.md"
    "docs/doctrine/privacy.md"
    "docs/doctrine/tiny-leafs.md"
)

for doc in "${KEY_DOCS[@]}"; do
    if [[ -f "${doc}" ]]; then
        # Get last commit timestamp for this file
        LAST_TS=$(git log -1 --format=%ct -- "${doc}" 2>/dev/null || echo "0")
        if [[ "${LAST_TS}" -gt 0 ]]; then
            CURRENT_TS=$(date +%s)
            AGE_SECONDS=$((CURRENT_TS - LAST_TS))
            AGE_DAYS=$((AGE_SECONDS / 86400))
            
            if [[ "${AGE_DAYS}" -gt 365 ]]; then
                log_find "WARN" "${doc} has not been meaningfully updated in ${AGE_DAYS} days"
            elif [[ "${AGE_DAYS}" -gt 180 ]]; then
                log_find "INFO" "${doc} last meaningfully updated ${AGE_DAYS} days ago"
            else
                log_find "PASS" "${doc} recently meaningfully updated (${AGE_DAYS} days)"
            fi
        else
            log_find "WARN" "${doc} has no git history"
        fi
    fi
done

# === Health Check 7: Stable-Reference Hygiene ===
echo ""
echo "--- Check: Stable-Reference Hygiene (Forbidden Chat-Memory) ---"

# AI-native axiom: repo-local docs must not contain chat-memory references
# These patterns indicate the doc expects context from external chat history

# File-level allowlist: files that intentionally contain forbidden examples
# This file defines the forbidden patterns, so they must be listed here
STABLE_REF_ALLOWLIST=(
    "docs/doctrine/ai-native-code-discipline-axioms.md"
)

FORBIDDEN_CHAT_PATTERNS=(
    "previous chat"
    "the doc above"
    "that previous ACT"
    "the chat knows"
    "as we discussed earlier"
    "from the previous session"
)

# Build grep exclude pattern for allowlisted files
EXCLUDE_PATTERN=""
for f in "${STABLE_REF_ALLOWLIST[@]}"; do
    EXCLUDE_PATTERN="${EXCLUDE_PATTERN} --exclude=${f}"
done

CHAT_REFERENCE_FAILS=0
for pattern in "${FORBIDDEN_CHAT_PATTERNS[@]}"; do
    # Only scan committed docs, excluding allowlisted files
    # grep -v efficiently filters out allowlisted files from results
    COUNT=$(git ls-files '*.md' | xargs grep -l "${pattern}" 2>/dev/null | \
        grep -v -F "${STABLE_REF_ALLOWLIST[0]}" 2>/dev/null | wc -l || true)
    if [[ "${COUNT}" -gt 0 ]]; then
        FOUND_FILES=$(git ls-files '*.md' | xargs grep -l "${pattern}" 2>/dev/null | \
            grep -v -F "${STABLE_REF_ALLOWLIST[0]}" 2>/dev/null | tr '\n' ' ')
        log_find "FAIL" "Found chat-memory reference '${pattern}' in: ${FOUND_FILES}"
        CHAT_REFERENCE_FAILS=$((CHAT_REFERENCE_FAILS + COUNT))
    fi
done

if [[ "${CHAT_REFERENCE_FAILS}" -eq 0 ]]; then
    log_find "PASS" "No stable-reference violations (chat-memory patterns)"
fi

# === Health Check 8: LLM-Friendliness Gate ===
echo ""
echo "--- Check: LLM-Friendliness Gate ---"

if ./scripts/check_llm_friendliness.sh > /tmp/llm_check.log 2>&1; then
    log_find "PASS" "LLM-friendliness check passed"
else
    WARN_COUNT=$(grep -c "WARN" /tmp/llm_check.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "FAIL" /tmp/llm_check.log 2>/dev/null || echo "0")
    if [[ "${FAIL_COUNT}" -gt 0 ]]; then
        log_find "FAIL" "LLM-friendliness: ${FAIL_COUNT} failures, ${WARN_COUNT} warnings"
    else
        log_find "WARN" "LLM-friendliness: ${WARN_COUNT} warnings (advisory)"
    fi
fi

# === Health Check 9: Memory Ownership Hygiene Gate ===
echo ""
echo "--- Check: Memory Ownership Hygiene Gate ---"

if ./scripts/check_memory_ownership.sh > /tmp/mem_check.log 2>&1; then
    log_find "PASS" "Memory ownership hygiene passed"
else
    FAIL_COUNT=$(grep -c "FAIL" /tmp/mem_check.log 2>/dev/null || echo "0")
    if [[ "${FAIL_COUNT}" -gt 0 ]]; then
        log_find "FAIL" "Memory ownership: ${FAIL_COUNT} failures"
    else
        log_find "WARN" "Memory ownership hygiene issues detected"
    fi
fi

# === Health Check 10: Memory Ownership Self-Test ===
echo ""
echo "--- Check: Memory Ownership Self-Test ---"

if ./scripts/check_memory_ownership.sh --self-test > /tmp/mem_selftest.log 2>&1; then
    log_find "PASS" "Memory ownership self-test passed"
else
    log_find "FAIL" "Memory ownership self-test failed"
fi

# === Health Check 11: Git State ===
echo ""
echo "--- Check: Git State ---"

if git rev-parse --git-dir >/dev/null 2>&1; then
    if git diff-index --quiet HEAD -- 2>/dev/null; then
        log_find "PASS" "Git working tree is clean"
    else
        UNTRACKED=$(git status --porcelain 2>/dev/null | grep '^??' | wc -l || echo "0")
        CHANGED=$(git status --porcelain 2>/dev/null | grep -v '^??' | wc -l || echo "0")
        if [[ "${UNTRACKED}" -gt 5 || "${CHANGED}" -gt 0 ]]; then
            log_find "WARN" "Git has ${UNTRACKED} untracked + ${CHANGED} changed files"
        else
            log_find "PASS" "Git working tree has minimal changes"
        fi
    fi
else
    log_find "WARN" "Not a git repository"
fi

# === Summary ===
echo ""
echo "=== Audit Summary ==="
echo "Completed at: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo ""

FAIL_COUNT=0
WARN_COUNT=0
PASS_COUNT=0

for finding in "${FINDINGS[@]}"; do
    case "${finding}" in
        \[FAIL\]*) ((FAIL_COUNT++)) ;;
        \[WARN\]*) ((WARN_COUNT++)) ;;
        \[PASS\]*) ((PASS_COUNT++)) ;;
    esac
done

echo "Results: ${PASS_COUNT} passed, ${WARN_COUNT} warnings, ${FAIL_COUNT} failures"
echo ""

# Print failures distinctly for visibility
if [[ "${FAIL_COUNT}" -gt 0 ]]; then
    echo "=== FAILURES DETECTED ==="
    for finding in "${FINDINGS[@]}"; do
        if [[ "${finding}" == \[FAIL\]* ]]; then
            echo "  ${finding}"
        fi
    done
    echo ""
    echo "ADVISORY: ${FAIL_COUNT} failure(s) detected - review recommended"
    echo "          Advisory audit findings do not replace 'make gate'"
fi

if [[ "${WARN_COUNT}" -gt 0 ]]; then
    echo "ADVISORY: ${WARN_COUNT} warning(s) detected - monitor trend"
fi

if [[ "${FAIL_COUNT}" -eq 0 && "${WARN_COUNT}" -eq 0 ]]; then
    echo "ADVISORY: Repository health looks good"
fi

echo ""
echo "=== End of Health Audit ==="

# Exit with success - this is advisory only
# The failures are logged distinctly for GitHub Actions visibility
exit 0
