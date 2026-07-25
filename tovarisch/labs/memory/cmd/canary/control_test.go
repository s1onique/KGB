// control_test.go — Protocol tests for canary control client
//
// Tests strict decoding, required-field presence, envelope validation, and retry policy.
// CORRECTION33: Production-function tests with httptest.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ===== P0-1: Shared exact-object decoder tests =====

func TestDecodeExactJSONObject_EmptyInput(t *testing.T) {
	var target map[string]any
	err := decodeExactJSONObject([]byte{}, []string{"a"}, &target)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMalformedJSON {
		t.Errorf("expected ErrMalformedJSON, got %s", decodeErr.ErrClass)
	}
}

func TestDecodeExactJSONObject_SecondValue(t *testing.T) {
	var target map[string]any
	err := decodeExactJSONObject([]byte(`{} {}`), []string{}, &target)
	if err == nil {
		t.Fatal("expected error for second JSON value")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrTrailingJSON {
		t.Errorf("expected ErrTrailingJSON, got %s", decodeErr.ErrClass)
	}
}

func TestDecodeExactJSONObject_MalformedTrailingBytes(t *testing.T) {
	var target map[string]any
	err := decodeExactJSONObject([]byte(`{}INVALID`), []string{}, &target)
	if err == nil {
		t.Fatal("expected error for malformed trailing bytes")
	}
}

func TestDecodeExactJSONObject_UnknownMember(t *testing.T) {
	var target struct {
		A string `json:"a"`
	}
	err := decodeExactJSONObject([]byte(`{"a":"x","b":"y"}`), []string{"a"}, &target)
	if err == nil {
		t.Fatal("expected error for unknown member")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrUnknownJSONField {
		t.Errorf("expected ErrUnknownJSONField, got %s", decodeErr.ErrClass)
	}
}

func TestDecodeExactJSONObject_MissingMember(t *testing.T) {
	var target struct {
		A string `json:"a"`
		B string `json:"b"`
	}
	err := decodeExactJSONObject([]byte(`{"a":"x"}`), []string{"a", "b"}, &target)
	if err == nil {
		t.Fatal("expected error for missing member")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", decodeErr.ErrClass)
	}
}

func TestDecodeExactJSONObject_NullRequiredMember(t *testing.T) {
	var target struct {
		A string `json:"a"`
	}
	err := decodeExactJSONObject([]byte(`{"a":null}`), []string{"a"}, &target)
	if err == nil {
		t.Fatal("expected error for null required member")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", decodeErr.ErrClass)
	}
}

func TestDecodeExactJSONObject_Success(t *testing.T) {
	var target struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	err := decodeExactJSONObject([]byte(`{"a":"hello","b":42}`), []string{"a", "b"}, &target)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if target.A != "hello" {
		t.Errorf("expected a=hello, got %s", target.A)
	}
	if target.B != 42 {
		t.Errorf("expected b=42, got %d", target.B)
	}
}

// ===== P0-3: Health mode semantics tests =====

func TestStrictDecodeHealth_Success(t *testing.T) {
	data := []byte(`{"ready":true,"mode":"growing"}`)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !health.Ready {
		t.Error("expected Ready=true")
	}
	if health.Mode != "growing" {
		t.Errorf("expected mode=growing, got %s", health.Mode)
	}
}

func TestStrictDecodeHealth_ReadyFalse(t *testing.T) {
	// ready=false is valid JSON, but the caller should check health.Ready
	data := []byte(`{"ready":false,"mode":"growing"}`)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err != nil {
		t.Fatalf("strict decode should succeed for not-ready: %v", err)
	}
}

func TestStrictDecodeHealth_MissingMode(t *testing.T) {
	data := []byte(`{"ready":true}`)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err == nil {
		t.Fatal("expected error for missing mode")
	}
}

func TestStrictDecodeHealth_NullMode(t *testing.T) {
	data := []byte(`{"ready":true,"mode":null}`)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err == nil {
		t.Fatal("expected error for null mode")
	}
}

func TestStrictDecodeHealth_UnknownField(t *testing.T) {
	data := []byte(`{"ready":true,"mode":"growing","extra":"forbidden"}`)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestStrictDecodeHealth_EmptyBody(t *testing.T) {
	data := []byte{}
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestStrictDecodeHealth_WrongType(t *testing.T) {
	// ready should be bool, not string
	data := []byte(`{"ready":"true","mode":"growing"}`)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
}

// ===== P0-4: Envelope variant validation tests =====

func TestValidateControlEnvelope_SuccessHealth(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	err := ValidateControlEnvelope(&env)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestValidateControlEnvelope_SuccessState(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "state",
		Success:       true,
		HTTPStatus:    200,
		State:         &StatePayload{Mode: "growing", RetainedBlocks: 0, RetainedBytes: 0, OperationCount: 0, FDCount: 0, Ready: true},
	}
	err := ValidateControlEnvelope(&env)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestValidateControlEnvelope_SuccessOperate(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "operate",
		Success:       true,
		HTTPStatus:    200,
		Workload:      &WorkloadPayload{Requested: 5, Attempted: 5, Completed: 5},
	}
	err := ValidateControlEnvelope(&env)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestValidateControlEnvelope_Failure(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    ErrHealthNotReady,
	}
	err := ValidateControlEnvelope(&env)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestValidateControlEnvelope_SchemaMismatch(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: "wrong-version",
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for schema mismatch")
	}
}

func TestValidateControlEnvelope_InvalidOperation(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "invalid_op",
		Success:       true,
		HTTPStatus:    200,
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for invalid operation")
	}
}

func TestValidateControlEnvelope_SuccessWithErrorClass(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		ErrorClass:    ErrHealthNotReady, // Invalid for success
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for success with error_class")
	}
}

func TestValidateControlEnvelope_SuccessWithMultiplePayloads(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
		State:         &StatePayload{Mode: "growing", RetainedBlocks: 0, RetainedBytes: 0, OperationCount: 0, FDCount: 0, Ready: true},
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for health operation with state payload")
	}
}

func TestValidateControlEnvelope_FailureWithPayload(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    ErrHealthNotReady,
		Health:        &HealthPayload{Ready: true, Mode: "growing"}, // Invalid for failure
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for failure with payload")
	}
}

func TestValidateControlEnvelope_FailureHTTP200(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    200,
		ErrorClass:    ErrHealthNotReady,
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for failure with HTTP 200")
	}
}

func TestValidateControlEnvelope_FailureNoErrorClass(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		// Missing ErrorClass
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for failure without error_class")
	}
}

func TestValidateControlEnvelope_UnknownErrorClass(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    "unknown_error",
	}
	err := ValidateControlEnvelope(&env)
	if err == nil {
		t.Fatal("expected error for unknown error class")
	}
}

// ===== P0-5: Retry policy tests =====

func TestIsProtocolRetryable_ConnectionFailed(t *testing.T) {
	err := &DecodeError{ErrClass: ErrConnectionFailed, Message: "connection refused"}
	if !IsProtocolRetryable(err) {
		t.Error("expected connection_failed to be retryable")
	}
}

func TestIsProtocolRetryable_RequestTimeout(t *testing.T) {
	err := &DecodeError{ErrClass: ErrRequestTimeout, Message: "timeout"}
	if !IsProtocolRetryable(err) {
		t.Error("expected request_timeout to be retryable")
	}
}

func TestIsProtocolRetryable_HealthNotReady(t *testing.T) {
	err := &DecodeError{ErrClass: ErrHealthNotReady, Message: "not ready"}
	if !IsProtocolRetryable(err) {
		t.Error("expected health_not_ready to be retryable")
	}
}

func TestIsProtocolRetryable_NonRetryable(t *testing.T) {
	testCases := []ErrorClass{
		ErrInvalidArguments,
		ErrMalformedJSON,
		ErrUnknownJSONField,
		ErrMissingRequiredField,
		ErrTrailingJSON,
		ErrStateInvalid,
		ErrWorkloadCountMismatch,
		ErrSchemaVersionMismatch,
		ErrInvalidOperation,
	}
	for _, ec := range testCases {
		err := &DecodeError{ErrClass: ec, Message: "test"}
		if IsProtocolRetryable(err) {
			t.Errorf("expected %s to NOT be retryable", ec)
		}
	}
}

func TestIsProtocolRetryable_Nil(t *testing.T) {
	if IsProtocolRetryable(nil) {
		t.Error("expected nil to not be retryable")
	}
}

// ===== P0-6: Typed operation tests =====

func TestControlOperation_BuildArgv(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlHealth,
		Port:    8080,
		Count:   0,
		Timeout: 5 * time.Second,
	}
	argv := buildArgv(op)
	expected := []string{"/app/canary", "control", "health", "--port", "8080", "--timeout", "5s"}
	if len(argv) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(argv))
	}
	for i := range argv {
		if argv[i] != expected[i] {
			t.Errorf("argv[%d]: expected %q, got %q", i, expected[i], argv[i])
		}
	}
}

func TestControlOperation_BuildArgvOperate(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlOperate,
		Port:    9090,
		Count:   10,
		Timeout: 30 * time.Second,
	}
	argv := buildArgv(op)
	// Expected: ["/app/canary", "control", "operate", "--port", "9090", "--count", "10", "--timeout", "30s"] = 9 args
	if len(argv) != 9 {
		t.Fatalf("expected 9 args, got %d", len(argv))
	}
	if argv[0] != "/app/canary" || argv[1] != "control" || argv[2] != "operate" {
		t.Errorf("expected /app/canary control operate, got %s %s %s", argv[0], argv[1], argv[2])
	}
	if argv[5] != "--count" {
		t.Errorf("expected argv[5]=--count, got %s", argv[5])
	}
	if argv[6] != "10" {
		t.Errorf("expected argv[6]=10, got %s", argv[6])
	}
}

func TestControlOperation_Validate(t *testing.T) {
	// Valid operation
	op := ControlOperation{Kind: ControlHealth, Port: 8080, Timeout: 5 * time.Second}
	if err := op.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	// Invalid port
	op.Port = 0
	if err := op.Validate(); err == nil {
		t.Error("expected error for port=0")
	}
	op.Port = 70000
	if err := op.Validate(); err == nil {
		t.Error("expected error for port>65535")
	}
	op.Port = 8080

	// Invalid timeout
	op.Timeout = 0
	if err := op.Validate(); err == nil {
		t.Error("expected error for timeout=0")
	}
	op.Timeout = 5 * time.Second

	// Operate with invalid count
	op.Kind = ControlOperate
	op.Count = 0
	if err := op.Validate(); err == nil {
		t.Error("expected error for count=0")
	}

	// Unknown operation
	op.Kind = ControlOperationKind("unknown")
	if err := op.Validate(); err == nil {
		t.Error("expected error for unknown operation")
	}
}

// ===== P0-7: httptest production function tests =====

func TestDoHealthCheck_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected /health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":"growing"}`))
	}))
	defer server.Close()

	// Pass the full URL including /health path
	result := doHealthCheck(context.Background(), server.URL+"/health", 5*time.Second)
	if result.ErrorClass != "" {
		t.Errorf("expected no error, got %s", result.ErrorClass)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Health == nil || result.Health.Mode != "growing" {
		t.Error("expected health with mode=growing")
	}
}

func TestDoHealthCheck_HealthNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":false,"mode":"growing"}`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrHealthNotReady {
		t.Errorf("expected ErrHealthNotReady, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_EmptyMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":""}`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_MissingMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true}`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_NullMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":null}`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_UnknownField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":"growing","extra":"forbidden"}`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrUnknownJSONField {
		t.Errorf("expected ErrUnknownJSONField, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_BodyBoundaryUnderLimit(t *testing.T) {
	// Test with a valid body within MaxResponseBody limit
	body := []byte(`{"ready":true,"mode":"growing"}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != "" {
		t.Errorf("expected no error for valid body, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_64KBPlusOneBody(t *testing.T) {
	// Create 64KB + 1 byte body
	body := make([]byte, MaxResponseBody+1)
	body[0] = '{'
	body[MaxResponseBody] = '}'
	for i := 1; i < MaxResponseBody; i++ {
		body[i] = 'x'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrResponseTooLarge {
		t.Errorf("expected ErrResponseTooLarge, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_SecondJSONValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":"growing"}{"extra":1}`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrTrailingJSON {
		t.Errorf("expected ErrTrailingJSON, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_MalformedTrailingBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":"growing"}INVALID`))
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	// Implementation classifies trailing bytes as trailing_json, not malformed_json
	if result.ErrorClass != ErrTrailingJSON {
		t.Errorf("expected ErrTrailingJSON, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_HTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrUnexpectedHTTPStatus {
		t.Errorf("expected ErrUnexpectedHTTPStatus, got %s", result.ErrorClass)
	}
}

func TestDoStateCheck_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/state" {
			t.Errorf("expected /state, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL+"/state", 5*time.Second)
	if result.ErrorClass != "" {
		t.Errorf("expected no error, got %s", result.ErrorClass)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.State == nil || result.State.Mode != "growing" {
		t.Error("expected state with mode=growing")
	}
}

func TestDoStateCheck_MissingRequiredField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":0}`)) // Missing other fields
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoStateCheck_EmptyMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoOperateCheck_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/operate" {
			t.Errorf("expected /operate, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":5,"completed":5}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != "" {
		t.Errorf("expected no error, got %s", result.ErrorClass)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Workload == nil || result.Workload.Requested != 5 {
		t.Error("expected workload with requested=5")
	}
}

func TestDoOperateCheck_WorkloadCountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":3,"completed":3}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %s", result.ErrorClass)
	}
}

func TestDoOperateCheck_NegativeCounters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":-1,"completed":5}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	// Negative attempted should be caught as count mismatch
	if result.ErrorClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":"growing"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := doHealthCheck(ctx, server.URL, 50*time.Millisecond)
	if result.ErrorClass != ErrRequestTimeout {
		t.Errorf("expected ErrRequestTimeout, got %s", result.ErrorClass)
	}
}

// ===== Additional helper tests =====

func TestEncodeEnvelope_Success(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// Decode and verify
	var decoded ControlEnvelope
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if decoded.Success != true {
		t.Error("expected success=true")
	}
	if decoded.HTTPStatus != 200 {
		t.Errorf("expected http_status=200, got %d", decoded.HTTPStatus)
	}
	if decoded.ErrorClass != "" {
		t.Errorf("expected error_class empty, got %s", decoded.ErrorClass)
	}
}

func TestEncodeEnvelope_Failure(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    ErrHealthNotReady,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// Decode and verify
	var decoded ControlEnvelope
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if decoded.Success != false {
		t.Error("expected success=false")
	}
	if decoded.ErrorClass != ErrHealthNotReady {
		t.Errorf("expected error_class=%s, got %s", ErrHealthNotReady, decoded.ErrorClass)
	}
}

func TestAllowedErrorClasses_Complete(t *testing.T) {
	expected := []ErrorClass{
		ErrInvalidArguments,
		ErrRequestCreateFailed,
		ErrConnectionFailed,
		ErrRequestTimeout,
		ErrResponseTooLarge,
		ErrUnexpectedHTTPStatus,
		ErrMalformedJSON,
		ErrUnknownJSONField,
		ErrMissingRequiredField,
		ErrTrailingJSON,
		ErrHealthNotReady,
		ErrStateInvalid,
		ErrWorkloadCountMismatch,
		ErrSchemaVersionMismatch,
		ErrInvalidOperation,
	}

	for _, ec := range expected {
		if !AllowedErrorClasses[ec] {
			t.Errorf("error class %s not in AllowedErrorClasses", ec)
		}
	}
}

func TestBoundedReader_LargeBody(t *testing.T) {
	// Create a response larger than MaxResponseBody (64KB)
	largeData := make([]byte, MaxResponseBody+1)
	for i := range largeData {
		largeData[i] = 'x'
	}

	// A LimitReader wrapping this should stop at MaxResponseBody+1 (the limit)
	reader := io.LimitReader(bytes.NewReader(largeData), MaxResponseBody+1)
	read, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	// LimitReader allows up to n bytes, so we expect exactly MaxResponseBody+1 bytes
	if len(read) != MaxResponseBody+1 {
		t.Errorf("expected %d bytes from limit reader, got %d", MaxResponseBody+1, len(read))
	}
}

func TestControlEnvelope_TrailingWhitespace(t *testing.T) {
	// Trailing whitespace only is valid
	data := []byte(`{"ready":true,"mode":"growing"}   `)
	var health HealthPayload
	err := decodeExactJSONObject(data, []string{"ready", "mode"}, &health)
	if err != nil {
		t.Fatalf("expected success for trailing whitespace: %v", err)
	}
	if !health.Ready {
		t.Error("expected Ready=true")
	}
}

// ===== State payload tests =====

func TestStrictDecodeState_Valid(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	var state StatePayload
	err := decodeExactJSONObject(data, []string{"mode", "retained_blocks", "retained_bytes", "operation_count", "fd_count", "ready"}, &state)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if state.Mode != "growing" {
		t.Errorf("expected mode=growing, got %s", state.Mode)
	}
	if state.Ready != true {
		t.Error("expected ready=true")
	}
}

func TestStrictDecodeState_NullField(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":null,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	var state StatePayload
	err := decodeExactJSONObject(data, []string{"mode", "retained_blocks", "retained_bytes", "operation_count", "fd_count", "ready"}, &state)
	if err == nil {
		t.Fatal("expected error for null field")
	}
}

// ===== Workload payload tests =====

func TestStrictDecodeWorkload_Valid(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5,"completed":5}`)
	var workload WorkloadPayload
	err := decodeExactJSONObject(data, []string{"requested", "attempted", "completed"}, &workload)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if workload.Requested != 5 {
		t.Errorf("expected requested=5, got %d", workload.Requested)
	}
}

func TestStrictDecodeWorkload_NullField(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":null,"completed":5}`)
	var workload WorkloadPayload
	err := decodeExactJSONObject(data, []string{"requested", "attempted", "completed"}, &workload)
	if err == nil {
		t.Fatal("expected error for null field")
	}
}

func TestStrictDecodeWorkload_SecondJSONValue(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5,"completed":5}{"extra":1}`)
	var workload WorkloadPayload
	err := decodeExactJSONObject(data, []string{"requested", "attempted", "completed"}, &workload)
	if err == nil {
		t.Fatal("expected error for second JSON value")
	}
}
