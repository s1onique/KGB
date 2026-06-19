#!/bin/bash
# lab_uvb76_capture_netns_result_helpers.sh — Result helpers for UVB-76 capture netns lab
#
# Functions for verifying distinct event IDs and printing result summary.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Verify distinct event IDs across phases
verify_distinct_event_ids() {
    log_info ""
    log_info "=== Verifying Distinct Event IDs ==="

    # Only verify if all three phases found events
    if [[ -n "$PHASE1_EVENT_ID" && -n "$PHASE2_EVENT_ID" && -n "$PHASE3_EVENT_ID" ]]; then
        log_info "Phase 1 event_id: $PHASE1_EVENT_ID"
        log_info "Phase 2 event_id: $PHASE2_EVENT_ID"
        log_info "Phase 3 event_id: $PHASE3_EVENT_ID"

        # Check all three are distinct
        if [[ "$PHASE1_EVENT_ID" != "$PHASE2_EVENT_ID" && "$PHASE2_EVENT_ID" != "$PHASE3_EVENT_ID" && "$PHASE1_EVENT_ID" != "$PHASE3_EVENT_ID" ]]; then
            log_info "[PASS] All three event IDs are distinct"
            CONTRACT_DISTINCT_EVENT_IDS_OK=true
        else
            log_error "[FAIL] Event IDs are NOT distinct - phases may be sharing events"
            if [[ "$PHASE1_EVENT_ID" == "$PHASE2_EVENT_ID" ]]; then
                log_error "  Phase 1 and Phase 2 have same event_id: $PHASE1_EVENT_ID"
            fi
            if [[ "$PHASE2_EVENT_ID" == "$PHASE3_EVENT_ID" ]]; then
                log_error "  Phase 2 and Phase 3 have same event_id: $PHASE2_EVENT_ID"
            fi
            if [[ "$PHASE1_EVENT_ID" == "$PHASE3_EVENT_ID" ]]; then
                log_error "  Phase 1 and Phase 3 have same event_id: $PHASE1_EVENT_ID"
            fi
            CONTRACT_DISTINCT_EVENT_IDS_OK=false
        fi
    else
        log_warn "Cannot verify distinct event IDs - one or more phases missing event_id"
        log_warn "  Phase 1: ${PHASE1_EVENT_ID:-MISSING}"
        log_warn "  Phase 2: ${PHASE2_EVENT_ID:-MISSING}"
        log_warn "  Phase 3: ${PHASE3_EVENT_ID:-MISSING}"
        CONTRACT_DISTINCT_EVENT_IDS_OK=false
    fi
}

# Print lab result summary
print_lab_result_summary() {
    log_info ""
    log_info "Contract results:"
    log_info "  Phase 1 capture: $([ "$CONTRACT_PHASE1_CAPTURE_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info "  Phase 1 packet: $([ "$CONTRACT_PHASE1_PACKET_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info "  Phase 2 cooldown: $([ "$CONTRACT_PHASE2_COOLDOWN_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info "  Phase 3 capture: $([ "$CONTRACT_PHASE3_CAPTURE_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info "  Phase 3 packet: $([ "$CONTRACT_PHASE3_PACKET_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info "  Directory verification: $([ "$CONTRACT_DIR_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info "  Distinct event IDs: $([ "$CONTRACT_DISTINCT_EVENT_IDS_OK" = true ] && echo "PASS" || echo "FAIL")"
    log_info ""
}

# Determine and return exit code based on contract results
compute_lab_exit_code() {
    local exit_code=0
    if [[ "$PROBE_READY" != "true" ]]; then
        log_error "Probe readiness failed"
        exit_code=1
    fi
    if [[ "$CONTRACT_PHASE1_CAPTURE_OK" != "true" || "$CONTRACT_PHASE1_PACKET_OK" != "true" ]]; then
        log_error "Phase 1 contract failed"
        exit_code=1
    fi
    if [[ "$CONTRACT_PHASE2_COOLDOWN_OK" != "true" ]]; then
        log_error "Phase 2 cooldown contract failed"
        exit_code=1
    fi
    if [[ "$CONTRACT_PHASE3_CAPTURE_OK" != "true" || "$CONTRACT_PHASE3_PACKET_OK" != "true" ]]; then
        log_error "Phase 3 contract failed"
        exit_code=1
    fi
    if [[ "$CONTRACT_DIR_OK" != "true" ]]; then
        log_error "Directory contract verification failed"
        exit_code=1
    fi
    if [[ "$CONTRACT_DISTINCT_EVENT_IDS_OK" != "true" ]]; then
        log_error "Distinct event IDs verification failed"
        exit_code=1
    fi

    if [[ $exit_code -eq 0 ]]; then
        log_info "Result: PASS"
    else
        log_error "Result: FAIL"
    fi

    return $exit_code
}
