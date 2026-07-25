// Package canarycontrol is the shared control-protocol authority for KGB memory lab canaries.
//
// It defines the immutable protocol vocabulary used by BOTH the in-container canary
// (cmd/canary) and the host-side Docker controller (internal/dockerlab). Neither
// consumer is permitted to retain a private copy of any of these definitions.
//
// Authority references:
//   - SchemaVersion:           canonical wire format version
//   - ErrorClass:              stable error classification vocabulary
//   - Operation:               typed control operation names
//   - DecodeEnvelopeExactlyOne:strict serialized envelope decoder
//   - ValidateControlEnvelope: discriminated success/failure validator
//   - IsRetryable:             retry-classification authority
//
// CORRECTION34: extracted from cmd/canary to a non-main package so both consumers
// can compile against one shared authority.
package canarycontrol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// SchemaVersion is the canonical wire-format version for canary control envelopes.
const SchemaVersion = "canary-control/v1"

// MaxResponseBody is the canonical bounded response body size.
// 64 KiB is the hard cap for any control response; exceeding it must produce
// ErrResponseTooLarge.
const MaxResponseBody = 64 * 1024

// ControlDialTimeout is the canonical TCP dial timeout for control requests.
const ControlDialTimeout = 5 * time.Second

// ControlResponseHeaderTimeout is the canonical response-header read timeout.
const ControlResponseHeaderTimeout = 5 * time.Second

// Operation is the typed control operation name.
type Operation string

const (
	OpHealth  Operation = "health"
	OpState   Operation = "state"
	OpOperate Operation = "operate"
)

// validOperations is the closed set of permitted operation names.
var validOperations = map[Operation]bool{
	OpHealth:  true,
	OpState:   true,
	OpOperate: true,
}

// IsValidOperation reports whether op is a permitted operation name.
func IsValidOperation(op Operation) bool {
	return validOperations[op]
}

// ErrorClass is the stable classification vocabulary for control-protocol errors.
//
// Each ErrorClass is the single source of truth for a classification.
// Consumers MUST NOT introduce parallel vocabularies.
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
	ErrSchemaVersionMismatch ErrorClass = "schema_version_mismatch"
	ErrInvalidOperation      ErrorClass = "invalid_operation"
)

// AllowedErrorClasses is the closed vocabulary of valid error classes.
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
	ErrSchemaVersionMismatch: true,
	ErrInvalidOperation:      true,
}

// IsAllowedErrorClass reports whether ec is in the closed vocabulary.
func IsAllowedErrorClass(ec ErrorClass) bool {
	return AllowedErrorClasses[ec]
}

// ProtocolError is a typed control-protocol error with classification.
type ProtocolError struct {
	ErrClass ErrorClass
	Message  string
}

func (e *ProtocolError) Error() string {
	return e.Message
}

// IsProtocolError reports whether err is a *ProtocolError.
func IsProtocolError(err error) bool {
	var pe *ProtocolError
	return errors.As(err, &pe)
}

// AsProtocolError unwraps err to a *ProtocolError if possible.
func AsProtocolError(err error) (*ProtocolError, bool) {
	var pe *ProtocolError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}

// retryableErrorClasses is the closed vocabulary of retryable error classes.
var retryableErrorClasses = map[ErrorClass]bool{
	ErrConnectionFailed: true,
	ErrRequestTimeout:   true,
	ErrHealthNotReady:   true,
}

// IsRetryable reports whether err represents a transient failure that should be
// retried by the readiness loop. Returns false for nil err.
//
// Retryable errors:
//   - ErrConnectionFailed
//   - ErrRequestTimeout
//   - ErrHealthNotReady
//
// All other errors are protocol violations or argument defects and MUST stop
// the readiness loop immediately.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if pe, ok := AsProtocolError(err); ok {
		return retryableErrorClasses[pe.ErrClass]
	}
	return false
}

// HealthPayload represents the canary /health response.
type HealthPayload struct {
	Ready bool   `json:"ready"`
	Mode  string `json:"mode"` // required, non-empty
}

// StatePayload represents the canary /state response.
type StatePayload struct {
	Mode           string `json:"mode"`            // required, non-empty
	RetainedBlocks int    `json:"retained_blocks"` // required, >= 0
	RetainedBytes  int64  `json:"retained_bytes"`  // required, >= 0
	OperationCount int64  `json:"operation_count"` // required, >= 0
	FDCount        int    `json:"fd_count"`        // required, >= 0
	Ready          bool   `json:"ready"`           // required
}

// WorkloadPayload represents the canary /operate response.
type WorkloadPayload struct {
	Requested int `json:"requested"` // required, > 0
	Attempted int `json:"attempted"` // required, == Requested
	Completed int `json:"completed"` // required, == Attempted
}

// ControlEnvelope is the canonical wire envelope.
//
// Each command emits exactly one JSON envelope. The envelope is a discriminated
// union on Success. Success variants carry exactly one payload (typed by
// Operation); failure variants carry exactly one ErrorClass.
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

// RequiredEnvelopeFields are the four envelope members required on every wire document.
var RequiredEnvelopeFields = []string{
	"schema_version",
	"operation",
	"success",
	"http_status",
}

// RequiredHealthFields are the fields required on every HealthPayload.
var RequiredHealthFields = []string{"ready", "mode"}

// RequiredStateFields are the fields required on every StatePayload.
var RequiredStateFields = []string{
	"mode",
	"retained_blocks",
	"retained_bytes",
	"operation_count",
	"fd_count",
	"ready",
}

// RequiredWorkloadFields are the fields required on every WorkloadPayload.
var RequiredWorkloadFields = []string{"requested", "attempted", "completed"}

// decodeExactJSONObject is the shared strict exact-object decoder.
//
// Required sequence:
//  1. reject empty input
//  2. decode exactly one json.RawMessage
//  3. require the next decode to return io.EOF
//  4. reject a second JSON value
//  5. reject malformed trailing bytes
//  6. decode the isolated value into map[string]json.RawMessage
//  7. require a JSON object
//  8. reject missing required members
//  9. reject explicit null required members
//  10. strictly decode into the typed destination with DisallowUnknownFields
func decodeExactJSONObject(data []byte, requiredFields []string, target any) error {
	// 1. reject empty input
	if len(bytes.TrimSpace(data)) == 0 {
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: "empty body"}
	}

	// 2-5. decode exactly one JSON value, require EOF, reject trailing
	dec := json.NewDecoder(bytes.NewReader(data))

	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: err.Error()}
	}

	// 3-4. require the next decode to return io.EOF, reject second value
	var extra json.RawMessage
	decodeErr := dec.Decode(&extra)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		return &ProtocolError{ErrClass: ErrTrailingJSON, Message: "trailing data"}
	}
	if decodeErr == nil && len(extra) > 0 {
		return &ProtocolError{ErrClass: ErrTrailingJSON, Message: "second JSON value"}
	}

	// 6-7. decode the isolated value into map, require a JSON object
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: "not a JSON object"}
	}
	if rawMap == nil {
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: "explicit null object"}
	}

	// 8-9. reject missing or explicit-null required members
	for _, field := range requiredFields {
		v, ok := rawMap[field]
		if !ok {
			return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "missing " + field}
		}
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "null " + field}
		}
	}

	// 10. strictly decode into the typed destination with DisallowUnknownFields
	strictDec := json.NewDecoder(bytes.NewReader(raw))
	strictDec.DisallowUnknownFields()
	if err := strictDec.Decode(target); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return &ProtocolError{ErrClass: ErrUnknownJSONField, Message: err.Error()}
		}
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: err.Error()}
	}

	return nil
}

// DecodeEnvelopeExactlyOne is the canonical envelope decoder.
//
// It enforces:
//  1. reject empty input
//  2. decode exactly one JSON object
//  3. reject a second JSON value
//  4. reject malformed trailing bytes
//  5. reject unknown members
//  6. require schema_version, operation, success, http_status
//  7. reject explicit null for any required member
//  8. reject empty schema_version or operation
//  9. validate the discriminated success/failure variant via ValidateControlEnvelope
//
// Consumers MUST use this function — not encoding/json directly.
func DecodeEnvelopeExactlyOne(data []byte) (*ControlEnvelope, error) {
	var env ControlEnvelope
	if err := decodeExactJSONObject(data, RequiredEnvelopeFields, &env); err != nil {
		return nil, err
	}
	if err := ValidateControlEnvelope(&env); err != nil {
		return nil, err
	}
	return &env, nil
}

// DecodeHealth decodes a /health response body into a HealthPayload.
func DecodeHealth(data []byte) (*HealthPayload, error) {
	var h HealthPayload
	if err := decodeExactJSONObject(data, RequiredHealthFields, &h); err != nil {
		return nil, err
	}
	if !h.Ready {
		return nil, &ProtocolError{ErrClass: ErrHealthNotReady, Message: "ready=false"}
	}
	if h.Mode == "" {
		return nil, &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "empty mode"}
	}
	return &h, nil
}

// DecodeState decodes a /state response body into a StatePayload.
//
// All physical fields and semantic invariants are enforced.
func DecodeState(data []byte) (*StatePayload, error) {
	var s StatePayload
	if err := decodeExactJSONObject(data, RequiredStateFields, &s); err != nil {
		return nil, err
	}
	if err := ValidateStateSemantics(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ValidateStateSemantics enforces the StatePayload semantic invariants:
//   - mode non-empty
//   - retained_blocks >= 0
//   - retained_bytes >= 0
//   - operation_count >= 0
//   - fd_count >= 0
//   - ready == true for qualification
func ValidateStateSemantics(s *StatePayload) error {
	if s.Mode == "" {
		return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "empty mode"}
	}
	if s.RetainedBlocks < 0 {
		return &ProtocolError{ErrClass: ErrStateInvalid, Message: "negative retained_blocks"}
	}
	if s.RetainedBytes < 0 {
		return &ProtocolError{ErrClass: ErrStateInvalid, Message: "negative retained_bytes"}
	}
	if s.OperationCount < 0 {
		return &ProtocolError{ErrClass: ErrStateInvalid, Message: "negative operation_count"}
	}
	if s.FDCount < 0 {
		return &ProtocolError{ErrClass: ErrStateInvalid, Message: "negative fd_count"}
	}
	if !s.Ready {
		return &ProtocolError{ErrClass: ErrStateInvalid, Message: "ready=false"}
	}
	return nil
}

// DecodeWorkload decodes a /operate response body into a WorkloadPayload,
// verifying that requested, attempted, and completed are all > 0 and equal
// to the supplied expected count.
func DecodeWorkload(data []byte, expectedRequest int) (*WorkloadPayload, error) {
	var w WorkloadPayload
	if err := decodeExactJSONObject(data, RequiredWorkloadFields, &w); err != nil {
		return nil, err
	}
	if err := ValidateWorkloadSemantics(&w, expectedRequest); err != nil {
		return nil, err
	}
	return &w, nil
}

// ValidateWorkloadSemantics enforces WorkloadPayload invariants:
//   - requested > 0
//   - requested == expectedRequest (caller-supplied request)
//   - attempted == requested
//   - completed == attempted
func ValidateWorkloadSemantics(w *WorkloadPayload, expectedRequest int) error {
	if w.Requested <= 0 {
		return &ProtocolError{ErrClass: ErrWorkloadCountMismatch, Message: "non-positive requested"}
	}
	if expectedRequest > 0 && w.Requested != expectedRequest {
		return &ProtocolError{
			ErrClass: ErrWorkloadCountMismatch,
			Message:  fmt.Sprintf("requested=%d != expected=%d", w.Requested, expectedRequest),
		}
	}
	if w.Attempted != w.Requested {
		return &ProtocolError{
			ErrClass: ErrWorkloadCountMismatch,
			Message:  fmt.Sprintf("attempted=%d != requested=%d", w.Attempted, w.Requested),
		}
	}
	if w.Completed != w.Attempted {
		return &ProtocolError{
			ErrClass: ErrWorkloadCountMismatch,
			Message:  fmt.Sprintf("completed=%d != attempted=%d", w.Completed, w.Attempted),
		}
	}
	return nil
}

// ValidateControlEnvelope validates the discriminated success/failure variant.
//
// Success invariants:
//   - http_status == 200
//   - error_class absent
//   - exactly one payload, payload type matches operation
//
// Failure invariants:
//   - http_status == 0 or non-2xx
//   - error_class present and in AllowedErrorClasses
//   - no payloads
//
// Errors return *ProtocolError.
func ValidateControlEnvelope(env *ControlEnvelope) error {
	// schema_version
	if env.SchemaVersion == "" {
		return &ProtocolError{ErrClass: ErrSchemaVersionMismatch, Message: "empty schema_version"}
	}
	if env.SchemaVersion != SchemaVersion {
		return &ProtocolError{
			ErrClass: ErrSchemaVersionMismatch,
			Message:  "schema_version mismatch: " + env.SchemaVersion,
		}
	}

	// operation
	if env.Operation == "" {
		return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "missing operation"}
	}
	op := Operation(env.Operation)
	if !IsValidOperation(op) {
		return &ProtocolError{
			ErrClass: ErrInvalidOperation,
			Message:  "invalid operation: " + env.Operation,
		}
	}

	// discriminated variant
	if env.Success {
		return validateSuccessEnvelope(env, op)
	}
	return validateFailureEnvelope(env)
}

func validateSuccessEnvelope(env *ControlEnvelope, op Operation) error {
	if env.HTTPStatus != 200 {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  fmt.Sprintf("success envelope must have HTTP 200, got %d", env.HTTPStatus),
		}
	}
	if env.ErrorClass != "" {
		return &ProtocolError{
			ErrClass: ErrInvalidArguments,
			Message:  "success envelope must not have error_class",
		}
	}
	// exactly one payload, type matches operation
	switch op {
	case OpHealth:
		if env.Health == nil {
			return &ProtocolError{
				ErrClass: ErrMissingRequiredField,
				Message:  "health operation requires health payload",
			}
		}
		if env.State != nil || env.Workload != nil {
			return &ProtocolError{
				ErrClass: ErrInvalidArguments,
				Message:  "health operation must not have state or workload",
			}
		}
	case OpState:
		if env.State == nil {
			return &ProtocolError{
				ErrClass: ErrMissingRequiredField,
				Message:  "state operation requires state payload",
			}
		}
		if env.Health != nil || env.Workload != nil {
			return &ProtocolError{
				ErrClass: ErrInvalidArguments,
				Message:  "state operation must not have health or workload",
			}
		}
	case OpOperate:
		if env.Workload == nil {
			return &ProtocolError{
				ErrClass: ErrMissingRequiredField,
				Message:  "operate operation requires workload payload",
			}
		}
		if env.Health != nil || env.State != nil {
			return &ProtocolError{
				ErrClass: ErrInvalidArguments,
				Message:  "operate operation must not have health or state",
			}
		}
	}
	return nil
}

func validateFailureEnvelope(env *ControlEnvelope) error {
	if env.HTTPStatus == 200 {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  "failure envelope must not have HTTP 200",
		}
	}
	if env.HTTPStatus != 0 && (env.HTTPStatus < 200 || env.HTTPStatus >= 300) {
		// non-2xx is fine for failure
	} else if env.HTTPStatus == 0 {
		// local failure (e.g., connection failure) may have http_status == 0
	} else {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  fmt.Sprintf("failure envelope must not have 2xx HTTP status, got %d", env.HTTPStatus),
		}
	}
	if env.ErrorClass == "" {
		return &ProtocolError{
			ErrClass: ErrMissingRequiredField,
			Message:  "failure envelope must have error_class",
		}
	}
	if !IsAllowedErrorClass(env.ErrorClass) {
		return &ProtocolError{
			ErrClass: ErrInvalidArguments,
			Message:  "unknown error class: " + string(env.ErrorClass),
		}
	}
	if env.Health != nil || env.State != nil || env.Workload != nil {
		return &ProtocolError{
			ErrClass: ErrInvalidArguments,
			Message:  "failure envelope must not have payloads",
		}
	}
	return nil
}

// DecodeExactJSONObjectForTest exposes the strict exact-object decoder
// for the legacy in-package tests in cmd/canary/control_test.go.
//
// This is a thin wrapper around the unexported decodeExactJSONObject so
// that the canonical decoder is still owned by the shared package but
// remains accessible to existing tests that target the canary binary
// directly. New tests SHOULD target the exported DecodeEnvelopeExactlyOne,
// DecodeHealth, DecodeState, and DecodeWorkload helpers instead.
func DecodeExactJSONObjectForTest(data []byte, requiredFields []string, target any) error {
	return decodeExactJSONObject(data, requiredFields, target)
}
