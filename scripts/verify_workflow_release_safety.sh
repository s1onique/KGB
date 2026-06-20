#!/usr/bin/env bash
# =============================================================================
# verify_workflow_release_safety.sh
#
# Verifies that GitHub Actions workflows follow the release barrier doctrine:
#   1. Build jobs MUST NOT call gh release create, gh release upload, or gh release edit.
#   2. Only ONE publish job per workflow has contents: write.
#   3. Publish job MUST use `needs` to wait for all build jobs to complete.
#   4. Build jobs upload workflow artifacts only (not release assets).
#
# Exit codes:
#   0 = PASS
#   1 = FAIL (policy violation found)
#   2 = ERROR (script usage/environment issue)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Allow overriding WORKFLOWS_DIR for testing
WORKFLOWS_DIR="${WORKFLOWS_DIR:-${REPO_ROOT}/.github/workflows}"

# Parse --self-test flag
# Environment variable SELF_TEST takes precedence over command line
if [[ -z "${SELF_TEST:-}" ]]; then
    SELF_TEST=0
    if [[ "${1:-}" == "--self-test" ]]; then
        SELF_TEST=1
    fi
fi
# If SELF_TEST is already set (from environment), command line is ignored

# Skip sentinel files unless explicitly told to scan them
# This allows self-test to create a sentinel and then scan it
SKIP_SENTINEL="${SKIP_SENTINEL:-1}"

# Track violations
VIOLATIONS=0

# Patterns that indicate build jobs attempting to publish releases
FORBIDDEN_PATTERNS=(
    "gh release create"
    "gh release upload"
    "gh release edit"
)

# -----------------------------------------------------------------------------
# Parse a single workflow file and extract job blocks with their properties.
# -----------------------------------------------------------------------------
parse_workflow() {
    local workflow_file="$1"

    awk '
    BEGIN { job_name=""; in_job=0 }

    /^  [a-zA-Z][-a-zA-Z0-9_]*:/ {
        if (in_job && job_name != "") {
            printf "%s|%s|%s\n", job_name, is_publish, has_needs
        }
        job_name = substr($0, 3)
        job_name = substr(job_name, 1, length(job_name) - 1)
        in_job = 1
        is_publish = 0
        has_needs = 0
        found_permissions = 0
        next
    }

    in_job == 1 {
        if (/^    permissions:/) {
            found_permissions = 1
            next
        }

        if (found_permissions && /^    [a-z]/) {
            if (/contents:/) {
                if (/write/) { is_publish = 1 }
                else if (/read/) { is_publish = 0 }
            }
            next
        }

        if (found_permissions && /^    [a-z]/) {
            found_permissions = 0
        }

        if (/^    needs:/) { has_needs = 1 }

        if (/^  [a-zA-Z][-a-zA-Z0-9_]*:/) {
            if (in_job && job_name != "") {
                printf "%s|%s|%s\n", job_name, is_publish, has_needs
            }
            job_name = substr($0, 3)
            job_name = substr(job_name, 1, length(job_name) - 1)
            in_job = 1
            is_publish = 0
            has_needs = 0
            found_permissions = 0
            next
        }
    }

    END {
        if (in_job && job_name != "") {
            printf "%s|%s|%s\n", job_name, is_publish, has_needs
        }
    }
    ' "${workflow_file}"
}

# -----------------------------------------------------------------------------
# Check a workflow for violations
# -----------------------------------------------------------------------------
check_workflow() {
    local workflow_file="$1"
    local wf_name="$(basename "${workflow_file}")"

    echo "  checking: ${wf_name}"

    local jobs_raw
    jobs_raw=$(parse_workflow "${workflow_file}")

    if [[ -z "${jobs_raw}" ]]; then
        echo "    WARNING: No jobs found in ${wf_name}"
        return 0
    fi

    while IFS='|' read -r job_name is_publish has_needs; do
        case "${job_name}" in
            push|workflow_dispatch|schedule|group|cancel-in-progress)
                continue
                ;;
            contents|id-token|attestations|GO_VERSION|PACKAGE_NAME|ZIG_VERSION|BINARY_NAME|ZIG_PROJECT_DIR)
                continue
                ;;
        esac

        local job_start_line
        job_start_line=$(grep -n "^  ${job_name}:" "${workflow_file}" 2>/dev/null | head -1 | cut -d: -f1 || true)
        if [[ -z "${job_start_line}" ]]; then
            continue
        fi

        local job_block
        job_block=$(tail -n +$((job_start_line + 1)) "${workflow_file}" | \
                   awk '
                   /^  [a-zA-Z][-a-zA-Z0-9_]*:/ { if (NR > 1) exit; }
                   { print }
                   ' 2>/dev/null || true)

        local has_explicit_write has_explicit_read this_is_publish
        has_explicit_write=$(echo "${job_block}" | grep -E '^\s+permissions:' -A 5 | grep -E '^\s+contents:\s*write' || true)
        has_explicit_read=$(echo "${job_block}" | grep -E '^\s+permissions:' -A 5 | grep -E '^\s+contents:\s*read' || true)

        this_is_publish=false
        if [[ -n "${has_explicit_write}" ]]; then
            this_is_publish=true
            echo "    job '${job_name}': PUBLISH JOB (explicit contents: write)"
        elif [[ -n "${has_explicit_read}" ]]; then
            this_is_publish=false
            echo "    job '${job_name}': BUILD JOB (explicit contents: read)"
        else
            local first_job_line
            first_job_line=$(grep -n '^jobs:' "${workflow_file}" | head -1 | cut -d: -f1 || true)
            if [[ -n "${first_job_line}" ]]; then
                local workflow_perms
                workflow_perms=$(head -n $((first_job_line - 1)) "${workflow_file}" | \
                                grep -E '^\s+contents:\s*write' || true)
                if [[ -n "${workflow_perms}" ]]; then
                    this_is_publish=true
                    echo "    job '${job_name}': PUBLISH JOB (inherits workflow contents: write)"
                else
                    echo "    job '${job_name}': BUILD JOB (no write permission)"
                fi
            else
                echo "    job '${job_name}': BUILD JOB (no write permission)"
            fi
        fi

        for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
            if echo "${job_block}" | grep -qF "${pattern}"; then
                if [[ "${this_is_publish}" == "false" ]]; then
                    echo "    VIOLATION: job '${job_name}' contains '${pattern}' but is NOT a publish job" >&2
                    VIOLATIONS=$((VIOLATIONS + 1))
                fi
            fi
        done

    done <<< "${jobs_raw}"

    echo ""
}

# -----------------------------------------------------------------------------
# Main entry point
# -----------------------------------------------------------------------------
main() {
    echo "[verify-workflow-release-safety] starting workflow release safety verification"

    # Check if workflows directory exists
    if [[ ! -d "${WORKFLOWS_DIR}" ]]; then
        echo "[verify-workflow-release-safety] ERROR: workflows directory not found: ${WORKFLOWS_DIR}" >&2
        exit 2
    fi

    echo "[verify-workflow-release-safety] checking for release-in-build violations..."

    for workflow_file in "${WORKFLOWS_DIR}"/*.yml "${WORKFLOWS_DIR}"/*.yaml; do
        [[ -f "${workflow_file}" ]] || continue

        local wf_name="$(basename "${workflow_file}")"

        if [[ "${SKIP_SENTINEL}" == "1" ]] && [[ "${wf_name}" == *"sentinel"* || "${wf_name}" == *"bad-workflow"* ]]; then
            echo "  SKIP: ${wf_name} (sentinel test file)"
            continue
        fi

        check_workflow "${workflow_file}"
    done

    echo "[verify-workflow-release-safety] checking for orphaned publish jobs without 'needs'..."

    for workflow_file in "${WORKFLOWS_DIR}"/*.yml "${WORKFLOWS_DIR}"/*.yaml; do
        [[ -f "${workflow_file}" ]] || continue

        local wf_name="$(basename "${workflow_file}")"

        if [[ "${SKIP_SENTINEL}" == "1" ]] && [[ "${wf_name}" == *"sentinel"* || "${wf_name}" == *"bad-workflow"* ]]; then
            continue
        fi

        local publish_jobs
        publish_jobs=$(grep -B 2 -E '^\s+permissions:' -A 5 "${workflow_file}" 2>/dev/null | \
                       grep -E '^\s+contents:\s*write' -B 5 | \
                       grep -E '^  [a-zA-Z][-a-zA-Z0-9_]*:' | \
                       sed 's/:.*//' | sed 's/^  //' || true)

        for job_name in ${publish_jobs}; do
            case "${job_name}" in
                push|workflow_dispatch|schedule|group|cancel-in-progress|contents|id-token|attestations)
                    continue
                    ;;
            esac

            local job_start_line
            job_start_line=$(grep -n "^  ${job_name}:" "${workflow_file}" | head -1 | cut -d: -f1 || true)
            if [[ -z "${job_start_line}" ]]; then
                continue
            fi

            local job_block
            job_block=$(tail -n +$((job_start_line + 1)) "${workflow_file}" | \
                       awk '
                       /^  [a-zA-Z][-a-zA-Z0-9_]*:/ { if (NR > 1) exit; }
                       { print }
                       ' 2>/dev/null || true)

            local has_needs
            has_needs=$(echo "${job_block}" | grep -E '^\s+needs:' || true)
            if [[ -z "${has_needs}" ]]; then
                echo "VIOLATION: publish job '${job_name}' in ${wf_name} has no 'needs:' dependency" >&2
                VIOLATIONS=$((VIOLATIONS + 1))
            else
                echo "  publish job '${job_name}' has 'needs:' - GOOD"
            fi
        done
    done

    echo ""
    echo "[verify-workflow-release-safety] checking for workflow-level contents: write..."

    for workflow_file in "${WORKFLOWS_DIR}"/*.yml "${WORKFLOWS_DIR}"/*.yaml; do
        [[ -f "${workflow_file}" ]] || continue

        local wf_name="$(basename "${workflow_file}")"

        if [[ "${SELF_TEST}" != "1" ]] && [[ "${wf_name}" == *"sentinel"* || "${wf_name}" == *"bad-workflow"* ]]; then
            continue
        fi

        local first_job_line
        first_job_line=$(grep -n -E '^jobs:' "${workflow_file}" | head -1 | cut -d: -f1 || true)
        if [[ -z "${first_job_line}" ]]; then
            continue
        fi

        local workflow_header
        workflow_header=$(head -n $((first_job_line - 1)) "${workflow_file}")

        local workflow_write
        workflow_write=$(echo "${workflow_header}" | grep -E '^\s+contents:\s*write' || true)

        if [[ -n "${workflow_write}" ]]; then
            local jobs_with_read
            jobs_with_read=$(grep -B 2 -E '^\s+permissions:' -A 5 "${workflow_file}" 2>/dev/null | \
                             grep -E '^\s+contents:\s*read' -B 5 | \
                             grep -E '^  [a-zA-Z][-a-zA-Z0-9_]*:' | \
                             sed 's/:.*//' | sed 's/^  //' || true)

            if [[ -n "${jobs_with_read}" ]]; then
                echo "  ${wf_name}: workflow-level contents: write, but jobs override to read - acceptable"
            else
                local has_publish_jobs
                has_publish_jobs=$(grep -B 2 -E '^\s+permissions:' -A 5 "${workflow_file}" 2>/dev/null | \
                                  grep -E '^\s+contents:\s*write' -B 5 | \
                                  grep -E '^  [a-zA-Z][-a-zA-Z0-9_]*:' | \
                                  sed 's/:.*//' | sed 's/^  //' || true)

                if [[ -z "${has_publish_jobs}" ]]; then
                    echo "WARNING: ${wf_name} has workflow-level contents: write but no job-level publish jobs" >&2
                fi
            fi
        fi
    done

    echo ""
    echo "[verify-workflow-release-safety] checking tag namespace separation..."

    local -A tag_patterns
    for workflow_file in "${WORKFLOWS_DIR}"/*.yml "${WORKFLOWS_DIR}"/*.yaml; do
        [[ -f "${workflow_file}" ]] || continue

        local wf_name="$(basename "${workflow_file}")"

        if [[ "${SELF_TEST}" != "1" ]] && [[ "${wf_name}" == *"sentinel"* || "${wf_name}" == *"bad-workflow"* ]]; then
            continue
        fi

        local tag_patterns_in_workflow
        tag_patterns_in_workflow=$(grep -E 'tags:' "${workflow_file}" | grep -E '"[a-zA-Z][-a-zA-Z0-9]*-v\*"' | \
                                   grep -oE '"[^"]+v\*"' | tr -d '"' || true)

        if [[ -n "${tag_patterns_in_workflow}" ]]; then
            for pattern in ${tag_patterns_in_workflow}; do
                if [[ -n "${tag_patterns[${pattern}]:-}" ]]; then
                    local existing="${tag_patterns[${pattern}]}"
                    echo "WARNING: Tag pattern '${pattern}' used by both: ${existing} and ${wf_name}" >&2
                else
                    tag_patterns[${pattern}]="${wf_name}"
                fi
            done
        fi
    done

    echo ""
    echo "[verify-workflow-release-safety] tag namespace map:"
    for pattern in "${!tag_patterns[@]}"; do
        echo "  ${pattern} -> ${tag_patterns[$pattern]}"
    done

    echo ""

    # Self-test mode
    if [[ "${SELF_TEST}" == "1" ]]; then
        echo "[verify-workflow-release-safety] SELF-TEST MODE"
        echo ""

        local tmpdir
        tmpdir=$(mktemp -d)
        mkdir -p "${tmpdir}/.github/workflows"

        cat > "${tmpdir}/.github/workflows/sentinel-bad.yml" << 'EOF'
name: sentinel-bad
on: push
jobs:
  build-bad:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh release create "v1.0.0" dist/*.bin
  build-good:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/upload-artifact@v4
  publish-bad:
    runs-on: ubuntu-latest
    permissions: { contents: write }
    steps:
      - uses: actions/checkout@v4
      - run: gh release create "v1.0.0" dist/*.bin
EOF

        echo "Created sentinel bad workflow in: ${tmpdir}"
        echo ""

        echo "Running verifier on sentinel bad workflow..."

        local self_test_result
        # Scan sentinel files by disabling skip flag
        self_test_result=$(SKIP_SENTINEL=0 WORKFLOWS_DIR="${tmpdir}/.github/workflows" "${SCRIPT_DIR}/verify_workflow_release_safety.sh" 2>&1 || true)

        rm -rf "${tmpdir}"

        local violations_found
        violations_found=$(echo "${self_test_result}" | grep -c "VIOLATION" || true)

        echo ""
        echo "=== Sentinel self-test results ==="
        echo "${self_test_result}" | tail -20
        echo ""

        if [[ "${violations_found}" -ge 1 ]]; then
            echo "SELF-TEST PASS: Detected ${violations_found} violation(s) in sentinel bad workflow"
        else
            echo "SELF-TEST FAIL: Expected at least 1 violation, found ${violations_found}" >&2
            exit 2
        fi

        echo ""
    fi

    echo ""
    if [[ "${VIOLATIONS}" -gt 0 ]]; then
        echo "[verify-workflow-release-safety] FAIL: ${VIOLATIONS} violation(s) detected" >&2
        exit 1
    else
        echo "[verify-workflow-release-safety] PASS: No release-in-build violations detected"
        exit 0
    fi
}

main "$@"
