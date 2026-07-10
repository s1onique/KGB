// Package redact provides pure deterministic secret redaction for UVB-76 artifacts.
//
// This file implements typed header redaction with explicit dispatching for
// each header type. Header matching is case-insensitive. The original
// header map is never mutated.
package redact

import (
	"net/http"
	"strings"
)

// ============================================================================
// Typed Header Redaction
// ============================================================================

// RedactAuthorizationHeader redacts the credential portion of an Authorization header.
// Handles Bearer, Basic, Digest, and other authentication schemes.
//
// Examples:
//   - "Bearer secret123" → "[REDACTED]"
//   - "Basic dXNlcjpwYXNz" → "[REDACTED]"
//   - "Digest username=..." → "[REDACTED]"
func RedactAuthorizationHeader(value string) string {
	if value == "" {
		return ""
	}
	// All authentication scheme values contain credentials
	return Redacted
}

// RedactProxyAuthorizationHeader redacts Proxy-Authorization credentials.
// These headers carry credentials for proxy authentication.
func RedactProxyAuthorizationHeader(value string) string {
	if value == "" {
		return ""
	}
	return Redacted
}

// RedactAPIKeyHeader redacts X-API-Key and similar custom API key headers.
func RedactAPIKeyHeader(value string) string {
	if value == "" {
		return ""
	}
	return Redacted
}

// RedactSessionTokenHeader redacts X-Session-Token and similar session headers.
func RedactSessionTokenHeader(value string) string {
	if value == "" {
		return ""
	}
	return Redacted
}

// ============================================================================
// Main Header Redaction Function
// ============================================================================

// RedactHeaders sanitizes HTTP headers, returning a new header map.
// The original input map is never modified.
//
// Case-insensitive header matching is used. Each sensitive header is processed
// by its dedicated typed function.
//
// Supported headers:
//   - Authorization: redacts authentication credentials
//   - Proxy-Authorization: redacts proxy credentials
//   - X-API-Key: redacts API keys
//   - X-Session-Token: redacts session tokens
//   - Cookie: redacts request cookie values
//   - Set-Cookie: redacts response cookie values (preserves attributes)
func RedactHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	result := make(http.Header)
	for name, values := range headers {
		nameLower := strings.ToLower(name)
		if sensitiveHeaders[nameLower] {
			switch nameLower {
			case "authorization":
				for _, v := range values {
					result.Add(name, RedactAuthorizationHeader(v))
				}
			case "proxy-authorization":
				for _, v := range values {
					result.Add(name, RedactProxyAuthorizationHeader(v))
				}
			case "x-api-key":
				for _, v := range values {
					result.Add(name, RedactAPIKeyHeader(v))
				}
			case "x-session-token":
				for _, v := range values {
					result.Add(name, RedactSessionTokenHeader(v))
				}
			case "cookie":
				for _, v := range values {
					result.Add(name, RedactRequestCookieHeader(v))
				}
			case "set-cookie":
				for _, v := range values {
					result.Add(name, RedactSetCookieHeader(v))
				}
			default:
				// Fallback: redact the value entirely
				for range values {
					result.Add(name, Redacted)
				}
			}
		} else {
			// Non-sensitive headers are preserved
			for _, v := range values {
				result.Add(name, v)
			}
		}
	}
	return result
}

// ============================================================================
// Deprecated Legacy Function
// ============================================================================

// redactHeadersLegacy is kept for backward compatibility during migration.
// DEPRECATED: Use RedactHeaders directly.
func redactHeadersLegacy(headers http.Header) http.Header {
	return RedactHeaders(headers)
}
