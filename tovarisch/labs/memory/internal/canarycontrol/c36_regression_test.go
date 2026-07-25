// c36_regression_test.go — Direct struct-validation regression tests for S36.
//
// These tests exercise the struct-validator path (ValidateControlEnvelope
// called directly on a struct value) rather than the serialized decoder.
// Every invalid case must produce a stable *ProtocolError with the
// correct ErrClass so equivalence with DecodeEnvelopeExactlyOne holds.
//
// CORRECTION37 P0-1.

package canarycontrol

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ===== Health (struct validation) =====

func TestValidateEnvelope_Health_ReadyFalse_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: false, Mode: "growing"},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for ready=false")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrHealthNotReady {
		t.Errorf("expected ErrHealthNotReady, got %v", err)
	}
}

func TestValidateEnvelope_Health_EmptyMode_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: ""},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for empty mode")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %v", err)
	}
}

// ===== State (struct validation) =====

func TestValidateEnvelope_State_EmptyMode_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "state",
		Success:       true,
		HTTPStatus:    200,
		State:         &StatePayload{Mode: "", Ready: true},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for empty mode")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrMissingRequiredField {
		t.Errorf("expected ErrMissingRequiredField, got %v", err)
	}
}

func TestValidateEnvelope_State_EachNegativeCounter_Rejected(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*StatePayload)
	}{
		{"retained_blocks=-1", func(s *StatePayload) { s.RetainedBlocks = -1 }},
		{"retained_bytes=-1", func(s *StatePayload) { s.RetainedBytes = -1 }},
		{"operation_count=-1", func(s *StatePayload) { s.OperationCount = -1 }},
		{"fd_count=-1", func(s *StatePayload) { s.FDCount = -1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &StatePayload{Mode: "growing", Ready: true}
			c.mutate(s)
			env := &ControlEnvelope{
				SchemaVersion: SchemaVersion,
				Operation:     "state",
				Success:       true,
				HTTPStatus:    200,
				State:         s,
			}
			err := ValidateControlEnvelope(env)
			if err == nil {
				t.Fatal("expected error")
			}
			pe, ok := AsProtocolError(err)
			if !ok || pe.ErrClass != ErrStateInvalid {
				t.Errorf("expected ErrStateInvalid, got %v", err)
			}
		})
	}
}

func TestValidateEnvelope_State_ReadyFalse_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "state",
		Success:       true,
		HTTPStatus:    200,
		State:         &StatePayload{Mode: "growing", Ready: false},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for ready=false")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrStateInvalid {
		t.Errorf("expected ErrStateInvalid, got %v", err)
	}
}

// ===== Workload (struct validation) =====

func TestValidateEnvelope_Workload_NonPositiveRequested_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "operate",
		Success:       true,
		HTTPStatus:    200,
		Workload:      &WorkloadPayload{Requested: 0, Attempted: 0, Completed: 0},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for non-positive requested")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %v", err)
	}
}

func TestValidateEnvelope_Workload_AttemptedMismatch_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "operate",
		Success:       true,
		HTTPStatus:    200,
		Workload:      &WorkloadPayload{Requested: 5, Attempted: 4, Completed: 4},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for attempted mismatch")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %v", err)
	}
}

func TestValidateEnvelope_Workload_CompletedMismatch_Rejected(t *testing.T) {
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "operate",
		Success:       true,
		HTTPStatus:    200,
		Workload:      &WorkloadPayload{Requested: 5, Attempted: 5, Completed: 4},
	}
	err := ValidateControlEnvelope(env)
	if err == nil {
		t.Fatal("expected error for completed mismatch")
	}
	pe, ok := AsProtocolError(err)
	if !ok || pe.ErrClass != ErrWorkloadCountMismatch {
		t.Errorf("expected ErrWorkloadCountMismatch, got %v", err)
	}
}

// ===== Equivalence: serialized decoder vs struct validator =====

func TestEquivalence_ValidHealthEnvelope(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	// Serialized decode:
	decoded, err := DecodeEnvelopeExactlyOne(body)
	if err != nil {
		t.Fatalf("serialized decode failed: %v", err)
	}
	// Struct validation: should also pass
	if err := ValidateControlEnvelope(decoded); err != nil {
		t.Errorf("struct validation rejected valid serialized envelope: %v", err)
	}
}

func TestEquivalence_InvalidNestedHealthRejectedByBoth(t *testing.T) {
	// null mode
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":null}}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("serialized decode should fail")
	}
	// Build same struct value
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "health",
		Success:       true,
		HTTPStatus:    200,
		Health:        &HealthPayload{Ready: true, Mode: ""}, // empty mode
	}
	pe1, _ := AsProtocolError(err)
	if err2 := ValidateControlEnvelope(env); err2 == nil {
		t.Fatal("struct validation should fail")
	} else {
		pe2, _ := AsProtocolError(err2)
		if pe1 != nil && pe2 != nil && pe1.ErrClass != pe2.ErrClass {
			t.Errorf("equivalence violated: serialized=%s struct=%s", pe1.ErrClass, pe2.ErrClass)
		}
	}
}

func TestEquivalence_InvalidWorkloadRejectedByBoth(t *testing.T) {
	body := []byte(`{"schema_version":"` + SchemaVersion + `","operation":"operate","success":true,"http_status":200,"workload":{"requested":5,"attempted":3,"completed":3}}`)
	_, err := DecodeEnvelopeExactlyOne(body)
	if err == nil {
		t.Fatal("serialized decode should fail")
	}
	env := &ControlEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     "operate",
		Success:       true,
		HTTPStatus:    200,
		Workload:      &WorkloadPayload{Requested: 5, Attempted: 3, Completed: 3},
	}
	err2 := ValidateControlEnvelope(env)
	if err2 == nil {
		t.Fatal("struct validation should fail")
	}
	pe1, _ := AsProtocolError(err)
	pe2, _ := AsProtocolError(err2)
	if pe1 != nil && pe2 != nil && pe1.ErrClass != pe2.ErrClass {
		t.Errorf("equivalence violated: serialized=%s struct=%s", pe1.ErrClass, pe2.ErrClass)
	}
}

// ===== Deterministic enumeration =====

func TestAllOperations_Deterministic(t *testing.T) {
	first := AllOperations()
	for i := 0; i < 100; i++ {
		got := AllOperations()
		if len(got) != len(first) {
			t.Fatalf("length drift: %d vs %d", len(got), len(first))
		}
		for j := range first {
			if got[j] != first[j] {
				t.Errorf("position %d: %q vs %q", j, got[j], first[j])
			}
		}
	}
}

func TestAllErrorClasses_Deterministic(t *testing.T) {
	first := AllErrorClasses()
	for i := 0; i < 100; i++ {
		got := AllErrorClasses()
		if len(got) != len(first) {
			t.Fatalf("length drift: %d vs %d", len(got), len(first))
		}
		for j := range first {
			if got[j] != first[j] {
				t.Errorf("position %d: %q vs %q", j, got[j], first[j])
			}
		}
	}
}

func TestAllOperations_Sorted(t *testing.T) {
	ops := AllOperations()
	for i := 1; i < len(ops); i++ {
		if ops[i-1] > ops[i] {
			t.Errorf("not sorted at %d: %q > %q", i, ops[i-1], ops[i])
		}
	}
}

func TestAllErrorClasses_Sorted(t *testing.T) {
	classes := AllErrorClasses()
	for i := 1; i < len(classes); i++ {
		if classes[i-1] > classes[i] {
			t.Errorf("not sorted at %d: %q > %q", i, classes[i-1], classes[i])
		}
	}
}

// ===== Count invariants (Validate + BuildArgv) =====

func TestCount_Health_Zero_Accepted(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 8080, Count: 0, Timeout: 5 * 1e9}
	if err := op.Validate(); err != nil {
		t.Errorf("expected accept, got %v", err)
	}
	if _, err := op.BuildArgv(); err != nil {
		t.Errorf("expected BuildArgv success, got %v", err)
	}
}

func TestCount_Health_Positive_Rejected(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 8080, Count: 5, Timeout: 5 * 1e9}
	if err := op.Validate(); err == nil {
		t.Error("expected reject for health count=5")
	}
	if _, err := op.BuildArgv(); err == nil {
		t.Error("expected BuildArgv failure for health count=5")
	}
}

func TestCount_Health_Negative_Rejected(t *testing.T) {
	op := ControlOperation{Kind: OpHealth, Port: 8080, Count: -1, Timeout: 5 * 1e9}
	if err := op.Validate(); err == nil {
		t.Error("expected reject for health count=-1")
	}
	if _, err := op.BuildArgv(); err == nil {
		t.Error("expected BuildArgv failure for health count=-1")
	}
}

func TestCount_State_Zero_Accepted(t *testing.T) {
	op := ControlOperation{Kind: OpState, Port: 8080, Count: 0, Timeout: 5 * 1e9}
	if err := op.Validate(); err != nil {
		t.Errorf("expected accept, got %v", err)
	}
	if _, err := op.BuildArgv(); err != nil {
		t.Errorf("expected BuildArgv success, got %v", err)
	}
}

func TestCount_State_Positive_Rejected(t *testing.T) {
	op := ControlOperation{Kind: OpState, Port: 8080, Count: 5, Timeout: 5 * 1e9}
	if err := op.Validate(); err == nil {
		t.Error("expected reject for state count=5")
	}
	if _, err := op.BuildArgv(); err == nil {
		t.Error("expected BuildArgv failure for state count=5")
	}
}

func TestCount_State_Negative_Rejected(t *testing.T) {
	op := ControlOperation{Kind: OpState, Port: 8080, Count: -1, Timeout: 5 * 1e9}
	if err := op.Validate(); err == nil {
		t.Error("expected reject for state count=-1")
	}
}

func TestCount_Operate_Positive_Accepted(t *testing.T) {
	op := ControlOperation{Kind: OpOperate, Port: 8080, Count: 5, Timeout: 5 * 1e9}
	if err := op.Validate(); err != nil {
		t.Errorf("expected accept, got %v", err)
	}
	if _, err := op.BuildArgv(); err != nil {
		t.Errorf("expected BuildArgv success, got %v", err)
	}
}

func TestCount_Operate_Zero_Rejected(t *testing.T) {
	op := ControlOperation{Kind: OpOperate, Port: 8080, Count: 0, Timeout: 5 * 1e9}
	if err := op.Validate(); err == nil {
		t.Error("expected reject for operate count=0")
	}
	if _, err := op.BuildArgv(); err == nil {
		t.Error("expected BuildArgv failure for operate count=0")
	}
}

func TestCount_Operate_Negative_Rejected(t *testing.T) {
	op := ControlOperation{Kind: OpOperate, Port: 8080, Count: -1, Timeout: 5 * 1e9}
	if err := op.Validate(); err == nil {
		t.Error("expected reject for operate count=-1")
	}
}

// ===== P0-2 boundary fixtures =====

func TestDecodeHealth_Exact65536ByteValidBody_Success(t *testing.T) {
	jsonPayload := []byte(`{"ready":true,"mode":"growing"}`)
	padding := bytes.Repeat([]byte{' '}, MaxResponseBody-len(jsonPayload))
	body := append(jsonPayload, padding...)
	if len(body) != MaxResponseBody {
		t.Fatalf("expected %d bytes, got %d", MaxResponseBody, len(body))
	}
	h, err := DecodeHealth(body)
	if err != nil {
		t.Fatalf("expected success for %d-byte body, got %v", MaxResponseBody, err)
	}
	if !h.Ready || h.Mode != "growing" {
		t.Errorf("unexpected payload: %+v", h)
	}
}

func TestDoHealthCheck_65536ByteValidBody_Success(t *testing.T) {
	jsonPayload := []byte(`{"ready":true,"mode":"growing"}`)
	padding := bytes.Repeat([]byte{' '}, MaxResponseBody-len(jsonPayload))
	body := append(jsonPayload, padding...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("client get failed: %v", err)
	}
	defer resp.Body.Close()
	recv, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if len(recv) != MaxResponseBody {
		t.Fatalf("expected %d bytes, got %d", MaxResponseBody, len(recv))
	}
	h, err := DecodeHealth(recv)
	if err != nil {
		t.Fatalf("expected success for exactly %d-byte body, got %v", MaxResponseBody, err)
	}
	if !h.Ready {
		t.Errorf("expected Ready=true, got false")
	}
}

func TestDoHealthCheck_65537ByteBody_Rejection(t *testing.T) {
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

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("client get failed: %v", err)
	}
	defer resp.Body.Close()
	recv, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if len(recv) <= MaxResponseBody {
		t.Fatalf("expected >%d bytes, got %d", MaxResponseBody, len(recv))
	}
	// Receiver should reject over-size body
	if _, err := DecodeHealth(recv); err == nil {
		t.Error("expected error for over-size body")
	}
}

// readBody reads up to MaxResponseBody+1 bytes and returns them.
func readBody(resp *http.Response) ([]byte, error) {
	const limit = MaxResponseBody + 1
	buf := make([]byte, limit)
	n := 0
	for n < limit {
		k, err := resp.Body.Read(buf[n:])
		n += k
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return buf[:n], err
		}
	}
	return buf[:n], nil
}

// ===== Null matrix via map[string]any =====

func buildHealthBody(omit, setNull string) []byte {
	m := map[string]any{
		"ready": true,
		"mode":  "growing",
	}
	if omit != "" {
		delete(m, omit)
	}
	if setNull != "" {
		m[setNull] = nil
	}
	out, _ := json.Marshal(m)
	return out
}

func TestNullMatrix_Health_Ready(t *testing.T) {
	body := buildHealthBody("", "ready")
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for null ready")
	}
}

func TestNullMatrix_Health_Mode(t *testing.T) {
	body := buildHealthBody("", "mode")
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for null mode")
	}
}

func TestMissingMatrix_Health_Ready(t *testing.T) {
	body := buildHealthBody("ready", "")
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for missing ready")
	}
}

func TestMissingMatrix_Health_Mode(t *testing.T) {
	body := buildHealthBody("mode", "")
	_, err := DecodeHealth(body)
	if err == nil {
		t.Fatal("expected error for missing mode")
	}
}

func buildStateBody(omit, setNull string) []byte {
	m := map[string]any{
		"mode":            "growing",
		"retained_blocks": 0,
		"retained_bytes":  0,
		"operation_count": 0,
		"fd_count":        0,
		"ready":           true,
	}
	if omit != "" {
		delete(m, omit)
	}
	if setNull != "" {
		m[setNull] = nil
	}
	out, _ := json.Marshal(m)
	return out
}

func TestNullMatrix_State(t *testing.T) {
	for _, f := range []string{"mode", "retained_blocks", "retained_bytes", "operation_count", "fd_count", "ready"} {
		body := buildStateBody("", f)
		_, err := DecodeState(body)
		if err == nil {
			t.Errorf("expected error for null %s", f)
		}
	}
}

func TestMissingMatrix_State(t *testing.T) {
	for _, f := range []string{"mode", "retained_blocks", "retained_bytes", "operation_count", "fd_count", "ready"} {
		body := buildStateBody(f, "")
		_, err := DecodeState(body)
		if err == nil {
			t.Errorf("expected error for missing %s", f)
		}
	}
}

func buildWorkloadBody(omit, setNull string) []byte {
	m := map[string]any{
		"requested": 5,
		"attempted": 5,
		"completed": 5,
	}
	if omit != "" {
		delete(m, omit)
	}
	if setNull != "" {
		m[setNull] = nil
	}
	out, _ := json.Marshal(m)
	return out
}

func TestNullMatrix_Workload(t *testing.T) {
	for _, f := range []string{"requested", "attempted", "completed"} {
		body := buildWorkloadBody("", f)
		_, err := DecodeWorkload(body, 5)
		if err == nil {
			t.Errorf("expected error for null %s", f)
		}
	}
}

func TestMissingMatrix_Workload(t *testing.T) {
	for _, f := range []string{"requested", "attempted", "completed"} {
		body := buildWorkloadBody(f, "")
		_, err := DecodeWorkload(body, 5)
		if err == nil {
			t.Errorf("expected error for missing %s", f)
		}
	}
}

func buildEnvelopeBody(omit, setNull string) []byte {
	m := map[string]any{
		"schema_version": SchemaVersion,
		"operation":      "health",
		"success":        true,
		"http_status":    200,
		"health":         map[string]any{"ready": true, "mode": "growing"},
	}
	if omit != "" {
		delete(m, omit)
	}
	if setNull != "" {
		m[setNull] = nil
	}
	out, _ := json.Marshal(m)
	return out
}

func TestNullMatrix_Envelope(t *testing.T) {
	for _, f := range []string{"schema_version", "operation", "success", "http_status"} {
		body := buildEnvelopeBody("", f)
		_, err := DecodeEnvelopeExactlyOne(body)
		if err == nil {
			t.Errorf("expected error for null %s", f)
		}
	}
}

func TestMissingMatrix_Envelope(t *testing.T) {
	for _, f := range []string{"schema_version", "operation", "success", "http_status"} {
		body := buildEnvelopeBody(f, "")
		_, err := DecodeEnvelopeExactlyOne(body)
		if err == nil {
			t.Errorf("expected error for missing %s", f)
		}
	}
}

// ===== Fixtures are valid JSON (not malformed) =====

func TestFixtures_AreValidJSON(t *testing.T) {
	for _, body := range [][]byte{
		buildHealthBody("", "mode"),
		buildStateBody("", "ready"),
		buildWorkloadBody("", "completed"),
		buildEnvelopeBody("", "operation"),
	} {
		if !json.Valid(body) {
			t.Errorf("fixture is not valid JSON: %s", string(body))
		}
		// Each must contain null
		if !strings.Contains(string(body), "null") {
			t.Errorf("fixture should contain null: %s", string(body))
		}
	}
}