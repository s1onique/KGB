// Setup for WireGuard generic-netlink proof harness.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// runSetup creates the wg-kgb0 interface.
// Hard blocker 3 fix: Tracks pre-existing interfaces to protect them.
func runSetup() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("setup requires Linux")
	}

	// Check if interface already exists BEFORE we touch anything
	ifacePath := filepath.Join("/sys/class/net", ifaceName)
	if _, err := os.Stat(ifacePath); err == nil {
		// Interface exists - mark as pre-existing and skip creation
		fmt.Printf("Interface %s already exists (pre-existing), skipping setup\n", ifaceName)
		gInterfaceState.preExisting = true
		gInterfaceState.createdByLab = false

		// Write interface state artifact
		state := InterfaceState{
			Interface:    ifaceName,
			PreExisting:   true,
			CreatedByLab:  false,
			TeardownAction: "preserve",
		}
		_ = writeInterfaceState(state)
		return nil
	}

	// Interface doesn't exist - we'll create it
	gInterfaceState.preExisting = false
	gInterfaceState.createdByLab = true

	// Create WireGuard interface using ip command
	fmt.Printf("Creating WireGuard interface %s...\n", ifaceName)

	cmd := exec.Command("ip", "link", "add", "dev", ifaceName, "type", "wireguard")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Write failure state
		state := InterfaceState{
			Interface:    ifaceName,
			PreExisting:  false,
			CreatedByLab: true,
			TeardownAction: "failed_create",
		}
		_ = writeInterfaceState(state)
		return fmt.Errorf("failed to create interface: %w", err)
	}

	// Set interface up
	cmd = exec.Command("ip", "link", "set", ifaceName, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Try to clean up
		_ = exec.Command("ip", "link", "del", ifaceName).Run()
		state := InterfaceState{
			Interface:    ifaceName,
			PreExisting:  false,
			CreatedByLab: true,
			TeardownAction: "failed_up",
		}
		_ = writeInterfaceState(state)
		return fmt.Errorf("failed to set interface up: %w", err)
	}

	// Write success state
	state := InterfaceState{
		Interface:    ifaceName,
		PreExisting:  false,
		CreatedByLab: true,
		TeardownAction: "delete",
	}
	_ = writeInterfaceState(state)

	fmt.Printf("Interface %s created and up\n", ifaceName)
	return nil
}
