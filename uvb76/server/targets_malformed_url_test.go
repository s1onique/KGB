// Package server provides tests for malformed URL validation.
package server

import (
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

// TestValidateDiagPeerBaseURL_MalformedURLs tests that validation rejects
// malformed base_urls including query strings and fragments.
func TestValidateDiagPeerBaseURL_MalformedURLs(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{"empty", "", "base_url is required"},
		{"query string", "http://host:8317?foo=bar", "query string is not allowed"},
		{"fragment", "http://host:8317#section", "fragment is not allowed"},
		{"userinfo username", "http://user@host:8317", "userinfo"},
		{"userinfo password", "http://user:pass@host:8317", "userinfo"},
		{"missing scheme", "host:8317", "scheme"},
		{"invalid scheme", "ftp://host:8317", "scheme must be http or https"},
		{"missing host", "http://", "host is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateDiagPeerBaseURL(tt.baseURL)
			if err == nil {
				t.Errorf("Expected error for %q, got nil", tt.baseURL)
				return
			}
			if tt.wantErr != "" && !contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
