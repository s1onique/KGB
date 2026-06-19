#!/bin/bash
# lab_uvb76_capture_netns_contract_normalizers.sh — Normalization helpers for phase artifacts
#
# Normalizes spike rows from the API to contract verifier shape.

# Normalize spike row's capture info to contract verifier's expected shape.
#
# EXTRACTION LOGIC (must match wait_for_spike_capture_after_event):
#   - Use the first capture's .captures[0]
#   - If .capture_status is present AND non-empty, use it
#   - Else if .status is present AND non-empty, use it
#   - Else unknown
#
# CANONICAL NORMALIZATION:
#   - ok|captured -> captured
#   - timeout|error|failed -> failed
#   - skipped_cooldown -> skipped_cooldown
#   - disabled -> disabled
#   - not_configured -> not_configured
#   - not_attempted -> not_attempted
#   - in_progress|pending -> pending
#   - empty/unknown -> pending (for missing captures)
#
normalize_spike_row_capture_contract() {
    local raw_row_file="$1"
    local out_file="$2"

    if [[ ! -f "$raw_row_file" ]]; then
        log_error "[FAIL] normalize_spike_row_capture_contract: raw file not found: $raw_row_file"
        return 1
    fi

    # Select the first capture from .captures[]
    local capture_info
    capture_info=$(jq '.captures[0] // null' "$raw_row_file" 2>/dev/null || echo "null")

    # Check if capture exists at all
    if [[ "$capture_info" == "null" || "$capture_info" == "" ]]; then
        # No captures - this is a missing capture case
        # Emit not_attempted for the normalized status
        jq -n \
            --arg capture_status "not_attempted" \
            --argjson capture_exists false \
            --argjson is_protected false \
            --argjson suppressed_by_cooldown false \
            --argjson cooldown_info null \
            '{
                capture_status: $capture_status,
                capture_exists: $capture_exists,
                is_protected: $is_protected,
                cooldown_info: $cooldown_info,
                suppressed_by_cooldown: $suppressed_by_cooldown
            }' > "$out_file"
        log_info "Normalized spike row saved (no captures): $out_file"
        return 0
    fi

    # EXTRACT raw status using the same rule as wait_for_spike_capture_after_event:
    # - Use .capture_status if present and non-empty
    # - Else use .status
    local raw_status
    local has_capture_status
    has_capture_status=$(echo "$capture_info" | jq -r '.capture_status // empty' 2>/dev/null)
    if [[ -n "$has_capture_status" ]]; then
        raw_status=$(echo "$capture_info" | jq -r '.capture_status' 2>/dev/null)
    else
        raw_status=$(echo "$capture_info" | jq -r '.status // "unknown"' 2>/dev/null)
    fi

    # NORMALIZE raw status to canonical form
    # This must match normalize_capture_status from lab_uvb76_capture_netns_capture_poll.sh
    local capture_status
    case "$raw_status" in
        ok|captured)      capture_status="captured" ;;
        timeout|error|failed) capture_status="failed" ;;
        skipped_cooldown)  capture_status="skipped_cooldown" ;;
        disabled)          capture_status="disabled" ;;
        not_configured)    capture_status="not_configured" ;;
        not_attempted)     capture_status="not_attempted" ;;
        in_progress|pending) capture_status="pending" ;;
        *)                 capture_status="unknown" ;;
    esac

    # Extract other fields
    # For captured rows, default capture_exists and is_protected to true
    # since the canonical status means the capture succeeded and packet exists
    local capture_exists is_protected suppressed_by_cooldown
    if [[ "$capture_status" == "captured" ]]; then
        capture_exists=$(jq -r '.capture_exists // true' <<< "$capture_info" 2>/dev/null)
        is_protected=$(jq -r '.is_protected // true' <<< "$capture_info" 2>/dev/null)
    else
        capture_exists=$(jq -r '.capture_exists // false' <<< "$capture_info" 2>/dev/null)
        is_protected=$(jq -r '.is_protected // false' <<< "$capture_info" 2>/dev/null)
    fi

    local cooldown_info_json="null"
    if jq -e '.cooldown_info != null' <<< "$capture_info" >/dev/null 2>&1; then
        cooldown_info_json=$(jq '.cooldown_info' <<< "$capture_info" 2>/dev/null)
        suppressed_by_cooldown="true"
    else
        suppressed_by_cooldown="false"
    fi

    jq -n \
        --arg capture_status "$capture_status" \
        --argjson capture_exists "$capture_exists" \
        --argjson is_protected "$is_protected" \
        --argjson suppressed_by_cooldown "$suppressed_by_cooldown" \
        --argjson cooldown_info "$cooldown_info_json" \
        '{
            capture_status: $capture_status,
            capture_exists: $capture_exists,
            is_protected: $is_protected,
            cooldown_info: $cooldown_info,
            suppressed_by_cooldown: $suppressed_by_cooldown
        }' > "$out_file"

    log_info "Normalized spike row saved: $out_file (raw_status=$raw_status -> normalized=$capture_status)"
    return 0
}

# Save Phase N spike row from spikes API response (normalized contract row)
save_phase_spike_row() {
    local phase_num="$1"
    local phase_row_file="$2"
    local spikes_file="$3"
    local event_id="$4"

    log_info "Saving Phase $phase_num spike row: event_id=$event_id"

    local raw_row_file
    raw_row_file=$(mktemp "/tmp/phase${phase_num}-raw-row-XXXXXX.json")

    local spike_row
    spike_row=$(jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0]' "$spikes_file" 2>/dev/null || echo "{}")

    if [[ "$spike_row" == "null" || "$spike_row" == "{}" ]]; then
        log_error "[FAIL] Phase $phase_num: spike event not found: event_id=$event_id"
        rm -f "$raw_row_file"
        return 1
    fi

    echo "$spike_row" | jq '.' > "$raw_row_file"

    if ! normalize_spike_row_capture_contract "$raw_row_file" "$phase_row_file"; then
        log_error "[FAIL] Phase $phase_num: failed to normalize spike row"
        rm -f "$raw_row_file"
        return 1
    fi

    rm -f "$raw_row_file"
    log_info "Phase $phase_num spike row saved: $phase_row_file"
    return 0
}

# Save Phase N capture packet from raw spike row.
# Extracts network_diag from various possible locations:
#   - .captures[].network_diag (direct)
#   - .captures[].packet.network_diag
#   - .captures[].diagnostics.network_diag
#   - .captures[].network_diag_packet.network_diag
#   - .network_diag (if row is already packet-shaped)
# Fails if no network_diag object exists.
save_phase_capture_packet_from_raw_row() {
    local phase_num="${1:-unknown}"
    local packet_file="${2:-}"
    local spike_row_file="${3:-}"

    # Validate required arguments
    if [[ -z "$packet_file" ]]; then
        log_error "[FAIL] save_phase_capture_packet_from_raw_row: missing packet_file argument"
        return 1
    fi
    if [[ -z "$spike_row_file" ]]; then
        log_error "[FAIL] save_phase_capture_packet_from_raw_row: missing spike_row_file argument"
        return 1
    fi
    if [[ ! -f "$spike_row_file" ]]; then
        log_error "[FAIL] save_phase_capture_packet_from_raw_row: file not found: $spike_row_file"
        return 1
    fi

    log_info "Extracting capture packet from spike row for Phase $phase_num"

    local network_diag="null"
    local network_diag_found=false

    # Try .captures[].network_diag (direct)
    if [[ "$network_diag_found" != "true" ]]; then
        if jq -e '.captures[0].network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            network_diag_found=true
            log_info "Found network_diag in .captures[0].network_diag"
        fi
    fi

    # Try .captures[].packet.network_diag
    if [[ "$network_diag_found" != "true" ]]; then
        if jq -e '.captures[0].packet.network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].packet.network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            network_diag_found=true
            log_info "Found network_diag in .captures[0].packet.network_diag"
        fi
    fi

    # Try .captures[].diagnostics.network_diag
    if [[ "$network_diag_found" != "true" ]]; then
        if jq -e '.captures[0].diagnostics.network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].diagnostics.network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            network_diag_found=true
            log_info "Found network_diag in .captures[0].diagnostics.network_diag"
        fi
    fi

    # Try .captures[].network_diag_packet.network_diag
    if [[ "$network_diag_found" != "true" ]]; then
        if jq -e '.captures[0].network_diag_packet.network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].network_diag_packet.network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            network_diag_found=true
            log_info "Found network_diag in .captures[0].network_diag_packet.network_diag"
        fi
    fi

    # Try .network_diag (if row is already packet-shaped)
    if [[ "$network_diag_found" != "true" ]]; then
        if jq -e '.network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            network_diag_found=true
            log_info "Found network_diag in root .network_diag"
        fi
    fi

    if [[ "$network_diag_found" != "true" ]]; then
        log_error "[FAIL] Phase $phase_num: no network_diag found in spike row"
        # Log available keys for debugging
        local available_keys
        available_keys=$(jq 'path(scalars) | map(tostring) | join(".")' "$spike_row_file" 2>/dev/null | head -20 || echo "unknown")
        log_error "  Sample paths in row: $available_keys"
        return 1
    fi

    # Verify network_diag is an object
    if ! echo "$network_diag" | jq -e 'type == "object"' >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase_num: network_diag is not an object"
        return 1
    fi

    jq -n \
        --arg phase "phase${phase_num}" \
        --argjson network_diag "$network_diag" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            network_diag: $network_diag,
            timestamp: $timestamp
        }' > "$packet_file"

    log_info "Phase $phase_num capture packet saved: $packet_file"
    return 0
}

# Legacy wrapper for backward compatibility
save_phase_capture_packet() {
    local phase_num="${1:-unknown}"
    local packet_file="${2:-}"
    local spike_row_file="${3:-}"

    save_phase_capture_packet_from_raw_row "$phase_num" "$packet_file" "$spike_row_file"
}

# Save Phase N spike event (full raw debug artifact)
save_phase_spike_event() {
    local phase_num="$1"
    local phase_event_file="$2"
    local spikes_response="$3"
    local event_id="$4"
    local reasons="$5"

    log_info "Saving Phase $phase_num spike event: event_id=$event_id"

    local spike_row
    spike_row=$(echo "$spikes_response" | jq --arg eid "$event_id" \
        '[.spikes[] | select(.event_id == $eid)] | .[0] // {}' 2>/dev/null || echo "{}")

    jq -n \
        --arg phase "phase${phase_num}" \
        --arg event_id "$event_id" \
        --arg reasons "$reasons" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --argjson spike_row "$spike_row" \
        '{
            phase: $phase,
            event_id: $event_id,
            reasons: $reasons,
            timestamp: $timestamp,
            spike_row: $spike_row
        }' > "$phase_event_file"

    log_info "Phase $phase_num spike event saved: $phase_event_file"
}

# Save Phase N contract summary.
# Arguments:
#   phase_num: phase identifier (e.g., "1", "2", "3")
#   contract_file: output file path (required)
#   spike_row_file: normalized spike row file (optional, can be empty for skipped phases)
#   packet_file: capture packet file (optional, can be empty for skipped cooldown phases)
save_phase_contract_summary() {
    local phase_num="${1:-unknown}"
    local contract_file="${2:-}"
    local spike_row_file="${3:-}"
    local packet_file="${4:-}"

    # Validate required arguments
    if [[ -z "$contract_file" ]]; then
        log_error "[FAIL] save_phase_contract_summary: missing contract_file argument"
        return 1
    fi

    log_info "Generating contract summary for Phase $phase_num"

    local capture_status="unknown" capture_exists="false" is_protected="false"
    local cooldown_info_present="false" last_successful_capture_at_present="false"
    local next_capture_eligible_at_present="false" network_diag_present="false" packet_contract_ok="false"

    if [[ -n "$spike_row_file" && -f "$spike_row_file" ]]; then
        capture_status=$(jq -r '.capture_status // "unknown"' "$spike_row_file" 2>/dev/null || echo "unknown")
        capture_exists=$(jq -r '.capture_exists // false' "$spike_row_file" 2>/dev/null || echo "false")
        is_protected=$(jq -r '.is_protected // false' "$spike_row_file" 2>/dev/null || echo "false")

        if jq -e '.cooldown_info != null' "$spike_row_file" >/dev/null 2>&1; then
            cooldown_info_present="true"
            jq -e '.cooldown_info.last_successful_capture_at != null' "$spike_row_file" >/dev/null 2>&1 && last_successful_capture_at_present="true"
            jq -e '.cooldown_info.next_capture_eligible_at != null' "$spike_row_file" >/dev/null 2>&1 && next_capture_eligible_at_present="true"
        fi
    fi

    if [[ -n "$packet_file" && -f "$packet_file" ]]; then
        if jq -e '.network_diag != null' "$packet_file" >/dev/null 2>&1; then
            network_diag_present="true"
        fi
        "${SCRIPT_DIR}/verify_uvb76_diag_packet_contract.sh" --capture "$packet_file" --phase "phase${phase_num}" >/dev/null 2>&1 && packet_contract_ok="true"
    fi

    jq -n \
        --arg phase "phase${phase_num}" \
        --arg capture_status "$capture_status" \
        --argjson capture_exists "$capture_exists" \
        --argjson is_protected "$is_protected" \
        --argjson cooldown_info_present "$cooldown_info_present" \
        --argjson last_successful_capture_at_present "$last_successful_capture_at_present" \
        --argjson next_capture_eligible_at_present "$next_capture_eligible_at_present" \
        --argjson network_diag_present "$network_diag_present" \
        --argjson packet_contract_ok "$packet_contract_ok" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            capture_status: $capture_status,
            capture_exists: $capture_exists,
            is_protected: $is_protected,
            cooldown_info_present: $cooldown_info_present,
            last_successful_capture_at_present: $last_successful_capture_at_present,
            next_capture_eligible_at_present: $next_capture_eligible_at_present,
            network_diag_present: $network_diag_present,
            packet_contract_ok: $packet_contract_ok,
            timestamp: $timestamp
        }' > "$contract_file"

    log_info "Phase $phase_num contract summary saved: $contract_file"
}
