// Proof execution for WireGuard generic-netlink proof harness.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ProofResult captures the netlink proof result.
type ProofResult struct {
	Success         bool   `json:"success"`
	Interface       string `json:"interface,omitempty"`
	PeerCount       uint32 `json:"peer_count,omitempty"`
	ListenPort      *uint16 `json:"listen_port,omitempty"`
	BackendKind     string `json:"backend_kind"`
	Error           string `json:"error,omitempty"`
	NoSensitiveData bool   `json:"no_sensitive_data"`
}

// getProofBinaryPath returns the absolute path to the proof binary.
// Resolves relative to repo root, not the binary directory.
func getProofBinaryPath() (string, error) {
	// proofBinary is relative: "./tovarisch/zig-out/bin/wg_netlink_proof"
	// Resolve relative to current working directory
	absPath, err := filepath.Abs(proofBinary)
	if err != nil {
		return "", fmt.Errorf("failed to resolve proof binary path: %w", err)
	}
	return absPath, nil
}

// runProof runs the wg_netlink_proof binary.
// Hard blocker 2 fix: Returns non-zero if proof fails.
func runProof() error {
	result := ProofResult{
		Success:     false,
		BackendKind: "generic_netlink",
	}

	if runtime.GOOS != "linux" {
		result.Error = "unsupported_platform"
		_ = outputProof(result)
		return fmt.Errorf("proof requires Linux")
	}

	ifacePath := filepath.Join("/sys/class/net", ifaceName)
	if _, err := os.Stat(ifacePath); err != nil {
		result.Error = fmt.Sprintf("interface_missing: %s", ifaceName)
		_ = outputProof(result)
		return fmt.Errorf("interface %s not found", ifaceName)
	}

	// Get absolute path to proof binary
	proofBin, err := getProofBinaryPath()
	if err != nil {
		result.Error = fmt.Sprintf("proof_binary_path_error: %s", err)
		_ = outputProof(result)
		return fmt.Errorf("failed to resolve proof binary: %w", err)
	}

	fmt.Printf("Running wg_netlink_proof against %s...\n", ifaceName)
	fmt.Printf("Proof binary: %s\n", proofBin)

	// Check if binary exists
	if _, err := os.Stat(proofBin); err != nil {
		result.Error = "proof_binary_not_found"
		_ = outputProof(result)
		return fmt.Errorf("proof binary not found at %s", proofBin)
	}

	// Run proof binary with absolute path (no cmd.Dir to avoid path doubling)
	cmd := exec.Command(proofBin, "--interface", ifaceName)

	output, err := cmd.Output()
	if err != nil {
		// Hard blocker 2 fix: Non-zero exit means proof failed
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "Proof binary exited with code %d\n", exitErr.ExitCode())
			if len(exitErr.Stderr) > 0 {
				fmt.Fprintf(os.Stderr, "stderr: %s\n", string(exitErr.Stderr))
			}
			if len(output) > 0 {
				fmt.Printf("stdout: %s\n", string(output))
			}
		}
		result.Error = fmt.Sprintf("proof_binary_failed: %v", err)
		_ = outputProof(result)
		return fmt.Errorf("proof binary failed: %w", err)
	}

	// Parse JSON output from proof binary
	var proofOutput struct {
		Success         bool   `json:"success"`
		BackendKind     string `json:"backend_kind"`
		Interface       string `json:"interface"`
		PeerCount       uint32 `json:"peer_count"`
		ListenPort      *uint16 `json:"listen_port"`
		Error           string `json:"error"`
		NoSensitiveData bool   `json:"no_sensitive_data"`
	}

	if err := json.Unmarshal(output, &proofOutput); err != nil {
		result.Error = fmt.Sprintf("json_parse_error: %s", err)
		_ = outputProof(result)
		return fmt.Errorf("failed to parse proof output: %w", err)
	}

	// Map proof output to our result
	result.Success = proofOutput.Success
	result.Interface = proofOutput.Interface
	result.PeerCount = proofOutput.PeerCount
	result.ListenPort = proofOutput.ListenPort
	result.BackendKind = proofOutput.BackendKind
	result.Error = proofOutput.Error
	result.NoSensitiveData = proofOutput.NoSensitiveData

	// Hard blocker 2 fix: Fail-closed - return error if proof failed
	if !result.Success {
		_ = outputProof(result)
		return fmt.Errorf("proof failed: %s", result.Error)
	}

	// Also fail if sensitive data was leaked
	if !result.NoSensitiveData {
		_ = outputProof(result)
		return fmt.Errorf("proof failed: sensitive data detected in output")
	}

	// Hard blocker 3 fix: Assert required proof fields
	if result.BackendKind != "generic_netlink" {
		_ = outputProof(result)
		return fmt.Errorf("proof failed: expected backend_kind=generic_netlink, got %s", result.BackendKind)
	}

	if result.Interface != ifaceName {
		_ = outputProof(result)
		return fmt.Errorf("proof failed: expected interface=%s, got %s", ifaceName, result.Interface)
	}

	if result.PeerCount != 0 {
		_ = outputProof(result)
		return fmt.Errorf("proof failed: expected peer_count=0 for empty-interface lab, got %d", result.PeerCount)
	}

	return outputProof(result)
}

func outputProof(result ProofResult) error {
	_ = os.MkdirAll(artifactDir, 0755)

	statusPath := filepath.Join(artifactDir, "backend-status.json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal proof result: %w", err)
	}
	if err := os.WriteFile(statusPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backend-status.json: %w", err)
	}

	fmt.Printf("\n=== WireGuard Netlink Proof ===\n")
	fmt.Printf("Interface: %s\n", result.Interface)
	fmt.Printf("Backend: %s\n", result.BackendKind)
	fmt.Printf("Success: %v\n", result.Success)
	if result.Error != "" {
		fmt.Printf("Error: %s\n", result.Error)
	}
	fmt.Printf("No sensitive data: %v\n", result.NoSensitiveData)
	fmt.Printf("\nProof artifact: %s\n", statusPath)

	return nil
}
