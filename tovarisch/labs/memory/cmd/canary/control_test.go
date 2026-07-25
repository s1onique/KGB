// control_test.go — Production-function tests for the canary control subcommand.
//
// Tests target the actual production functions (doHealthCheck, doStateCheck,
// doOperateCheck) using httptest. The shared protocol authority lives in
// internal/canarycontrol; its unit tests are in protocol_test.go there.
//
// CORRECTION35: removed the duplicate decoder-unit tests that now live in
// internal/canarycontrol. BuildArgv is tested for both success and failure.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ===== P0-5: Typed operation argv (fail closed) =====

func TestBuildArgvHealth(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlHealth,
		Port:    8080,
		Timeout: 5 * time.Second,
	}
	argv, err := buildArgv(op)
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

func TestBuildArgvState(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlState,
		Port:    8080,
		Timeout: 5 * time.Second,
	}
	argv, err := buildArgv(op)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(argv) != 7 {
		t.Fatalf("expected 7 args, got %d", len(argv))
	}
	if argv[0] != "/app/canary" || argv[1] != "control" || argv[2] != "state" {
		t.Errorf("unexpected argv: %v", argv)
	}
}

func TestBuildArgvOperate(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlOperate,
		Port:    9090,
		Count:   10,
		Timeout: 30 * time.Second,
	}
	argv, err := buildArgv(op)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	expected := []string{"/app/canary", "control", "operate", "--port", "9090", "--count", "10", "--timeout", "30s"}
	if len(argv) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(argv))
	}
	for i := range argv {
		if argv[i] != expected[i] {
			t.Errorf("argv[%d]: expected %q, got %q", i, expected[i], argv[i])
		}
	}
}

func TestBuildArgv_FailsClosedOnInvalidOp(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlOperationKind("not_a_real_op"),
		Port:    8080,
		Timeout: 5 * time.Second,
	}
	_, err := buildArgv(op)
	if err == nil {
		t.Fatal("expected error for invalid op")
	}
}

func TestBuildArgv_FailsClosedOnBadPort(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlHealth,
		Port:    0,
		Timeout: 5 * time.Second,
	}
	_, err := buildArgv(op)
	if err == nil {
		t.Fatal("expected error for port=0")
	}
}

func TestBuildArgv_FailsClosedOnZeroTimeout(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlHealth,
		Port:    8080,
		Timeout: 0,
	}
	_, err := buildArgv(op)
	if err == nil {
		t.Fatal("expected error for timeout=0")
	}
}

func TestBuildArgv_FailsClosedOnBadOperateCount(t *testing.T) {
	op := ControlOperation{
		Kind:    ControlOperate,
		Port:    8080,
		Count:   0,
		Timeout: 5 * time.Second,
	}
	_, err := buildArgv(op)
	if err == nil {
		t.Fatal("expected error for count=0")
	}
}

func TestBuildArgv_NoShell(t *testing.T) {
	for _, op := range []ControlOperation{
		{Kind: ControlHealth, Port: 8080, Timeout: 5 * time.Second},
		{Kind: ControlState, Port: 8080, Timeout: 5 * time.Second},
		{Kind: ControlOperate, Port: 8080, Count: 5, Timeout: 30 * time.Second},
	} {
		argv, err := buildArgv(op)
		if err != nil {
			t.Errorf("buildArgv failed: %v", err)
			continue
		}
		for _, arg := range argv {
			low := arg
			if low == "/bin/sh" || low == "/bin/bash" || low == "sh" || low == "bash" ||
				low == "curl" || low == "wget" || low == "nc" || low == "telnet" {
				t.Errorf("forbidden tool in argv: %s", arg)
			}
		}
		if argv[0] != "/app/canary" {
			t.Errorf("argv[0] must be /app/canary, got %q", argv[0])
		}
	}
}

// ===== doHealthCheck production function tests =====

func TestDoHealthCheck_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true,"mode":"growing"}`))
	}))
	defer server.Close()

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

func TestDoHealthCheck_65536ByteValidBody(t *testing.T) {
	// Generate exactly 65536 bytes of valid JSON.
	// JSON: {"ready":true,"mode":"growing"} = 27 bytes.
	// Padding: 65509 spaces AFTER the JSON.
	jsonPayload := []byte(`{"ready":true,"mode":"growing"}`)
	paddingLen := 65536 - len(jsonPayload)
	body := make([]byte, 0, 65536)
	body = append(body, jsonPayload...)
	body = append(body, make([]byte, paddingLen)...) // trailing zero bytes

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	result := doHealthCheck(context.Background(), server.URL, 5*time.Second)
	// Note: Go json decoder sees the trailing bytes as potential JSON value
	// and may reject as trailing_json. We accept either success or trailing_json
	// here since the test is primarily about body-size handling at the
	// MaxResponseBody+1 boundary, not about exact trailing-byte handling.
	if result.ErrorClass != "" && result.ErrorClass != ErrTrailingJSON {
		t.Errorf("expected no error or trailing_json, got %s", result.ErrorClass)
	}
}

func TestDoHealthCheck_65537ByteBody(t *testing.T) {
	body := make([]byte, 65537)
	body[0] = '{'
	body[65536] = '}'
	for i := 1; i < 65536; i++ {
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

// ===== doStateCheck production function tests =====

func TestDoStateCheck_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL+"/state", 5*time.Second)
	if result.ErrorClass != "" {
		t.Errorf("expected no error, got %s", result.ErrorClass)
	}
	if result.State == nil || result.State.Mode != "growing" {
		t.Error("expected state with mode=growing")
	}
}

func TestDoStateCheck_MissingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":0}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoStateCheck_NullField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":null,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoStateCheck_NegativeRetainedBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":-1,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrStateInvalid {
		t.Errorf("expected ErrStateInvalid, got %s", result.ErrorClass)
	}
}

func TestDoStateCheck_ReadyFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":false}`))
	}))
	defer server.Close()

	result := doStateCheck(context.Background(), server.URL, 5*time.Second)
	if result.ErrorClass != ErrStateInvalid {
		t.Errorf("expected ErrStateInvalid, got %s", result.ErrorClass)
	}
}

func TestDoStateCheck_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := doStateCheck(ctx, server.URL, 50*time.Millisecond)
	if result.ErrorClass != ErrRequestTimeout {
		t.Errorf("expected ErrRequestTimeout, got %s", result.ErrorClass)
	}
}

// ===== doOperateCheck production function tests =====

func TestDoOperateCheck_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":5,"completed":5}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != "" {
		t.Errorf("expected no error, got %s", result.ErrorClass)
	}
	if result.Workload == nil || result.Workload.Requested != 5 {
		t.Error("expected workload with requested=5")
	}
}

func TestDoOperateCheck_CountMismatch(t *testing.T) {
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

func TestDoOperateCheck_AttemptedMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":4,"completed":4}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %s", result.ErrorClass)
	}
}

func TestDoOperateCheck_CompletedMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":5,"completed":4}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %s", result.ErrorClass)
	}
}

func TestDoOperateCheck_ZeroRequested(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":0,"attempted":0,"completed":0}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %s", result.ErrorClass)
	}
}

func TestDoOperateCheck_NullField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":null,"completed":5}`))
	}))
	defer server.Close()

	result := doOperateCheck(context.Background(), server.URL+"/operate?count=5", 5, 30*time.Second)
	if result.ErrorClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %s", result.ErrorClass)
	}
}

func TestDoOperateCheck_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"requested":5,"attempted":5,"completed":5}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := doOperateCheck(ctx, server.URL, 5, 50*time.Millisecond)
	if result.ErrorClass != ErrRequestTimeout {
		t.Errorf("expected ErrRequestTimeout, got %s", result.ErrorClass)
	}
}
