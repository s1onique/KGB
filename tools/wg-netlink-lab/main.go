// wg-netlink-lab — WireGuard generic-netlink Linux runtime proof harness
//
// Command-line tool to prove the GenericNetlinkBackend works against a real
// WireGuard kernel interface. Creates wg-kgb0, queries it via generic netlink,
// and verifies no sensitive data is surfaced.
//
// Usage:
//   wg-netlink-lab preflight     # Check prerequisites (exits non-zero if can't run)
//   wg-netlink-lab setup        # Create wg-kgb0 interface
//   wg-netlink-lab proof        # Run wg_netlink_proof binary
//   wg-netlink-lab teardown     # Remove wg-kgb0 interface (only if we created it)
//   wg-netlink-lab full         # Run full proof (preflight -> setup -> proof -> teardown)
//
// Prerequisites:
//   - Linux kernel
//   - WireGuard kernel module (wireguard.ko)
//   - CAP_NET_ADMIN or root
//   - Generic netlink socket support
//
// NOT part of make gate - manual CI target for privileged runners.
//
// Hard blocker fixes implemented:
//   - Hard blocker 2: Returns non-zero if proof fails (fail-closed)
//   - Hard blocker 3: Tracks interface ownership to protect pre-existing interfaces
//   - Hard blocker 4: Preflight fails the full mode with proper exit codes
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runFull()
	}

	switch args[0] {
	case "preflight":
		return runPreflight()
	case "setup":
		return runSetup()
	case "proof":
		return runProof()
	case "teardown":
		return runTeardown()
	case "full":
		return runFull()
	default:
		return fmt.Errorf("unknown command: %s (valid: preflight, setup, proof, teardown, full)", args[0])
	}
}

// runFull runs the complete proof: preflight -> setup -> proof -> teardown.
// Hard blocker 4 fix: Preflight fails the full mode with proper exit codes.
func runFull() error {
	fmt.Printf("=== WireGuard Generic-Netlink Full Proof ===\n")
	fmt.Printf("Started: %s\n", time.Now().Format(time.RFC3339))

	// Step 1: Preflight (fail-closed)
	fmt.Printf("\n--- Step 1: Preflight ---\n")
	if err := runPreflight(); err != nil {
		return fmt.Errorf("full proof aborted at preflight: %w", err)
	}

	// Step 2: Setup (tracks ownership)
	fmt.Printf("\n--- Step 2: Setup ---\n")
	if err := runSetup(); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	// Step 3: Proof (fail-closed)
	fmt.Printf("\n--- Step 3: Run Proof ---\n")
	if err := runProof(); err != nil {
		return fmt.Errorf("proof failed: %w", err)
	}

	// Step 4: Teardown (protects pre-existing interfaces)
	fmt.Printf("\n--- Step 4: Teardown ---\n")
	if err := runTeardown(); err != nil {
		fmt.Printf("Teardown warning: %v\n", err)
	}

	fmt.Printf("\n--- Complete: %s ---\n", time.Now().Format(time.RFC3339))

	return nil
}
