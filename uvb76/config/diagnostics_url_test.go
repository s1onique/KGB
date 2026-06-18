package config

import "testing"

func TestDiagPeerStatusURL_Basic(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{
			name:     "simple host port",
			baseURL:  "http://host:8317",
			expected: "http://host:8317/status?include=network_diag",
		},
		{
			name:     "trailing slash trimmed",
			baseURL:  "http://host:8317/",
			expected: "http://host:8317/status?include=network_diag",
		},
		{
			name:     "with base path",
			baseURL:  "http://host:8317/api",
			expected: "http://host:8317/api/status?include=network_diag",
		},
		{
			name:     "with base path and trailing slash",
			baseURL:  "http://host:8317/api/",
			expected: "http://host:8317/api/status?include=network_diag",
		},
		{
			name:     "https scheme",
			baseURL:  "https://secure.host:8443",
			expected: "https://secure.host:8443/status?include=network_diag",
		},
		{
			name:     "IP address",
			baseURL:  "http://10.149.149.1:8317",
			expected: "http://10.149.149.1:8317/status?include=network_diag",
		},
		{
			name:     "localhost",
			baseURL:  "http://localhost:8080",
			expected: "http://localhost:8080/status?include=network_diag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiagPeerStatusURL(tt.baseURL)
			if result != tt.expected {
				t.Errorf("DiagPeerStatusURL(%q) = %q, want %q", tt.baseURL, result, tt.expected)
			}
		})
	}
}

func TestDiagPeerStatusURL_QueryOrder(t *testing.T) {
	// URL params may be reordered by net/url, verify include param is present
	result := DiagPeerStatusURL("http://host:8317")
	
	// Check that include=network_diag is in the result
	if result == "" {
		t.Error("DiagPeerStatusURL returned empty string")
	}
	
	// The URL should end with include=network_diag or contain it
	found := false
	for _, param := range []string{
		"include=network_diag",
		"include=network_diag&",
		"&include=network_diag",
	} {
		if len(result) > len(param) && result[len(result)-len(param):] == param {
			found = true
			break
		}
		// Also check contains
		if contains(result, param) {
			found = true
			break
		}
	}
	
	if !found {
		t.Errorf("DiagPeerStatusURL result %q does not contain expected param", result)
	}
}

func TestDiagPeerStatusURL_ConstantExported(t *testing.T) {
	// Verify the constant is exported and has expected value
	if DiagPeerStatusInclude != "network_diag" {
		t.Errorf("DiagPeerStatusInclude = %q, want %q", DiagPeerStatusInclude, "network_diag")
	}
}

func TestValidateDiagPeerBaseURL_UserinfoRejected(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"username only", "http://user@host:8317", true},
		{"username password", "http://user:pass@host:8317", true},
		{"valid URL", "http://host:8317", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDiagPeerBaseURL(tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDiagPeerBaseURL(%q) error = %v, wantErr %v", tt.baseURL, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDiagPeerBaseURL_QueryRejected(t *testing.T) {
	// Query strings in base_url should be rejected by validation
	err := ValidateDiagPeerBaseURL("http://host:8317?foo=bar")
	if err == nil {
		t.Error("expected error for base_url with query string")
	}
}

// contains is a simple helper for string containment check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
