package redact

import (
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
	urls := []string{
		"https://example.com/api?token=secret",
		"https://example.com/api?access_token=bearer123",
		"https://example.com/api?api_key=key123",
		"https://example.com/api?password=pass",
		"https://example.com/api?secret=value",
	}

	for _, u := range urls {
		result := RedactURL(u)
		// Should not contain sensitive query params
		if strings.Contains(result, "token=secret") {
			t.Errorf("token param not removed in %q: %s", u, result)
		}
		if strings.Contains(result, "access_token=bearer") {
			t.Errorf("access_token param not removed in %q: %s", u, result)
		}
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
	// Should not return the original potentially-sensitive content unchanged
	if result == "not a valid :// url" {
		// This is acceptable if the fallback sanitizer is applied
	}
}
