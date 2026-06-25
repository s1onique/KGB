// wg-netlink-lab — WireGuard generic-netlink Linux runtime proof harness
//
// Shared constants and types for the lab harness.
package main

const (
	// Interface name used for proof.
	ifaceName = "wg-kgb0"

	// Lab artifact directory.
	artifactDir = "./artifacts/wg-netlink-lab"

	// Proof binary path (relative to repo root).
	proofBinary = "./tovarisch/zig-out/bin/wg_netlink_proof"
)

// Interface ownership tracking:
// - preExisting tracks if the interface existed before we started
// - createdByLab tracks whether we (the lab) created the interface
type interfaceState struct {
	preExisting  bool
	createdByLab bool
}

var gInterfaceState = interfaceState{}
