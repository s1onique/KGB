// cmd/canary/control.go — Canary Control Client Subcommand
//
// Canonical control protocol using typed JSON envelopes.
// Each command emits exactly one JSON document to stdout.
//
// CORRECTION30: Presence-aware strict protocol with correct trailing rejection.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// DecodeError represents a decoding error with classification.
type DecodeError struct {
	ErrClass ErrorClass
	Message  string
}

func (e *DecodeError) Error() string {
	return e.Message
}

const (
	// SchemaVersion is the canonical protocol version.
	SchemaVersion = "canary-control/v1"

	// MaxResponseBody is the maximum response body size to prevent memory issues.
	MaxResponseBody = 64 * 1024 // 64KB

	// ControlDialTimeout is the timeout for establishing a connection.
	ControlDialTimeout = 5 * time.Second

	// ControlResponseHeaderTimeout is the timeout for reading response headers.
	ControlResponseHeaderTimeout = 5 * time.Second
)

// ErrorClass defines stable error classification.
type ErrorClass string

const (
	ErrInvalidArguments      ErrorClass = "invalid_arguments"
	ErrRequestCreateFailed   ErrorClass = "request_create_failed"
	ErrConnectionFailed      ErrorClass = "connection_failed"
	ErrRequestTimeout        ErrorClass = "request_timeout"
	ErrResponseTooLarge      ErrorClass = "response_too_large"
	ErrUnexpectedHTTPStatus  ErrorClass = "unexpected_http_status"
	ErrMalformedJSON         ErrorClass = "malformed_json"
	ErrUnknownJSONField      ErrorClass = "unknown_json_field"
	ErrMissingRequiredField  ErrorClass = "missing_required_field"
	ErrTrailingJSON          ErrorClass = "trailing_json"
	ErrHealthNotReady        ErrorClass = "health_not_ready"
	ErrStateInvalid          ErrorClass = "state_invalid"
	ErrWorkloadCountMismatch ErrorClass = "workload_count_mismatch"
)

// AllowedErrorClasses is the set of valid error classes.
var AllowedErrorClasses = map[ErrorClass]bool{
	ErrInvalidArguments:      true,
	ErrRequestCreateFailed:   true,
	ErrConnectionFailed:      true,
	ErrRequestTimeout:        true,
	ErrResponseTooLarge:      true,
	ErrUnexpectedHTTPStatus:  true,
	ErrMalformedJSON:         true,
	ErrUnknownJSONField:      true,
	ErrMissingRequiredField:  true,
	ErrTrailingJSON:          true,
	ErrHealthNotReady:        true,
	ErrStateInvalid:          true,
	ErrWorkloadCountMismatch: true,
}

// ControlEnvelope is the canonical protocol envelope.
type ControlEnvelope struct {
	SchemaVersion string           `json:"schema_version"`
	Operation     string           `json:"operation"`
	Success       bool             `json:"success"`
	HTTPStatus    int              `json:"http_status"`
	Health        *HealthPayload   `json:"health,omitempty"`
	State         *StatePayload    `json:"state,omitempty"`
	Workload      *WorkloadPayload `json:"workload,omitempty"`
	ErrorClass    ErrorClass       `json:"error_class,omitempty"`
}

// HealthPayload represents health check result.
type HealthPayload struct {
	Ready bool   `json:"ready"`
	Mode  string `json:"mode,omitempty"`
}

// StatePayload represents state result.
type StatePayload struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int64  `json:"operation_count"`
	FDCount        int    `json:"fd_count"`
	Ready          bool   `json:"ready"`
}

// WorkloadPayload represents operate result.
type WorkloadPayload struct {
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
		emitFailureEnvelope("health", ErrInvalidArguments, 0)
		return 1
	}

	if *timeout <= 0 || *port <= 0 || *port > 65535 {
		emitFailureEnvelope("health", ErrInvalidArguments, 0)
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/health", *port)
	result := doHealthCheck(context.Background(), url, *timeout)

	if result.ErrorClass != "" {
		emitFailureEnvelope("health", result.ErrorClass, result.HTTPStatus)
		return 1
	}

	emitSuccessEnvelope("health", 200, result.Health, nil, nil)
	return 0
}

func runState(args []string) int {
	fs := flag.NewFlagSet("canary control state", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		emitFailureEnvelope("state", ErrInvalidArguments, 0)
		return 1
	}

	if *timeout <= 0 || *port <= 0 || *port > 65535 {
		emitFailureEnvelope("state", ErrInvalidArguments, 0)
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/state", *port)
	result := doStateCheck(context.Background(), url, *timeout)

	if result.ErrorClass != "" {
		emitFailureEnvelope("state", result.ErrorClass, result.HTTPStatus)
		return 1
	}

	emitSuccessEnvelope("state", 200, nil, result.State, nil)
	return 0
}

func runOperate(args []string) int {
	fs := flag.NewFlagSet("canary control operate", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Canary HTTP port")
	count := fs.Int("count", 1, "Number of operations")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	if err := fs.Parse(args); err != nil {
		emitFailureEnvelope("operate", ErrInvalidArguments, 0)
		return 1
	}

	if *timeout <= 0 || *port <= 0 || *port > 65535 || *count <= 0 {
		emitFailureEnvelope("operate", ErrInvalidArguments, 0)
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/operate?count=%d", *port, *count)
	result := doOperateCheck(context.Background(), url, *count, *timeout)

	if result.ErrorClass != "" {
		emitFailureEnvelope("operate", result.ErrorClass, result.HTTPStatus)
		return 1
	}

	emitSuccessEnvelope("operate", 200, nil, nil, result.Workload)
	return 0
}

// emitSuccessEnvelope emits a successful control envelope.
func emitSuccessEnvelope(operation string, httpStatus int, health *HealthPayload, state *StatePayload, workload *WorkloadPayload) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     operation,
		Success:       true,
		HTTPStatus:    httpStatus,
		Health:        health,
		State:         state,
		Workload:      workload,
	}
	emitEnvelope(env)
}

// emitFailureEnvelope emits a failed control envelope.
func emitFailureEnvelope(operation string, errClass ErrorClass, httpStatus int) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     operation,
		Success:       false,
		HTTPStatus:    httpStatus,
		ErrorClass:    errClass,
	}
	emitEnvelope(env)
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
	Health     *HealthPayload
}

// doHealthCheck performs a health check using Go's HTTP client.
func doHealthCheck(ctx context.Context, url string, timeout time.Duration) HealthCheckResult {
	result := HealthCheckResult{ExitCode: 1, HTTPStatus: 0}

	// Use bounded context for timeout classification
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ControlDialTimeout,
			}).DialContext,
			ResponseHeaderTimeout: ControlResponseHeaderTimeout,
		},
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(opCtx, "GET", url, nil)
	if err != nil {
		result.ErrorClass = ErrRequestCreateFailed
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if opCtx.Err() == context.DeadlineExceeded {
			result.ErrorClass = ErrRequestTimeout
		} else if errors.Is(err, context.DeadlineExceeded) {
			result.ErrorClass = ErrRequestTimeout
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody+1))
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	// Strict decoding: decode one value, require EOF
	health, err := strictDecodeHealth(body)
	if err != nil {
		if decodeErr, ok := err.(*DecodeError); ok {
			result.ErrorClass = decodeErr.ErrClass
		} else {
			result.ErrorClass = ErrMalformedJSON
		}
		return result
	}

	// Validate required fields
	if !health.Ready {
		result.ErrorClass = ErrHealthNotReady
		return result
	}

	result.Health = health
	result.ExitCode = 0
	return result
}

// strictDecodeHealth strictly decodes a health payload.
func strictDecodeHealth(data []byte) (*HealthPayload, error) {
	// Check for empty input
	if len(data) == 0 {
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: "empty body"}
	}

	// First pass: check required fields are present
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: "invalid JSON"}
	}

	// Verify required fields exist and are not null
	if v, ok := raw["ready"]; !ok || v == nil || string(v) == "null" {
		return nil, &DecodeError{ErrClass: ErrMissingRequiredField, Message: "missing ready"}
	}
	if v, ok := raw["mode"]; !ok || v == nil || string(v) == "null" {
		return nil, &DecodeError{ErrClass: ErrMissingRequiredField, Message: "missing mode"}
	}

	// Second pass: strict decode with DisallowUnknownFields
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var health HealthPayload
	if err := dec.Decode(&health); err != nil {
		// Check for unknown field error by checking error message
		if strings.Contains(err.Error(), "unknown field") {
			return nil, &DecodeError{ErrClass: ErrUnknownJSONField, Message: err.Error()}
		}
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: err.Error()}
	}

	// Third pass: require exactly one value (EOF on next decode)
	var extra json.RawMessage
	decodeErr := dec.Decode(&extra)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		return nil, &DecodeError{ErrClass: ErrTrailingJSON, Message: "trailing data"}
	}
	if decodeErr == nil && len(extra) > 0 {
		return nil, &DecodeError{ErrClass: ErrTrailingJSON, Message: "second JSON value"}
	}

	return &health, nil
}

// StateCheckResult represents the result of a state check.
type StateCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	State      *StatePayload
}

// doStateCheck performs a state check using Go's HTTP client.
func doStateCheck(ctx context.Context, url string, timeout time.Duration) StateCheckResult {
	result := StateCheckResult{ExitCode: 1, HTTPStatus: 0}

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ControlDialTimeout,
			}).DialContext,
			ResponseHeaderTimeout: ControlResponseHeaderTimeout,
		},
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(opCtx, "GET", url, nil)
	if err != nil {
		result.ErrorClass = ErrRequestCreateFailed
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if opCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
			result.ErrorClass = ErrRequestTimeout
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody+1))
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	state, err := strictDecodeState(body)
	if err != nil {
		if decodeErr, ok := err.(*DecodeError); ok {
			result.ErrorClass = decodeErr.ErrClass
		} else {
			result.ErrorClass = ErrMalformedJSON
		}
		return result
	}

	// Validate required fields
	if state.Mode == "" {
		result.ErrorClass = ErrMissingRequiredField
		return result
	}

	result.State = state
	result.ExitCode = 0
	return result
}

// strictDecodeState strictly decodes a state payload.
func strictDecodeState(data []byte) (*StatePayload, error) {
	if len(data) == 0 {
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: "empty body"}
	}

	// Check required fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: "invalid JSON"}
	}

	requiredFields := []string{"mode", "retained_blocks", "retained_bytes", "operation_count", "fd_count", "ready"}
	for _, field := range requiredFields {
		if v, ok := raw[field]; !ok || v == nil || string(v) == "null" {
			return nil, &DecodeError{ErrClass: ErrMissingRequiredField, Message: "missing " + field}
		}
	}

	// Strict decode
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var state StatePayload
	if err := dec.Decode(&state); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return nil, &DecodeError{ErrClass: ErrUnknownJSONField, Message: err.Error()}
		}
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: err.Error()}
	}

	// Require exactly one value
	var extra json.RawMessage
	decodeErr := dec.Decode(&extra)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		return nil, &DecodeError{ErrClass: ErrTrailingJSON, Message: "trailing data"}
	}
	if decodeErr == nil && len(extra) > 0 {
		return nil, &DecodeError{ErrClass: ErrTrailingJSON, Message: "second JSON value"}
	}

	return &state, nil
}

// OperateCheckResult represents the result of an operate request.
type OperateCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	Workload   *WorkloadPayload
}

// doOperateCheck performs an operate request using Go's HTTP client.
func doOperateCheck(ctx context.Context, url string, requested int, timeout time.Duration) OperateCheckResult {
	result := OperateCheckResult{ExitCode: 1, HTTPStatus: 0}

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ControlDialTimeout,
			}).DialContext,
			ResponseHeaderTimeout: ControlResponseHeaderTimeout,
		},
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(opCtx, "POST", url, nil)
	if err != nil {
		result.ErrorClass = ErrRequestCreateFailed
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if opCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
			result.ErrorClass = ErrRequestTimeout
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody+1))
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	workload, err := strictDecodeWorkload(body)
	if err != nil {
		if decodeErr, ok := err.(*DecodeError); ok {
			result.ErrorClass = decodeErr.ErrClass
		} else {
			result.ErrorClass = ErrMalformedJSON
		}
		return result
	}

	// Validate workload counts - CORRECTION30 P0-3
	if workload.Requested != requested || workload.Attempted != requested || workload.Completed != workload.Attempted {
		result.ErrorClass = ErrWorkloadCountMismatch
		return result
	}

	result.Workload = workload
	result.ExitCode = 0
	return result
}

// strictDecodeWorkload strictly decodes a workload payload.
func strictDecodeWorkload(data []byte) (*WorkloadPayload, error) {
	if len(data) == 0 {
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: "empty body"}
	}

	// Check required fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: "invalid JSON"}
	}

	requiredFields := []string{"requested", "attempted", "completed"}
	for _, field := range requiredFields {
		if v, ok := raw[field]; !ok || v == nil || string(v) == "null" {
			return nil, &DecodeError{ErrClass: ErrMissingRequiredField, Message: "missing " + field}
		}
	}

	// Strict decode
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var workload WorkloadPayload
	if err := dec.Decode(&workload); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return nil, &DecodeError{ErrClass: ErrUnknownJSONField, Message: err.Error()}
		}
		return nil, &DecodeError{ErrClass: ErrMalformedJSON, Message: err.Error()}
	}

	// Require exactly one value
	var extra json.RawMessage
	decodeErr := dec.Decode(&extra)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		return nil, &DecodeError{ErrClass: ErrTrailingJSON, Message: "trailing data"}
	}
	if decodeErr == nil && len(extra) > 0 {
		return nil, &DecodeError{ErrClass: ErrTrailingJSON, Message: "second JSON value"}
	}

	return &workload, nil
}
