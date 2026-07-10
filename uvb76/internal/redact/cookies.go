// Package redact provides pure deterministic secret redaction for UVB-76 artifacts.
//
// This file implements typed cookie redaction with explicit handling for:
// - Request Cookie headers (all name=value pairs are credentials)
// - Response Set-Cookie headers (first value is credential, rest are attributes)
package redact

import (
	"strings"
)

// ============================================================================
// Request Cookie Redaction
// ============================================================================

// RedactRequestCookieHeader redacts all cookie name=value pairs in a Cookie header.
// In request context, cookies are simple name=value pairs separated by semicolons.
//
// Request cookies never have attributes like Path, Domain, Secure, HttpOnly, etc.
// Any name=value pair is a credential that should be redacted.
//
// Examples:
//   - "session=abc123" → "session=[REDACTED]"
//   - "session=abc; uvb76_session=xyz" → "session=[REDACTED]; uvb76_session=[REDACTED]"
//   - "path=safe; name=admin" → "path=[REDACTED]; name=[REDACTED]"
//
// The names "path", "domain", "secure", "httponly" are NOT treated as Set-Cookie
// attributes in request context—they are ordinary cookie names.
func RedactRequestCookieHeader(cookie string) string {
	if cookie == "" {
		return ""
	}

	parts := strings.Split(cookie, ";")
	var resultParts []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		eqIdx := strings.Index(part, "=")
		if eqIdx > 0 {
			name := part[:eqIdx]
			resultParts = append(resultParts, name+"="+Redacted)
		} else {
			// Cookie without = is treated as a value-less cookie name
			resultParts = append(resultParts, part+"="+Redacted)
		}
	}

	if len(resultParts) == 0 {
		return ""
	}
	return strings.Join(resultParts, "; ")
}

// ============================================================================
// Response Set-Cookie Redaction
// ============================================================================

// setCookieSafeAttributes are recognized Set-Cookie attributes that should be
// preserved when redacting. These are NOT credentials - they are metadata about
// the cookie, not the cookie value itself.
//
// IMPORTANT: These are only preserved for segments AFTER the first cookie name=value.
// The first segment ALWAYS contains the cookie name and value, regardless of what
// the name is. A cookie named "Path" or "Domain" is still a credential in its value.
var setCookieSafeAttributes = map[string]bool{
	"path":        true,
	"domain":      true,
	"max-age":     true,
	"expires":     true,
	"secure":      true,
	"httponly":    true,
	"samesite":    true,
	"partitioned": true,
	"priority":    true,
}

// RedactSetCookieHeader redacts the cookie value in a Set-Cookie header,
// preserving recognized attributes.
//
// SEMANTICS:
// 1. Locate the first non-empty segment
// 2. Require it to contain a valid non-empty cookie name followed by '='
// 3. Redact its value unconditionally (the name "Path" or "Domain" doesn't matter)
// 4. Only then interpret later segments as attributes
//
// Examples:
//   - "session=abc123; Path=/; HttpOnly" → "session=[REDACTED]; Path=/; HttpOnly"
//   - "token=xyz; Domain=.example.com; Secure" → "token=[REDACTED]; Domain=.example.com; Secure"
//   - "Path=credential" → "Path=[REDACTED]" (Path is the cookie NAME, its value is redacted)
//   - "Domain=credential" → "Domain=[REDACTED]" (Domain is the cookie NAME, its value is redacted)
//
// Malformed first segments return [REDACTED].
func RedactSetCookieHeader(cookie string) string {
	if cookie == "" {
		return ""
	}

	parts := strings.Split(cookie, ";")
	if len(parts) == 0 {
		return Redacted
	}

	var resultParts []string
	isFirstSegment := true

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		eqIdx := strings.Index(part, "=")
		if isFirstSegment {
			// First segment MUST be cookie name=value
			if eqIdx <= 0 {
				// Malformed: no '=' or empty cookie name
				return Redacted
			}
			// Always redact the first value, regardless of name
			resultParts = append(resultParts, part[:eqIdx+1]+Redacted)
			isFirstSegment = false
			continue
		}

		// Subsequent segments are treated as attributes
		if eqIdx > 0 {
			name := strings.ToLower(part[:eqIdx])
			if setCookieSafeAttributes[name] {
				// Preserve recognized attributes
				resultParts = append(resultParts, part)
			} else {
				// Unknown attribute - redact the value, preserve name
				resultParts = append(resultParts, part[:eqIdx+1]+Redacted)
			}
		} else {
			// Valueless attribute (e.g., HttpOnly, Secure, Partitioned)
			partLower := strings.ToLower(part)
			if setCookieSafeAttributes[partLower] {
				// Preserve recognized boolean attributes
				resultParts = append(resultParts, part)
			} else {
				// Unknown valueless attribute - do not preserve without explicit policy
				// Fall through - attribute is dropped
			}
		}
	}

	if len(resultParts) == 0 {
		return Redacted
	}
	return strings.Join(resultParts, "; ")
}

// ============================================================================
// Legacy Cookie Redaction (deprecated)
// ============================================================================

// redactCookieValue is the legacy function used by RedactHeaders.
// It delegates to typed functions based on header name.
// DEPRECATED: Use RedactRequestCookieHeader or RedactSetCookieHeader directly.
func redactCookieValue(cookie string, isSetCookie bool) string {
	if isSetCookie {
		return RedactSetCookieHeader(cookie)
	}
	return RedactRequestCookieHeader(cookie)
}
