#!/usr/bin/env bash
#===============================================================================
# Frontend Test Wrapper — bounded, observable, cleanup-safe
#
# Prevents unbounded Vitest worker fan-out and stale process accumulation.
# All normal local/quality-gate test entrypoints must route through this script.
#
# Environment:
#   UVB76_VITEST_MAX_WORKERS       - max parallel workers (default: 4)
#   UVB76_FRONTEND_TEST_TIMEOUT_SECONDS - outer timeout (default: 600 = 10min)
#   UVB76_REPO_PATH                - repo root for process detection (auto-detected)
#===============================================================================

set -euo pipefail

#-------------------------------------------------------------------------------
# Defaults
#-------------------------------------------------------------------------------
UVB76_VITEST_MAX_WORKERS="${UVB76_VITEST_MAX_WORKERS:-4}"
UVB76_FRONTEND_TEST_TIMEOUT_SECONDS="${UVB76_FRONTEND_TEST_TIMEOUT_SECONDS:-600}"
UVB76_REPO_PATH="${UVB76_REPO_PATH:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

# Vitest test timeout in milliseconds (10 seconds)
UVB76_VITEST_TEST_TIMEOUT="${UVB76_VITEST_TEST_TIMEOUT:-10000}"

# Lock directory
LOCK_DIR="${UVB76_REPO_PATH}/.tmp/frontend-test-locks"
LOCK_FILE="${LOCK_DIR}/wrapper.lock"

#-------------------------------------------------------------------------------
# Script arguments
#-------------------------------------------------------------------------------
PROFILE=0
KILL_STALE=0
SHARD_ARG=""
SHARD_N=""
SHARD_K=""

#-------------------------------------------------------------------------------
# Usage
#-------------------------------------------------------------------------------
usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Run frontend tests with bounded workers, process hygiene, and cleanup safety.

OPTIONS:
  --profile        Print timing/logging for slow file discovery
  --shard N K      Run shard K of N (deterministic file sharding)
  --kill-stale     Kill stale Vitest/node processes for this repo before running
  -h, --help       Show this help

ENVIRONMENT:
  UVB76_VITEST_MAX_WORKERS           Max parallel workers (default: 4)
  UVB76_FRONTEND_TEST_TIMEOUT_SECONDS Outer timeout in seconds (default: 600)
  UVB76_VITEST_TEST_TIMEOUT          Per-test timeout in ms (default: 10000)
  UVB76_REPO_PATH                    Repo path for process detection

EXIT CODES:
  0   Tests passed
  1   Tests failed or hygiene error
  2   Lock conflict (another test run in progress)
  3   Stale processes detected (use --kill-stale to resolve)
  4   Timeout or process cleanup failure
EOF
}

#-------------------------------------------------------------------------------
# Parse arguments
#-------------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      PROFILE=1
      shift
      ;;
    --shard)
      if [[ $# -lt 3 ]]; then
        echo "[error] --shard requires N and K arguments" >&2
        exit 1
      fi
      SHARD_N="$2"
      SHARD_K="$3"
      shift 3
      ;;
    --kill-stale)
      KILL_STALE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[error] Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

# Reject incompatible options: profile and shard together can cause confusing output
if [[ "$PROFILE" -eq 1 && -n "$SHARD_N" ]]; then
  echo "[error] --profile and --shard cannot be used together" >&2
  exit 1
fi

#-------------------------------------------------------------------------------
# Logging helpers
#-------------------------------------------------------------------------------
log() { echo "[frontend-test] $*"; }
warn() { echo "[frontend-test] WARNING: $*" >&2; }
error() { echo "[frontend-test] ERROR: $*" >&2; }

#-------------------------------------------------------------------------------
# Lock management
#-------------------------------------------------------------------------------
acquire_lock() {
  mkdir -p "${LOCK_DIR}"

  # Check for existing lock
  if [[ -f "${LOCK_FILE}" ]]; then
    local existing_info
    existing_info=$(cat "${LOCK_FILE}" 2>/dev/null || echo "")

    local existing_pid=""
    local existing_start=""
    if [[ -n "$existing_info" ]]; then
      existing_pid=$(echo "$existing_info" | head -1)
      existing_start=$(echo "$existing_info" | tail -n +2 | head -1)
    fi

    # Check if process is still alive
    if [[ -n "$existing_pid" ]] && kill -0 "$existing_pid" 2>/dev/null; then
      echo "[lock] Another frontend test run is active (PID: ${existing_pid})" >&2
      echo "[lock] Started at: ${existing_start:-unknown}" >&2
      echo "[lock] Use --kill-stale if this is stale, or wait for it to complete" >&2
      echo "[lock] Lock file: ${LOCK_FILE}" >&2
      exit 2
    else
      # Stale lock, remove it
      log "Removing stale lock file (PID: ${existing_pid})"
      rm -f "${LOCK_FILE}"
    fi
  fi

  # Write lock info
  echo "$$" > "${LOCK_FILE}"
  echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >> "${LOCK_FILE}"
  echo "${UVB76_REPO_PATH}" >> "${LOCK_FILE}"
}

release_lock() {
  rm -f "${LOCK_FILE}"
}

#-------------------------------------------------------------------------------
# Cleanup handler
#-------------------------------------------------------------------------------
CLEANUP_PIDS=()
trap 'cleanup_on_exit' EXIT INT TERM

cleanup_on_exit() {
  local exit_code=$?

  # Kill tracked PIDs
  for pid in "${CLEANUP_PIDS[@]:-}"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      log "Terminating child process $pid"
      kill -TERM "$pid" 2>/dev/null || true
    fi
  done

  # Kill any remaining Vitest workers for this repo
  kill_matching_vitest_workers

  # Release lock on any exit path
  release_lock 2>/dev/null || true

  return "$exit_code"
}

track_pid() {
  CLEANUP_PIDS+=("$$")
}

#-------------------------------------------------------------------------------
# Process detection
#-------------------------------------------------------------------------------
find_matching_vitest_processes() {
  # Find node/vitest processes that have our repo path in their command line
  # Exclude this wrapper script and its child processes to avoid false positives
  local my_pid=$$
  local my_ppid=$PPID
  local pids=()

  if [[ "$(uname)" == "Darwin" ]]; then
    # macOS: use -o with trailing = to omit headers, awk to filter
    # Must match vitest/node AND repo path, but NOT this wrapper or its children
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      local pid
      pid=$(echo "$line" | awk '{print $1}')
      # Skip invalid PIDs, and skip self/children (they contain repo path for other reasons)
      [[ -z "$pid" || ! "$pid" =~ ^[0-9]+$ || "$pid" -le 1 ]] && continue
      [[ "$pid" == "$my_pid" || "$pid" == "$my_ppid" ]] && continue
      pids+=("$pid")
    done < <(ps -axo pid=,command= 2>/dev/null | awk -v repo="${UVB76_REPO_PATH}" -v me="${BASH_SOURCE[0]}" '/vitest|node/ && index($0, repo) > 0 && index($0, me) == 0 {print $1}')
  else
    # Linux: use -o to omit headers, awk to filter
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      local pid
      pid=$(echo "$line" | awk '{print $1}')
      [[ -z "$pid" || ! "$pid" =~ ^[0-9]+$ || "$pid" -le 1 ]] && continue
      [[ "$pid" == "$my_pid" || "$pid" == "$my_ppid" ]] && continue
      pids+=("$pid")
    done < <(ps -eo pid=,args= 2>/dev/null | awk -v repo="${UVB76_REPO_PATH}" -v me="${BASH_SOURCE[0]}" '/vitest|node/ && index($0, repo) > 0 && index($0, me) == 0 {print $1}')
  fi

  printf '%s\n' "${pids[@]+"${pids[@]}"}"
}

kill_matching_vitest_workers() {
  local pids
  mapfile -t pids < <(find_matching_vitest_processes)

  if [[ ${#pids[@]} -eq 0 ]]; then
    return 0
  fi

  log "Found ${#pids[@]} matching Vitest/node processes to clean up: ${pids[*]}"

  # Graceful termination
  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      log "Sending SIGTERM to $pid"
      kill -TERM "$pid" 2>/dev/null || true
    fi
  done

  # Wait briefly
  sleep 1

  # Force kill any remaining
  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      log "Sending SIGKILL to $pid"
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
}

#-------------------------------------------------------------------------------
# Stale process detection
#-------------------------------------------------------------------------------
check_stale_processes() {
  local pids
  mapfile -t pids < <(find_matching_vitest_processes)

  # Filter out any invalid PIDs (non-numeric or empty)
  local valid_pids=()
  for pid in "${pids[@]}"; do
    if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ && "$pid" -gt 1 ]]; then
      valid_pids+=("$pid")
    fi
  done

  if [[ ${#valid_pids[@]} -gt 0 ]]; then
    echo "[hygiene] Found ${#valid_pids[@]} stale Vitest/node processes for this repo:"
    for pid in "${valid_pids[@]}"; do
      local cmd=""
      if [[ "$(uname)" == "Darwin" ]]; then
        cmd=$(ps -axo pid,command 2>/dev/null | grep "^ *$pid " | head -1)
      else
        cmd=$(ps aux 2>/dev/null | grep "^[^ ]* *$pid " | head -1)
      fi
      echo "  PID $pid: $cmd"
    done
    echo "[hygiene] Run with --kill-stale to clean them up before testing"
    return 1
  fi

  return 0
}

#-------------------------------------------------------------------------------
# Pre-flight checks
#-------------------------------------------------------------------------------
preflight_checks() {
  log "Starting frontend test hygiene gate"
  log "  Repo path: ${UVB76_REPO_PATH}"
  log "  Max workers: ${UVB76_VITEST_MAX_WORKERS}"
  log "  Test timeout: ${UVB76_VITEST_TEST_TIMEOUT}ms"
  log "  Outer timeout: ${UVB76_FRONTEND_TEST_TIMEOUT_SECONDS}s"

  # Check for stale processes
  if ! check_stale_processes; then
    if [[ "$KILL_STALE" -eq 1 ]]; then
      log "Killing stale processes..."
      kill_matching_vitest_workers
      sleep 2
      # Verify cleanup
      if ! check_stale_processes; then
        error "Stale process cleanup failed"
        exit 4
      fi
      log "Stale processes cleaned up successfully"
    else
      error "Stale Vitest/node processes detected. Use --kill-stale to resolve."
      exit 3
    fi
  fi
}

#-------------------------------------------------------------------------------
# Post-run verification
#-------------------------------------------------------------------------------
verify_no_leftover_processes() {
  local pids
  mapfile -t pids < <(find_matching_vitest_processes)

  # Filter out any invalid PIDs (non-numeric or empty)
  local valid_pids=()
  for pid in "${pids[@]}"; do
    if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ && "$pid" -gt 1 ]]; then
      valid_pids+=("$pid")
    fi
  done

  if [[ ${#valid_pids[@]} -gt 0 ]]; then
    warn "Found ${#valid_pids[@]} leftover Vitest/node processes after test completion"
    warn "Cleaning up leftovers..."
    kill_matching_vitest_workers
    error "Leftover processes detected and cleaned up. Test run may have issues."
    exit 4
  fi
}

#-------------------------------------------------------------------------------
# Main execution
#-------------------------------------------------------------------------------
main() {
  local web_dir="${UVB76_REPO_PATH}/uvb76/web"

  if [[ ! -d "$web_dir" ]]; then
    error "Frontend directory not found: ${web_dir}"
    exit 1
  fi

  if [[ ! -f "${web_dir}/package.json" ]]; then
    error "package.json not found in ${web_dir}"
    exit 1
  fi

  # Acquire lock
  acquire_lock

  # Pre-flight checks
  preflight_checks

  # Build Vitest command
  local vitest_cmd=("npx" "vitest" "run")

  # Add worker limit
  vitest_cmd+=("--maxWorkers" "${UVB76_VITEST_MAX_WORKERS}")

  # Add test timeout
  vitest_cmd+=("--testTimeout" "${UVB76_VITEST_TEST_TIMEOUT}")

  # Add profile flag if requested
  if [[ "$PROFILE" -eq 1 ]]; then
    vitest_cmd+=("--reporter=verbose")
  fi

  # Add shard if requested
  if [[ -n "$SHARD_N" && -n "$SHARD_K" ]]; then
    vitest_cmd+=("--shard" "${SHARD_N}/${SHARD_K}")
  fi

  log "Running: ${vitest_cmd[*]}"
  log "Working directory: ${web_dir}"

  # Run tests with timeout
  local test_start
  test_start=$(date +%s)

  (
    cd "$web_dir"
    "${vitest_cmd[@]}"
  ) &
  local test_pid=$!
  CLEANUP_PIDS+=("$test_pid")

  # Wait with timeout
  local timed_out=0
  while kill -0 "$test_pid" 2>/dev/null; do
    local elapsed
    elapsed=$(($(date +%s) - test_start))
    if [[ "$elapsed" -gt "$UVB76_FRONTEND_TEST_TIMEOUT_SECONDS" ]]; then
      warn "Test timeout reached (${UVB76_FRONTEND_TEST_TIMEOUT_SECONDS}s)"
      timed_out=1
      break
    fi
    sleep 1
  done

  local test_exit_code=0
  if [[ "$timed_out" -eq 1 ]]; then
    warn "Killing timed-out test process (PID: $test_pid)"
    kill -TERM "$test_pid" 2>/dev/null || true
    sleep 1
    kill -KILL "$test_pid" 2>/dev/null || true
    test_exit_code=124
  else
    wait "$test_pid" || test_exit_code=$?
  fi

  # Remove test_pid from cleanup list (it's done)
  CLEANUP_PIDS=("${CLEANUP_PIDS[@]/"$test_pid"}")

  # Verify no leftover processes
  verify_no_leftover_processes

  # Release lock
  release_lock

  local test_duration=$(( $(date +%s) - test_start ))
  log "Test run completed in ${test_duration}s with exit code ${test_exit_code}"

  if [[ "$test_exit_code" -eq 0 ]]; then
    log "All tests passed"
  else
    error "Tests failed with exit code ${test_exit_code}"
  fi

  exit "$test_exit_code"
}

main "$@"
