// cmd/canary/control.go — Canary Control Client Subcommand
//
// Canonical control protocol using typed JSON envelopes.
// Each command emits exactly one JSON document to stdout.
//
// CORRECTION29: Defines one canonical machine-readable protocol.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	// SchemaVersion is the canonical protocol version.
	SchemaVersion = "canary-control/v1"

	// MaxResponseBody is the maximum response body size to prevent memory issues.
	MaxResponseBody = 64 * 1024 // 64KB

	// ControlDialTimeout is the timeout for establishing a connection.
	ControlDialTimeout = 5 * time.Second

	// ControlResponseHeaderTimeout is the timeout for reading response headers.
	ControlResponseHeaderTimeout = 5 * time.Second

	// ControlReadTimeout is the total timeout for reading the response body.
	ControlReadTimeout = 10 * time.Second
)

// ErrorClass defines stable error classification.
type ErrorClass string

const (
	ErrInvalidArguments       ErrorClass = "invalid_arguments"
	ErrRequestCreateFailed    ErrorClass = "request_create_failed"
	ErrConnectionFailed       ErrorClass = "connection_failed"
	ErrRequestTimeout         ErrorClass = "request_timeout"
	ErrResponseTooLarge       ErrorClass = "response_too_large"
	ErrUnexpectedHTTPStatus   ErrorClass = "unexpected_http_status"
	ErrMalformedJSON          ErrorClass = "malformed_json"
	ErrUnknownJSONField       ErrorClass = "unknown_json_field"
	ErrMissingRequiredField   ErrorClass = "missing_required_field"
	ErrTrailingJSON          ErrorClass = "trailing_json"
	ErrHealthNotReady        ErrorClass = "health_not_ready"
	ErrStateInvalid          ErrorClass = "state_invalid"
	ErrWorkloadCountMismatch ErrorClass = "workload_count_mismatch"
)

// ControlEnvelope is the canonical protocol envelope.
// Exactly one envelope is emitted per command invocation.
type ControlEnvelope struct {
	SchemaVersion string           `json:"schema_version"`
	Operation     string           `json:"operation"`
	Success       bool             `json:"success"`
	HTTPStatus   int              `json:"http_status"`
	Health       *HealthResult   `json:"health,omitempty"`
	State        *StateResult    `json:"state,omitempty"`
	Workload     *WorkloadResult `json:"workload,omitempty"`
	ErrorClass   ErrorClass      `json:"error_class,omitempty"`
}

// HealthResult represents the health check payload.
type HealthResult struct {
	Ready bool   `json:"ready"`
	Mode  string `json:"mode,omitempty"`
}

// StateResult represents the state payload.
type StateResult struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int64  `json:"operation_count"`
	FDCount        int    `json:"fd_count"`
	Ready          bool   `json:"ready"`
}

// WorkloadResult represents the operate payload.
type WorkloadResult struct {
	Requested int `json:"requested"`
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
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "health",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	if *timeout <= 0 {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "health",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/health", *port)
	result := doHealthCheck(context.Background(), url, *timeout)

	if result.ErrorClass != "" {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "health",
			Success:       false,
			HTTPStatus:    result.HTTPStatus,
			ErrorClass:    result.ErrorClass,
		})
	} else {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "health",
			Success:       true,
			HTTPStatus:    200,
			Health:        result.Health,
		})
	}
	return result.ExitCode
}

func runState(args []string) int {
	fs := flag.NewFlagSet("canary control state", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "state",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	if *timeout <= 0 {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "state",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/state", *port)
	result := doStateCheck(context.Background(), url, *timeout)

	if result.ErrorClass != "" {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "state",
			Success:       false,
			HTTPStatus:    result.HTTPStatus,
			ErrorClass:    result.ErrorClass,
		})
	} else {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "state",
			Success:       true,
			HTTPStatus:    200,
			State:         result.State,
		})
	}
	return result.ExitCode
}

func runOperate(args []string) int {
	fs := flag.NewFlagSet("canary control operate", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	count := fs.Int("count", 1, "Number of operations")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "operate",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	if *count <= 0 {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "operate",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	if *timeout <= 0 {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "operate",
			Success:       false,
			HTTPStatus:    0,
			ErrorClass:    ErrInvalidArguments,
		})
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/operate?count=%d", *port, *count)
	result := doOperateCheck(context.Background(), url, *count, *timeout)

	if result.ErrorClass != "" {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "operate",
			Success:       false,
			HTTPStatus:    result.HTTPStatus,
			ErrorClass:    result.ErrorClass,
		})
	} else {
		emitEnvelope(ControlEnvelope{
			SchemaVersion: SchemaVersion,
			Operation:     "operate",
			Success:       true,
			HTTPStatus:    200,
			Workload:      result.Workload,
		})
	}
	return result.ExitCode
}

// emitEnvelope emits exactly one JSON envelope to stdout.
func emitEnvelope(env ControlEnvelope) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.Encode(env)
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	Health     *HealthResult
}

// doHealthCheck performs a health check using Go's HTTP client.
func doHealthCheck(ctx context.Context, url string, timeout time.Duration) HealthCheckResult {
	result := HealthCheckResult{ExitCode: 1, HTTPStatus: 0}

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
		result.ErrorClass = ErrRequestCreateFailed
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			result.ErrorClass = ErrRequestTimeout
		} else {
			result.ErrorClass = ErrConnectionFailed
		}
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	if resp.StatusCode != 200 {
		result.ErrorClass = ErrUnexpectedHTTPStatus
		return result
	}

	// Read bounded response body
	limited := io.LimitReader(resp.Body, MaxResponseBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	// Strict JSON decoding
	var health HealthResult
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&health); err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	// Check for trailing data
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err == nil && len(trailing) > 0 {
		result.ErrorClass = ErrTrailingJSON
		return result
	}

	// Validate required fields
	if !health.Ready {
		result.ErrorClass = ErrHealthNotReady
		return result
	}

	result.Health = &health
	result.ExitCode = 0
	return result
}

// StateCheckResult represents the result of a state check.
type StateCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	State      *StateResult
}

// doStateCheck performs a state check using Go's HTTP client.
func doStateCheck(ctx context.Context, url string, timeout time.Duration) StateCheckResult {
	result := StateCheckResult{ExitCode: 1, HTTPStatus: 0}

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
		result.ErrorClass = ErrRequestCreateFailed
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			result.ErrorClass = ErrRequestTimeout
		} else {
			result.ErrorClass = ErrConnectionFailed
		}
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	if resp.StatusCode != 200 {
		result.ErrorClass = ErrUnexpectedHTTPStatus
		return result
	}

	// Read bounded response body
	limited := io.LimitReader(resp.Body, MaxResponseBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	// Strict JSON decoding
	var state StateResult
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	// Check for trailing data
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err == nil && len(trailing) > 0 {
		result.ErrorClass = ErrTrailingJSON
		return result
	}

	// Validate required fields
	if state.Mode == "" {
		result.ErrorClass = ErrMissingRequiredField
		return result
	}

	result.State = &state
	result.ExitCode = 0
	return result
}

// OperateCheckResult represents the result of an operate request.
type OperateCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	Workload   *WorkloadResult
}

// doOperateCheck performs an operate request using Go's HTTP client.
func doOperateCheck(ctx context.Context, url string, requested int, timeout time.Duration) OperateCheckResult {
	result := OperateCheckResult{ExitCode: 1, HTTPStatus: 0}

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
		result.ErrorClass = ErrRequestCreateFailed
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			result.ErrorClass = ErrRequestTimeout
		} else {
			result.ErrorClass = ErrConnectionFailed
		}
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	if resp.StatusCode != 200 {
		result.ErrorClass = ErrUnexpectedHTTPStatus
		return result
	}

	// Read bounded response body
	limited := io.LimitReader(resp.Body, MaxResponseBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	// Strict JSON decoding
	var workload WorkloadResult
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&workload); err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	// Check for trailing data
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err == nil && len(trailing) > 0 {
		result.ErrorClass = ErrTrailingJSON
		return result
	}

	// Validate required fields
	if workload.Attempted != requested || workload.Completed != workload.Attempted {
		result.ErrorClass = ErrWorkloadCountMismatch
		return result
	}

	result.Workload = &workload
	result.ExitCode = 0
	return result
}
