#!/bin/bash
# lab_uvb76_capture_netns_fetch_helpers.sh — Fetch helpers for network diag extraction
#
# Extracted fetch helper functions to keep contract normalizers under LLM-friendly limits.
# Sourced by lab_uvb76_capture_netns_lib.sh.
#
# This file contains the extract_network_diag_from_spike_row function which extracts
# the event-specific stored capture packet from spike rows.

# Extract network_diag from spike row's stored capture.
# This extracts the event-specific stored capture packet, NOT a live /status fetch.
# Uses spike row .captures[].network_diag as the canonical stored artifact.
#
# Outputs:
#   - ${packet_file} = extracted network_diag object
#   - ${summary_file} = fetch summary with is_fallback=false (stored artifact used)
#
# Returns 0 on success, non-zero on failure.
extract_network_diag_from_spike_row() {
    local phase="${1:-unknown}"
    local spike_row_file="${2:-}"
    local packet_file="${3:-}"
    local summary_file="${4:-}"
    
    # Validate required arguments
    if [[ -z "$spike_row_file" ]]; then
        log_error "[FAIL] extract_network_diag_from_spike_row: missing spike_row_file argument"
        return 1
    fi
    if [[ ! -f "$spike_row_file" ]]; then
        log_error "[FAIL] extract_network_diag_from_spike_row: file not found: $spike_row_file"
        return 1
    fi
    if [[ -z "$packet_file" ]]; then
        log_error "[FAIL] extract_network_diag_from_spike_row: missing packet_file argument"
        return 1
    fi
    
    log_info "Extracting network_diag from stored spike row for phase $phase"
    
    # Extract network_diag from spike row's captures[]
    # Try various possible locations for the stored network_diag
    local network_diag="null"
    local found_location=""
    
    # Try .captures[0].network_diag (direct - this is the canonical location)
    if [[ "$network_diag" == "null" ]]; then
        if jq -e '.captures[0].network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            if [[ "$network_diag" != "null" ]]; then
                found_location=".captures[0].network_diag"
                log_info "Found network_diag in $found_location"
            fi
        fi
    fi
    
    # Try .captures[0].packet.network_diag (alternative location)
    if [[ "$network_diag" == "null" ]]; then
        if jq -e '.captures[0].packet.network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].packet.network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            if [[ "$network_diag" != "null" ]]; then
                found_location=".captures[0].packet.network_diag"
                log_info "Found network_diag in $found_location"
            fi
        fi
    fi
    
    # Try .captures[0].diagnostics.network_diag (another alternative)
    if [[ "$network_diag" == "null" ]]; then
        if jq -e '.captures[0].diagnostics.network_diag != null' "$spike_row_file" >/dev/null 2>&1; then
            network_diag=$(jq '.captures[0].diagnostics.network_diag' "$spike_row_file" 2>/dev/null || echo "null")
            if [[ "$network_diag" != "null" ]]; then
                found_location=".captures[0].diagnostics.network_diag"
                log_info "Found network_diag in $found_location"
            fi
        fi
    fi
    
    if [[ "$network_diag" == "null" ]]; then
        log_error "[FAIL] Phase $phase: no network_diag found in spike row"
        # Log available keys for debugging
        local available_keys
        available_keys=$(jq 'path(scalars) | map(tostring) | join(".")' "$spike_row_file" 2>/dev/null | head -20 || echo "unknown")
        log_error "  Sample paths in row: $available_keys"
        
        # Write failure summary
        if [[ -n "$summary_file" ]]; then
            jq -n \
                --arg phase "phase${phase}" \
                --arg status "no_network_diag_in_spike_row" \
                --arg reason "network_diag_not_found_in_stored_capture" \
                --argjson is_fallback false \
                --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                '{
                    phase: $phase,
                    status: $status,
                    reason: $reason,
                    is_fallback: $is_fallback,
                    timestamp: $timestamp
                }' > "$summary_file" 2>/dev/null || true
        fi
        return 1
    fi
    
    # Verify network_diag is an object
    if ! echo "$network_diag" | jq -e 'type == "object"' >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: network_diag is not an object"
        return 1
    fi
    
    # Write packet file with network_diag
    jq -n \
        --arg phase "phase${phase}" \
        --argjson network_diag "$network_diag" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            network_diag: $network_diag,
            timestamp: $timestamp
        }' > "$packet_file"
    
    log_info "[PASS] Phase $phase capture packet saved from stored artifact: $packet_file"
    
    # Extract metadata for summary
    local event_id source requested_path capture_id
    event_id=$(jq -r '.event_id // empty' "$spike_row_file" 2>/dev/null || echo "")
    source=$(jq -r '.captures[0].source // empty' "$spike_row_file" 2>/dev/null || echo "")
    requested_path=$(jq -r '.captures[0].requested_path // empty' "$spike_row_file" 2>/dev/null || echo "")
    capture_id=$(jq -r '.captures[0].referenced_capture_id // .captures[0].capture_id // empty' "$spike_row_file" 2>/dev/null || echo "")
    
    # Write success summary with is_fallback=false (stored artifact used)
    if [[ -n "$summary_file" ]]; then
        jq -n \
            --arg phase "phase${phase}" \
            --arg status "success" \
            --argjson is_fallback false \
            --arg summary_source "stored_spike_row_capture" \
            --arg found_location "$found_location" \
            --arg event_id "$event_id" \
            --arg capture_source "$source" \
            --arg requested_path "$requested_path" \
            --arg capture_id "$capture_id" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{
                phase: $phase,
                status: $status,
                is_fallback: $is_fallback,
                summary_source: $summary_source,
                found_location: $found_location,
                event_id: $event_id,
                capture_source: $capture_source,
                requested_path: (if $requested_path == "" then null else $requested_path end),
                capture_id: (if $capture_id == "" then null else $capture_id end),
                timestamp: $timestamp
            }' > "$summary_file" 2>/dev/null || true
        log_info "  Saved fetch summary: $summary_file (is_fallback=false, stored artifact)"
    fi
    
    return 0
}
