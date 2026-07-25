// protocol_test.go — Tests for the shared canary control protocol.
//
// Covers:
//   - exact-object decoder empty/second-value/trailing-bytes cases
//   - envelope required-member and null rejection
//   - success/failure variant disjointness
//   - payload semantics (health, state, workload)
//   - retry policy authority
//   - typed operation argv authority
//   - immutable shared vocabulary (P0-1)
//
// CORRECTION35: shared collections are private. Tests target the accessor
// functions and verify mutation-resistance.
package canarycontrol

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// ===== decodeExactJSONObject tests =====

func TestDecodeEnvelopeExactlyOne_EmptyInput(t *testing.T) {
	_, err := DecodeEnvelopeExactlyOne([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrMalformedJSON {
		t.Errorf("expected ErrMalformedJSON, got %v", err)
	}
}

func TestDecodeEnvelopeExactlyOne_SecondValue(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200}{"extra":1}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for second JSON value")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrTrailingJSON {
		t.Errorf("expected ErrTrailingJSON, got %v", err)
	}
}

func TestDecodeEnvelopeExactlyOne_MalformedTrailingBytes(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200}INVALID`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for malformed trailing bytes")
	}
}

func TestDecodeEnvelopeExactlyOne_UnknownField(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200,"extra":"forbidden"}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrUnknownJSONField {
		t.Errorf("expected ErrUnknownJSONField, got %v", err)
	}
}

func TestDecodeEnvelopeExactlyOne_NullRequiredMember(t *testing.T) {
	body := []byte(`{"schema_version":null,"operation":"health","success":true,"http_status":200}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for null schema_version")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %v", err)
	}
}

func TestDecodeEnvelopeExactlyOne_MissingMember(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for missing http_status")
	}
}

func TestDecodeEnvelopeExactlyOne_EmptySchemaVersion(t *testing.T) {
	body := []byte(`{"schema_version":"","operation":"health","success":true,"http_status":200}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for empty schema_version")
	}
}

func TestDecodeEnvelopeExactlyOne_EmptyOperation(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"","success":true,"http_status":200}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for empty operation")
	}
}

func TestDecodeEnvelopeExactlyOne_SchemaVersionMismatch(t *testing.T) {
	body := []byte(`{"schema_version":"wrong-version","operation":"health","success":true,"http_status":200}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for schema mismatch")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrSchemaVersionMismatch {
		t.Errorf("expected ErrSchemaVersionMismatch, got %v", err)
	}
}

func TestDecodeEnvelopeExactlyOne_InvalidOperation(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"bad","success":true,"http_status":200}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for invalid operation")
	}
}

// ===== Nested payload validation =====

func TestDecodeEnvelope_NestedHealthNull(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200,"health":null}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for null health payload")
	}
}

func TestDecodeEnvelope_NestedHealthMissing(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for missing health payload")
	}
}

func TestDecodeEnvelope_NestedHealthUnknownField(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing","extra":"forbidden"}}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for unknown field in health payload")
	}
}

func TestDecodeEnvelope_NestedStateNullField(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"state","success":true,"http_status":200,"state":{"mode":"growing","retained_blocks":null,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for null state field")
	}
}

func TestDecodeEnvelope_NestedWorkloadMismatch(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"operate","success":true,"http_status":200,"workload":{"requested":5,"attempted":3,"completed":3}}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for workload mismatch")
	}
}

func TestDecodeEnvelope_HealthEnvelopeWithStatePayload(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"},"state":{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("expected error for health envelope with state payload")
	}
}

// ===== Health payload tests =====

func TestDecodeHealth_Success(t *testing.T) {
	body := []byte(`{"ready":true,"mode":"growing"}`)
	h, err := DecodeHealth(body)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !h.Ready || h.Mode != "growing" {
		t.Errorf("unexpected payload: %+v", h)
	}
}

func TestDecodeHealth_NotReady(t *testing.T) {
	body := []byte(`{"ready":false,"mode":"growing"}`)
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for ready=false")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrHealthNotReady {
		t.Errorf("expected ErrHealthNotReady, got %v", err)
	}
}

func TestDecodeHealth_NullMode(t *testing.T) {
	body := []byte(`{"ready":true,"mode":null}`)
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for null mode")
	}
}

func TestDecodeHealth_EmptyMode(t *testing.T) {
	body := []byte(`{"ready":true,"mode":""}`)
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for empty mode")
	}
}

func TestDecodeHealth_UnknownField(t *testing.T) {
	body := []byte(`{"ready":true,"mode":"growing","extra":"forbidden"}`)
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

// ===== State payload tests =====

func TestDecodeState_Valid(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	s, err := DecodeState(body)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if s.Mode != "growing" {
		t.Errorf("unexpected mode: %s", s.Mode)
	}
}

func TestDecodeState_NullField(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":null,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for null field")
	}
}

func TestDecodeState_NegativeRetainedBlocks(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":-1,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for negative counter")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrStateInvalid {
		t.Errorf("expected ErrStateInvalid, got %v", err)
	}
}

func TestDecodeState_NegativeRetainedBytes(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":-1,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for negative retained_bytes")
	}
}

func TestDecodeState_NegativeOperationCount(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":-1,"fd_count":0,"ready":true}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for negative operation_count")
	}
}

func TestDecodeState_NegativeFDCount(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":-1,"ready":true}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for negative fd_count")
	}
}

func TestDecodeState_NotReady(t *testing.T) {
	body := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":false}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for ready=false")
	}
}

func TestDecodeState_EmptyMode(t *testing.T) {
	body := []byte(`{"mode":"","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	_, err := DecodeState(body)
	if err == nil {
		t.Fatal("expected error for empty mode")
	}
}

func TestDecodeState_EachFieldMissing(t *testing.T) {
	// mode missing
	body := []byte(`{"retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	if _, err := DecodeState(body); err == nil {
		t.Error("expected error for missing mode")
	}
	// retained_blocks missing
	body = []byte(`{"mode":"growing","retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
	if _, err := DecodeState(body); err == nil {
		t.Error("expected error for missing retained_blocks")
	}
	// ready missing
	body = []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0}`)
	if _, err := DecodeState(body); err == nil {
		t.Error("expected error for missing ready")
	}
}

func TestDecodeState_EachFieldNull(t *testing.T) {
	fields := []string{"mode", "retained_blocks", "retained_bytes", "operation_count", "fd_count", "ready"}
	for _, f := range fields {
		body := []byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`)
		// Replace "f":value with "f":null
		bodyStr := strings.Replace(string(body), `"`+f+`":`, `"`+f+`":null,`, 1)
		if _, err := DecodeState([]byte(bodyStr)); err == nil {
			t.Errorf("expected error for null %s", f)
		}
	}
}

// ===== Workload payload tests =====

func TestDecodeWorkload_Valid(t *testing.T) {
	body := []byte(`{"requested":5,"attempted":5,"completed":5}`)
	w, err := DecodeWorkload(body, 5)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if w.Requested != 5 || w.Attempted != 5 || w.Completed != 5 {
		t.Errorf("unexpected payload: %+v", w)
	}
}

func TestDecodeWorkload_CountMismatch(t *testing.T) {
	body := []byte(`{"requested":5,"attempted":3,"completed":3}`)
	_, err := DecodeWorkload(body, 5)
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
	if pe, ok := AsProtocolError(err); !ok || pe.ErrClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %v", err)
	}
}

func TestDecodeWorkload_AttemptedMismatch(t *testing.T) {
	body := []byte(`{"requested":5,"attempted":4,"completed":4}`)
	_, err := DecodeWorkload(body, 5)
	if err == nil {
		t.Fatal("expected error for attempted mismatch")
	}
}

func TestDecodeWorkload_CompletedMismatch(t *testing.T) {
	body := []byte(`{"requested":5,"attempted":5,"completed":4}`)
	_, err := DecodeWorkload(body, 5)
	if err == nil {
		t.Fatal("expected error for completed mismatch")
	}
}

func TestDecodeWorkload_ZeroRequested(t *testing.T) {
	body := []byte(`{"requested":0,"attempted":0,"completed":0}`)
	_, err := DecodeWorkload(body, 5)
	if err == nil {
		t.Fatal("expected error for zero requested")
	}
}

func TestDecodeWorkload_NegativeRequested(t *testing.T) {
	body := []byte(`{"requested":-1,"attempted":-1,"completed":-1}`)
	_, err := DecodeWorkload(body, 5)
	if err == nil {
		t.Fatal("expected error for negative requested")
	}
}

func TestDecodeWorkload_ExpectedMismatch(t *testing.T) {
	body := []byte(`{"requested":5,"attempted":5,"completed":5}`)
	_, err := DecodeWorkload(body, 10)
	if err == nil {
		t.Fatal("expected error for requested != expected")
	}
}

func TestDecodeWorkload_NullField(t *testing.T) {
	body := []byte(`{"requested":5,"attempted":null,"completed":5}`)
	_, err := DecodeWorkload(body, 5)
	if err == nil {
		t.Fatal("expected error for null field")
	}
}

// ===== Failure HTTP status strict boundaries =====

func TestValidateEnvelope_FailureHTTPStatus_Rejects_0(t *testing.T) {
	// 0 is allowed for local failures (no HTTP status)
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    0,
		ErrorClass:    ErrHealthNotReady,
	}
	if err := ValidateControlEnvelope(env); err != nil {
		t.Errorf("expected success for http_status=0, got %v", err)
	}
}

func TestValidateEnvelope_FailureHTTPStatus_Accepts_300(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    301,
		ErrorClass:    ErrHealthNotReady,
	}
	if err := ValidateControlEnvelope(env); err != nil {
		t.Errorf("expected success for http_status=301, got %v", err)
	}
}

func TestValidateEnvelope_FailureHTTPStatus_Rejects_Negative(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    -1,
		ErrorClass:    ErrHealthNotReady,
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for negative http_status")
	}
}

func TestValidateEnvelope_FailureHTTPStatus_Rejects_99(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    99,
		ErrorClass:    ErrHealthNotReady,
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for http_status=99")
	}
}

func TestValidateEnvelope_FailureHTTPStatus_Rejects_600(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    600,
		ErrorClass:    ErrHealthNotReady,
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for http_status=600")
	}
}

// ===== Envelope validation tests =====

func TestValidateEnvelope_SuccessHealth(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	if err := ValidateControlEnvelope(env); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestValidateEnvelope_SuccessState(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "state",
		Success:       true,
		HTTPStatus:    200,
		State:         &StatePayload{Mode: "growing", Ready: true},
	}
	if err := ValidateControlEnvelope(env); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestValidateEnvelope_SuccessOperate(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "operate",
		Success:       true,
		HTTPStatus:    200,
		Workload:      &WorkloadPayload{Requested: 5, Attempted: 5, Completed: 5},
	}
	if err := ValidateControlEnvelope(env); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestValidateEnvelope_Failure(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    ErrHealthNotReady,
	}
	if err := ValidateControlEnvelope(env); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestValidateEnvelope_SuccessWithErrorClass(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		ErrorClass:    ErrHealthNotReady,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for success with error_class")
	}
}

func TestValidateEnvelope_FailureWithPayload(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    ErrHealthNotReady,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for failure with payload")
	}
}

func TestValidateEnvelope_FailureNoErrorClass(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for failure without error_class")
	}
}

func TestValidateEnvelope_UnknownErrorClass(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       false,
		HTTPStatus:    500,
		ErrorClass:    "unknown_error",
	}
	if err := ValidateControlEnvelope(env); err == nil {
		t.Error("expected error for unknown error class")
	}
}

// ===== Retry policy tests =====

func TestIsRetryable_Retryable(t *testing.T) {
	cases := []ErrorClass{ErrConnectionFailed, ErrRequestTimeout, ErrHealthNotReady}
	for _, ec := range cases {
		if !IsRetryable(&ProtocolError{ErrClass: ec}) {
			t.Errorf("expected %s to be retryable", ec)
		}
	}
}

func TestIsRetryable_NonRetryable(t *testing.T) {
	cases := []ErrorClass{
		ErrInvalidArguments, ErrMalformedJSON, ErrUnknownJSONField,
		ErrMissingRequiredField, ErrTrailingJSON, ErrStateInvalid,
		ErrWorkloadCountMismatch, ErrSchemaVersionMismatch, ErrInvalidOperation,
		ErrUnexpectedHTTPStatus, ErrResponseTooLarge,
	}
	for _, ec := range cases {
		if IsRetryable(&ProtocolError{ErrClass: ec}) {
			t.Errorf("expected %s to NOT be retryable", ec)
		}
	}
}

func TestIsRetryable_Nil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("expected nil to not be retryable")
	}
}

func TestIsRetryable_NonProtocolError(t *testing.T) {
	if IsRetryable(errors.New("plain error")) {
		t.Error("expected plain error to not be retryable")
	}
}

// ===== Operation argv authority (fails closed) =====

func TestControlOperation_BuildArgvHealth(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 8080, Timeout: 5 * time.Second}
	argv, err := op.BuildArgv()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
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

func TestControlOperation_BuildArgvState(t *testing.T) {
	op := ControlOperation{Kind: OpState, Port: 8080, Timeout: 5 * time.Second}
	argv, err := op.BuildArgv()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	expected := []string{"/app/canary", "control", "state", "--port", "8080", "--timeout", "5s"}
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
	op := ControlOperation{Kind: OpOperate, Port: 8080, Count: 10, Timeout: 30 * time.Second}
	argv, err := op.BuildArgv()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	expected := []string{"/app/canary", "control", "operate", "--port", "8080", "--count", "10", "--timeout", "30s"}
	if len(argv) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(argv))
	}
	for i := range argv {
		if argv[i] != expected[i] {
			t.Errorf("argv[%d]: expected %q, got %q", i, expected[i], argv[i])
		}
	}
}

func TestControlOperation_NoShell(t *testing.T) {
	for _, op := range []ControlOperation{
		{Kind: OpHealth, Port: 8080, Timeout: 5 * time.Second},
		{Kind: OpState, Port: 8080, Timeout: 5 * time.Second},
		{Kind: OpOperate, Port: 8080, Count: 5, Timeout: 30 * time.Second},
	} {
		argv, err := op.BuildArgv()
		if err != nil {
			t.Errorf("BuildArgv failed: %v", err)
			continue
		}
		for _, arg := range argv {
			low := strings.ToLower(arg)
			if low == "/bin/sh" || low == "/bin/bash" || low == "sh" || low == "bash" ||
				low == "curl" || low == "wget" || low == "nc" || low == "telnet" {
				t.Errorf("forbidden tool in argv: %s", arg)
			}
		}
		if argv[0] != CanaryExecutable {
			t.Errorf("argv[0] must be %q, got %q", CanaryExecutable, argv[0])
		}
	}
}

func TestControlOperation_BuildArgvFailsClosedOnBadOp(t *testing.T) {
	op := ControlOperation{Kind: Operation("nope"), Port: 8080, Timeout: 5 * time.Second}
	_, err := op.BuildArgv()
	if err == nil {
		t.Error("expected error for invalid op")
	}
}

func TestControlOperation_BuildArgvFailsClosedOnBadPort(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 0, Timeout: 5 * time.Second}
	_, err := op.BuildArgv()
	if err == nil {
		t.Error("expected error for port=0")
	}
}

func TestControlOperation_BuildArgvFailsClosedOnBadCount(t *testing.T) {
	op := ControlOperation{Kind: OpOperate, Port: 8080, Count: 0, Timeout: 5 * time.Second}
	_, err := op.BuildArgv()
	if err == nil {
		t.Error("expected error for count=0")
	}
}

func TestControlOperation_BuildArgvFailsClosedOnZeroTimeout(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 8080, Timeout: 0}
	_, err := op.BuildArgv()
	if err == nil {
		t.Error("expected error for timeout=0")
	}
}

func TestNewControlOperation(t *testing.T) {
	op, err := NewControlOperation(OpHealth, 8080, 0, 5*time.Second)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if op.Port != 8080 {
		t.Errorf("expected port=8080, got %d", op.Port)
	}
	if _, err := NewControlOperation(OpHealth, 0, 0, 5*time.Second); err == nil {
		t.Error("expected error for port=0")
	}
	if _, err := NewControlOperation(OpOperate, 8080, 0, 5*time.Second); err == nil {
		t.Error("expected error for count=0")
	}
}

func TestControlOperation_Validate(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 8080, Timeout: 5 * time.Second}
	if err := op.Validate(); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
	op.Port = 0
	if err := op.Validate(); err == nil {
		t.Error("expected error for port=0")
	}
	op.Port = 70000
	if err := op.Validate(); err == nil {
		t.Error("expected error for port>65535")
	}
	op.Port = 8080
	op.Timeout = 0
	if err := op.Validate(); err == nil {
		t.Error("expected error for timeout=0")
	}
	op.Timeout = 5 * time.Second
	op.Kind = OpOperate
	op.Count = 0
	if err := op.Validate(); err == nil {
		t.Error("expected error for count=0")
	}
	op.Kind = Operation("unknown")
	if err := op.Validate(); err == nil {
		t.Error("expected error for unknown operation")
	}
	op.Kind = ""
	if err := op.Validate(); err == nil {
		t.Error("expected error for empty operation")
	}
}

// ===== P0-1: Mutation-resistance tests =====

func TestAllErrorClasses_ReturnsDefensiveCopy(t *testing.T) {
	classes := AllErrorClasses()
	if len(classes) == 0 {
		t.Fatal("expected non-empty error classes")
	}
	// Mutate the returned slice
	classes[0] = "hacked"
	// Verify package-level authority is unaffected
	if AllErrorClasses()[0] == "hacked" {
		t.Error("AllErrorClasses must return a defensive copy")
	}
	if !IsAllowedErrorClass(ErrInvalidArguments) {
		t.Error("mutating slice should not affect IsAllowedErrorClass")
	}
}

func TestAllOperations_ReturnsDefensiveCopy(t *testing.T) {
	ops := AllOperations()
	if len(ops) == 0 {
		t.Fatal("expected non-empty operations")
	}
	ops[0] = "hacked"
	if AllOperations()[0] == "hacked" {
		t.Error("AllOperations must return a defensive copy")
	}
	if !IsValidOperation(OpHealth) {
		t.Error("mutating slice should not affect IsValidOperation")
	}
}

func TestRequiredEnvelopeFields_ReturnsDefensiveCopy(t *testing.T) {
	fields := RequiredEnvelopeFields()
	fields[0] = "hacked"
	if RequiredEnvelopeFields()[0] == "hacked" {
		t.Error("RequiredEnvelopeFields must return a defensive copy")
	}
}

func TestRequiredHealthFields_ReturnsDefensiveCopy(t *testing.T) {
	fields := RequiredHealthFields()
	fields[0] = "hacked"
	if RequiredHealthFields()[0] == "hacked" {
		t.Error("RequiredHealthFields must return a defensive copy")
	}
}

func TestRequiredStateFields_ReturnsDefensiveCopy(t *testing.T) {
	fields := RequiredStateFields()
	fields[0] = "hacked"
	if RequiredStateFields()[0] == "hacked" {
		t.Error("RequiredStateFields must return a defensive copy")
	}
}

func TestRequiredWorkloadFields_ReturnsDefensiveCopy(t *testing.T) {
	fields := RequiredWorkloadFields()
	fields[0] = "hacked"
	if RequiredWorkloadFields()[0] == "hacked" {
		t.Error("RequiredWorkloadFields must return a defensive copy")
	}
}

func TestAllowedErrorClasses_AllPresent(t *testing.T) {
	expected := []ErrorClass{
		ErrInvalidArguments, ErrRequestCreateFailed, ErrConnectionFailed,
		ErrRequestTimeout, ErrResponseTooLarge, ErrUnexpectedHTTPStatus,
		ErrMalformedJSON, ErrUnknownJSONField, ErrMissingRequiredField,
		ErrTrailingJSON, ErrHealthNotReady, ErrStateInvalid,
		ErrWorkloadCountMismatch, ErrSchemaVersionMismatch, ErrInvalidOperation,
	}
	for _, ec := range expected {
		if !IsAllowedErrorClass(ec) {
			t.Errorf("error class %s not in AllowedErrorClasses", ec)
		}
	}
}

func TestRequiredEnvelopeFields_AllPresent(t *testing.T) {
	expected := []string{"schema_version", "operation", "success", "http_status"}
	fields := RequiredEnvelopeFields()
	if len(fields) != len(expected) {
		t.Fatalf("expected %d fields, got %d", len(expected), len(fields))
	}
	for i, f := range expected {
		if fields[i] != f {
			t.Errorf("field[%d]: expected %q, got %q", i, f, fields[i])
		}
	}
}

// ===== JSON encoding round-trip =====

func TestControlEnvelope_RoundTrip_Success(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: "growing"},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	decoded, err := DecodeEnvelopeExactlyOne(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !decoded.Success || decoded.Operation != "health" {
		t.Errorf("unexpected decoded: %+v", decoded)
	}
}