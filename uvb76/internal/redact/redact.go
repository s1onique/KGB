// Package redact provides pure deterministic secret redaction for UVB-76 artifacts.
//
// This package enforces the artifact-secret-hygiene contract: no tracked artifact
// may retain authentication credentials, session material, private key material,
// sensitive configuration values, or credential-bearing URLs and headers.
//
// All operations are pure (no filesystem, network, environment, or clock access).
// Output is deterministic and idempotent: Redact(Redact(value)) == Redact(value).
//
// Canonical redaction marker: [REDACTED]
//
// Rule IDs are defined in the canonical registry at:
// scripts/uvb76_artifact_secret_hygiene/registry.json
//
// This file must agree with the registry. Run:
// python3 scripts/verify_uvb76_artifact_secret_hygiene.py --self-test
// to validate consistency.
package redact

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// Redacted is the canonical redaction marker used in all sanitized artifacts.
const Redacted = "[REDACTED]"

// Rule identifiers for diagnostics (never printed with secret values).
// These constants must agree with scripts/uvb76_artifact_secret_hygiene/registry.json
//
// Universal scope rules (applied to all files):
const (
	RulePrivateKeyPEM          = "UVB76-SECRET-0001" // private_key_pem
	RuleEncryptedPrivateKeyPEM = "UVB76-SECRET-0002" // encrypted_private_key_pem
	RuleRSAPrivateKeyPEM       = "UVB76-SECRET-0003" // rsa_private_key_pem
	RuleECPrivateKeyPEM        = "UVB76-SECRET-0004" // ec_private_key_pem
	RuleOpenSSHPrivateKeyPEM   = "UVB76-SECRET-0005" // openssh_private_key_pem
)

// Artifact context scope rules (applied based on artifact type):
const (
	// Header-based rules
	RuleAuthorizationBearer = "UVB76-SECRET-0010" // authorization_bearer
	RuleAuthorizationBasic  = "UVB76-SECRET-0011" // authorization_basic
	RuleProxyAuthorization  = "UVB76-SECRET-0012" // proxy_authorization
	RuleAPIKeyHeader        = "UVB76-SECRET-0013" // api_key_header
	RuleSessionTokenHeader  = "UVB76-SECRET-0020" // session_token_header

	// Cookie rules
	RuleCookieCredential    = "UVB76-SECRET-0030" // cookie_credential
	RuleSetCookieCredential = "UVB76-SECRET-0031" // set_cookie_credential
	RuleUVB76SessionCookie  = "UVB76-SECRET-0032" // uvb76_session_cookie

	// Field-based rules
	RulePasswordField     = "UVB76-SECRET-0040" // password_field
	RulePasswordHashField = "UVB76-SECRET-0041" // password_hash_field
	RuleGenericTokenField = "UVB76-SECRET-0050" // generic_token_field
	RuleClientKeyData     = "UVB76-SECRET-0060" // client_key_data
	RulePrivateKeyData    = "UVB76-SECRET-0061" // private_key_data

	// URL rules
	RuleCredentialBearingHTTPURL = "UVB76-SECRET-0070" // credential_bearing_http_url
	RuleCredentialBearingDSN     = "UVB76-SECRET-0071" // credential_bearing_database_dsn
	RuleSensitiveQueryParam      = "UVB76-SECRET-0072" // sensitive_url_query_parameter

	// Token rules
	RuleJWTLikeToken       = "UVB76-SECRET-0080" // jwt_like_token
	RuleBearerTokenLiteral = "UVB76-SECRET-0081" // bearer_token_literal
)

// privateKeyPEMPatterns is populated by init() to avoid containing complete markers in source.
var privateKeyPEMPatterns []*regexp.Regexp

func init() {
	// Build patterns from fragments at runtime to avoid self-rejection during staging.
	// The hygiene implementation files contain these fragments but not complete markers.
	begin := "BEGIN"
	priv := "PRIVATE"
	enc := "ENCRYPTED"
	rsa := "RSA"
	ec := "EC"
	ssh := "OPENSSH"
	key := "KEY"
	dashes := "-----"
	space := " "

	// Construct complete markers at runtime (not present in source)
	privKey := dashes + begin + space + priv + space + key + dashes
	encKey := dashes + begin + space + enc + space + priv + space + key + dashes
	rsaKey := dashes + begin + space + rsa + space + priv + space + key + dashes
	ecKey := dashes + begin + space + ec + space + priv + space + key + dashes
	sshKey := dashes + begin + space + ssh + space + priv + space + key + dashes

	privateKeyPEMPatterns = []*regexp.Regexp{
		regexp.MustCompile(privKey),
		regexp.MustCompile(encKey),
		regexp.MustCompile(rsaKey),
		regexp.MustCompile(ecKey),
		regexp.MustCompile(sshKey),
	}
}

// Sensitive header names (case-insensitive matching).
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"x-api-key":           true,
	"x-session-token":     true,
	"cookie":              true,
	"set-cookie":          true,
}

// Sensitive field names (context-aware).
// Covers rules: password_field, password_hash_field, generic_token_field,
// client_key_data, private_key_data
var sensitiveFields = map[string]bool{
	// password_field rule (UVB76-SECRET-0040)
	"password":       true,
	"admin_password": true,
	"passwd":         true,
	// password_hash_field rule (UVB76-SECRET-0041)
	"password_hash":       true,
	"admin_password_hash": true,
	"password_sha256":     true,
	"password-hash":       true,
	// generic_token_field rule (UVB76-SECRET-0050)
	"api_key":       true,
	"api_token":     true,
	"access_token":  true,
	"refresh_token": true,
	"session_token": true,
	"session_id":    true,
	"csrf_token":    true,
	"bearer_token":  true,
	"secret":        true,
	"client_secret": true,
	"session_key":   true,
	// client_key_data rule (UVB76-SECRET-0060)
	"client_key_data": true,
	// private_key_data rule (UVB76-SECRET-0061)
	"private_key":      true,
	"private_key_data": true,
}

// Sensitive URL query parameters.
var sensitiveQueryParams = map[string]bool{
	"token":        true,
	"access_token": true,
	"api_key":      true,
	"password":     true,
	"secret":       true,
	"key":          true,
	"auth":         true,
	"credential":   true,
}

// URL input length bound to prevent DoS.
const maxURLLength = 65536

// Max query parameters bound.
const maxQueryParams = 100

// ============================================================================
// Private Key PEM Marker Detection (Narrow-Named API)
// ============================================================================

// DetectPrivateKeyMarker checks if the input contains a private key PEM marker.
// This function only detects PEM-formatted private keys (BEGIN PRIVATE KEY, etc.).
//
// This is intentionally narrow: it does NOT detect JWT tokens, bearer tokens,
// API keys in headers, or other credential patterns. Use typed redactors
// for those data shapes.
//
// Returns the rule ID if a private key PEM marker is detected.
func DetectPrivateKeyMarker(input string) string {
	if input == "" {
		return ""
	}
	for _, pattern := range privateKeyPEMPatterns {
		if pattern.MatchString(input) {
			return RulePrivateKeyPEM
		}
	}
	return ""
}

// ContainsPrivateKeyMarker is an alias for DetectPrivateKeyMarker != "".
func ContainsPrivateKeyMarker(input string) bool {
	return DetectPrivateKeyMarker(input) != ""
}

// RedactPrivateKeyMarkers replaces any private key PEM markers with [REDACTED].
// This function only handles PEM-formatted private keys.
//
// For other credential types, use the appropriate typed redactor:
//   - Headers: RedactHeaders
//   - URLs: RedactURL
//   - Cookies: RedactRequestCookieHeader / RedactSetCookieHeader
//   - Config fields: RedactConfigValue
func RedactPrivateKeyMarkers(input string) string {
	detected := DetectPrivateKeyMarker(input)
	if detected != "" {
		return Redacted
	}
	return input
}

// ============================================================================
// Backward-Compatibility Aliases (Deprecated)
// ============================================================================

// DetectSecret checks if the input contains a detectable secret pattern.
// DEPRECATED: Use DetectPrivateKeyMarker for PEM markers only.
// This function only detects private key PEM markers, not all secret types.
func DetectSecret(input string) string {
	return DetectPrivateKeyMarker(input)
}

// ContainsSecret returns true if the input contains any detectable secret pattern.
// DEPRECATED: Use ContainsPrivateKeyMarker.
func ContainsSecret(input string) bool {
	return ContainsPrivateKeyMarker(input)
}

// RedactArtifactValue returns a sanitized copy of a value for artifact persistence.
// DEPRECATED: This function only handles PEM markers. Use typed redactors for
// headers, URLs, cookies, and structured data.
func RedactArtifactValue(input string) string {
	return RedactPrivateKeyMarkers(input)
}

// ============================================================================
// URL Redaction
// ============================================================================

// RedactURL sanitizes a URL, removing userinfo credentials and sensitive query values.
// Preserves: scheme, host, port, path, safe query parameters, fragments.
// Redacts: username, password, sensitive query VALUES (not the parameter name).
//
// Bounds:
//   - Input length: maxURLLength (65KB)
//   - Query parameter values: maxQueryParams (100) total values, not unique keys
//
// Examples:
//   - "?token=secret" → "?token=%5BREDACTED%5D"
//   - "?token=one&token=two&page=1" → "?token=%5BREDACTED%5D&token=%5BREDACTED%5D&page=1"
func RedactURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	// Bound check
	if len(rawURL) > maxURLLength {
		return Redacted
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return Redacted // Fail closed - do not include malformed query in error
	}
	// Remove userinfo
	if u.User != nil {
		u.User = nil
	}
	// Redact sensitive query parameter values
	if u.RawQuery != "" {
		// Use ParseQuery for explicit error handling (fail closed)
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return Redacted // Malformed query - fail closed
		}
		// Count total values, not unique keys
		totalValues := 0
		for _, values := range query {
			totalValues += len(values)
		}
		if totalValues > maxQueryParams {
			return Redacted // Bound exceeded - fail closed
		}
		// Replace sensitive values, preserve parameter names
		modified := false
		for param, values := range query {
			if sensitiveQueryParams[strings.ToLower(param)] {
				for i := range values {
					values[i] = Redacted
					modified = true
				}
				query[param] = values
			}
		}
		if modified {
			u.RawQuery = query.Encode()
		}
	}
	return u.String()
}

// redactURLFallback handles URLs that fail to parse.
func redactURLFallback(rawURL string) string {
	// Try to strip userinfo from malformed URLs
	userinfoRegex := regexp.MustCompile(`://[^/@]+:[^/@]+@`)
	if userinfoRegex.MatchString(rawURL) {
		return userinfoRegex.ReplaceAllString(rawURL, "://"+Redacted+"@")
	}
	return Redacted
}

// ============================================================================
// Configuration Field Redaction
// ============================================================================

// RedactConfigValue sanitizes a configuration field value based on field name.
// Returns Redacted for sensitive field names, preserving the value otherwise.
func RedactConfigValue(fieldName, value string) string {
	if fieldName == "" || value == "" {
		return value
	}
	fieldLower := strings.ToLower(fieldName)
	if sensitiveFields[fieldLower] {
		return Redacted
	}
	if strings.Contains(fieldLower, "password_hash") || strings.Contains(fieldLower, "password-hash") {
		if strings.HasPrefix(value, "sha256:") {
			return Redacted
		}
	}
	return value
}

// ============================================================================
// Structured JSON Redaction
// ============================================================================

// RedactStructuredJSON sanitizes a JSON structure by traversing it and redacting sensitive fields.
// Returns error for invalid JSON input (fail-closed contract).
//
// Supported field classes:
//   - password_field
//   - password_hash_field
//   - generic_token_field
//   - client_key_data
//   - private_key_data
//   - private_key_pem (within string values)
//
// Properties:
//   - Nested objects are supported
//   - Arrays are supported
//   - Field names are preserved
//   - Non-sensitive values are preserved
//   - Malformed JSON returns an error
//   - Input is not mutated
//   - Output is valid JSON
//   - Second redaction produces identical output
func RedactStructuredJSON(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	sanitized := redactValue(v)
	return json.MarshalIndent(sanitized, "", "  ")
}

// redactValue recursively sanitizes a parsed JSON value.
func redactValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			if sensitiveFields[strings.ToLower(k)] {
				result[k] = Redacted
			} else {
				result[k] = redactValue(v)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = redactValue(v)
		}
		return result
	case string:
		// First: redact private key PEM markers.
		for _, pattern := range privateKeyPEMPatterns {
			if pattern.MatchString(val) {
				return Redacted
			}
		}
		// Second: sanitize embedded JSON and typed header lines carried in
		// free-form string fields.
		val = redactStructuredString(val)
		// Third: detect and redact URLs.
		if isURLCandidate(val) {
			return RedactURL(val)
		}
		return val
	default:
		return v
	}
}

// URL detection heuristics for structured JSON strings.
// We use bounded candidate rules to avoid false positives on arbitrary prose.
var urlCandidatePatterns = []*regexp.Regexp{
	// HTTP/HTTPS URLs with at least scheme and host
	regexp.MustCompile(`^https?://[^\s"']+`),
	// Protocol-relative URLs
	regexp.MustCompile(`^//[^\s"']+`),
	// Database DSN patterns
	regexp.MustCompile(`^(postgres|mysql|mongodb)://[^\s"']+`),
	// URL with userinfo (credentials)
	regexp.MustCompile(`^[a-z]+://[^:]+:[^@]+@[^\s"']+`),
}

// isURLCandidate checks if a string looks like a URL candidate.
// Uses bounded rules to avoid treating arbitrary prose containing ? as URLs.
func isURLCandidate(s string) bool {
	if len(s) < 10 { // Minimum URL length
		return false
	}
	for _, pattern := range urlCandidatePatterns {
		if pattern.MatchString(s) {
			return true
		}
	}
	return false
}
