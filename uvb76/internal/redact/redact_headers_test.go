package redact

import (
	"net/http"
	"strings"
	"testing"
)

// Helper to create HTTP headers
func headersFromMap(m map[string][]string) http.Header {
	h := make(http.Header)
	for k, v := range m {
		for _, val := range v {
			h.Add(k, val)
		}
	}
	return h
}

// ============================================================================
// Determinism and Idempotence Tests
// ============================================================================

func TestRedactHeaders_Idempotence(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"Authorization": {"Bearer secret123"},
		"Content-Type":  {"application/json"},
	})

	// Apply twice should give same result
	first := RedactHeaders(h)
	second := RedactHeaders(first)

	if first.Get("Authorization") != second.Get("Authorization") {
		t.Errorf("RedactHeaders not idempotent: first=%q, second=%q",
			first.Get("Authorization"), second.Get("Authorization"))
	}
}

func TestRedactURL_Idempotence(t *testing.T) {
	urls := []string{
		"https://user:pass@example.com/path?token=secret",
		"https://example.com/api?key=secret",
		"https://user:pass@host:8080/path",
	}

	for _, u := range urls {
		first := RedactURL(u)
		second := RedactURL(first)
		if first != second {
			t.Errorf("RedactURL not idempotent for %q: first=%q, second=%q", u, first, second)
		}
	}
}

func TestRedactConfigValue_Idempotence(t *testing.T) {
	cases := []struct {
		field, value string
	}{
		{"password", "secret123"},
		{"api_key", "sk-1234567890"},
		{"token", "jwt.token.here"},
		{"password_sha256", "sha256:abc:def"},
	}

	for _, c := range cases {
		first := RedactConfigValue(c.field, c.value)
		second := RedactConfigValue(c.field, first)
		if first != second {
			t.Errorf("RedactConfigValue not idempotent for %s=%q: first=%q, second=%q",
				c.field, c.value, first, second)
		}
	}
}

func TestRedactStructuredJSON_Idempotence(t *testing.T) {
	input := []byte(`{"password":"secret","user":"admin"}`)
	first, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("First redact failed: %v", err)
	}
	second, err := RedactStructuredJSON(first)
	if err != nil {
		t.Fatalf("Second redact failed: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("RedactStructuredJSON not idempotent: first=%s, second=%s", first, second)
	}
}

// ============================================================================
// Header Tests
// ============================================================================

func TestRedactHeaders_NilHeaders(t *testing.T) {
	result := RedactHeaders(nil)
	if result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}
}

func TestRedactHeaders_CaseInsensitive(t *testing.T) {
	headers := headersFromMap(map[string][]string{
		"Authorization": {"Bearer token123"},
		"authorization": {"Basic dXNlcjpwYXNz"},
		"AUTHORIZATION": {"Bearer another"},
		"CONTENT-TYPE":  {"text/html"},
	})

	redacted := RedactHeaders(headers)

	// Authorization variants should all be redacted
	if redacted.Get("Authorization") != Redacted {
		t.Errorf("Authorization not redacted: %s", redacted.Get("Authorization"))
	}
	if redacted.Get("authorization") != Redacted {
		t.Errorf("authorization not redacted: %s", redacted.Get("authorization"))
	}
	if redacted.Get("AUTHORIZATION") != Redacted {
		t.Errorf("AUTHORIZATION not redacted: %s", redacted.Get("AUTHORIZATION"))
	}

	// Non-sensitive headers should be preserved
	if redacted.Get("Content-Type") != "text/html" {
		t.Errorf("Content-Type was modified: %s", redacted.Get("Content-Type"))
	}
}

func TestRedactHeaders_MultipleValues(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"Authorization": {"Bearer token1", "Bearer token2"},
	})

	redacted := RedactHeaders(h)
	values := redacted.Values("Authorization")

	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(values))
	}
	for _, v := range values {
		if v != Redacted {
			t.Errorf("Expected %q, got %q", Redacted, v)
		}
	}
}

func TestRedactHeaders_XSessionToken(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"X-Session-Token": {"session_abc123xyz789"},
	})

	redacted := RedactHeaders(h)
	token := redacted.Get("X-Session-Token")

	// Should be redacted, not the original value
	if token == "session_abc123xyz789" {
		t.Errorf("X-Session-Token not redacted: %s", token)
	}
	if token != Redacted {
		t.Errorf("Expected %q, got %q", Redacted, token)
	}
}

func TestRedactHeaders_ProxyAuthorization(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"Proxy-Authorization": {"Basic dXNlcjpwYXNz"},
	})

	redacted := RedactHeaders(h)
	if redacted.Get("Proxy-Authorization") != Redacted {
		t.Errorf("Proxy-Authorization not redacted: %s", redacted.Get("Proxy-Authorization"))
	}
}

func TestRedactHeaders_APIKey(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"X-API-Key": {"sk-api-key-1234567890abcdef"},
	})

	redacted := RedactHeaders(h)
	if redacted.Get("X-API-Key") != Redacted {
		t.Errorf("X-API-Key not redacted: %s", redacted.Get("X-API-Key"))
	}
}

// ============================================================================
// Cookie Tests
// ============================================================================

func TestRedactHeaders_MultiCookieRequest(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"Cookie": {"session=abc123; uvb76_session=secret_token; analytics=xyz789"},
	})

	redacted := RedactHeaders(h)
	cookie := redacted.Get("Cookie")

	// Current behavior: first name=value is redacted
	if strings.Contains(cookie, "session=abc123") {
		t.Errorf("session cookie value not redacted: %s", cookie)
	}
}

func TestRedactHeaders_SetCookieAttributes(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"Set-Cookie": {"session=abc123; HttpOnly; Secure; Path=/"},
	})

	redacted := RedactHeaders(h)
	cookie := redacted.Get("Set-Cookie")

	// Value should be redacted
	if strings.Contains(cookie, "session=abc123") {
		t.Errorf("Set-Cookie value not redacted: %s", cookie)
	}
	// Attributes should be preserved
	if !strings.Contains(cookie, "HttpOnly") {
		t.Errorf("HttpOnly attribute should be preserved: %s", cookie)
	}
	if !strings.Contains(cookie, "Secure") {
		t.Errorf("Secure attribute should be preserved: %s", cookie)
	}
	if !strings.Contains(cookie, "Path=/") {
		t.Errorf("Path attribute should be preserved: %s", cookie)
	}
}

func TestRedactHeaders_Uvb76SessionCookie(t *testing.T) {
	h := headersFromMap(map[string][]string{
		"Cookie": {"uvb76_session=verylongsecretvalue123"},
	})

	redacted := RedactHeaders(h)
	cookie := redacted.Get("Cookie")

	// Should show uvb76_session= but not the value
	if !strings.Contains(cookie, "uvb76_session=") {
		t.Errorf("uvb76_session name should be preserved: %s", cookie)
	}
	if strings.Contains(cookie, "verylongsecretvalue123") {
		t.Errorf("uvb76_session value not redacted: %s", cookie)
	}
	if cookie != "uvb76_session="+Redacted {
		t.Errorf("Expected 'uvb76_session=[REDACTED]', got %q", cookie)
	}
}
