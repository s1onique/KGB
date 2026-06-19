#!/bin/bash
# lab_uvb76_capture_netns_contract_assertions.sh — Contract assertion helpers
#
# NOTE: The canonical assertion functions are defined in
# lab_uvb76_capture_netns_contract.sh. This file provides additional
# helpers that are sourced after to avoid conflicts.
#
# DO NOT redefine assert_captured_row_contract or assert_skipped_cooldown_row_contract
# here - they are already defined in contract.sh with normalized row validation.
