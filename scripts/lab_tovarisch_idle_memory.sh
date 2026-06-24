#!/bin/bash
# Idle staircase memory lab for tovarisch.
# Runs tovarisch idle and samples RSS/VmData to detect stepwise memory growth.
#
# Usage: $0 [--duration SECS] [--interval SECS] [--status-burst] [--strace] [--run-id ID]
#        [--heartbeat-only] [--wg-only] [--bgp-bfd-only] [--no-subsystems]
#        [--native-events] [--native-events-path PATH]
#        [--disable-heartbeat] [--disable-wg-checks] [--disable-bgp] [--disable-bfd]
#        [--native-heartbeat-only] [--native-wg-only] [--native-bgp-bfd-only]
#        [--native-no-periodic]
#
# Exit: 0=ok, 1=no-Linux, 2=no-binary, 3=setup-failed
set -euo pipefail

# Defaults
DURATION="${DURATION:-600}"
INTERVAL="${INTERVAL:-5}"
STATUS_BURST="${STATUS_BURST:-false}"
STRACE="${STRACE:-false}"
RUN_ID="${RUN_ID:-}"
HEARTBEAT_ENABLED="${HEARTBEAT_ENABLED:-true}"
WG_CHECK_ENABLED="${WG_CHECK_ENABLED:-true}"
BGP_BFD_ENABLED="${BGP_BFD_ENABLED:-true}"
NO_SUBSYSTEMS="${NO_SUBSYSTEMS:-false}"

# Native event options (tovarisch-native, not shell synthetic)
NATIVE_EVENTS="${NATIVE_EVENTS:-false}"
NATIVE_EVENTS_PATH="${NATIVE_EVENTS_PATH:-}"
DISABLE_HEARTBEAT="${DISABLE_HEARTBEAT:-false}"
DISABLE_WG_CHECKS="${DISABLE_WG_CHECKS:-false}"
DISABLE_BGP="${DISABLE_BGP:-false}"
DISABLE_BFD="${DISABLE_BFD:-false}"

# CLI parsing
while [[ $# -gt 0 ]]; do
    case "$1" in
        --duration) DURATION="${2}"; shift 2 ;;
        --interval) INTERVAL="${2}"; shift 2 ;;
        --status-burst) STATUS_BURST="true"; shift ;;
        --strace) STRACE="true"; shift ;;
        --run-id) RUN_ID="${2}"; shift 2 ;;
        --heartbeat-only) HEARTBEAT_ENABLED="true"; WG_CHECK_ENABLED="false"; BGP_BFD_ENABLED="false"; shift ;;
        --wg-only) HEARTBEAT_ENABLED="false"; WG_CHECK_ENABLED="true"; BGP_BFD_ENABLED="false"; shift ;;
        --bgp-bfd-only) HEARTBEAT_ENABLED="false"; WG_CHECK_ENABLED="false"; BGP_BFD_ENABLED="true"; shift ;;
        --no-subsystems) NO_SUBSYSTEMS="true"; HEARTBEAT_ENABLED="false"; WG_CHECK_ENABLED="false"; BGP_BFD_ENABLED="false"; shift ;;
        # Native event flags
        --native-events) NATIVE_EVENTS="true"; shift ;;
        --native-events-path) NATIVE_EVENTS_PATH="${2}"; shift 2 ;;
        --disable-heartbeat) DISABLE_HEARTBEAT="true"; shift ;;
        --disable-wg-checks) DISABLE_WG_CHECKS="true"; shift ;;
        --disable-bgp) DISABLE_BGP="true"; shift ;;
        --disable-bfd) DISABLE_BFD="true"; shift ;;
        # Native isolation modes (combinations of disable flags)
        --native-heartbeat-only) NATIVE_EVENTS="true"; DISABLE_HEARTBEAT="false"; DISABLE_WG_CHECKS="true"; DISABLE_BGP="true"; DISABLE_BFD="true"; shift ;;
        --native-wg-only) NATIVE_EVENTS="true"; DISABLE_HEARTBEAT="true"; DISABLE_WG_CHECKS="false"; DISABLE_BGP="true"; DISABLE_BFD="true"; shift ;;
        --native-bgp-bfd-only) NATIVE_EVENTS="true"; DISABLE_HEARTBEAT="true"; DISABLE_WG_CHECKS="true"; DISABLE_BGP="false"; DISABLE_BFD="false"; shift ;;
        --native-no-periodic) NATIVE_EVENTS="true"; DISABLE_HEARTBEAT="true"; DISABLE_WG_CHECKS="true"; DISABLE_BGP="true"; DISABLE_BFD="true"; shift ;;
        --help)
            cat <<'HELP'
Usage: $0 [--duration SECS] [--interval SECS] [--status-burst] [--strace] [--run-id ID]
  --duration       Lab duration in seconds (default: 600)
  --interval       Memory sample interval in seconds (default: 5)
  --status-burst   Run /status burst test after idle window
  --strace         Enable strace syscall tracing (Linux only)
  --run-id         Custom run identifier (default: auto-generated)

Shell-side synthetic event toggles (do NOT disable actual tovarisch paths):
  --heartbeat-only  Emit only heartbeat synthetic events
  --wg-only         Emit only WG check synthetic events
  --bgp-bfd-only    Emit only BGP/BFD synthetic events
  --no-subsystems   Suppress all synthetic events

Tovarisch-native event flags (emit events from real runtime paths):
  --native-events          Enable native event emission
  --native-events-path     Path for native_event_timeline.tsv (default: artifact dir)

Native runtime toggles (disable actual tovarisch periodic paths):
  --disable-heartbeat      Disable heartbeat runtime loop
  --disable-wg-checks      Disable WireGuard periodic checks
  --disable-bgp            Disable BGP maintenance/reconnect loop
  --disable-bfd            Disable BFD timer/tick loop

Native isolation modes (combinations of disable flags):
  --native-heartbeat-only  Enable heartbeat only, disable all other periodic paths
  --native-wg-only         Enable WG checks only, disable all other periodic paths
  --native-bgp-bfd-only    Enable BGP/BFD only, disable heartbeat and WG
  --native-no-periodic     Disable all periodic paths for baseline measurement

Environment: DURATION, INTERVAL, STATUS_BURST, STRACE, HEARTBEAT_ENABLED,
  WG_CHECK_ENABLED, BGP_BFD_ENABLED, TOVARISCH_WG_COMMAND_PATH, LAB_TOVARISCH_PORT,
  NATIVE_EVENTS, NATIVE_EVENTS_PATH, DISABLE_HEARTBEAT, DISABLE_WG_CHECKS,
  DISABLE_BGP, DISABLE_BFD
HELP
            exit 0 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TOVARISCH_BINARY="${REPO_ROOT}/tovarisch/zig-out/bin/tovarisch"
ARTIFACT_DIR="${REPO_ROOT}/artifacts/memory-labs/tovarisch/idle-staircase"
ANALYZER_SCRIPT="${SCRIPT_DIR}/idle_staircase_analyzer.py"
LAB_PORT="${LAB_TOVARISCH_PORT:-8317}"
LAB_BIND="${LAB_BIND:-127.0.0.1}"

# ============================================================================
# Helpers
# ============================================================================

is_linux() { [[ "$(uname)" == "Linux" ]] && [[ -r /proc/self/status ]]; }

sample_memory() {
    # Returns: rss_kib vmdata_kib vmhwm_kib vmswap_kib vmpeak_kib vmrss_peak_kib
    if ! is_linux; then echo "0 0 0 0 0 0"; return; fi
    local pid="$1" status_file="/proc/${pid}/status"
    if ! [[ -r "${status_file}" ]]; then echo "0 0 0 0 0 0"; return; fi
    local rss=0 vmdata=0 vmhwm=0 vmswap=0 vmpeak=0
    while IFS=: read -r key value; do
        value="${value// /}" value="${value//kB/}"
        case "${key}" in
            "VmRSS") rss="${value}" ;; "VmData") vmdata="${value}" ;;
            "VmHWM") vmhwm="${value}" ;; "VmSwap") vmswap="${value}" ;;
            "VmPeak") vmpeak="${value}" ;;
        esac
    done < "${status_file}"
    echo "${rss} ${vmdata} ${vmhwm} ${vmswap} ${vmpeak} ${vmhwm}"
}

log_event() {
    # $1=tsv_path $2=elapsed $3=event $4=subsystem $5=detail
    echo -e "$(date +%Y-%m-%dT%H:%M:%S.%3N)\t${2}\t${3}\t${4:-unknown}\t${5:-}" >> "$1"
}

# Build tovarisch config file for native event/lab settings
build_tovarisch_config() {
    local config_path="$1"
    cat > "${config_path}" <<EOF
[lab]
native_events_enabled = ${NATIVE_EVENTS}
native_events_path = "${NATIVE_EVENTS_PATH:-}"
disable_heartbeat = ${DISABLE_HEARTBEAT}
disable_wg_checks = ${DISABLE_WG_CHECKS}
disable_bgp = ${DISABLE_BGP}
disable_bfd = ${DISABLE_BFD}
EOF
}

# ============================================================================
# Main
# ============================================================================

main() {
    echo "=== Idle Staircase Memory Lab ==="
    echo "Platform: $(uname -s), Duration: ${DURATION}s, Interval: ${INTERVAL}s"
    echo ""
    echo "Shell-side synthetic event toggles (NO actual runtime effect):"
    echo "  Heartbeat: ${HEARTBEAT_ENABLED}, WG: ${WG_CHECK_ENABLED}, BGP/BFD: ${BGP_BFD_ENABLED}"
    echo ""
    echo "Tovarisch-native event emission: ${NATIVE_EVENTS}"
    echo "Native runtime toggles (REAL runtime effect):"
    echo "  Disable heartbeat: ${DISABLE_HEARTBEAT}"
    echo "  Disable WG checks: ${DISABLE_WG_CHECKS}"
    echo "  Disable BGP: ${DISABLE_BGP}"
    echo "  Disable BFD: ${DISABLE_BFD}"

    if ! is_linux; then echo "SKIP: Linux with /proc required."; exit 0; fi
    if ! [[ -x "${TOVARISCH_BINARY}" ]]; then echo "ERROR: tovarisch binary missing"; exit 2; fi

    # Create artifact dir
    [[ -z "${RUN_ID}" ]] && RUN_ID="idle-$(date +%Y%m%d-%H%M%S)-$$"
    local artifact_path="${ARTIFACT_DIR}/${RUN_ID}"
    mkdir -p "${artifact_path}"
    echo "Artifact: ${artifact_path}"

    # Build native event path if not set
    if [[ "${NATIVE_EVENTS}" == "true" ]] && [[ -z "${NATIVE_EVENTS_PATH}" ]]; then
        NATIVE_EVENTS_PATH="${artifact_path}/native_event_timeline.tsv"
    fi

    # Build tovarisch config for native settings
    local tovarisch_config="${artifact_path}/tovarisch_lab.conf"
    build_tovarisch_config "${tovarisch_config}"

    # Write manifest
    cat > "${artifact_path}/manifest.yaml" <<EOF
# Idle Staircase Memory Lab Manifest
# Generated: $(date -Iseconds)
run_id: "${RUN_ID}"
platform: $(uname -s -r)
kernel: $(uname -r)
architecture: $(uname -m)
duration_seconds: ${DURATION}
sample_interval_seconds: ${INTERVAL}
status_burst: ${STATUS_BURST}
strace_enabled: ${STRACE}
lab_port: ${LAB_PORT}
lab_bind: ${LAB_BIND}
# CRITICAL: Shell-side synthetic events CANNOT produce confirmed_leak verdicts
event_source: shell_synthetic
heartbeat_enabled: ${HEARTBEAT_ENABLED}
wg_check_enabled: ${WG_CHECK_ENABLED}
bgp_bfd_enabled: ${BGP_BFD_ENABLED}
no_subsystems: ${NO_SUBSYSTEMS}
# Native event source (tovarisch-native, can be used for confirmed_leak)
native_events_enabled: ${NATIVE_EVENTS}
native_events_path: "${NATIVE_EVENTS_PATH:-}"
native_disable_heartbeat: ${DISABLE_HEARTBEAT}
native_disable_wg_checks: ${DISABLE_WG_CHECKS}
native_disable_bgp: ${DISABLE_BGP}
native_disable_bfd: ${DISABLE_BFD}
tovarisch_binary: "${TOVARISCH_BINARY}"
binary_exists: $([[ -x "${TOVARISCH_BINARY}" ]] && echo "true" || echo "false")
commit_sha: $(cd "${REPO_ROOT}" && git rev-parse HEAD 2>/dev/null || echo "unknown")
git_dirty: $(cd "${REPO_ROOT}" && git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
lab_start_iso: $(date -Iseconds)
EOF

    # Write TSV headers
    echo -e "timestamp\telapsed_sec\trss_kib\tvmdata_kib\tvmhwm_kib\tvmswap_kib\tvmpeak_kib\tvmrss_peak_kib" > "${artifact_path}/memory_samples.tsv"
    echo -e "timestamp\telapsed_sec\tevent\tsubsystem\tdetail" > "${artifact_path}/event_timeline.tsv"

    # Start tovarisch with config if native events enabled
    echo "Starting tovarisch on ${LAB_BIND}:${LAB_PORT}..."
    local tovarisch_args=("serve" "--config" "${tovarisch_config}" "--listen" "${LAB_BIND}:${LAB_PORT}")
    
    if [[ -n "${TOVARISCH_WG_COMMAND_PATH:-}" ]]; then
        TOVARISCH_WG_COMMAND_PATH="${TOVARISCH_WG_COMMAND_PATH}" "${TOVARISCH_BINARY}" "${tovarisch_args[@]}" &
    else
        "${TOVARISCH_BINARY}" "${tovarisch_args[@]}" &
    fi
    local tovarisch_pid=$!
    sleep 2
    if ! kill -0 "${tovarisch_pid}" 2>/dev/null; then echo "ERROR: tovarisch failed to start"; exit 3; fi

    # Start strace if requested
    local strace_pid=""
    if [[ "${STRACE}" == "true" ]] && command -v strace &>/dev/null; then
        strace -p "${tovarisch_pid}" -f -e trace=brk,mmap,mmap2,mremap,munmap,execve -o "${artifact_path}/strace.log" &
        strace_pid=$!
    fi

    # Log lab start
    log_event "${artifact_path}/event_timeline.tsv" 0 "lab_started" "lab" "PID=${tovarisch_pid}"
    log_event "${artifact_path}/event_timeline.tsv" 0 "subsystem_config" "lab" "heartbeat=${HEARTBEAT_ENABLED},wg=${WG_CHECK_ENABLED},bgp_bfd=${BGP_BFD_ENABLED}"
    
    # Log native config if enabled
    if [[ "${NATIVE_EVENTS}" == "true" ]]; then
        log_event "${artifact_path}/event_timeline.tsv" 0 "native_config" "lab" "events=${NATIVE_EVENTS},heartbeat=${DISABLE_HEARTBEAT},wg=${DISABLE_WG_CHECKS},bgp=${DISABLE_BGP},bfd=${DISABLE_BFD}"
    fi

    echo "Running idle for ${DURATION} seconds..."
    local start_time=$(date +%s) sample_count=0 heartbeat_tick_count=0 last_wg_check=0 last_heartbeat_log=0

    # Memory sampling loop
    while kill -0 "${tovarisch_pid}" 2>/dev/null; do
        local elapsed=$(( $(date +%s) - start_time ))
        read -r rss vmdata vmhwm vmswap vmpeak _ <<< "$(sample_memory "${tovarisch_pid}")"
        echo -e "$(date +%Y-%m-%dT%H:%M:%S.%3N)\t${elapsed}\t${rss}\t${vmdata}\t${vmhwm}\t${vmswap}\t${vmpeak}\t${vmhwm}" >> "${artifact_path}/memory_samples.tsv"
        sample_count=$((sample_count + 1))

        # Log heartbeat tick (every 30s) - SHELL SYNTHETIC ONLY
        if [[ "${HEARTBEAT_ENABLED}" == "true" ]] && (( elapsed > 0 && elapsed % 30 == 0 && elapsed != last_heartbeat_log )); then
            heartbeat_tick_count=$((heartbeat_tick_count + 1))
            log_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "heartbeat_tick" "heartbeat" "uptime=${elapsed}s,tick=${heartbeat_tick_count}"
            last_heartbeat_log=${elapsed}
        fi
        # Log WG check (every 60s) - SHELL SYNTHETIC ONLY
        if [[ "${WG_CHECK_ENABLED}" == "true" ]] && (( elapsed > 0 && elapsed % 60 == 0 && elapsed != last_wg_check )); then
            log_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "wg_check" "wireguard" "periodic_60s_check"
            last_wg_check=${elapsed}
        fi
        # Log BGP/BFD (every 10s) - SHELL SYNTHETIC ONLY
        if [[ "${BGP_BFD_ENABLED}" == "true" ]] && (( elapsed > 0 && elapsed % 10 == 0 )); then
            log_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "bgp_maintenance" "bgp" "periodic_maintenance"
            log_event "${artifact_path}/event_timeline.tsv" "${elapsed}" "bfd_tick" "bfd" "periodic_tick"
        fi

        (( elapsed >= DURATION )) && break
        sleep "${INTERVAL}"
    done

    log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "idle_complete" "lab" "sampled=${sample_count} times"

    # Status burst test
    if [[ "${STATUS_BURST}" == "true" ]]; then
        echo "Running /status burst test..."
        log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "status_burst_start" "status" "5000 requests"
        local burst_start=$(date +%s)
        for i in $(seq 1 5000); do curl -s "http://${LAB_BIND}:${LAB_PORT}/status" > /dev/null 2>&1 || true; done
        local final_elapsed=$(( $(date +%s) - start_time ))
        read -r rss vmdata vmhwm vmswap vmpeak _ <<< "$(sample_memory "${tovarisch_pid}")"
        echo -e "$(date +%Y-%m-%dT%H:%M:%S.%3N)\t${final_elapsed}\t${rss}\t${vmdata}\t${vmhwm}\t${vmswap}\t${vmpeak}\t${vmhwm}" >> "${artifact_path}/memory_samples.tsv"
        log_event "${artifact_path}/event_timeline.tsv" "${final_elapsed}" "status_burst_complete" "status" "duration=$(( $(date +%s) - burst_start ))s"
    fi

    # Cleanup
    [[ -n "${strace_pid}" ]] && kill "${strace_pid}" 2>/dev/null || true
    log_event "${artifact_path}/event_timeline.tsv" "${elapsed:-${DURATION}}" "shutdown" "lab" "stopping"
    kill "${tovarisch_pid}" 2>/dev/null || true
    wait "${tovarisch_pid}" 2>/dev/null || true

    # Copy native event timeline if exists
    if [[ "${NATIVE_EVENTS}" == "true" ]] && [[ -n "${NATIVE_EVENTS_PATH}" ]] && [[ -f "${NATIVE_EVENTS_PATH}" ]]; then
        cp "${NATIVE_EVENTS_PATH}" "${artifact_path}/native_event_timeline.tsv"
    fi

    # Analyze verdict
    echo ""
    echo "Analyzing memory samples..."
    local verdict
    verdict=$(python3 "${ANALYZER_SCRIPT}" "${artifact_path}/memory_samples.tsv" "${artifact_path}" "${artifact_path}/event_timeline.tsv" \
        --duration "${DURATION}" --heartbeat-enabled "${HEARTBEAT_ENABLED}" --wg-enabled "${WG_CHECK_ENABLED}" --bgp-bfd-enabled "${BGP_BFD_ENABLED}" \
        --native-events "${NATIVE_EVENTS}" --native-event-timeline "${artifact_path}/native_event_timeline.tsv" 2>&1)

    echo ""
    echo "=== Lab Complete ==="
    echo "Artifact: ${artifact_path}"
    echo "Verdict: ${verdict}"
    echo ""
    cat "${artifact_path}/verdict.txt"
    exit 0
}

main "$@"
