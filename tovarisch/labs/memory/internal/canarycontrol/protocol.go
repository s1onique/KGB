// Package canarycontrol is the shared control-protocol authority for KGB memory lab canaries.
//
// It defines the immutable protocol vocabulary used by BOTH the in-container canary
// (cmd/canary) and the host-side Docker controller (internal/dockerlab). Neither
// consumer is permitted to retain a private copy of any of these definitions.
//
// All mutable collections are kept private. Only read-only accessor functions and
// defensive-copy enumeration helpers are exported.
//
// Authority references:
//   - SchemaVersion:           canonical wire format version
//   - ErrorClass:              stable error classification vocabulary
//   - Operation:               typed control operation names
//   - DecodeEnvelopeExactlyOne:strict serialized envelope decoder (with nested payload validation)
//   - ValidateControlEnvelope: discriminated success/failure validator
//   - IsRetryable:             retry-classification authority
//
// CORRECTION35: shared collections are now private; nested envelope payload
// validation is enforced; BuildArgv fails closed.
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
// It is private and exposed only through read-only accessors.
var validOperations = map[Operation]struct{}{
	OpHealth:  {},
	OpState:   {},
	OpOperate: {},
}

// IsValidOperation reports whether op is a permitted operation name.
func IsValidOperation(op Operation) bool {
	_, ok := validOperations[op]
	return ok
}

// AllOperations returns a defensive copy of the valid-operation set.
// Callers cannot mutate the package-level authority.
func AllOperations() []Operation {
	out := make([]Operation, 0, len(validOperations))
	for op := range validOperations {
		out = append(out, op)
	}
	return out
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

// allowedErrorClasses is the closed vocabulary of valid error classes.
// It is private; callers can only query via IsAllowedErrorClass or enumerate
// via AllErrorClasses (which returns a defensive copy).
var allowedErrorClasses = map[ErrorClass]struct{}{
	ErrInvalidArguments:      {},
	ErrRequestCreateFailed:   {},
	ErrConnectionFailed:      {},
	ErrRequestTimeout:        {},
	ErrResponseTooLarge:      {},
	ErrUnexpectedHTTPStatus:  {},
	ErrMalformedJSON:         {},
	ErrUnknownJSONField:      {},
	ErrMissingRequiredField:  {},
	ErrTrailingJSON:          {},
	ErrHealthNotReady:        {},
	ErrStateInvalid:          {},
	ErrWorkloadCountMismatch: {},
	ErrSchemaVersionMismatch: {},
	ErrInvalidOperation:      {},
}

// IsAllowedErrorClass reports whether ec is in the closed vocabulary.
func IsAllowedErrorClass(ec ErrorClass) bool {
	_, ok := allowedErrorClasses[ec]
	return ok
}

// AllErrorClasses returns a defensive copy of the error-class vocabulary.
// Callers cannot mutate the package-level authority.
func AllErrorClasses() []ErrorClass {
	out := make([]ErrorClass, 0, len(allowedErrorClasses))
	for ec := range allowedErrorClasses {
		out = append(out, ec)
	}
	return out
}

// retryableErrorClasses is the closed vocabulary of retryable error classes.
var retryableErrorClasses = map[ErrorClass]struct{}{
	ErrConnectionFailed: {},
	ErrRequestTimeout:   {},
	ErrHealthNotReady:   {},
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
		_, ok := retryableErrorClasses[pe.ErrClass]
		return ok
	}
	return false
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

// requiredEnvelopeFields are the envelope members required on every wire document.
var requiredEnvelopeFields = []string{
	"schema_version",
	"operation",
	"success",
	"http_status",
}

// RequiredEnvelopeFields returns a defensive copy of the required envelope fields.
func RequiredEnvelopeFields() []string {
	out := make([]string, len(requiredEnvelopeFields))
	copy(out, requiredEnvelopeFields)
	return out
}

// requiredHealthFields are the fields required on every HealthPayload.
var requiredHealthFields = []string{"ready", "mode"}

// RequiredHealthFields returns a defensive copy of the required health fields.
func RequiredHealthFields() []string {
	out := make([]string, len(requiredHealthFields))
	copy(out, requiredHealthFields)
	return out
}

// requiredStateFields are the fields required on every StatePayload.
var requiredStateFields = []string{
	"mode",
	"retained_blocks",
	"retained_bytes",
	"operation_count",
	"fd_count",
	"ready",
}

// RequiredStateFields returns a defensive copy of the required state fields.
func RequiredStateFields() []string {
	out := make([]string, len(requiredStateFields))
	copy(out, requiredStateFields)
	return out
}

// requiredWorkloadFields are the fields required on every WorkloadPayload.
var requiredWorkloadFields = []string{"requested", "attempted", "completed"}

// RequiredWorkloadFields returns a defensive copy of the required workload fields.
func RequiredWorkloadFields() []string {
	out := make([]string, len(requiredWorkloadFields))
	copy(out, requiredWorkloadFields)
	return out
}

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
//  5. reject unknown top-level members
//  6. require schema_version, operation, success, http_status (presence and non-null)
//  7. reject empty schema_version or operation
//  8. validate the discriminated success/failure variant
//  9. for success envelopes, decode and validate the nested payload
//     (presence, null, required-field, semantic invariants) per Operation
// 10. for failure envelopes, reject any nested payload and validate HTTP status
//
// Consumers MUST use this function — not encoding/json directly.
func DecodeEnvelopeExactlyOne(data []byte) (*ControlEnvelope, error) {
	var env ControlEnvelope
	if err := decodeExactJSONObject(data, requiredEnvelopeFields, &env); err != nil {
		return nil, err
	}

	// Decoded top-level shape is structurally OK. Now validate semantics.
	if env.SchemaVersion != SchemaVersion {
		return nil, &ProtocolError{
			ErrClass: ErrSchemaVersionMismatch,
			Message:  "schema_version mismatch: " + env.SchemaVersion,
		}
	}
	op := Operation(env.Operation)
	if !IsValidOperation(op) {
		return nil, &ProtocolError{
			ErrClass: ErrInvalidOperation,
			Message:  "invalid operation: " + env.Operation,
		}
	}

	// Re-decode the body to enforce nested payload presence/null/required-field
	// rules before typed decode succeeds.
	if err := validateEnvelopePayloads(data, &env, op); err != nil {
		return nil, err
	}

	if env.Success {
		if err := validateSuccessEnvelope(&env, op); err != nil {
			return nil, err
		}
	} else {
		if err := validateFailureEnvelope(&env); err != nil {
			return nil, err
		}
	}
	return &env, nil
}

// validateEnvelopePayloads enforces the presence, null, and required-field
// rules for the nested payload member of a successful envelope. It re-decodes
// the body to preserve the raw payload for typed semantic validation.
func validateEnvelopePayloads(data []byte, env *ControlEnvelope, op Operation) error {
	// Re-decode top-level to inspect the nested payload member.
	if len(bytes.TrimSpace(data)) == 0 {
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: "empty body"}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var rawMap map[string]json.RawMessage
	if err := dec.Decode(&rawMap); err != nil {
		return &ProtocolError{ErrClass: ErrMalformedJSON, Message: "not a JSON object"}
	}

	if env.Success {
		// For success: exactly one payload of the correct type must be present and non-null.
		switch op {
		case OpHealth:
			raw, ok := rawMap["health"]
			if !ok {
				return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "missing health"}
			}
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "null health"}
			}
			// Run full presence/null/unknown-field/semantic validation on health payload
			// (DecodeHealth does this, but we re-call it here so envelope decode catches
			// nested-payload shape errors in one pass).
			if _, err := decodeHealthRaw(raw); err != nil {
				return err
			}
			// Also ensure state/workload payloads are NOT present in health envelope
			if _, present := rawMap["state"]; present {
				return &ProtocolError{
					ErrClass: ErrInvalidArguments,
					Message:  "health operation must not have state payload",
				}
			}
			if _, present := rawMap["workload"]; present {
				return &ProtocolError{
					ErrClass: ErrInvalidArguments,
					Message:  "health operation must not have workload payload",
				}
			}
		case OpState:
			raw, ok := rawMap["state"]
			if !ok {
				return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "missing state"}
			}
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "null state"}
			}
			if _, err := decodeStateRaw(raw); err != nil {
				return err
			}
			if _, present := rawMap["health"]; present {
				return &ProtocolError{
					ErrClass: ErrInvalidArguments,
					Message:  "state operation must not have health payload",
				}
			}
			if _, present := rawMap["workload"]; present {
				return &ProtocolError{
					ErrClass: ErrInvalidArguments,
					Message:  "state operation must not have workload payload",
				}
			}
		case OpOperate:
			raw, ok := rawMap["workload"]
			if !ok {
				return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "missing workload"}
			}
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "null workload"}
			}
			if _, err := decodeWorkloadRaw(raw, 0); err != nil {
				return err
			}
			if _, present := rawMap["health"]; present {
				return &ProtocolError{
					ErrClass: ErrInvalidArguments,
					Message:  "operate operation must not have health payload",
				}
			}
			if _, present := rawMap["state"]; present {
				return &ProtocolError{
					ErrClass: ErrInvalidArguments,
					Message:  "operate operation must not have state payload",
				}
			}
		}
	} else {
		// For failure: no payloads permitted.
		if _, present := rawMap["health"]; present {
			return &ProtocolError{
				ErrClass: ErrInvalidArguments,
				Message:  "failure envelope must not have health payload",
			}
		}
		if _, present := rawMap["state"]; present {
			return &ProtocolError{
				ErrClass: ErrInvalidArguments,
				Message:  "failure envelope must not have state payload",
			}
		}
		if _, present := rawMap["workload"]; present {
			return &ProtocolError{
				ErrClass: ErrInvalidArguments,
				Message:  "failure envelope must not have workload payload",
			}
		}
	}
	return nil
}

// decodeHealthRaw decodes the raw bytes of a health payload.
func decodeHealthRaw(raw json.RawMessage) (*HealthPayload, error) {
	return DecodeHealth(raw)
}

// decodeStateRaw decodes the raw bytes of a state payload.
func decodeStateRaw(raw json.RawMessage) (*StatePayload, error) {
	return DecodeState(raw)
}

// decodeWorkloadRaw decodes the raw bytes of a workload payload.
func decodeWorkloadRaw(raw json.RawMessage, expectedRequest int) (*WorkloadPayload, error) {
	return DecodeWorkload(raw, expectedRequest)
}

// DecodeHealth decodes a /health response body into a HealthPayload.
func DecodeHealth(data []byte) (*HealthPayload, error) {
	var h HealthPayload
	if err := decodeExactJSONObject(data, requiredHealthFields, &h); err != nil {
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
	if err := decodeExactJSONObject(data, requiredStateFields, &s); err != nil {
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
	if s == nil {
		return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "nil state"}
	}
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
// to the supplied expected count (if > 0).
func DecodeWorkload(data []byte, expectedRequest int) (*WorkloadPayload, error) {
	var w WorkloadPayload
	if err := decodeExactJSONObject(data, requiredWorkloadFields, &w); err != nil {
		return nil, err
	}
	if err := ValidateWorkloadSemantics(&w, expectedRequest); err != nil {
		return nil, err
	}
	return &w, nil
}

// ValidateWorkloadSemantics enforces WorkloadPayload invariants:
//   - requested > 0
//   - if expectedRequest > 0: requested == expectedRequest
//   - attempted == requested
//   - completed == attempted
func ValidateWorkloadSemantics(w *WorkloadPayload, expectedRequest int) error {
	if w == nil {
		return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "nil workload"}
	}
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

// ValidateControlEnvelope validates a pre-decoded envelope's semantic invariants.
// Callers who already have a *ControlEnvelope (e.g., from a struct literal in tests)
// should call this directly.
func ValidateControlEnvelope(env *ControlEnvelope) error {
	if env == nil {
		return &ProtocolError{ErrClass: ErrMissingRequiredField, Message: "nil envelope"}
	}
	if env.SchemaVersion == "" {
		return &ProtocolError{ErrClass: ErrSchemaVersionMismatch, Message: "empty schema_version"}
	}
	if env.SchemaVersion != SchemaVersion {
		return &ProtocolError{
			ErrClass: ErrSchemaVersionMismatch,
			Message:  "schema_version mismatch: " + env.SchemaVersion,
		}
	}
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
	if err := validateFailureHTTPStatus(env.HTTPStatus); err != nil {
		return err
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

// validateFailureHTTPStatus enforces the strict failure HTTP-status rules:
//   - http_status == 0 (local failure)
//   - 100 <= http_status <= 599 AND NOT 2xx
//
// Rejects:
//   - negative status
//   - 1..99
//   - 600 and above
//   - any 2xx (200, 201, 204, etc.)
func validateFailureHTTPStatus(httpStatus int) error {
	if httpStatus == 0 {
		return nil
	}
	if httpStatus < 0 {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  fmt.Sprintf("negative http_status: %d", httpStatus),
		}
	}
	if httpStatus < 100 {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  fmt.Sprintf("informational http_status not allowed on failure: %d", httpStatus),
		}
	}
	if httpStatus >= 600 {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  fmt.Sprintf("http_status out of range: %d", httpStatus),
		}
	}
	if httpStatus >= 200 && httpStatus < 300 {
		return &ProtocolError{
			ErrClass: ErrUnexpectedHTTPStatus,
			Message:  fmt.Sprintf("failure envelope must not have 2xx http_status, got %d", httpStatus),
		}
	}
	return nil
}