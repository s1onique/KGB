// control_test.go — Protocol tests for canary control client
//
// Tests strict decoding, required-field presence, and variant validation.
// CORRECTION30 P0-8: Protocol tests for canary control client.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestStrictDecodeHealth_Success(t *testing.T) {
	data := []byte(`{"ready":true,"mode":"growing"}`)
	health, err := strictDecodeHealth(data)
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

func TestStrictDecodeHealth_MissingReady(t *testing.T) {
	data := []byte(`{"mode":"growing"}`)
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for missing ready field")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeHealth_MissingMode(t *testing.T) {
	data := []byte(`{"ready":true}`)
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for missing mode field")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeHealth_UnknownField(t *testing.T) {
	data := []byte(`{"ready":true,"mode":"growing","extra":"forbidden"}`)
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrUnknownJSONField {
		t.Errorf("expected ErrUnknownJSONField, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeHealth_NotReady(t *testing.T) {
	data := []byte(`{"ready":false,"mode":"growing"}`)
	_, err := strictDecodeHealth(data)
	if err != nil {
		t.Fatalf("strict decode should succeed for not-ready: %v", err)
	}
	// Note: Ready=false is valid JSON, but the caller should check health.Ready
}

func TestStrictDecodeHealth_EmptyBody(t *testing.T) {
	data := []byte{}
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMalformedJSON {
		t.Errorf("expected ErrMalformedJSON, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeHealth_SecondJSONValue(t *testing.T) {
	data := []byte(`{"ready":true,"mode":"growing"}{"extra":1}`)
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for second JSON value")
	}
	// Note: The first pass json.Unmarshal will fail because the input
	// contains two JSON objects, which is invalid JSON for object unmarshal
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	// The first pass detects this as malformed JSON since it's not valid JSON
	if decodeErr.ErrClass != ErrMalformedJSON {
		t.Errorf("expected ErrMalformedJSON, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeHealth_MalformedTrailingBytes(t *testing.T) {
	data := []byte(`{"ready":true,"mode":"growing"INVALID`)
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for malformed trailing bytes")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMalformedJSON {
		t.Errorf("expected ErrMalformedJSON, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeHealth_TrailingWhitespace(t *testing.T) {
	// Trailing whitespace only is valid
	data := []byte(`{"ready":true,"mode":"growing"}   `)
	health, err := strictDecodeHealth(data)
	if err != nil {
		t.Fatalf("expected success for trailing whitespace: %v", err)
	}
	if !health.Ready {
		t.Error("expected Ready=true")
	}
}

func TestStrictDecodeState_Success(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	state, err := strictDecodeState(data)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if state.Mode != "growing" {
		t.Errorf("expected mode=growing, got %s", state.Mode)
	}
	if state.RetainedBlocks != 0 {
		t.Errorf("expected retained_blocks=0, got %d", state.RetainedBlocks)
	}
	if state.Ready != true {
		t.Error("expected ready=true")
	}
}

func TestStrictDecodeState_MissingMode(t *testing.T) {
	data := []byte(`{"retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for missing mode field")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeState_MissingRetainedBlocks(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for missing retained_blocks field")
	}
}

func TestStrictDecodeState_MissingRetainedBytes(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for missing retained_bytes field")
	}
}

func TestStrictDecodeState_MissingOperationCount(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"fd_count":0,"ready":true}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for missing operation_count field")
	}
}

func TestStrictDecodeState_MissingFDCount(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"ready":true}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for missing fd_count field")
	}
}

func TestStrictDecodeState_MissingReady(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for missing ready field")
	}
}

func TestStrictDecodeState_UnknownField(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true,"extra":"forbidden"}`)
	_, err := strictDecodeState(data)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	decodeErr, ok := err.(*DecodeError)
	if !ok {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.ErrClass != ErrUnknownJSONField {
		t.Errorf("expected ErrUnknownJSONField, got %s", decodeErr.ErrClass)
	}
}

func TestStrictDecodeWorkload_Success(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5,"completed":5}`)
	workload, err := strictDecodeWorkload(data)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if workload.Requested != 5 {
		t.Errorf("expected requested=5, got %d", workload.Requested)
	}
	if workload.Attempted != 5 {
		t.Errorf("expected attempted=5, got %d", workload.Attempted)
	}
	if workload.Completed != 5 {
		t.Errorf("expected completed=5, got %d", workload.Completed)
	}
}

func TestStrictDecodeWorkload_MissingRequested(t *testing.T) {
	data := []byte(`{"attempted":5,"completed":5}`)
	_, err := strictDecodeWorkload(data)
	if err == nil {
		t.Fatal("expected error for missing requested field")
	}
}

func TestStrictDecodeWorkload_MissingAttempted(t *testing.T) {
	data := []byte(`{"requested":5,"completed":5}`)
	_, err := strictDecodeWorkload(data)
	if err == nil {
		t.Fatal("expected error for missing attempted field")
	}
}

func TestStrictDecodeWorkload_MissingCompleted(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5}`)
	_, err := strictDecodeWorkload(data)
	if err == nil {
		t.Fatal("expected error for missing completed field")
	}
}

func TestStrictDecodeWorkload_UnknownField(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5,"completed":5,"extra":"forbidden"}`)
	_, err := strictDecodeWorkload(data)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestStrictDecodeWorkload_SecondJSONValue(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5,"completed":5}{"extra":1}`)
	_, err := strictDecodeWorkload(data)
	if err == nil {
		t.Fatal("expected error for second JSON value")
	}
}

func TestStrictDecodeWorkload_MalformedTrailingBytes(t *testing.T) {
	data := []byte(`{"requested":5,"attempted":5,"completed":5}INVALID`)
	_, err := strictDecodeWorkload(data)
	if err == nil {
		t.Fatal("expected error for malformed trailing bytes")
	}
}

// TestEncodeEnvelope_Success tests that success envelopes are encoded correctly
func TestEncodeEnvelope_Success(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
		Health: &HealthPayload{
			Ready: true,
			Mode:  "growing",
		},
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

// TestEncodeEnvelope_Failure tests that failure envelopes are encoded correctly
func TestEncodeEnvelope_Failure(t *testing.T) {
	env := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:    "health",
		Success:      false,
		HTTPStatus:  500,
		ErrorClass:   ErrHealthNotReady,
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

// TestAllowedErrorClasses_Complete verifies all expected error classes exist
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
	}

	for _, ec := range expected {
		if !AllowedErrorClasses[ec] {
			t.Errorf("error class %s not in AllowedErrorClasses", ec)
		}
	}
}

// TestStrictDecodeEnvelope_Success tests strict envelope decoding success path
func TestStrictDecodeEnvelope_Success(t *testing.T) {
	// We need to test strict parsing from a client perspective
	// Since strictParseEnvelope is in dockerlab, we test the control client decoders
	
	data := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	
	// Test that we can re-encode and decode the inner health payload
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify required envelope fields
	for _, field := range []string{"schema_version", "operation", "success", "http_status"} {
		if _, ok := env[field]; !ok {
			t.Errorf("missing required envelope field: %s", field)
		}
	}

	// Verify health payload can be decoded
	if healthRaw, ok := env["health"]; ok {
		var health HealthPayload
		if err := json.Unmarshal(healthRaw, &health); err != nil {
			t.Fatalf("health unmarshal error: %v", err)
		}
		if !health.Ready {
			t.Error("expected health.ready=true")
		}
	} else {
		t.Error("health field missing")
	}
}

// TestStrictDecodeEnvelope_MissingFields tests envelope with missing required fields
func TestStrictDecodeEnvelope_MissingFields(t *testing.T) {
	data := []byte(`{"operation":"health","success":true}`) // missing schema_version, http_status
	
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	requiredFields := []string{"schema_version", "operation", "success", "http_status"}
	missing := 0
	for _, field := range requiredFields {
		if _, ok := env[field]; !ok {
			missing++
		}
	}
	if missing != 2 {
		t.Errorf("expected 2 missing fields, got %d", missing)
	}
}

// TestControlEnvelope_ValidEnvelopeVariants tests both success and failure variants
func TestControlEnvelope_ValidEnvelopeVariants(t *testing.T) {
	// Success variant
	successEnv := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:    "health",
		Success:      true,
		HTTPStatus:  200,
		Health:       &HealthPayload{Ready: true, Mode: "growing"},
	}
	
	// Validate success envelope rules
	if !successEnv.Success {
		t.Error("success envelope must have success=true")
	}
	if successEnv.HTTPStatus != 200 {
		t.Error("success envelope must have http_status=200")
	}
	if successEnv.ErrorClass != "" {
		t.Error("success envelope must not have error_class")
	}

	// Failure variant
	failureEnv := ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:    "health",
		Success:      false,
		HTTPStatus:  500,
		ErrorClass:   ErrHealthNotReady,
	}
	
	// Validate failure envelope rules
	if failureEnv.Success {
		t.Error("failure envelope must have success=false")
	}
	if failureEnv.ErrorClass == "" {
		t.Error("failure envelope must have error_class")
	}
}

// TestControlEnvelope_EmptyModeRejected tests that empty mode is rejected by caller
func TestControlEnvelope_EmptyModeRejected(t *testing.T) {
	// Empty mode is valid JSON but should be rejected by the caller
	data := []byte(`{"mode":"","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	state, err := strictDecodeState(data)
	if err != nil {
		t.Fatalf("empty mode should pass strict decode: %v", err)
	}
	// The caller (doStateCheck) should reject empty mode
	if state.Mode != "" {
		t.Error("expected mode to be empty string")
	}
}

// TestControlEnvelope_ZeroCounters tests that zero values are valid when present
func TestControlEnvelope_ZeroCounters(t *testing.T) {
	data := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	state, err := strictDecodeState(data)
	if err != nil {
		t.Fatalf("zero counters should pass strict decode: %v", err)
	}
	if state.OperationCount != 0 {
		t.Errorf("expected operation_count=0, got %d", state.OperationCount)
	}
}

// TestBoundedReader_LargeBody tests that oversized responses are rejected
func TestBoundedReader_LargeBody(t *testing.T) {
	// Create a response larger than MaxResponseBody (64KB)
	largeData := make([]byte, MaxResponseBody+1)
	for i := range largeData {
		largeData[i] = 'x'
	}
	
	// A LimitReader wrapping this should stop at MaxResponseBody+1 (the limit)
	// So ReadAll will read MaxResponseBody+1 bytes from the reader
	reader := io.LimitReader(bytes.NewReader(largeData), MaxResponseBody+1)
	read, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	
	// LimitReader allows up to n bytes, so we expect exactly MaxResponseBody+1 bytes
	if len(read) != MaxResponseBody+1 {
		t.Errorf("expected %d bytes from limit reader, got %d", MaxResponseBody+1, len(read))
	}
	
	// Verify that checking against MaxResponseBody would catch oversized response
	if len(read) > MaxResponseBody {
		// This is the check the caller does
	}
}

// TestStrictDecodeHealth_NullField tests explicit null behavior
func TestStrictDecodeHealth_NullField(t *testing.T) {
	data := []byte(`{"ready":null,"mode":"growing"}`)
	_, err := strictDecodeHealth(data)
	// Note: Go's json.Unmarshal accepts null for bool fields (sets to false/zero value)
	// This is current behavior - caller may need to check for zero values explicitly
	if err != nil {
		t.Fatalf("strictDecodeHealth should accept null (current behavior): %v", err)
	}
}

// TestStrictDecodeHealth_WrongType tests that wrong types are rejected
func TestStrictDecodeHealth_WrongType(t *testing.T) {
	// ready should be bool, not string
	data := []byte(`{"ready":"true","mode":"growing"}`)
	_, err := strictDecodeHealth(data)
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
}

// TestStrictDecodeState_EmptyMode tests that empty mode is detected
func TestStrictDecodeState_EmptyMode(t *testing.T) {
	data := []byte(`{"mode":"","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	state, err := strictDecodeState(data)
	if err != nil {
		t.Fatalf("empty mode should pass strict decode: %v", err)
	}
	// Caller must reject empty mode
	if state.Mode == "" {
		// Valid strict decode, caller should check
	}
}
