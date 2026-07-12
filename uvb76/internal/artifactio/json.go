package artifactio

import (
	"encoding/json"
	"fmt"

	"github.com/s1onique/KGB/uvb76/internal/redact"
)

// WriteRedactedJSONBytes publishes pre-serialized JSON bytes to the
// destination after passing them through structured redaction.
//
// The boundary contract:
//
//   - input bytes are bounded by policy.MaxInputBytes,
//   - sanitized bytes are bounded by policy.MaxOutputBytes,
//   - sanitization runs in-memory before any filesystem write,
//   - post-sanitization validates that the result contains no known
//     prohibited secret (via the canonical registry detector),
//   - the result is re-parsed as JSON when policy.PreserveStructure is set.
func WriteRedactedJSONBytes(
	surfaceID string,
	destination string,
	data []byte,
	policy WritePolicy,
) error {
	ctx := &writeContext{
		SurfaceID:   surfaceID,
		Destination: destination,
		Sanitizer:   "redact_structured_json",
		Policy:      policy,
	}

	if err := ctx.Policy.validate(); err != nil {
		return newError(ctx, "policy_invalid", err)
	}

	if len(data) > ctx.Policy.MaxInputBytes {
		return newError(ctx, "input_too_large",
			fmt.Errorf("input size %d exceeds MaxInputBytes %d",
				len(data), ctx.Policy.MaxInputBytes))
	}

	sanitized, err := redact.RedactStructuredJSON(data)
	if err != nil {
		return newError(ctx, "sanitize", err)
	}

	if err := postValidateOpenText(sanitized, ctx); err != nil {
		return err
	}

	// Policy states PreserveStructure implies JSON. Re-parse to confirm
	// sanitization preserved structure.
	if ctx.Policy.PreserveStructure {
		if !json.Valid(sanitized) {
			return newError(ctx, "post_validate",
				fmt.Errorf("sanitized output is not valid JSON"))
		}
	}

	if _, err := publish(ctx, sanitized); err != nil {
		return err
	}
	return nil
}

// WriteRedactedJSON marshals the value and publishes the marshalled
// sanitized bytes to the destination path.
//
// The serializer is bounded; marshal failure leaves the destination absent.
func WriteRedactedJSON(
	surfaceID string,
	destination string,
	value any,
	policy WritePolicy,
) error {
	ctx := &writeContext{
		SurfaceID:   surfaceID,
		Destination: destination,
		Sanitizer:   "redact_structured_json",
		Policy:      policy,
	}
	if err := ctx.Policy.validate(); err != nil {
		return newError(ctx, "policy_invalid", err)
	}

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return newError(ctx, "serialize", err)
	}

	if len(raw) > ctx.Policy.MaxInputBytes {
		return newError(ctx, "input_too_large",
			fmt.Errorf("serialized size %d exceeds MaxInputBytes %d",
				len(raw), ctx.Policy.MaxInputBytes))
	}

	sanitized, err := redact.RedactStructuredJSON(raw)
	if err != nil {
		return newError(ctx, "sanitize", err)
	}

	if err := postValidateOpenText(sanitized, ctx); err != nil {
		return err
	}

	if ctx.Policy.PreserveStructure {
		if !json.Valid(sanitized) {
			return newError(ctx, "post_validate",
				fmt.Errorf("sanitized output is not valid JSON"))
		}
	}

	if _, err := publish(ctx, sanitized); err != nil {
		return err
	}
	return nil
}
