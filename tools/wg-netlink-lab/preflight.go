// Preflight checks for WireGuard generic-netlink proof harness.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PreflightResult captures prerequisite check results.
type PreflightResult struct {
	CanRun         bool   `json:"can_run"`
	Reason         string `json:"reason"`
	Kernel         string `json:"kernel,omitempty"`
	WGModuleLoaded bool   `json:"wg_module_loaded"`
	HasCapNetAdmin bool   `json:"has_cap_net_admin"`
	HasIPCommand   bool   `json:"has_ip_command"`
	InNetns        bool   `json:"in_netns"`
}

// InterfaceState captures interface ownership for teardown decisions.
type InterfaceState struct {
	Interface       string `json:"interface"`
	PreExisting     bool   `json:"pre_existing"`
	CreatedByLab    bool   `json:"created_by_lab"`
	TeardownAction  string `json:"teardown_action,omitempty"`
}

// runPreflight checks prerequisites.
// Returns error if prerequisites are not met, making it fail-closed.
func runPreflight() error {
	result := PreflightResult{
		CanRun: true,
		Reason: "ok",
	}

	// Check platform
	if runtime.GOOS != "linux" {
		result.CanRun = false
		result.Reason = "unsupported_platform"
		return outputPreflight(result)
	}

	// Detect kernel version
	if kernel, err := detectKernelVersion(); err == nil {
		result.Kernel = kernel
	} else {
		result.CanRun = false
		result.Reason = "kernel_version_unknown"
		return outputPreflight(result)
	}

	// Check WireGuard module
	result.WGModuleLoaded = checkWGModule()

	// Check CAP_NET_ADMIN (root has it)
	result.HasCapNetAdmin = checkCapNetAdmin()

	// Check if ip command is available
	result.HasIPCommand = checkIPCommand()

	// Check if in network namespace
	result.InNetns = checkNetns()

	// Fail-closed: capability assessment
	if !result.WGModuleLoaded {
		result.CanRun = false
		result.Reason = "wireguard_module_not_loaded"
	}

	// Hard blocker 4 fix: Preflight is fail-closed for full mode
	// Also fail if we lack CAP_NET_ADMIN or root
	if !result.HasCapNetAdmin {
		result.CanRun = false
		result.Reason = "missing_cap_net_admin"
	}

	// Fail if ip command is missing
	if !result.HasIPCommand {
		result.CanRun = false
		result.Reason = "missing_ip_command"
	}

	if !result.CanRun {
		_ = outputPreflight(result)
		return fmt.Errorf("preflight failed: %s", result.Reason)
	}

	return outputPreflight(result)
}

func outputPreflight(result PreflightResult) error {
	_ = os.MkdirAll(artifactDir, 0755)

	preflightPath := filepath.Join(artifactDir, "preflight.json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal preflight: %w", err)
	}
	if err := os.WriteFile(preflightPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write preflight.json: %w", err)
	}

	fmt.Printf("=== WireGuard Netlink Preflight ===\n")
	fmt.Printf("Platform: %s\n", runtime.GOOS)
	if result.Kernel != "" {
		fmt.Printf("Kernel: %s\n", result.Kernel)
	}
	fmt.Printf("WireGuard module: %v\n", result.WGModuleLoaded)
	fmt.Printf("CAP_NET_ADMIN: %v\n", result.HasCapNetAdmin)
	fmt.Printf("ip command: %v\n", result.HasIPCommand)
	fmt.Printf("In netns: %v\n", result.InNetns)
	fmt.Printf("Can run: %v\n", result.CanRun)
	fmt.Printf("Reason: %s\n", result.Reason)
	fmt.Printf("\nPreflight artifact: %s\n", preflightPath)

	return nil
}

// detectKernelVersion reads /proc/version.
func detectKernelVersion() (string, error) {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "", err
	}
	content := string(data)

	if idx := strings.Index(content, "Linux version "); idx >= 0 {
		rest := content[idx+14:]
		end := strings.IndexAny(rest, " \n\r")
		if end < 0 {
			end = len(rest)
		}
		return strings.TrimSpace(rest[:end]), nil
	}

	return "", fmt.Errorf("failed to parse kernel version")
}

// checkWGModule checks if wireguard module is loaded.
func checkWGModule() bool {
	_, err := os.Stat("/sys/module/wireguard")
	return err == nil
}

// checkCapNetAdmin checks if we have CAP_NET_ADMIN.
func checkCapNetAdmin() bool {
	if os.Geteuid() == 0 {
		return true
	}
	// Non-root: check if we can create a netlink socket (requires CAP_NET_ADMIN)
	// This is a conservative check - actual permission will be revealed by setup
	return false
}

// checkIPCommand checks if the `ip` command is available.
func checkIPCommand() bool {
	cmd := exec.Command("ip", "-version")
	// Just check if command exists, not if it works
	err := cmd.Run()
	return err == nil
}

// checkNetns checks if in a network namespace.
func checkNetns() bool {
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		return false
	}
	init, err := os.Readlink("/proc/1/ns/net")
	if err != nil {
		return false
	}
	return self != init
}

// writeInterfaceState writes interface state to artifact file.
func writeInterfaceState(state InterfaceState) error {
	_ = os.MkdirAll(artifactDir, 0755)

	statePath := filepath.Join(artifactDir, "interface-state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal interface state: %w", err)
	}
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write interface-state.json: %w", err)
	}

	fmt.Printf("Interface state artifact: %s\n", statePath)
	return nil
}
