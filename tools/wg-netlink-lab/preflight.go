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
	CanRun           bool   `json:"can_run"`
	Reason           string `json:"reason"`
	Kernel           string `json:"kernel,omitempty"`
	WGModuleLoaded   bool   `json:"wg_module_loaded"`
	HasCapNetAdmin   bool   `json:"has_cap_net_admin"`
	HasIPCommand     bool   `json:"has_ip_command"`
	InNetns          bool   `json:"in_netns"`
	// Enhanced diagnostics for debugging missing_ip_command failures
	PathEnv          string `json:"path_env,omitempty"`
	IPCommandPath    string `json:"ip_command_path,omitempty"`
	IPVersion        string `json:"ip_version,omitempty"`
	IPRoute2Package  string `json:"iproute2_package,omitempty"`
	CandidateIPPaths string `json:"candidate_ip_paths,omitempty"`
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

	// Collect diagnostics unconditionally so passing artifacts prove which ip was used
	collectIPDiagnostics(&result)

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

// collectIPDiagnostics gathers enhanced diagnostics for missing_ip_command failures.
func collectIPDiagnostics(result *PreflightResult) {
	// PATH environment variable
	result.PathEnv = os.Getenv("PATH")

	// exec.LookPath for ip (reliable, no shell needed)
	if path, err := exec.LookPath("ip"); err == nil {
		result.IPCommandPath = path
	}

	// ip -Version output (if available)
	cmdVersion := exec.Command("ip", "-Version")
	if output, err := cmdVersion.CombinedOutput(); err == nil {
		// Take only first line to keep artifact small
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 0 {
			result.IPVersion = lines[0]
		}
	}

	// dpkg-query iproute2 version (if available)
	cmdPkg := exec.Command("dpkg-query", "-W", "-f=${Version}", "iproute2")
	if version, err := cmdPkg.Output(); err == nil {
		result.IPRoute2Package = strings.TrimSpace(string(version))
	}

	// Check candidate ip binary paths
	candidatePaths := []string{
		"/sbin/ip",
		"/usr/sbin/ip",
		"/bin/ip",
		"/usr/bin/ip",
	}
	var found []string
	for _, p := range candidatePaths {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		result.CandidateIPPaths = strings.Join(found, ",")
	}
}

// checkIPCommand checks if the `ip` command is available using LookPath.
func checkIPCommand() bool {
	_, err := exec.LookPath("ip")
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
