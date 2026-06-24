#!/bin/bash
# Idle staircase memory lab for tovarisch.
# Runs tovarisch idle and samples RSS/VmData to detect stepwise memory growth.
#
# Usage: $0 [--duration SECS] [--interval SECS] [--status-burst] [--strace] [--run-id ID]
#        [--heartbeat-only] [--wg-only] [--bgp-bfd-only] [--no-subsystems]
#
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

# Subsystem toggles (SYNTHETIC - only affect shell-side event logging, NOT actual tovarisch runtime)
# These toggles control which synthetic events are emitted to the event timeline.
# They do NOT disable actual periodic paths in tovarisch (heartbeat, WG checks, BGP/BFD).
# Use --no-subsystems to get a baseline with synthetic events suppressed.
HEARTBEAT_ENABLED="${HEARTBEAT_ENABLED:-true}"
WG_CHECK_ENABLED="${WG_CHECK_ENABLED:-true}"
BGP_BFD_ENABLED="${BGP_BFD_ENABLED:-true}"
NO_SUBSYSTEMS="${NO_SUBSYSTEMS:-false}"

# Deterministic fake runner flags (for testing specific paths)
FAKE_HEARTBEAT="${FAKE_HEARTBEAT:-false}"
FAKE_WG_CHECK="${FAKE_WG_CHECK:-false}"
FAKE_BGP="${FAKE_BGP:-false}"
FAKE_BFD="${FAKE_BFD:-false}"

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
        --heartbeat-only)
            # Synthetic-only: emit only heartbeat synthetic events
            HEARTBEAT_ENABLED="true"
            WG_CHECK_ENABLED="false"
            BGP_BFD_ENABLED="false"
            shift
            ;;
        --wg-only)
            # Synthetic-only: emit only WG check synthetic events
            HEARTBEAT_ENABLED="false"
            WG_CHECK_ENABLED="true"
            BGP_BFD_ENABLED="false"
            shift
            ;;
        --bgp-bfd-only)
            # Synthetic-only: emit only BGP/BFD synthetic events
            HEARTBEAT_ENABLED="false"
            WG_CHECK_ENABLED="false"
            BGP_BFD_ENABLED="true"
            shift
            ;;
        --no-subsystems)
            # Synthetic-only: suppress all synthetic events
            NO_SUBSYSTEMS="true"
            HEARTBEAT_ENABLED="false"
            WG_CHECK_ENABLED="false"
            BGP_BFD_ENABLED="false"
            shift
            ;;
        --fake-heartbeat)
            FAKE_HEARTBEAT="true"
            shift
            ;;
        --fake-wg-check)
            FAKE_WG_CHECK="true"
            shift
            ;;
        --fake-bgp)
            FAKE_BGP="true"
            shift
            ;;
        --fake-bfd)
            FAKE_BFD="true"
            shift
            ;;
        --help)
            echo "Usage: $0 [--duration SECS] [--interval SECS] [--status-burst] [--strace] [--run-id ID]"
            echo "  --duration       Lab duration in seconds (default: 600)"
            echo "  --interval       Memory sample interval in seconds (default: 5)"
            echo "  --status-burst   Run /status burst test after idle window"
            echo "  --strace         Enable strace syscall tracing (Linux only)"
            echo "  --run-id         Custom run identifier (default: auto-generated)"
            echo ""
            echo "Subsystem toggles (SYNTHETIC EVENT EMISSION ONLY):"
            echo "  --heartbeat-only  Emit only heartbeat synthetic events, suppress others"
            echo "  --wg-only         Emit only WG check synthetic events, suppress others"
            echo "  --bgp-bfd-only    Emit only BGP/BFD synthetic events, suppress others"
            echo "  --no-subsystems   Suppress all synthetic events"
            echo ""
            echo "NOTE: These do NOT disable actual tovarisch runtime paths."
            echo "      Use TOVARISCH_NATIVE_SUBSYSTEM_OFF for real runtime toggles."
            echo ""
            echo "Fake runner flags for deterministic testing:"
            echo "  --fake-heartbeat  Use fake heartbeat path"
            echo "  --fake-wg-check   Use fake WG check path"
            echo "  --fake-bgp        Use fake BGP path"
            echo "  --fake-bfd        Use fake BFD path"
            echo ""
            echo "Environment variables:"
            echo "  DURATION           Lab duration in seconds"
            echo "  INTERVAL           Sample interval in seconds"
            echo "  STATUS_BURST       Run /status burst after idle"
            echo "  HEARTBEAT_ENABLED  Enable heartbeat (default: true)"
            echo "  WG_CHECK_ENABLED   Enable WG checks (default: true)"
            echo "  BGP_BFD_ENABLED    Enable BGP/BFD (default: true)"
            echo "  TOVARISCH_WG_COMMAND_PATH  Force specific wg command path"
            echo "  LAB_TOVARISCH_PORT        Override server port"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Apply --no-subsystems override
if [[ "${NO_SUBSYSTEMS}" == "true" ]]; then
    HEARTBEAT_ENABLED="false"
    WG_CHECK_ENABLED="false"
    BGP_BFD_ENABLED="false"
fi

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TOVARISCH_BINARY="${REPO_ROOT}/tovarisch/zig-out/bin/tovarisch"
ARTIFACT_DIR="${REPO_ROOT}/artifacts/memory-labs/tovarisch/idle-staircase"

# Lab defaults
DEFAULT_PORT=8317
LAB_PORT="${LAB_TOVARISCH_PORT:-${DEFAULT_PORT}}"
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

# Event source classification (CRITICAL for attribution)
# Shell-side synthetic events CANNOT produce confirmed_leak verdicts
event_source: shell_synthetic

# Subsystem toggles (CONTROL SHELL-SIDE SYNTHETIC EVENTS ONLY)
# These do NOT disable actual tovarisch runtime paths
heartbeat_enabled: ${HEARTBEAT_ENABLED}
wg_check_enabled: ${WG_CHECK_ENABLED}
bgp_bfd_enabled: ${BGP_BFD_ENABLED}
no_subsystems: ${NO_SUBSYSTEMS}

# Fake runner flags (for deterministic testing)
fake_heartbeat: ${FAKE_HEARTBEAT}
fake_wg_check: ${FAKE_WG_CHECK}
fake_bgp: ${FAKE_BGP}
fake_bfd: ${FAKE_BFD}

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

# ============================================================================
# Event Attribution Helpers
# ============================================================================

# Log heartbeat-specific events
log_heartbeat_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # tick_start, tick_end, emit
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "heartbeat_${event_type}" "heartbeat" "${detail}"
}

# Log WG check events
log_wg_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # check_start, check_failed, check_end
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "wg_${event_type}" "wireguard" "${detail}"
}

# Log health/status collection events
log_health_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # collect_start, collect_end
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "health_${event_type}" "health" "${detail}"
}

# Log BGP maintenance events
log_bgp_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # maintenance_start, maintenance_end, reconnect
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "bgp_${event_type}" "bgp" "${detail}"
}

# Log BFD tick events
log_bfd_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # tick_start, tick_end
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "bfd_${event_type}" "bfd" "${detail}"
}

# Log status burst events
log_status_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # burst_start, burst_complete
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "status_${event_type}" "status" "${detail}"
}

# Log log emission events (for logging path attribution)
log_log_emit_event() {
    local tsv_path="$1"
    local elapsed="$2"
    local event_type="$3"  # emit
    local detail="${4:-}"
    log_event "${tsv_path}" "${elapsed}" "log_${event_type}" "logging" "${detail}"
}

analyze_staircase() {
    local tsv_path="$1"
    local artifact_path="$2"
    local event_timeline="$3"
    
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
                    steps_detected=$((steps_detected + 1))
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
    
    # Analyze event correlation
    local suspected_owner=""
    local owner_evidence=""
    local correlated_events=""
    
    # Count events by subsystem
    local heartbeat_count=0
    local wg_count=0
    local bgp_count=0
    local bfd_count=0
    local health_count=0
    local status_count=0
    
    while IFS=$'\t' read -r ts elapsed event subsystem detail; do
        case "${event}" in
            heartbeat_*) heartbeat_count=$((heartbeat_count + 1)) ;;
            wg_*) wg_count=$((wg_count + 1)) ;;
            bgp_*) bgp_count=$((bgp_count + 1)) ;;
            bfd_*) bfd_count=$((bfd_count + 1)) ;;
            health_*) health_count=$((health_count + 1)) ;;
            status_*) status_count=$((status_count + 1)) ;;
        esac
    done < <(tail -n +2 "${event_timeline}" 2>/dev/null || true)
    
    # Determine verdict based on evidence
    local verdict="inconclusive"
    local reason=""
    
    # NOTE: Shell-side synthetic events CANNOT produce confirmed_leak.
    # Real attribution requires tovarisch-native event emission.
    # Shell-side events may only enrich an inconclusive artifact.
    if [[ ${steps_detected} -ge 3 ]] && [[ ${total_growth} -gt 500 ]]; then
        verdict="inconclusive"
        reason="Staircase growth detected (${steps_detected} steps, ${total_growth} KiB total, ${growth_rate_per_min} KiB/min) but owner is unattributed. Events are shell-side synthetic and cannot be used for attribution. Need tovarisch-native event emission to identify the periodic background owner."
    elif [[ ${steps_detected} -ge 5 ]] && [[ ${total_growth} -gt 200 ]]; then
        # Medium confidence: multiple steps but less growth
        verdict="inconclusive"
        reason="Possible staircase pattern: ${steps_detected} steps, ${total_growth} KiB. Event counts: heartbeat=${heartbeat_count}, wg=${wg_count}, bgp=${bgp_count}, bfd=${bfd_count}. Need longer observation or targeted testing."
    elif [[ ${total_growth} -gt 1000 ]]; then
        verdict="bounded_warmup_or_allocator_highwater"
        reason="Detected ${total_growth} KiB growth but no clear staircase pattern (may be normal warmup or allocator high water mark settling). Event counts: heartbeat=${heartbeat_count}, wg=${wg_count}, bgp=${bgp_count}, bfd=${bfd_count}."
    elif [[ ${total_growth} -lt 200 ]]; then
        verdict="bounded_warmup_or_allocator_highwater"
        reason="Minimal growth detected (${total_growth} KiB) - likely bounded by allocator high water mark or normal warmup. Event counts: heartbeat=${heartbeat_count}, wg=${wg_count}, bgp=${bgp_count}, bfd=${bfd_count}."
    else
        verdict="inconclusive"
        reason="Growth pattern unclear: ${total_growth} KiB over ${DURATION}s with ${steps_detected} steps. Event counts: heartbeat=${heartbeat_count}, wg=${wg_count}, bgp=${bgp_count}, bfd=${bfd_count}."
    fi
    
    # Build correlated events string
    correlated_events="heartbeat=${heartbeat_count},wg=${wg_count},bgp=${bgp_count},bfd=${bfd_count},health=${health_count},status=${status_count}"
    
    # Write verdict with enhanced attribution fields
    cat > "${artifact_path}/verdict.txt" <<EOF
verdict: ${verdict}
owner: ${suspected_owner}
reason: ${reason}
steps_detected: ${steps_detected}
total_growth_kib: ${total_growth}
growth_rate_kib_per_min: ${growth_rate_per_min}
samples_count: ${#rss_values[@]}
suspected_owner: ${suspected_owner}
owner_evidence: ${owner_evidence}
correlated_events: ${correlated_events}
enabled_subsystems: heartbeat=${HEARTBEAT_ENABLED},wg=${WG_CHECK_ENABLED},bgp_bfd=${BGP_BFD_ENABLED}
disabled_subsystems: heartbeat=$([[ "${HEARTBEAT_ENABLED}" == "false" ]] && echo "heartbeat" || echo ""),wg=$([[ "${WG_CHECK_ENABLED}" == "false" ]] && echo "wg" || echo ""),bgp_bfd=$([[ "${BGP_BFD_ENABLED}" == "false" ]] && echo "bgp_bfd" || echo "")
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
    
    # Trace memory-related syscalls: brk, mmap, munmap, mremap
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
    echo "Subsystem toggles (SYNTHETIC - shell-side event logging only):"
    echo "  Heartbeat: ${HEARTBEAT_ENABLED}"
    echo "  WG checks: ${WG_CHECK_ENABLED}"
    echo "  BGP/BFD:   ${BGP_BFD_ENABLED}"
    echo "  No-subsystems (baseline): ${NO_SUBSYSTEMS}"
    echo ""
    echo "NOTE: These toggles only affect synthetic event emission to the event timeline."
    echo "      They do NOT disable actual tovarisch periodic paths (heartbeat, WG, BGP/BFD)."
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
    
    # Build environment for tovarisch
    local tovarisch_env=()
    if [[ -n "${TOVARISCH_WG_COMMAND_PATH:-}" ]]; then
        echo "Forcing WG command path: ${TOVARISCH_WG_COMMAND_PATH}"
        tovarisch_env+=("TOVARISCH_WG_COMMAND_PATH=${TOVARISCH_WG_COMMAND_PATH}")
    fi
    
    # Start tovarisch in background
    echo "Starting tovarisch on ${LAB_BIND}:${LAB_PORT}..."
    local tovarisch_pid
    
    if [[ ${#tovarisch_env[@]} -gt 0 ]]; then
        env "${tovarisch_env[@]}" "${TOVARISCH_BINARY}" serve --bind "${LAB_BIND}:${LAB_PORT}" &
    else
        "${TOVARISCH_BINARY}" serve --bind "${LAB_BIND}:${LAB_PORT}" &
    fi
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
    
    # Log lab start event
    log_event "${artifact_path}/event_timeline.tsv" 0 "lab_started" "lab" "tovarisch PID=${tovarisch_pid}"
    
    # Log subsystem configuration
    log_event "${artifact_path}/event_timeline.tsv" 0 "subsystem_config" "lab" "heartbeat=${HEARTBEAT_ENABLED},wg=${WG_CHECK_ENABLED},bgp_bfd=${BGP_BFD_ENABLED}"
    
    # Log periodic heartbeat events (every ~30 seconds) - but track whether it's actually enabled
    if [[ "${HEARTBEAT_ENABLED}" == "true" ]]; then
        log_event "${artifact_path}/event_timeline.tsv" 0 "heartbeat_enabled" "heartbeat" "30-second heartbeat interval"
    fi
    
    echo "Running idle for ${DURATION} seconds..."
    
    # Memory sampling loop
    local start_time
    start_time=$(date +%s)
    local sample_count=0
    local heartbeat_tick_count=0
    local last_wg_check=0
    local last_heartbeat_log=0
    
    while kill -0 "${tovarisch_pid}" 2>/dev/null; do
        local elapsed=$(( $(date +%s) - start_time ))
        
        # Sample memory
        local mem_values
        mem_values=$(sample_memory "${tovarisch_pid}")
        read -r rss vmdata vmhwm vmswap vmpeak vmrss_peak <<< "${mem_values}"
        
        local timestamp
        timestamp=$(date +%Y-%m-%dT%H:%M:%S.%3N)
        
        echo -e "${timestamp}\t${elapsed}\t${rss}\t${vmdata}\t${vmhwm}\t${vmswap}\t${vmpeak}\t${vmrss_peak}" >> "${artifact_path}/memory_samples.tsv"
        sample_count=$((sample_count + 1))
        
        # Log heartbeat tick events (every 30 seconds) - if heartbeat is enabled
        if [[ "${HEARTBEAT_ENABLED}" == "true" ]]; then
            if (( elapsed > 0 && elapsed % 30 == 0 && elapsed != last_heartbeat_log )); then
                heartbeat_tick_count=$((heartbeat_tick_count + 1))
                log_heartbeat_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "tick" "uptime=${elapsed}s,tick=${heartbeat_tick_count}"
                last_heartbeat_log=${elapsed}
            fi
        fi
        
        # Log periodic WG check events (every 60 seconds) - if WG checks enabled
        if [[ "${WG_CHECK_ENABLED}" == "true" ]]; then
            if (( elapsed > 0 && elapsed % 60 == 0 && elapsed != last_wg_check )); then
                log_wg_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "check" "periodic_60s_check"
                last_wg_check=${elapsed}
            fi
        fi
        
        # Log periodic BGP/BFD events (every 100ms) - if enabled, but at 10s intervals for noise reduction
        if [[ "${BGP_BFD_ENABLED}" == "true" ]]; then
            if (( elapsed > 0 && elapsed % 10 == 0 )); then
                log_bgp_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "maintenance" "periodic_maintenance"
                log_bfd_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "tick" "periodic_tick"
            fi
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
        log_status_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "burst_start" "5000 requests"
        
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
        
        log_status_event "${artifact_path}/event_timeline.tsv" "${final_elapsed}" "burst_complete" "duration=${burst_duration}s"
    fi
    
    # Stop strace if running
    if [[ -n "${strace_pid}" ]]; then
        kill "${strace_pid}" 2>/dev/null || true
    fi
    
    # Stop tovarisch
    log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "shutdown" "lab" "stopping tovarisch"
    kill "${tovarisch_pid}" 2>/dev/null || true
    wait "${tovarisch_pid}" 2>/dev/null || true
    
    # Analyze results with event correlation
    echo ""
    echo "Analyzing memory samples with event correlation..."
    local verdict
    verdict=$(analyze_staircase "${artifact_path}/memory_samples.tsv" "${artifact_path}" "${artifact_path}/event_timeline.tsv")
    
    echo ""
    echo "=== Lab Complete ==="
    echo "Artifact: ${artifact_path}"
    echo "Verdict: ${verdict}"
    echo ""
    cat "${artifact_path}/verdict.txt"
    
    exit 0
}

main "$@"
