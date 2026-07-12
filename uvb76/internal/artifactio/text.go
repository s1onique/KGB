package artifactio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/s1onique/KGB/uvb76/internal/redact"
)

// WriteRedactedText publishes sanitized text to the destination path.
//
// Text sanitization uses a multi-pass strategy:
//
//  1. redact.RedactArtifactValue (private key markers),
//  2. per-line URL redaction via typed URL redactor,
//  3. per-line header detection + dispatch to typed header redactors,
//  4. per-line cookie dispatch to typed Cookie / Set-Cookie redactors.
//
// The boundary contract: bound the input, sanitize in-memory,
// post-validate, then atomic publish.
func WriteRedactedText(
	surfaceID string,
	destination string,
	value string,
	policy WritePolicy,
) error {
	ctx := &writeContext{
		SurfaceID:   surfaceID,
		Destination: destination,
		Sanitizer:   "redact_text",
		Policy:      policy,
	}
	if err := ctx.Policy.validate(); err != nil {
		return newError(ctx, "policy_invalid", err)
	}

	if len(value) > ctx.Policy.MaxInputBytes {
		return newError(ctx, "input_too_large",
			fmt.Errorf("input size %d exceeds MaxInputBytes %d",
				len(value), ctx.Policy.MaxInputBytes))
	}

	sanitized := sanitizeText(value)

	if err := postValidateOpenText([]byte(sanitized), ctx); err != nil {
		return err
	}

	if _, err := publish(ctx, []byte(sanitized)); err != nil {
		return err
	}
	return nil
}

// WriteRedactedConfig publishes a sanitized configuration document.
//
// Configuration sanitization uses structured JSON redaction (which
// redacts known sensitive fields and URL-like string values) and
// post-validates the result.
func WriteRedactedConfig(
	surfaceID string,
	destination string,
	value any,
	policy WritePolicy,
) error {
	ctx := &writeContext{
		SurfaceID:   surfaceID,
		Destination: destination,
		Sanitizer:   "redact_config",
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

	if ctx.Policy.PreserveStructure && !json.Valid(sanitized) {
		return newError(ctx, "post_validate",
			fmt.Errorf("sanitized config is not valid JSON"))
	}

	if _, err := publish(ctx, sanitized); err != nil {
		return err
	}
	return nil
}

// sanitizeText applies the typed redaction passes to a text document.
func sanitizeText(value string) string {
	// Pass 1: generic artifact redactor (private keys).
	s := redact.RedactArtifactValue(value)
	// Pass 2: per-line URL redaction (covers URLs that are not in headers).
	s = redactURLLines(s)
	// Pass 3: per-line header detection.
	s = redactHeaderLinesImpl(s)
	// Pass 4: per-line cookie detection.
	s = redactCookieLinesImpl(s)
	return s
}

// redactURLLines rewrites every URL in every line via the typed URL redactor.
func redactURLLines(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = redactURLsImpl(line)
	}
	return strings.Join(lines, "\n")
}

// postValidateOpenText verifies the sanitized bytes no longer contain
// prohibited private-key markers, headers with credential-bearing values,
// credential-bearing URLs, or known credential-bearing patterns.
//
// Diagnostics include the surface ID, destination, sanitizer type, and
// (when known) the rule ID — never the secret bytes.
func postValidateOpenText(sanitized []byte, ctx *writeContext) error {
	if hit := redact.ContainsPrivateKeyMarker(string(sanitized)); hit {
		return newErrorWithRule(ctx, "post_validate",
			redact.RulePrivateKeyPEM, "",
			fmt.Errorf("sanitized output still contains a private-key marker"))
	}

	if hit := redact.ContainsSecret(string(sanitized)); hit {
		if startsWithCaseInsensitive(string(sanitized), "Authorization: Bearer ") {
			return newErrorWithRule(ctx, "post_validate",
				redact.RuleAuthorizationBearer, "",
				fmt.Errorf("sanitized output still contains a non-redacted Bearer credential"))
		}
		if startsWithCaseInsensitive(string(sanitized), "Authorization: Basic ") {
			return newErrorWithRule(ctx, "post_validate",
				redact.RuleAuthorizationBasic, "",
				fmt.Errorf("sanitized output still contains a non-redacted Basic credential"))
		}
		if startsWithCaseInsensitive(string(sanitized), "Proxy-Authorization:") {
			return newErrorWithRule(ctx, "post_validate",
				redact.RuleProxyAuthorization, "",
				fmt.Errorf("sanitized output still contains a non-redacted Proxy-Authorization credential"))
		}
		if startsWithCaseInsensitive(string(sanitized), "X-API-Key:") {
			return newErrorWithRule(ctx, "post_validate",
				redact.RuleAPIKeyHeader, "",
				fmt.Errorf("sanitized output still contains a non-redacted X-API-Key"))
		}
		if startsWithCaseInsensitive(string(sanitized), "X-Session-Token:") {
			return newErrorWithRule(ctx, "post_validate",
				redact.RuleSessionTokenHeader, "",
				fmt.Errorf("sanitized output still contains a non-redacted X-Session-Token"))
		}
		return newErrorWithRule(ctx, "post_validate",
			redact.RuleBearerTokenLiteral, "",
			fmt.Errorf("sanitized output still contains a credential-bearing literal"))
	}

	if hasCredentialBearingURL(sanitized) {
		return newErrorWithRule(ctx, "post_validate",
			redact.RuleCredentialBearingHTTPURL, "",
			fmt.Errorf("sanitized output still contains a credential-bearing URL"))
	}

	return nil
}

// hasCredentialBearingURL is a low-cost heuristic that fails closed if
// any "scheme://user:pass@host" pattern is observed.
func hasCredentialBearingURL(b []byte) bool {
	s := string(b)
	for i := 0; i < len(s); i++ {
		if i+6 > len(s) {
			break
		}
		if s[i] != ':' || s[i+1] != '/' || s[i+2] != '/' {
			continue
		}
		j := i + 3
		atIdx := -1
		for k := j; k < len(s); k++ {
			c := s[k]
			if c == '@' {
				atIdx = k
				break
			}
			if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
				break
			}
		}
		if atIdx > j {
			seg := s[j:atIdx]
			hasColon := false
			for _, c := range seg {
				if c == ':' {
					hasColon = true
					break
				}
			}
			if hasColon {
				return true
			}
		}
	}
	return false
}

func startsWithCaseInsensitive(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		sc := s[i]
		pc := prefix[i]
		if sc >= 'A' && sc <= 'Z' {
			sc += 'a' - 'A'
		}
		if pc >= 'A' && pc <= 'Z' {
			pc += 'a' - 'A'
		}
		if sc != pc {
			return false
		}
	}
	return true
}

// sha256Hex returns the hex-encoded SHA-256 of the input bytes.
// Used by tests as a fingerprint helper.
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
