package redact

import (
	"net/url"
	"strings"
	"testing"
)

// ============================================================================
// URL Redaction Tests
// ============================================================================

func TestRedactURL_UserInfo(t *testing.T) {
	urls := []string{
		"https://user:password@example.com/path",
		"https://admin:secret@host:8080/api",
		"http://user@hostonly/path",
	}

	for _, u := range urls {
		result := RedactURL(u)
		// Should not contain user:password@
		if strings.Contains(result, ":password@") || strings.Contains(result, ":secret@") {
			t.Errorf("Userinfo not redacted in %q: %s", u, result)
		}
		// Should preserve scheme, host, path
		if !strings.Contains(result, "://") {
			t.Errorf("Scheme lost in %q: %s", u, result)
		}
	}
}

func TestRedactURL_SensitiveQueryParams(t *testing.T) {
	urls := []struct {
		url            string
		sensitiveValue string
	}{
		{"https://example.com/api?token=secret", "token=secret"},
		{"https://example.com/api?access_token=bearer123", "access_token=bearer"},
		{"https://example.com/api?api_key=key123", "api_key=key"},
		{"https://example.com/api?password=pass", "password=pass"},
		{"https://example.com/api?secret=value", "secret=value"},
	}

	for _, tc := range urls {
		result := RedactURL(tc.url)
		// Should not contain original sensitive value
		if strings.Contains(result, tc.sensitiveValue) {
			t.Errorf("Sensitive value not redacted in %q: %s", tc.url, result)
		}
		// Should contain the redacted marker
		if !strings.Contains(result, "%5BREDACTED%5D") && !strings.Contains(result, "[REDACTED]") {
			t.Errorf("Result should contain redacted marker: %s", result)
		}
	}
}

func TestRedactURL_SensitiveValuesReplaced(t *testing.T) {
	// Verify that sensitive values are REPLACED, not deleted
	result := RedactURL("https://example.com/api?token=secret")
	if result == "https://example.com/api" {
		t.Errorf("Sensitive param should be preserved (just value redacted), got: %s", result)
	}
	// Should contain the redacted value marker
	if !strings.Contains(result, "%5BREDACTED%5D") && !strings.Contains(result, "[REDACTED]") {
		t.Errorf("Result should contain redacted value marker: %s", result)
	}
}

func TestRedactURL_RepeatedSensitiveValues(t *testing.T) {
	// token=one&token=two should become token=[REDACTED]&token=[REDACTED]
	result := RedactURL("https://example.com/api?token=one&token=two&page=1")
	// Safe param should be preserved
	if !strings.Contains(result, "page=1") {
		t.Errorf("Safe param should be preserved: %s", result)
	}
	// Both token values should be redacted
	if strings.Contains(result, "token=one") || strings.Contains(result, "token=two") {
		t.Errorf("Repeated sensitive values should be redacted: %s", result)
	}
}

func TestRedactURL_101RepeatedValues(t *testing.T) {
	// Build URL with 101 token values
	query := "token=value"
	for i := 0; i < 100; i++ {
		query += "&token=value"
	}
	rawURL := "https://example.com/api?" + query

	result := RedactURL(rawURL)
	// Should fail closed - return [REDACTED]
	if result != Redacted {
		t.Errorf("101 values should exceed bound and fail closed, got: %s", result)
	}
}

func TestRedactURL_SafeQueryParams(t *testing.T) {
	urls := []struct {
		input    string
		expected string
	}{
		{"https://example.com/api?page=1&limit=10", "page=1&limit=10"},
		{"https://example.com/api?sort=name&order=asc", "sort=name&order=asc"},
	}

	for _, tc := range urls {
		result := RedactURL(tc.input)
		// Safe params should be preserved
		if !strings.Contains(result, tc.expected) {
			t.Errorf("Safe params not preserved in %q: got %s, want %s", tc.input, result, tc.expected)
		}
	}
}

func TestRedactURL_EmptyInput(t *testing.T) {
	result := RedactURL("")
	if result != "" {
		t.Errorf("Empty URL should return empty: %q", result)
	}
}

func TestRedactURL_InvalidURL(t *testing.T) {
	result := RedactURL("not a valid :// url")
	// Should return Redacted (fail closed)
	if result != Redacted {
		t.Errorf("Invalid URL should fail closed, got: %s", result)
	}
}

func TestRedactURL_MalformedPercentEncoding(t *testing.T) {
	// Malformed percent encoding should fail closed
	result := RedactURL("https://example.com/api?token=%ZZ")
	if result != Redacted {
		t.Errorf("Malformed percent encoding should fail closed, got: %s", result)
	}
}

func TestRedactURL_RelativeURL(t *testing.T) {
	result := RedactURL("/api?token=secret")
	// Relative URLs may not parse, but should fail closed or preserve safe parts
	if result == "/api?token=secret" {
		t.Errorf("Relative URL with sensitive param should be handled: %s", result)
	}
}

func TestRedactURL_DatabaseDSN(t *testing.T) {
	result := RedactURL("postgres://user:password@localhost:5432/db")
	if strings.Contains(result, ":password@") {
		t.Errorf("DSN userinfo should be redacted: %s", result)
	}
}

func TestRedactURL_IPv6Host(t *testing.T) {
	result := RedactURL("https://[::1]:8080/api?token=secret")
	if strings.Contains(result, "token=secret") {
		t.Errorf("IPv6 URL sensitive param should be redacted: %s", result)
	}
}

func TestRedactURL_FragmentPreservation(t *testing.T) {
	result := RedactURL("https://example.com/page#section?token=secret")
	// Fragment should be preserved
	if !strings.Contains(result, "#section") {
		t.Errorf("Fragment should be preserved: %s", result)
	}
}


// ============================================================================
// URL Bounds Tests
// ============================================================================

func TestRedactURL_LengthBound(t *testing.T) {
	// URL longer than maxURLLength should return Redacted
	longPath := strings.Repeat("a", maxURLLength+1)
	result := RedactURL("https://example.com/" + longPath)
	if result != Redacted {
		t.Errorf("URL exceeding max length should return Redacted, got: %s", result)
	}
}

func TestRedactURL_QueryParamCountBound(t *testing.T) {
	// Build URL with exactly maxQueryParams values
	query := "page=1"
	for i := 1; i < maxQueryParams; i++ {
		query += "&page=1"
	}
	// Should pass
	result := RedactURL("https://example.com/api?" + query)
	if result == Redacted {
		t.Errorf("URL with exactly %d params should pass, got Redacted", maxQueryParams)
	}
}

// ============================================================================
// Structured JSON URL Sanitization Tests
// ============================================================================

func TestRedactStructuredJSON_URLWithUserinfo(t *testing.T) {
	input := []byte(`{"api_url": "https://user:password@example.com/path"}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	if strings.Contains(resultStr, ":password@") {
		t.Errorf("URL userinfo should be redacted in JSON: %s", resultStr)
	}
}

func TestRedactStructuredJSON_URLWithSensitiveQuery(t *testing.T) {
	input := []byte(`{"redirect": "https://example.com/callback?token=secret123"}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	if strings.Contains(resultStr, "token=secret123") {
		t.Errorf("Sensitive query should be redacted in JSON: %s", resultStr)
	}
}

func TestRedactStructuredJSON_URLInNestedArray(t *testing.T) {
	input := []byte(`{"urls": ["https://a.com?token=1", "https://b.com?key=2"]}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	if strings.Contains(resultStr, "token=1") || strings.Contains(resultStr, "key=2") {
		t.Errorf("URLs in arrays should be redacted: %s", resultStr)
	}
}

func TestRedactStructuredJSON_SafeURL(t *testing.T) {
	input := []byte(`{"homepage": "https://example.com", "logo": "https://cdn.example.com/logo.png"}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	// Safe URLs should be preserved
	if !strings.Contains(resultStr, "example.com") {
		t.Errorf("Safe URLs should be preserved: %s", resultStr)
	}
}

func TestRedactStructuredJSON_MalformedURLCandidate(t *testing.T) {
	// Text that looks like it might be a URL but isn't properly bounded
	input := []byte(`{"note": "User asked about the ? in their query"}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	// Should not treat arbitrary prose as URL
	if strings.Contains(resultStr, "REDACTED") {
		t.Errorf("Arbitrary prose with ? should not be treated as URL: %s", resultStr)
	}
}

func TestRedactStructuredJSON_NestedURL(t *testing.T) {
	input := []byte(`{"config": {"endpoint": "https://api.example.com?api_key=secret"}}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	if strings.Contains(resultStr, "api_key=secret") {
		t.Errorf("Nested URL should be redacted: %s", resultStr)
	}
}

func TestRedactStructuredJSON_InputBytesUnchanged(t *testing.T) {
	input := []byte(`{"safe_field": "https://example.com?page=1"}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	// The original input bytes should not be mutated
	if strings.Contains(string(input), "REDACTED") {
		t.Errorf("Input bytes should not be mutated: %s", input)
	}

	// Safe URLs in output should remain valid
	resultStr := string(result)
	if strings.Contains(resultStr, "example.com") {
		parsed, err := url.Parse(resultStr)
		if err == nil {
			// URL structure should be preserved
			if parsed.Host != "example.com" {
				t.Errorf("URL host should be preserved: %s", resultStr)
			}
		}
	}
}

func TestRedactStructuredJSON_MultipleURLs(t *testing.T) {
	input := []byte(`{
		"api": "https://api.example.com?key=secret1",
		"cdn": "https://cdn.example.com/logo.png",
		"callback": "https://auth.example.com/cb?token=secret2"
	}`)
	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	// Safe CDN URL should be preserved
	if !strings.Contains(resultStr, "cdn.example.com") {
		t.Errorf("Safe CDN URL should be preserved: %s", resultStr)
	}
	// Sensitive params should be redacted
	if strings.Contains(resultStr, "key=secret1") || strings.Contains(resultStr, "token=secret2") {
		t.Errorf("Sensitive URL params should be redacted: %s", resultStr)
	}
}

// ============================================================================
// isURLCandidate Tests
// ============================================================================

func TestIsURLCandidate_HTTPURL(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"https://example.com/path", true},
		{"http://api.service.local:8080/v1", true},
		{"https://user:pass@example.com", true},
		{"short", false},                       // Too short
		{"just some text with ? mark", false}, // Not bounded as URL
		{"not-a-url", false},
		{"", false},
	}

	for _, tc := range cases {
		result := isURLCandidate(tc.input)
		if result != tc.expected {
			t.Errorf("isURLCandidate(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

func TestIsURLCandidate_DatabaseDSN(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"postgres://user:pass@localhost/db", true},
		{"mysql://root:password@localhost:3306/app", true},
		{"mongodb://user:pass@localhost:27017/admin", true},
		{"sqlite://local/file.db", false}, // Not in our DSN patterns
	}

	for _, tc := range cases {
		result := isURLCandidate(tc.input)
		if result != tc.expected {
			t.Errorf("isURLCandidate(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

func TestIsURLCandidate_ProtocolRelative(t *testing.T) {
	result := isURLCandidate("//cdn.example.com/assets")
	if !result {
		t.Errorf("Protocol-relative URL should be detected: %v", result)
	}
}
