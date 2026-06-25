// Teardown for WireGuard generic-netlink proof harness.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// loadInterfaceState reads interface state from artifact file if present.
func loadInterfaceState() *InterfaceState {
	statePath := filepath.Join(artifactDir, "interface-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil
	}
	var state InterfaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return &state
}

// runTeardown removes the wg-kgb0 interface.
// Hard blocker 3 fix: Only deletes if we created it.
func runTeardown() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("teardown requires Linux")
	}

	// Try to load interface state from artifact
	savedState := loadInterfaceState()
	if savedState != nil {
		fmt.Printf("Loaded interface state from artifact: pre_existing=%v, created_by_lab=%v\n",
			savedState.PreExisting, savedState.CreatedByLab)

		// Protect pre-existing interfaces
		if savedState.PreExisting {
			fmt.Printf("Interface %s was pre-existing, NOT deleting\n", ifaceName)
			return nil
		}

		// Only delete if we created it
		if !savedState.CreatedByLab {
			ifacePath := filepath.Join("/sys/class/net", ifaceName)
			if _, err := os.Stat(ifacePath); err != nil {
				fmt.Printf("Interface %s does not exist, skipping teardown\n", ifaceName)
				return nil
			}
			fmt.Printf("Interface %s exists but was not created by lab, NOT deleting\n", ifaceName)
			return nil
		}

		// We created it, delete it
		fmt.Printf("Deleting WireGuard interface %s (created by lab)...\n", ifaceName)
		cmd := exec.Command("ip", "link", "del", ifaceName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to delete interface: %w", err)
		}

		// Update artifact with teardown action
		savedState.TeardownAction = "deleted"
		_ = writeInterfaceState(*savedState)

		fmt.Printf("Interface %s deleted\n", ifaceName)
		return nil
	}

	// No artifact found - use in-memory state (for full mode)
	// Protect pre-existing interfaces
	if gInterfaceState.preExisting {
		fmt.Printf("Interface %s was pre-existing, NOT deleting\n", ifaceName)
		state := InterfaceState{
			Interface:       ifaceName,
			PreExisting:     true,
			CreatedByLab:    false,
			TeardownAction:  "preserve",
		}
		_ = writeInterfaceState(state)
		return nil
	}

	// Only delete if we created it
	if !gInterfaceState.createdByLab {
		ifacePath := filepath.Join("/sys/class/net", ifaceName)
		if _, err := os.Stat(ifacePath); err != nil {
			fmt.Printf("Interface %s does not exist, skipping teardown\n", ifaceName)
			return nil
		}
		fmt.Printf("Interface %s exists but was not created by lab, NOT deleting\n", ifaceName)
		state := InterfaceState{
			Interface:       ifaceName,
			PreExisting:     false,
			CreatedByLab:    false,
			TeardownAction:  "preserve_unknown",
		}
		_ = writeInterfaceState(state)
		return nil
	}

	// Delete interface (we created it)
	fmt.Printf("Deleting WireGuard interface %s (created by lab)...\n", ifaceName)

	cmd := exec.Command("ip", "link", "del", ifaceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete interface: %w", err)
	}

	state := InterfaceState{
		Interface:       ifaceName,
		PreExisting:     false,
		CreatedByLab:    true,
		TeardownAction: "deleted",
	}
	_ = writeInterfaceState(state)

	fmt.Printf("Interface %s deleted\n", ifaceName)
	return nil
}
