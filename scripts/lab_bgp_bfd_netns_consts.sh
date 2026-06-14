#!/bin/bash
# lab_bgp_bfd_netns_consts.sh — Lab constants for netns harness
#
# Constants for the BGP/BFD netns lab harness.
# Sourced by lab_bgp_bfd_netns_lib.sh.

# Lab constants
LAB_NAME="kgb-bgp-bfd-lab"
NS_TOVARISCH="kgb-lab-tovarisch"
NS_BIRD="kgb-lab-bird"
VETH_TOVARISCH="veth-tovarisch"
VETH_BIRD="veth-bird"
IP_TOVARISCH="10.77.0.2"
IP_BIRD="10.77.0.1"
BIRD_AS="65002"
TOVARISCH_AS="65001"
BIRD_ROUTER_ID="10.77.0.1"
TOVARISCH_ROUTER_ID="10.77.0.2"
BFD_INTERVAL_MS="800"
BFD_MULTIPLIER="3"

# Bounded wait defaults (seconds)
WAIT_BIRD_START="5"
WAIT_BFD_CONVERGE="15"
WAIT_BGP_CONVERGE="20"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color
