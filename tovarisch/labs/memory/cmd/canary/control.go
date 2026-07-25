// cmd/canary/control.go — Canary Control Client Subcommand
//
// Canonical control protocol using typed JSON envelopes.
// Each command emits exactly one JSON document to stdout.
//
// CORRECTION34: This file now wraps the shared protocol authority in
// internal/canarycontrol. All vocabulary (error classes, envelope schema,
// retry policy, typed operations, validators) is owned by that package.
// This file only owns: process boundary, HTTP client transport, envelope
// emission, and the CLI subcommand dispatch.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// Re-export commonly-used names from the shared package for the existing
// in-package tests. This avoids churn in the test file while making the
// shared package the single source of truth.
type (
	DecodeError      = canarycontrol.ProtocolError
	HealthPayload    = canarycontrol.HealthPayload
	StatePayload     = canarycontrol.StatePayload
	WorkloadPayload  = canarycontrol.WorkloadPayload
	ControlEnvelope  = canarycontrol.ControlEnvelope
	ErrorClass       = canarycontrol.ErrorClass
	ControlOperation = canarycontrol.ControlOperation
)

// Operation-name aliases used by runControl dispatch.
const (
	ControlHealth  = canarycontrol.OpHealth
	ControlState   = canarycontrol.OpState
	ControlOperate = canarycontrol.OpOperate
)

// ControlOperationKind is the typed control operation name (re-exported
// for tests that distinguish Operation values).
type ControlOperationKind = canarycontrol.Operation

// Constants re-exported for backward compatibility with existing tests.
const (
	SchemaVersion                = canarycontrol.SchemaVersion
	MaxResponseBody              = canarycontrol.MaxResponseBody
	ControlDialTimeout           = canarycontrol.ControlDialTimeout
	ControlResponseHeaderTimeout = canarycontrol.ControlResponseHeaderTimeout
)

// Error-class aliases re-exported for backward compatibility with existing tests.
var (
	ErrInvalidArguments      = canarycontrol.ErrInvalidArguments
	ErrRequestCreateFailed   = canarycontrol.ErrRequestCreateFailed
	ErrConnectionFailed      = canarycontrol.ErrConnectionFailed
	ErrRequestTimeout        = canarycontrol.ErrRequestTimeout
	ErrResponseTooLarge      = canarycontrol.ErrResponseTooLarge
	ErrUnexpectedHTTPStatus  = canarycontrol.ErrUnexpectedHTTPStatus
	ErrMalformedJSON         = canarycontrol.ErrMalformedJSON
	ErrUnknownJSONField      = canarycontrol.ErrUnknownJSONField
	ErrMissingRequiredField  = canarycontrol.ErrMissingRequiredField
	ErrTrailingJSON          = canarycontrol.ErrTrailingJSON
	ErrHealthNotReady        = canarycontrol.ErrHealthNotReady
	ErrStateInvalid          = canarycontrol.ErrStateInvalid
	ErrWorkloadCountMismatch = canarycontrol.ErrWorkloadCountMismatch
	ErrSchemaVersionMismatch = canarycontrol.ErrSchemaVersionMismatch
	ErrInvalidOperation      = canarycontrol.ErrInvalidOperation
)

// AllowedErrorClasses is a defensive-copy helper that exposes the closed
// error-class vocabulary via the shared package. Callers SHOULD use
// canarycontrol.IsAllowedErrorClass for membership checks.
var AllowedErrorClasses = func() map[canarycontrol.ErrorClass]bool {
	out := make(map[canarycontrol.ErrorClass]bool)
	for _, ec := range canarycontrol.AllErrorClasses() {
		out[ec] = true
	}
	return out
}()

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
	emitEnvelope(&env)
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
	emitEnvelope(&env)
}

// emitEnvelope emits exactly one JSON envelope. When the
// optional writer is nil, the envelope is written to stdout
// (the control subcommand path); otherwise it is written to
// the supplied writer (the HTTP handler path).
func emitEnvelope(env *ControlEnvelope) {
	emitEnvelopeTo(nil, env)
}

// emitEnvelopeTo is the explicit form. When w is nil, the
// envelope is written to stdout.
func emitEnvelopeTo(w io.Writer, env *ControlEnvelope) {
	if w == nil {
		w = os.Stdout
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(env)
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	Health     *HealthPayload
}

// doHealthCheck performs a health check using Go's HTTP client and the
// shared canarycontrol.DecodeHealth authority.
func doHealthCheck(ctx context.Context, url string, timeout time.Duration) HealthCheckResult {
	result := HealthCheckResult{ExitCode: 1, HTTPStatus: 0}

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
		result.ErrorClass = classifyTransportErr(opCtx, err)
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	if resp.StatusCode != 200 {
		result.ErrorClass = ErrUnexpectedHTTPStatus
		return result
	}

	// Read bounded response body
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBody+1)))
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	h, err := canarycontrol.DecodeHealth(body)
	if err != nil {
		if pe, ok := canarycontrol.AsProtocolError(err); ok {
			result.ErrorClass = pe.ErrClass
		} else {
			result.ErrorClass = ErrMalformedJSON
		}
		return result
	}

	result.Health = h
	result.ExitCode = 0
	return result
}

// StateCheckResult represents the result of a state check.
type StateCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	State      *StatePayload
}

// doStateCheck performs a state check using Go's HTTP client and the
// shared canarycontrol.DecodeState authority.
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
		result.ErrorClass = classifyTransportErr(opCtx, err)
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	if resp.StatusCode != 200 {
		result.ErrorClass = ErrUnexpectedHTTPStatus
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBody+1)))
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	s, err := canarycontrol.DecodeState(body)
	if err != nil {
		if pe, ok := canarycontrol.AsProtocolError(err); ok {
			result.ErrorClass = pe.ErrClass
		} else {
			result.ErrorClass = ErrMalformedJSON
		}
		return result
	}

	result.State = s
	result.ExitCode = 0
	return result
}

// OperateCheckResult represents the result of an operate request.
type OperateCheckResult struct {
	ExitCode   int
	HTTPStatus int
	ErrorClass ErrorClass
	Workload   *WorkloadPayload
}

// doOperateCheck performs an operate request using Go's HTTP client and the
// shared canarycontrol.DecodeWorkload authority.
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
		result.ErrorClass = classifyTransportErr(opCtx, err)
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	if resp.StatusCode != 200 {
		result.ErrorClass = ErrUnexpectedHTTPStatus
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBody+1)))
	if err != nil {
		result.ErrorClass = ErrMalformedJSON
		return result
	}

	if len(body) > MaxResponseBody {
		result.ErrorClass = ErrResponseTooLarge
		return result
	}

	w, err := canarycontrol.DecodeWorkload(body, requested)
	if err != nil {
		if pe, ok := canarycontrol.AsProtocolError(err); ok {
			result.ErrorClass = pe.ErrClass
		} else {
			result.ErrorClass = ErrMalformedJSON
		}
		return result
	}

	result.Workload = w
	result.ExitCode = 0
	return result
}

// classifyTransportErr converts a transport error into a typed ErrorClass.
// Timeout errors map to ErrRequestTimeout; everything else maps to ErrConnectionFailed.
func classifyTransportErr(opCtx context.Context, err error) ErrorClass {
	if opCtx.Err() == context.DeadlineExceeded {
		return ErrRequestTimeout
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrRequestTimeout
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return ErrRequestTimeout
	}
	return ErrConnectionFailed
}

// IsProtocolRetryable reports whether err represents a transient failure
// that should be retried. Delegates to the shared authority.
func IsProtocolRetryable(err error) bool {
	return canarycontrol.IsRetryable(err)
}

// ValidateControlEnvelope validates a control envelope. Delegates to the
// shared authority.
func ValidateControlEnvelope(env *ControlEnvelope) error {
	return canarycontrol.ValidateControlEnvelope(env)
}

// buildArgv exposes the shared typed-operation argv authority for the
// in-package tests. BuildArgv fails closed (returns an error if the
// operation is not validated).
func buildArgv(op ControlOperation) ([]string, error) {
	return op.BuildArgv()
}
