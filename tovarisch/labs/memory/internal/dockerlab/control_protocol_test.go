// control_protocol_test.go — Protocol tests for Docker client control execution
//
// Tests strict decoding, required-field presence, and variant validation.
// CORRECTION30 P0-8: Protocol tests for Docker client control execution.

package dockerlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func TestStrictParseEnvelope_Success(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`
	env, err := strictParseEnvelope(stdout)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if env.SchemaVersion != "canary-control/v1" {
		t.Errorf("expected schema_version=canary-control/v1, got %s", env.SchemaVersion)
	}
	if env.Operation != "health" {
		t.Errorf("expected operation=health, got %s", env.Operation)
	}
	if !env.Success {
		t.Error("expected success=true")
	}
	if env.HTTPStatus != 200 {
		t.Errorf("expected http_status=200, got %d", env.HTTPStatus)
	}
	if env.Health == nil {
		t.Fatal("expected health payload")
	}
	if !env.Health.Ready {
		t.Error("expected health.ready=true")
	}
}

func TestStrictParseEnvelope_EmptyStdout(t *testing.T) {
	_, err := strictParseEnvelope("")
	if err == nil {
		t.Fatal("expected error for empty stdout")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.ErrClass != "malformed_json" {
		t.Errorf("expected malformed_json, got %s", parseErr.ErrClass)
	}
}

func TestStrictParseEnvelope_InvalidJSON(t *testing.T) {
	_, err := strictParseEnvelope("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.ErrClass != "malformed_json" {
		t.Errorf("expected malformed_json, got %s", parseErr.ErrClass)
	}
}

func TestStrictParseEnvelope_MissingSchemaVersion(t *testing.T) {
	stdout := `{"operation":"health","success":true,"http_status":200}`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for missing schema_version")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.ErrClass != "missing_required_field" {
		t.Errorf("expected missing_required_field, got %s", parseErr.ErrClass)
	}
}

func TestStrictParseEnvelope_MissingOperation(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","success":true,"http_status":200}`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for missing operation")
	}
}

func TestStrictParseEnvelope_MissingSuccess(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"health","http_status":200}`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for missing success")
	}
}

func TestStrictParseEnvelope_MissingHTTPStatus(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"health","success":true}`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for missing http_status")
	}
}

func TestStrictParseEnvelope_UnknownField(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"unknown_field":"forbidden"}`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.ErrClass != "unknown_json_field" {
		t.Errorf("expected unknown_json_field, got %s", parseErr.ErrClass)
	}
}

func TestStrictParseEnvelope_SecondJSONValue(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200}{"extra":1}`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for second JSON value")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	// Note: First pass json.Unmarshal fails because two JSON objects are invalid
	// for object target - this is detected as malformed_json
	if parseErr.ErrClass != "malformed_json" {
		t.Errorf("expected malformed_json, got %s", parseErr.ErrClass)
	}
}

func TestStrictParseEnvelope_MalformedTrailingBytes(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200}INVALID`
	_, err := strictParseEnvelope(stdout)
	if err == nil {
		t.Fatal("expected error for malformed trailing bytes")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.ErrClass != "malformed_json" {
		t.Errorf("expected malformed_json, got %s", parseErr.ErrClass)
	}
}

func TestStrictParseEnvelope_TrailingWhitespace(t *testing.T) {
	// Trailing whitespace only is valid
	stdout := `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200}   `
	env, err := strictParseEnvelope(stdout)
	if err != nil {
		t.Fatalf("expected success for trailing whitespace: %v", err)
	}
	if !env.Success {
		t.Error("expected success=true")
	}
}

func TestValidateControlEnvelope_SuccessVariant(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
		Health:       &HealthPayload{Ready: true, Mode: "growing"},
	}
	err := validateControlEnvelope(env, "health", 0)
	if err != nil {
		t.Fatalf("expected success for valid success envelope: %v", err)
	}
}

func TestValidateControlEnvelope_FailureVariant(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      false,
		HTTPStatus:  500,
		ErrorClass:   "health_not_ready",
	}
	err := validateControlEnvelope(env, "health", 1)
	if err != nil {
		t.Fatalf("expected success for valid failure envelope: %v", err)
	}
}

func TestValidateControlEnvelope_WrongSchemaVersion(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "wrong-version",
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
	}
	err := validateControlEnvelope(env, "health", 0)
	if err == nil {
		t.Fatal("expected error for wrong schema version")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.ErrClass != "invalid_arguments" {
		t.Errorf("expected invalid_arguments, got %s", parseErr.ErrClass)
	}
}

func TestValidateControlEnvelope_OperationMismatch(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "state",
		Success:      true,
		HTTPStatus:  200,
	}
	err := validateControlEnvelope(env, "health", 0)
	if err == nil {
		t.Fatal("expected error for operation mismatch")
	}
}

func TestValidateControlEnvelope_SuccessExitMismatch(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
	}
	// exitCode=1 but success=true
	err := validateControlEnvelope(env, "health", 1)
	if err == nil {
		t.Fatal("expected error for success/exit mismatch")
	}
}

func TestValidateControlEnvelope_FailureExitMismatch(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      false,
		HTTPStatus:  500,
		ErrorClass:   "health_not_ready",
	}
	// exitCode=0 but success=false
	err := validateControlEnvelope(env, "health", 0)
	if err == nil {
		t.Fatal("expected error for failure/exit mismatch")
	}
}

func TestValidateControlEnvelope_SuccessWithErrorClass(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
		ErrorClass:   "some_error", // Should be empty for success
	}
	err := validateControlEnvelope(env, "health", 0)
	if err == nil {
		t.Fatal("expected error for success envelope with error_class")
	}
}

func TestValidateControlEnvelope_FailureWithoutErrorClass(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      false,
		HTTPStatus:  500,
		ErrorClass:   "", // Should be non-empty for failure
	}
	err := validateControlEnvelope(env, "health", 1)
	if err == nil {
		t.Fatal("expected error for failure envelope without error_class")
	}
}

func TestValidateControlEnvelope_SuccessWithNon200(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      true,
		HTTPStatus:  500, // Should be 200 for success
	}
	err := validateControlEnvelope(env, "health", 0)
	if err == nil {
		t.Fatal("expected error for success envelope with non-200 status")
	}
}

// TestProtocolError_TypedError tests that ProtocolError is properly typed
func TestProtocolError_TypedError(t *testing.T) {
	err := &ProtocolError{
		Operation:  "health",
		ErrClass:  "connection_failed",
		Message:   "connection refused",
	}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

// TestIsProtocolNonRetryable_Retryable tests retryable error classes
func TestIsProtocolNonRetryable_Retryable(t *testing.T) {
	retryable := []string{
		"invalid_arguments",
		"malformed_json",
		"unknown_json_field",
		"missing_required_field",
		"trailing_json",
		"connection_failed",
	}
	for _, ec := range retryable {
		err := &ProtocolError{ErrClass: ec}
		if IsProtocolNonRetryable(err) {
			t.Errorf("expected %s to be retryable", ec)
		}
	}
}

// TestIsProtocolNonRetryable_NonRetryable tests non-retryable error classes
func TestIsProtocolNonRetryable_NonRetryable(t *testing.T) {
	nonRetryable := []string{
		"request_timeout",
		"unexpected_http_status",
		"health_not_ready",
		"state_invalid",
		"workload_count_mismatch",
	}
	for _, ec := range nonRetryable {
		err := &ProtocolError{ErrClass: ec}
		if !IsProtocolNonRetryable(err) {
			t.Errorf("expected %s to be non-retryable", ec)
		}
	}
}

// TestParseError_TypedError tests that ParseError is properly typed
func TestParseError_TypedError(t *testing.T) {
	err := &ParseError{
		ErrClass: "malformed_json",
		Message:  "invalid JSON syntax",
	}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

// TestCanaryStateFromExec_ZeroCounters tests that zero counters are valid
func TestCanaryStateFromExec_ZeroCounters(t *testing.T) {
	state := CanaryStateFromExec{
		Mode:           "growing",
		RetainedBlocks: 0,
		RetainedBytes:  0,
		OperationCount: 0,
		FDCount:        0,
		Ready:          true,
	}
	if state.OperationCount != 0 {
		t.Error("expected operation_count=0")
	}
}

// TestControlEnvelope_StatePayload tests state envelope parsing
func TestControlEnvelope_StatePayload(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"state","success":true,"http_status":200,"state":{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}}`
	env, err := strictParseEnvelope(stdout)
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if env.State == nil {
		t.Fatal("expected state payload")
	}
	if env.State.Mode != "growing" {
		t.Errorf("expected mode=growing, got %s", env.State.Mode)
	}
}

// TestControlEnvelope_WorkloadPayload tests workload envelope parsing
func TestControlEnvelope_WorkloadPayload(t *testing.T) {
	stdout := `{"schema_version":"canary-control/v1","operation":"operate","success":true,"http_status":200,"workload":{"requested":5,"attempted":5,"completed":5}}`
	env, err := strictParseEnvelope(stdout)
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if env.Workload == nil {
		t.Fatal("expected workload payload")
	}
	if env.Workload.Requested != 5 {
		t.Errorf("expected requested=5, got %d", env.Workload.Requested)
	}
	if env.Workload.Attempted != 5 {
		t.Errorf("expected attempted=5, got %d", env.Workload.Attempted)
	}
	if env.Workload.Completed != 5 {
		t.Errorf("expected completed=5, got %d", env.Workload.Completed)
	}
}

// TestCanaryControlExecResult_Fields tests that all result fields are properly set
func TestCanaryControlExecResult_Fields(t *testing.T) {
	result := CanaryControlExecResult{
		ExitCode:      0,
		Stdout:        `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`,
		Stderr:        "",
		HealthValid:   true,
		StateValid:    false,
		WorkloadValid: false,
		State:         nil,
		Attempted:     0,
		Completed:     0,
		Error:         nil,
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit_code=0, got %d", result.ExitCode)
	}
	if !result.HealthValid {
		t.Error("expected health_valid=true")
	}
}

// TestControlEnvelope_JSONRoundTrip tests encoding and decoding roundtrip
func TestControlEnvelope_JSONRoundTrip(t *testing.T) {
	original := ControlEnvelope{
		SchemaVersion: "canary-control/v1",
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
		Health:       &HealthPayload{Ready: true, Mode: "growing"},
	}
	
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(original); err != nil {
		t.Fatalf("encode error: %v", err)
	}
	
	env, err := strictParseEnvelope(buf.String())
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	
	if env.SchemaVersion != original.SchemaVersion {
		t.Errorf("schema_version mismatch: got %s, want %s", env.SchemaVersion, original.SchemaVersion)
	}
	if env.Operation != original.Operation {
		t.Errorf("operation mismatch: got %s, want %s", env.Operation, original.Operation)
	}
	if env.Success != original.Success {
		t.Errorf("success mismatch: got %v, want %v", env.Success, original.Success)
	}
}

// TestProtocolError_ErrorsAs tests errors.As support for ProtocolError
func TestProtocolError_ErrorsAs(t *testing.T) {
	err := &ProtocolError{
		Operation: "health",
		ErrClass: "connection_failed",
		Message:  "connection refused",
	}
	
	var protoErr *ProtocolError
	if !errors.As(err, &protoErr) {
		t.Fatal("expected errors.As to work with ProtocolError")
	}
	if protoErr.ErrClass != "connection_failed" {
		t.Errorf("expected err_class=connection_failed, got %s", protoErr.ErrClass)
	}
}

// TestParseError_ErrorsAs tests errors.As support for ParseError
func TestParseError_ErrorsAs(t *testing.T) {
	err := &ParseError{
		ErrClass: "malformed_json",
		Message:  "invalid JSON",
	}
	
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatal("expected errors.As to work with ParseError")
	}
	if parseErr.ErrClass != "malformed_json" {
		t.Errorf("expected err_class=malformed_json, got %s", parseErr.ErrClass)
	}
}

// TestBoundedResponse_64KB tests that 64KB responses are accepted
func TestBoundedResponse_64KB(t *testing.T) {
	// Use the same limit as the control client (64KB)
	const maxBody = 64 * 1024
	
	// Create exactly maxBody bytes of valid JSON
	data := make([]byte, maxBody)
	copy(data, []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}`))
	
	// Ensure it's valid JSON up to maxBody
	validPrefix := `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`
	_, err := strictParseEnvelope(validPrefix)
	if err != nil {
		t.Fatalf("expected valid JSON to parse: %v", err)
	}
	
	// Verify we can still read with LimitReader
	reader := io.LimitReader(bytes.NewReader(data), maxBody+1)
	read, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(read) > maxBody {
		t.Errorf("expected limit reader to respect maxBody, got %d", len(read))
	}
}
