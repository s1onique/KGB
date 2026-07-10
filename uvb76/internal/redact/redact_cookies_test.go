package redact

import (
	"strings"
	"testing"
)

// ============================================================================
// Request Cookie Header Tests
// ============================================================================

func TestRedactRequestCookieHeader_Basic(t *testing.T) {
	result := RedactRequestCookieHeader("session=abc123")
	expected := "session=" + Redacted
	if result != expected {
		t.Errorf("Basic cookie not redacted: got %q, want %q", result, expected)
	}
}

func TestRedactRequestCookieHeader_MultipleCookies(t *testing.T) {
	result := RedactRequestCookieHeader("session=abc; uvb76_session=xyz; analytics=xyz789")
	if strings.Contains(result, "abc") || strings.Contains(result, "xyz") {
		t.Errorf("Cookie values not fully redacted: %s", result)
	}
}

func TestRedactRequestCookieHeader_PathCookieName(t *testing.T) {
	// "path" is an attribute name in Set-Cookie, but NOT in request cookies
	result := RedactRequestCookieHeader("path=safe; name=admin")
	// Both should be redacted as they're cookie names in request context
	if !strings.Contains(result, "path="+Redacted) {
		t.Errorf("path cookie name not redacted: %s", result)
	}
	if !strings.Contains(result, "name="+Redacted) {
		t.Errorf("name cookie not redacted: %s", result)
	}
}

func TestRedactRequestCookieHeader_DomainCookieName(t *testing.T) {
	// "domain" is an attribute name in Set-Cookie, but NOT in request cookies
	result := RedactRequestCookieHeader("domain=credential")
	if !strings.Contains(result, "domain="+Redacted) {
		t.Errorf("domain cookie name not redacted: %s", result)
	}
}

func TestRedactRequestCookieHeader_DuplicateNames(t *testing.T) {
	result := RedactRequestCookieHeader("token=one; token=two")
	// Both instances should be redacted
	if !strings.Contains(result, "token="+Redacted) {
		t.Errorf("Duplicate cookie names not redacted: %s", result)
	}
}

func TestRedactRequestCookieHeader_MalformedPair(t *testing.T) {
	result := RedactRequestCookieHeader("sessionvalue")
	// No = sign, should still be handled
	if result == "" {
		t.Errorf("Malformed cookie without = should be handled")
	}
}

func TestRedactRequestCookieHeader_EmptyPair(t *testing.T) {
	result := RedactRequestCookieHeader("session=; other=test")
	if !strings.Contains(result, "session="+Redacted) {
		t.Errorf("Empty cookie value not redacted: %s", result)
	}
}

func TestRedactRequestCookieHeader_EmptyInput(t *testing.T) {
	result := RedactRequestCookieHeader("")
	if result != "" {
		t.Errorf("Empty input should return empty: %q", result)
	}
}

func TestRedactRequestCookieHeader_Idempotence(t *testing.T) {
	input := "session=abc; token=xyz"
	first := RedactRequestCookieHeader(input)
	second := RedactRequestCookieHeader(first)
	if first != second {
		t.Errorf("RedactRequestCookieHeader not idempotent: first=%q, second=%q", first, second)
	}
}

// ============================================================================
// Set-Cookie Header Tests
// ============================================================================

func TestRedactSetCookieHeader_NormalCookie(t *testing.T) {
	result := RedactSetCookieHeader("session=abc123; Path=/; HttpOnly; Secure")
	if !strings.Contains(result, "session="+Redacted) {
		t.Errorf("Cookie value not redacted: %s", result)
	}
	if !strings.Contains(result, "Path=/") {
		t.Errorf("Path attribute not preserved: %s", result)
	}
	if !strings.Contains(result, "HttpOnly") {
		t.Errorf("HttpOnly attribute not preserved: %s", result)
	}
}

func TestRedactSetCookieHeader_CookieNamedPath(t *testing.T) {
	// "Path" in FIRST position is the cookie NAME, not an attribute
	result := RedactSetCookieHeader("Path=credential; Secure")
	// Should redact the value
	if !strings.Contains(result, "Path="+Redacted) {
		t.Errorf("Cookie named Path should have value redacted: got %q", result)
	}
	// Secure attribute should be preserved
	if !strings.Contains(result, "Secure") {
		t.Errorf("Secure attribute should be preserved: got %q", result)
	}
}

func TestRedactSetCookieHeader_CookieNamedDomain(t *testing.T) {
	// "Domain" in FIRST position is the cookie NAME, not an attribute
	result := RedactSetCookieHeader("Domain=credential; Secure")
	// Should redact the value
	if !strings.Contains(result, "Domain="+Redacted) {
		t.Errorf("Cookie named Domain should have value redacted: got %q", result)
	}
	// Secure attribute should be preserved
	if !strings.Contains(result, "Secure") {
		t.Errorf("Secure attribute should be preserved: got %q", result)
	}
}

func TestRedactSetCookieHeader_CookieNamedSameSite(t *testing.T) {
	result := RedactSetCookieHeader("SameSite=credential")
	if result != "SameSite="+Redacted {
		t.Errorf("Cookie named SameSite should have value redacted: got %q, want %q", result, "SameSite="+Redacted)
	}
}

func TestRedactSetCookieHeader_NormalPathAttribute(t *testing.T) {
	// "Path" AFTER the first segment is an attribute
	result := RedactSetCookieHeader("token=abc; Path=/admin; Secure")
	if !strings.Contains(result, "Path=/admin") {
		t.Errorf("Normal Path attribute should be preserved: %s", result)
	}
}

func TestRedactSetCookieHeader_ExpiresAttribute(t *testing.T) {
	result := RedactSetCookieHeader("session=abc; Expires=Thu, 01 Jan 2030 00:00:00 GMT")
	if !strings.Contains(result, "Expires=Thu, 01 Jan 2030 00:00:00 GMT") {
		t.Errorf("Expires attribute should be preserved: %s", result)
	}
}

func TestRedactSetCookieHeader_UnknownValueAttribute(t *testing.T) {
	result := RedactSetCookieHeader("session=abc; UnknownAttr=secret; Secure")
	if !strings.Contains(result, "UnknownAttr="+Redacted) {
		t.Errorf("Unknown attribute value should be redacted: %s", result)
	}
	if !strings.Contains(result, "Secure") {
		t.Errorf("Secure boolean attribute should be preserved: %s", result)
	}
}

func TestRedactSetCookieHeader_UnknownBareAttribute(t *testing.T) {
	result := RedactSetCookieHeader("session=abc; UnknownFlag; Secure")
	// Unknown bare attributes are dropped (no explicit policy)
	if strings.Contains(result, "UnknownFlag") {
		t.Errorf("Unknown bare attribute should be dropped: %s", result)
	}
	if !strings.Contains(result, "Secure") {
		t.Errorf("Secure boolean attribute should be preserved: %s", result)
	}
}

func TestRedactSetCookieHeader_MalformedFirstSegment(t *testing.T) {
	// No = sign in first segment
	result := RedactSetCookieHeader("sessionvalue; Path=/")
	if result != Redacted {
		t.Errorf("Malformed first segment should return Redacted: got %q", result)
	}
}

func TestRedactSetCookieHeader_EmptyFirstSegment(t *testing.T) {
	// Empty segments are skipped, first non-empty segment is treated as cookie
	result := RedactSetCookieHeader("; Path=/")
	// Path=/ is the first non-empty segment, but it has no = so it's malformed
	if result == "" || result == "; Path=/" {
		t.Errorf("Empty leading segment should be handled: got %q", result)
	}
}

func TestRedactSetCookieHeader_EmptyCookieName(t *testing.T) {
	result := RedactSetCookieHeader("=value; Path=/")
	if result != Redacted {
		t.Errorf("Empty cookie name should return Redacted: got %q", result)
	}
}

func TestRedactSetCookieHeader_EmptySegments(t *testing.T) {
	result := RedactSetCookieHeader("session=abc; ; ; Path=/")
	if !strings.Contains(result, "session="+Redacted) {
		t.Errorf("Empty segments should be skipped: %s", result)
	}
}

func TestRedactSetCookieHeader_DuplicateAttributes(t *testing.T) {
	result := RedactSetCookieHeader("session=abc; Path=/first; Path=/second; Secure")
	// First Path attribute is preserved, second is unknown (gets redacted)
	if !strings.Contains(result, "Path=/first") {
		t.Errorf("First Path attribute should be preserved: %s", result)
	}
}

func TestRedactSetCookieHeader_MultipleSetCookieValues(t *testing.T) {
	// Test with duplicate cookie names
	result := RedactSetCookieHeader("session=abc; session=xyz; Path=/")
	if !strings.Contains(result, "session="+Redacted) {
		t.Errorf("Cookie value should be redacted: %s", result)
	}
}

func TestRedactSetCookieHeader_Idempotence(t *testing.T) {
	input := "session=abc123; Path=/; HttpOnly; Secure"
	first := RedactSetCookieHeader(input)
	second := RedactSetCookieHeader(first)
	if first != second {
		t.Errorf("RedactSetCookieHeader not idempotent: first=%q, second=%q", first, second)
	}
}

func TestRedactSetCookieHeader_EmptyInput(t *testing.T) {
	result := RedactSetCookieHeader("")
	if result != "" {
		t.Errorf("Empty input should return empty: %q", result)
	}
}

func TestRedactSetCookieHeader_RecognizedAttributeInvalidForm(t *testing.T) {
	// Path= (no value) - recognized boolean-like attribute with invalid form
	// This gets preserved as-is since eqIdx > 0 but the value is empty
	result := RedactSetCookieHeader("session=abc; Path=; Secure")
	// Path= is preserved with empty value (not redacted further)
	if !strings.Contains(result, "session="+Redacted) {
		t.Errorf("session value should be redacted: %s", result)
	}
}
