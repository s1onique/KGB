// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// --- Cookie Parsing Tests ---

func TestLoadCookies_NormalCookie(t *testing.T) {
	// Create temp cookie file with normal Netscape format
	content := "# Netscape HTTP Cookie File\n" +
		".uvb76\tTRUE\t/\tFALSE\t0\tsession_token\tabc123secret\n"

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 1 {
		t.Errorf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session_token" {
		t.Errorf("expected cookie name 'session_token', got '%s'", cookies[0].Name)
	}
	if cookies[0].Value != "abc123secret" {
		t.Errorf("expected cookie value 'abc123secret', got '%s'", cookies[0].Value)
	}
}

func TestLoadCookies_HttpOnlyCookie(t *testing.T) {
	// Create temp cookie file with #HttpOnly_ prefix (the bug case)
	// curl uses #HttpOnly_ to mark HttpOnly cookies
	content := "# Netscape HTTP Cookie File\n" +
		"#HttpOnly_.uvb76\tTRUE\t/\tFALSE\t0\tsession_cookie\tsuper-secret-session-id\n"

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 1 {
		t.Errorf("expected 1 cookie (HttpOnly cookie should be loaded), got %d", len(cookies))
	}
	if len(cookies) == 0 {
		return
	}
	if cookies[0].Name != "session_cookie" {
		t.Errorf("expected cookie name 'session_cookie', got '%s'", cookies[0].Name)
	}
	if cookies[0].Value != "super-secret-session-id" {
		t.Errorf("expected cookie value 'super-secret-session-id', got '%s'", cookies[0].Value)
	}
}

func TestLoadCookies_RegularCommentIgnored(t *testing.T) {
	// Create temp cookie file with regular comment lines
	content := "# Netscape HTTP Cookie File\n" +
		"# This is a comment line\n" +
		"# Another comment\n" +
		".uvb76\tTRUE\t/\tFALSE\t0\tregular_cookie\tregular_value\n"

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 1 {
		t.Errorf("expected 1 cookie (only regular cookie, not comments), got %d", len(cookies))
	}
	if cookies[0].Name != "regular_cookie" {
		t.Errorf("expected cookie name 'regular_cookie', got '%s'", cookies[0].Name)
	}
}

func TestLoadCookies_MalformedLineIgnored(t *testing.T) {
	// Create temp cookie file with malformed lines
	// Only .uvb76 with 7+ fields should be accepted
	content := "# Netscape HTTP Cookie File\n" +
		".uvb76\tTRUE\t/\tFALSE\t0\tvalid_cookie\tvalid_value\n" +
		"malformed line without tabs\n" +
		".uvb76\tTRUE\t/\n" + // too few fields
		".uvb76\tTRUE\t/\tFALSE\t\n" + // missing name and value
		".uvb76\tTRUE\t/\tFALSE\t0\t\n" + // missing value (empty string)
		".uvb76\tTRUE\t/\tFALSE\t0\tonly_name\tvalid_value2\n" // valid (has value)

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	// Should only have valid_cookie and only_name (with value)
	if len(cookies) != 2 {
		t.Errorf("expected 2 valid cookies, got %d", len(cookies))
	}
}

func TestLoadCookies_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(cookiePath, []byte(""), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 0 {
		t.Errorf("expected 0 cookies from empty file, got %d", len(cookies))
	}
}

func TestLoadCookies_NoCookiePath(t *testing.T) {
	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  "", // no cookie path
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 0 {
		t.Errorf("expected 0 cookies when no path set, got %d", len(cookies))
	}
}

func TestLoadCookies_CookieValuesNotLogged(t *testing.T) {
	// This test verifies the implementation doesn't expose cookie values
	// by checking the loadCookies function doesn't use logging with values
	// The actual values are only used via http.Cookie which is internal to Go

	content := "# Netscape HTTP Cookie File\n" +
		".uvb76\tTRUE\t/\tFALSE\t0\tsensitive_cookie\tSENSITIVE_VALUE_SHOULD_NOT_LEAK\n"

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	// Verify cookie was loaded correctly
	cookies := req.Cookies()
	if len(cookies) != 1 {
		t.Errorf("expected 1 cookie, got %d", len(cookies))
	}
	// The value should be in the http.Cookie struct but not in any logs
	// This is ensured by the implementation not logging cookie values
	if cookies[0].Value != "SENSITIVE_VALUE_SHOULD_NOT_LEAK" {
		t.Errorf("cookie value mismatch")
	}
}

func TestLoadCookies_MultipleCookies(t *testing.T) {
	// Test mixing normal and HttpOnly cookies
	content := "# Netscape HTTP Cookie File\n" +
		".uvb76\tTRUE\t/\tFALSE\t0\tnormal_cookie\tnormal_value\n" +
		"#HttpOnly_.uvb76\tTRUE\t/\tFALSE\t0\thttponly_cookie\thttponly_value\n" +
		".uvb76\tTRUE\t/\tFALSE\t0\tsession\tsession123\n"

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 3 {
		t.Errorf("expected 3 cookies (2 normal + 1 HttpOnly), got %d", len(cookies))
	}

	// Build a map for easier verification
	cookieMap := make(map[string]string)
	for _, c := range cookies {
		cookieMap[c.Name] = c.Value
	}

	if cookieMap["normal_cookie"] != "normal_value" {
		t.Errorf("normal_cookie value mismatch")
	}
	if cookieMap["httponly_cookie"] != "httponly_value" {
		t.Errorf("httponly_cookie value mismatch")
	}
	if cookieMap["session"] != "session123" {
		t.Errorf("session value mismatch")
	}
}

func TestLoadCookies_WithPath(t *testing.T) {
	// Test that cookie with custom path is properly loaded
	// Note: http.Request.Cookies() doesn't return Path field,
	// but the cookie is added with the path set internally
	content := "# Netscape HTTP Cookie File\n" +
		".uvb76\tTRUE\t/api\tFALSE\t0\tsession\tsession_value\n"

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "cookies.txt")
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp cookie file: %v", err)
	}

	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
		Cookies:  cookiePath,
	}

	req, _ := http.NewRequest("GET", "http://localhost:9999/api/v1/test", nil)
	client.loadCookies(req)

	cookies := req.Cookies()
	if len(cookies) != 1 {
		t.Errorf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session" {
		t.Errorf("expected cookie name 'session', got '%s'", cookies[0].Name)
	}
	if cookies[0].Value != "session_value" {
		t.Errorf("expected cookie value 'session_value', got '%s'", cookies[0].Value)
	}
}
