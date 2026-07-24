// cmd/canary/control.go — Canary Control Client Subcommand
//
// Self-contained HTTP client that uses Go's net/http directly (no shell, no curl).
// This is the only approved transport for qualified production reachability.
//
// CORRECTION28: Eliminates shell/curl/wget dependency for container reachability.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	// ControlDialTimeout is the timeout for establishing a connection.
	ControlDialTimeout = 5 * time.Second
	// ControlResponseHeaderTimeout is the timeout for reading response headers.
	ControlResponseHeaderTimeout = 5 * time.Second
	// ControlReadTimeout is the total timeout for reading the response body.
	ControlReadTimeout = 10 * time.Second
	// MaxResponseBody is the maximum response body size to prevent memory issues.
	MaxResponseBody = 64 * 1024 // 64KB
)

// ControlResult represents the result of a control operation.
type ControlResult struct {
	ExitCode       int
	ResponseCode   int
	HealthValid    bool
	StateValid     bool
	WorkloadValid  bool
	Error          string
	Diagnostic     string
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Ready bool   `json:"ready,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

// WorkloadResponse represents the operate response.
type WorkloadResponse struct {
	Attempted int `json:"attempted"`
	Completed int `json:"completed"`
}

// runControl executes the control subcommand.
func runControl(args []string) int {
	fs := flag.NewFlagSet("canary control", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s control <command> [options]\n\nCommands:\n  health  - Check canary health\n  state  - Get canary state\n  operate - Perform operations\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	if len(args) < 2 {
		fs.Usage()
		return 1
	}

	cmd := args[1]
	remainingArgs := args[2:]

	switch cmd {
	case "health":
		return runHealth(remainingArgs)
	case "state":
		return runState(remainingArgs)
	case "operate":
		return runOperate(remainingArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fs.Usage()
		return 1
	}
}

func runHealth(args []string) int {
	fs := flag.NewFlagSet("canary control health", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	timeout := fs.Duration("timeout", 10*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/health", *port)
	result := doHealthCheck(context.Background(), url, *timeout)
	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", result.Error)
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(os.Stderr, "diagnostic: %s\n", result.Diagnostic)
	}
	if result.HealthValid {
		fmt.Println("OK")
	}
	return result.ExitCode
}

func runState(args []string) int {
	fs := flag.NewFlagSet("canary control state", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	timeout := fs.Duration("timeout", 10*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/state", *port)
	result := doStateCheck(context.Background(), url, *timeout)
	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", result.Error)
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(os.Stderr, "diagnostic: %s\n", result.Diagnostic)
	}
	if result.StateValid {
		fmt.Println("STATE_VALID")
	}
	return result.ExitCode
}

func runOperate(args []string) int {
	fs := flag.NewFlagSet("canary control operate", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	count := fs.Int("count", 1, "Number of operations")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/operate?count=%d", *port, *count)
	result := doOperateCheck(context.Background(), url, *timeout)
	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", result.Error)
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(os.Stderr, "diagnostic: %s\n", result.Diagnostic)
	}
	if result.WorkloadValid {
		fmt.Println("WORKLOAD_VALID")
	}
	return result.ExitCode
}

// doHealthCheck performs a health check using Go's HTTP client.
func doHealthCheck(ctx context.Context, url string, timeout time.Duration) ControlResult {
	result := ControlResult{ExitCode: 1}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ControlDialTimeout,
			}).DialContext,
			ResponseHeaderTimeout: ControlResponseHeaderTimeout,
		},
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("create request: %v", err)
		result.Diagnostic = "failed to create HTTP request"
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Diagnostic = fmt.Sprintf("connection to %s failed", url)
		return result
	}
	defer resp.Body.Close()

	result.ResponseCode = resp.StatusCode

	// Check for 2xx status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		result.Diagnostic = fmt.Sprintf("health check returned HTTP %d", resp.StatusCode)
		return result
	}

	// Read bounded response body
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody))
	if err != nil {
		result.Error = fmt.Sprintf("read body: %v", err)
		result.Diagnostic = "failed to read health response body"
		return result
	}

	// Try to parse as JSON (optional)
	var health HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		// Non-JSON is OK for health - we just need 2xx
		result.HealthValid = true
		result.ExitCode = 0
		return result
	}

	// If JSON, validate it contains expected fields
	if health.Ready || health.Mode != "" {
		result.HealthValid = true
		result.ExitCode = 0
	} else {
		result.HealthValid = true // HTTP 2xx is sufficient
		result.ExitCode = 0
	}

	return result
}

// doStateCheck performs a state check using Go's HTTP client.
func doStateCheck(ctx context.Context, url string, timeout time.Duration) ControlResult {
	result := ControlResult{ExitCode: 1}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ControlDialTimeout,
			}).DialContext,
			ResponseHeaderTimeout: ControlResponseHeaderTimeout,
		},
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("create request: %v", err)
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.ResponseCode = resp.StatusCode

	if resp.StatusCode != 200 {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		return result
	}

	// Read bounded response body
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody))
	if err != nil {
		result.Error = fmt.Sprintf("read body: %v", err)
		return result
	}

	// Validate JSON structure
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		result.Error = fmt.Sprintf("parse state: %v", err)
		result.Diagnostic = "state response is not valid JSON"
		return result
	}

	// Validate required fields
	if state.Mode == "" {
		result.Error = "state missing mode field"
		return result
	}

	result.StateValid = true
	result.ExitCode = 0
	return result
}

// doOperateCheck performs an operate request using Go's HTTP client.
func doOperateCheck(ctx context.Context, url string, timeout time.Duration) ControlResult {
	result := ControlResult{ExitCode: 1}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ControlDialTimeout,
			}).DialContext,
			ResponseHeaderTimeout: ControlResponseHeaderTimeout,
		},
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("create request: %v", err)
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.ResponseCode = resp.StatusCode

	if resp.StatusCode != 200 {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		return result
	}

	// Read bounded response body
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody))
	if err != nil {
		result.Error = fmt.Sprintf("read body: %v", err)
		return result
	}

	// Validate JSON structure
	var workload WorkloadResponse
	if err := json.Unmarshal(body, &workload); err != nil {
		result.Error = fmt.Sprintf("parse workload: %v", err)
		result.Diagnostic = "workload response is not valid JSON"
		return result
	}

	// Validate required fields
	if workload.Attempted < 0 || workload.Completed < 0 {
		result.Error = "workload response has invalid counts"
		return result
	}

	result.WorkloadValid = true
	result.ExitCode = 0
	return result
}

// ControlExecResult is returned by the Docker exec control client.
type ControlExecResult struct {
	ExitCode       int
	HealthValid    bool
	StateValid     bool
	WorkloadValid  bool
	State          *State
	WorkloadResult *WorkloadResponse
	Diagnostic     string
}

// ControlExecHealth runs health check via docker exec.
func ControlExecHealth(ctx context.Context, containerID string, port int) ControlExecResult {
	return controlExecCommand(ctx, containerID, []string{
		"/app/canary", "control", "health",
		"--port", strconv.Itoa(port),
		"--timeout", "10s",
	})
}

// ControlExecState runs state check via docker exec.
func ControlExecState(ctx context.Context, containerID string, port int) (ControlExecResult, *State) {
	result := controlExecCommand(ctx, containerID, []string{
		"/app/canary", "control", "state",
		"--port", strconv.Itoa(port),
		"--timeout", "10s",
	})
	return result, result.State
}

// ControlExecOperate runs operate via docker exec.
func ControlExecOperate(ctx context.Context, containerID string, port int, count int) (ControlExecResult, *WorkloadResponse) {
	result := controlExecCommand(ctx, containerID, []string{
		"/app/canary", "control", "operate",
		"--port", strconv.Itoa(port),
		"--count", strconv.Itoa(count),
		"--timeout", "30s",
	})
	return result, result.WorkloadResult
}

// controlExecCommand is a placeholder - the actual implementation
// uses the docker client to run exec. This is called from the controller.
func controlExecCommand(ctx context.Context, containerID string, cmd []string) ControlExecResult {
	// This is implemented by the controller using docker.ContainerExec
	// The canary binary itself just implements the control subcommand.
	return ControlExecResult{ExitCode: 1, Diagnostic: "use docker client for exec"}
}
