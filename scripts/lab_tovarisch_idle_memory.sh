#!/bin/bash
# Idle staircase memory lab for tovarisch.
# Runs tovarisch idle and samples RSS/VmData to detect stepwise memory growth.
# Usage: $0 [--duration SECS] [--interval SECS] [--status-burst] [--strace] [--run-id ID]
# Exit: 0=ok, 1=no-Linux, 2=no-binary, 3=setup-failed
set -euo pipefail

# ============================================================================
# CLI Argument Parsing
# ============================================================================

DURATION="${DURATION:-600}"        # 10 minutes default
INTERVAL="${INTERVAL:-5}"          # 5 seconds default
STATUS_BURST="${STATUS_BURST:-false}"
STRACE="${STRACE:-false}"
RUN_ID="${RUN_ID:-}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --duration)
            DURATION="${2}"
            shift 2
            ;;
        --interval)
            INTERVAL="${2}"
            shift 2
            ;;
        --status-burst)
            STATUS_BURST="true"
            shift
            ;;
        --strace)
            STRACE="true"
            shift
            ;;
        --run-id)
            RUN_ID="${2}"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [--duration SECS] [--interval SECS] [--status-burst] [--strace] [--run-id ID]"
            echo "  --duration     Lab duration in seconds (default: 600)"
            echo "  --interval     Memory sample interval in seconds (default: 5)"
            echo "  --status-burst Run /status burst test after idle window"
            echo "  --strace       Enable strace syscall tracing (Linux only)"
            echo "  --run-id       Custom run identifier (default: auto-generated)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TOVARISCH_BINARY="${REPO_ROOT}/tovarisch/zig-out/bin/tovarisch"
ARTIFACT_DIR="${REPO_ROOT}/artifacts/memory-labs/tovarisch/idle-staircase"

# Lab defaults
DEFAULT_PORT=8317
LAB_PORT="${LAB_PORT:-${DEFAULT_PORT}}"
LAB_BIND="${LAB_BIND:-127.0.0.1}"

# ============================================================================
# Platform Detection
# ============================================================================

is_linux() {
    [[ "$(uname)" == "Linux" ]]
}

check_proc_available() {
    [[ -r /proc/self/status ]]
}

# ============================================================================
# Artifact Helpers
# ============================================================================

create_artifact_dir() {
    if [[ -z "${RUN_ID}" ]]; then
        RUN_ID="idle-$(date +%Y%m%d-%H%M%S)-$$"
    fi
    local artifact_path="${ARTIFACT_DIR}/${RUN_ID}"
    mkdir -p "${artifact_path}"
    echo "${artifact_path}"
}

write_manifest() {
    local artifact_path="$1"
    cat > "${artifact_path}/manifest.yaml" <<EOF
# Idle Staircase Memory Lab Manifest
# Generated: $(date -Iseconds)
run_id: "${RUN_ID}"
platform: $(uname -s -r)
kernel: $(uname -r)
architecture: $(uname -m)

# Lab configuration
duration_seconds: ${DURATION}
sample_interval_seconds: ${INTERVAL}
status_burst: ${STATUS_BURST}
strace_enabled: ${STRACE}
lab_port: ${LAB_PORT}
lab_bind: ${LAB_BIND}

# Build info
tovarisch_binary: "${TOVARISCH_BINARY}"
binary_exists: $([[ -x "${TOVARISCH_BINARY}" ]] && echo "true" || echo "false")

# Git state
commit_sha: $(cd "${REPO_ROOT}" && git rev-parse HEAD 2>/dev/null || echo "unknown")
git_dirty: $(cd "${REPO_ROOT}" && git status --porcelain 2>/dev/null | wc -l | tr -d ' ')

# Lab start time
lab_start_iso: $(date -Iseconds)
EOF
}

write_memory_header() {
    local tsv_path="$1"
    echo -e "timestamp\telapsed_sec\trss_kib\tvmdata_kib\tvmhwm_kib\tvmswap_kib\tvmpeak_kib\tvmrss_peak_kib" > "${tsv_path}"
}

write_event_header() {
    local tsv_path="$1"
    echo -e "timestamp\telapsed_sec\tevent\tsubsystem\tdetail" > "${tsv_path}"
}

sample_memory() {
    # Returns: rss_kib vmdata_kib vmhwm_kib vmswap_kib vmpeak_kib vmrss_peak_kib
    # All values in KiB
    
    if ! is_linux || ! check_proc_available; then
        echo "0 0 0 0 0 0"
        return
    fi
    
    local pid="$1"
    local status_file="/proc/${pid}/status"
    
    if ! [[ -r "${status_file}" ]]; then
        echo "0 0 0 0 0 0"
        return
    fi
    
    # Parse /proc/[pid]/status for memory metrics
    # Note: VmData is in kB, not KiB - we standardize to KiB
    local rss_kib=0
    local vmdata_kib=0
    local vmhwm_kib=0
    local vmswap_kib=0
    local vmpeak_kib=0
    local vmrss_peak_kib=0
    
    while IFS=: read -r key value; do
        # Strip leading/trailing whitespace
        value="${value// /}"
        value="${value//kB/}"
        
        case "${key}" in
            "VmRSS") rss_kib="${value}" ;;
            "VmData") vmdata_kib="${value}" ;;
            "VmHWM") vmhwm_kib="${value}" ;;
            "VmSwap") vmswap_kib="${value}" ;;
            "VmPeak") vmpeak_kib="${value}" ;;
        esac
    done < "${status_file}"
    
    # VmHWM is the high water mark RSS - this is what we compare against RSS
    vmrss_peak_kib="${vmhwm_kib}"
    
    echo "${rss_kib} ${vmdata_kib} ${vmhwm_kib} ${vmswap_kib} ${vmpeak_kib} ${vmrss_peak_kib}"
}

log_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event="$3"
    local subsystem="${4:-unknown}"
    local detail="${5:-}"
    
    local timestamp
    timestamp=$(date +%Y-%m-%dT%H:%M:%S.%3N)
    
    # Escape tabs in detail
    detail="${detail//$'\t'/\\t}"
    # Escape newlines in detail
    detail="${detail//$'\n'/\\n}"
    
    echo -e "${timestamp}\t${elapsed}\t${event}\t${subsystem}\t${detail}" >> "${tsv_path}"
}

analyze_staircase() {
    local tsv_path="$1"
    local artifact_path="$2"
    
    # Extract RSS values and detect staircase pattern
    local rss_values=()
    local rss_deltas=()
    local prev_rss=0
    local steps_detected=0
    
    # Skip header line, read RSS values
    while IFS=$'\t' read -r ts elapsed rss vmdata vmhwm vmswap vmpeak vmrss_peak; do
        if [[ "${rss}" =~ ^[0-9]+$ ]] && [[ ${rss} -gt 0 ]]; then
            if [[ ${prev_rss} -gt 0 ]]; then
                local delta=$((rss - prev_rss))
                if [[ ${delta} -gt 50 ]]; then  # > 50 KiB step threshold
                    ((steps_detected++))
                fi
                rss_deltas+=("${delta}")
            fi
            prev_rss=${rss}
            rss_values+=("${rss}")
        fi
    done < <(tail -n +2 "${tsv_path}" 2>/dev/null || true)
    
    # Calculate growth rate
    local total_growth=0
    local growth_rate_per_min=0
    
    if [[ ${#rss_values[@]} -ge 2 ]]; then
        local first_rss=${rss_values[0]}
        local last_rss=${rss_values[-1]}
        total_growth=$((last_rss - first_rss))
        
        # Calculate rate per minute
        local total_seconds=${DURATION}
        if [[ ${total_seconds} -gt 0 ]]; then
            growth_rate_per_min=$(( (total_growth * 60) / total_seconds ))
        fi
    fi
    
    # Determine verdict
    local verdict="inconclusive"
    local owner=""
    local reason=""
    
    if [[ ${steps_detected} -ge 3 ]] && [[ ${total_growth} -gt 500 ]]; then
        verdict="confirmed_leak"
        owner="unknown"  # Leak confirmed but owner requires event correlation to attribute
        reason="Detected ${steps_detected} staircase steps with ${total_growth} KiB total growth (${growth_rate_per_min} KiB/min). Owner requires event correlation."
    elif [[ ${total_growth} -gt 1000 ]]; then
        verdict="bounded_warmup_or_allocator_highwater"
        reason="Detected ${total_growth} KiB growth but no clear staircase pattern (may be normal warmup)"
    elif [[ ${total_growth} -lt 200 ]]; then
        verdict="bounded_warmup_or_allocator_highwater"
        reason="Minimal growth detected (${total_growth} KiB) - likely bounded by allocator high water mark"
    else
        verdict="inconclusive"
        reason="Growth pattern unclear: ${total_growth} KiB over ${DURATION}s"
    fi
    
    # Write verdict
    cat > "${artifact_path}/verdict.txt" <<EOF
verdict: ${verdict}
owner: ${owner}
reason: ${reason}
steps_detected: ${steps_detected}
total_growth_kib: ${total_growth}
growth_rate_kib_per_min: ${growth_rate_per_min}
samples_count: ${#rss_values[@]}
EOF
    
    echo "${verdict}"
}

# ============================================================================
# Strace Helper (Linux only)
# ============================================================================

run_with_strace() {
    local pid="$1"
    local output_file="$2"
    
    if ! is_linux || [[ "${STRACE}" != "true" ]]; then
        return 0
    fi
    
    # Only trace if strace is available
    if ! command -v strace &>/dev/null; then
        return 0
    fi
    
    # Trace memory-related syscalls: brk, mmap, munmap
    # Also trace execve for wg command
    strace -p "${pid}" -f \
        -e trace=brk,mmap,mmap2,mremap,munmap,execve \
        -o "${output_file}" &
    
    echo $!
}

# ============================================================================
# Main Lab
# ============================================================================

main() {
    echo "=== Idle Staircase Memory Lab ==="
    echo "Platform: $(uname -s)"
    echo "Duration: ${DURATION}s, Interval: ${INTERVAL}s"
    echo "Status burst: ${STATUS_BURST}"
    echo "Strace: ${STRACE}"
    echo ""
    
    # Check Linux requirement
    if ! is_linux; then
        echo "SKIP: This lab requires Linux with /proc filesystem."
        echo "Memory sampling would be meaningless on non-Linux platforms."
        exit 0  # Clean skip - no error, just not applicable
    fi
    
    if ! check_proc_available; then
        echo "ERROR: /proc not available or not readable."
        exit 1
    fi
    
    # Check tovarisch binary
    if ! [[ -x "${TOVARISCH_BINARY}" ]]; then
        echo "ERROR: tovarisch binary not found at ${TOVARISCH_BINARY}"
        echo "Run 'make tovarisch-build' first."
        exit 2
    fi
    
    # Create artifact directory
    local artifact_path
    artifact_path=$(create_artifact_dir)
    echo "Artifact directory: ${artifact_path}"
    
    # Write manifests
    write_manifest "${artifact_path}"
    write_memory_header "${artifact_path}/memory_samples.tsv"
    write_event_header "${artifact_path}/event_timeline.tsv"
    
    # Start tovarisch in background
    echo "Starting tovarisch on ${LAB_BIND}:${LAB_PORT}..."
    local tovarisch_pid
    "${TOVARISCH_BINARY}" serve --bind "${LAB_BIND}:${LAB_PORT}" &
    tovarisch_pid=$!
    
    # Wait for startup
    sleep 2
    
    # Check if still running
    if ! kill -0 "${tovarisch_pid}" 2>/dev/null; then
        echo "ERROR: tovarisch failed to start"
        exit 3
    fi
    
    # Optionally start strace
    local strace_pid=""
    if [[ "${STRACE}" == "true" ]]; then
        echo "Starting strace tracing..."
        strace_pid=$(run_with_strace "${tovarisch_pid}" "${artifact_path}/strace.log")
    fi
    
    log_event "${artifact_path}/event_timeline.tsv" 0 "lab_started" "lab" "tovarisch PID=${tovarisch_pid}"
    log_event "${artifact_path}/event_timeline.tsv" 0 "heartbeat_enabled" "heartbeat" "30-second heartbeat interval"
    
    echo "Running idle for ${DURATION} seconds..."
    
    # Memory sampling loop
    local start_time
    start_time=$(date +%s)
    local sample_count=0
    
    while kill -0 "${tovarisch_pid}" 2>/dev/null; do
        local elapsed=$(( $(date +%s) - start_time ))
        
        # Sample memory
        local mem_values
        mem_values=$(sample_memory "${tovarisch_pid}")
        read -r rss vmdata vmhwm vmswap vmpeak vmrss_peak <<< "${mem_values}"
        
        local timestamp
        timestamp=$(date +%Y-%m-%dT%H:%M:%S.%3N)
        
        echo -e "${timestamp}\t${elapsed}\t${rss}\t${vmdata}\t${vmhwm}\t${vmswap}\t${vmpeak}\t${vmrss_peak}" >> "${artifact_path}/memory_samples.tsv"
        ((sample_count++))
        
        # Log periodic heartbeat events (every ~30 seconds)
        if (( elapsed > 0 && elapsed % 30 == 0 )); then
            log_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "heartbeat_tick" "heartbeat" "uptime=${elapsed}s"
        fi
        
        # Check if duration reached
        if (( elapsed >= DURATION )); then
            break
        fi
        
        sleep "${INTERVAL}"
    done
    
    log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "idle_complete" "lab" "sampled=${sample_count} times"
    
    # Optional: status burst test
    if [[ "${STATUS_BURST}" == "true" ]]; then
        echo "Running /status burst test..."
        log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "status_burst_start" "status" "5000 requests"
        
        local burst_start
        burst_start=$(date +%s)
        for i in $(seq 1 5000); do
            curl -s "http://${LAB_BIND}:${LAB_PORT}/status" > /dev/null 2>&1 || true
        done
        local burst_duration=$(( $(date +%s) - burst_start ))
        
        # Final memory sample after burst
        mem_values=$(sample_memory "${tovarisch_pid}")
        read -r rss vmdata vmhwm vmswap vmpeak vmrss_peak <<< "${mem_values}"
        
        local final_elapsed=$(( $(date +%s) - start_time ))
        echo -e "$(date +%Y-%m-%dT%H:%M:%S.%3N)\t${final_elapsed}\t${rss}\t${vmdata}\t${vmhwm}\t${vmswap}\t${vmpeak}\t${vmrss_peak}" >> "${artifact_path}/memory_samples.tsv"
        
        log_event "${artifact_path}/event_timeline.tsv" "${final_elapsed}" "status_burst_complete" "status" "duration=${burst_duration}s"
    fi
    
    # Stop strace if running
    if [[ -n "${strace_pid}" ]]; then
        kill "${strace_pid}" 2>/dev/null || true
    fi
    
    # Stop tovarisch
    log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "shutdown" "lab" "stopping tovarisch"
    kill "${tovarisch_pid}" 2>/dev/null || true
    wait "${tovarisch_pid}" 2>/dev/null || true
    
    # Analyze results
    echo ""
    echo "Analyzing memory samples..."
    local verdict
    verdict=$(analyze_staircase "${artifact_path}/memory_samples.tsv" "${artifact_path}")
    
    echo ""
    echo "=== Lab Complete ==="
    echo "Artifact: ${artifact_path}"
    echo "Verdict: ${verdict}"
    echo ""
    cat "${artifact_path}/verdict.txt"
    
    exit 0
}

main "$@"
