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
