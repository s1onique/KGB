#!/bin/bash
# lab_uvb76_capture_netns_consts.sh — Lab constants for UVB-76 capture netns harness
#
# Constants for the UVB-76 diagnostic capture netns lab harness.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Lab constants
LAB_NAME="kgb-uvb76-capture-netns-lab"
NS_UVB76="kgb-lab-uvb76"
NS_TOVARISCH="kgb-lab-tovarisch"
VETH_UVB76="uvb76-veth"
VETH_TOVARISCH="tovarisch-veth"
IP_UVB76="10.88.76.1"
IP_TOVARISCH="10.88.76.2"
NETMASK="24"

# Port constants
TOVARISCH_PORT="8317"
UVB76_PORT="8316"

# Diagnostic peer base URL for tovarisch
DIAG_PEER_BASE_URL="http://10.88.76.2:8317"

# Wait times (seconds)
WAIT_TOVARISCH_START="5"
WAIT_UVB76_START="5"
WAIT_CAPTURE="10"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Network defect modes
DEFECT_MODE_NETEM="netem"
DEFECT_MODE_DROP="drop"

# Artifact file names (paths are set dynamically in lib based on LAB_DIR)
ARTIFACT_TOPOLOGY="topology.txt"
ARTIFACT_NS_UVB76_IP_ADDR="ns-uvb76-ip-addr.txt"
ARTIFACT_NS_UVB76_IP_ROUTE="ns-uvb76-ip-route.txt"
ARTIFACT_NS_TOVARISCH_IP_ADDR="ns-tovarisch-ip-addr.txt"
ARTIFACT_NS_TOVARISCH_IP_ROUTE="ns-tovarisch-ip-route.txt"
ARTIFACT_TOVARISCH_LISTEN_SOCKETS="tovarisch-listen-sockets.txt"
ARTIFACT_PING_BASELINE="ping-baseline.txt"
ARTIFACT_CURL_STATUS_BASELINE="curl-status-baseline.json"
ARTIFACT_CURL_STATUS_NETWORK_DIAG_BASELINE="curl-status-network-diag-baseline.json"
ARTIFACT_CURL_PEER_STATUS_NETWORK_DIAG="curl-peer-status-network-diag.txt"
ARTIFACT_CURL_PEER_STATUS_NETWORK_DIAG_EXITCODE="curl-peer-status-network-diag.exitcode"
ARTIFACT_CAPTURE_BASELINE="capture-baseline.json"
ARTIFACT_DEFECT_BEFORE="defect-before.txt"
ARTIFACT_DEFECT_TC_QDISC="defect-tc-qdisc.txt"
ARTIFACT_CAPTURE_DURING_DEFECT="capture-during-defect.json"
ARTIFACT_DEFECT_AFTER_CLEAR="defect-after-clear.txt"
ARTIFACT_CAPTURE_AFTER_RECOVERY="capture-after-recovery.json"
ARTIFACT_SPIKES_BASELINE="spikes-baseline.json"
ARTIFACT_SPIKES_DURING_DEFECT="spikes-during-defect.json"
ARTIFACT_SPIKES_AFTER_RECOVERY="spikes-after-recovery.json"
ARTIFACT_SPIKES_DURING_DEFECT_POLL="spikes-during-defect.poll-last.json"
ARTIFACT_SPIKES_AFTER_RECOVERY_POLL="spikes-after-recovery.poll-last.json"
ARTIFACT_LATENCY_DURING_DEFECT="latency-during-defect.json"
ARTIFACT_LATENCY_AFTER_RECOVERY="latency-after-recovery.json"
ARTIFACT_BASELINE_PROBE_READY="latency-baseline-probe-ready.json"
ARTIFACT_UVB76_PROBE_CAPTURE_EVENTS="uvb76-probe-capture-events.txt"
ARTIFACT_RESULT="result.json"
ARTIFACT_UVB76_LOG="uvb76.log"
ARTIFACT_TOVARISCH_LOG="tovarisch.log"
ARTIFACT_UVB76_CONFIG="uvb76.json"
ARTIFACT_TOVARISCH_CONFIG="tovarisch.conf"

# Phase-separated artifact names (for diagnostic packet contract verification)
# Phase 0: Baseline probe readiness
ARTIFACT_PHASE0_STATUS="phase0-status.json"

# Phase 1: First eligible spike (fresh-start capture)
ARTIFACT_PHASE1_SPIKE_EVENT="phase1-spike-event.json"
ARTIFACT_PHASE1_SPIKE_ROW="phase1-spike-row.json"
ARTIFACT_PHASE1_CAPTURE_PACKET="phase1-capture-packet.json"
ARTIFACT_PHASE1_CAPTURE_CONTRACT="phase1-capture-contract.json"

# Phase 2: Inside-cooldown spike (skipped_cooldown expected)
ARTIFACT_PHASE2_SPIKE_EVENT="phase2-spike-event.json"
ARTIFACT_PHASE2_SPIKE_ROW="phase2-spike-row.json"
ARTIFACT_PHASE2_CAPTURE_CONTRACT="phase2-capture-contract.json"

# Phase 3: Post-cooldown spike (re-capture expected)
ARTIFACT_PHASE3_SPIKE_EVENT="phase3-spike-event.json"
ARTIFACT_PHASE3_SPIKE_ROW="phase3-spike-row.json"
ARTIFACT_PHASE3_CAPTURE_PACKET="phase3-capture-packet.json"
ARTIFACT_PHASE3_CAPTURE_CONTRACT="phase3-capture-contract.json"

# Contract verification output
ARTIFACT_CONTRACT_VERIFIER_OUTPUT="contract-verifier-output.txt"
