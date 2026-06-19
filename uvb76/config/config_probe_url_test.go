package config

import (
	"testing"
)

func TestTargetProbeURL_ExplicitProbeURL(t *testing.T) {
	// When probe_url is set, it should be used directly
	tcfg := &TargetConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "http://example.com:8080",
		ProbeURL: "http://example.com:8080/lab/probe",
		Enabled: true,
	}

	result := TargetProbeURL(tcfg)
	expected := "http://example.com:8080/lab/probe"
	if result != expected {
		t.Errorf("TargetProbeURL() = %q, want %q", result, expected)
	}
}

func TestTargetProbeURL_EmptyProbeURL_FallsBackToStatus(t *testing.T) {
	// When probe_url is empty, fall back to base_url + /status
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{"simple", "http://example.com", "http://example.com/status"},
		{"with port", "http://example.com:8080", "http://example.com:8080/status"},
		{"with path", "http://example.com:8080/api", "http://example.com:8080/api/status"},
		{"with trailing slash", "http://example.com/", "http://example.com/status"},
		{"with path and trailing slash", "http://example.com:8080/api/", "http://example.com:8080/api/status"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tcfg := &TargetConfig{
				ID:      "test",
				Name:    "Test",
				BaseURL: tc.baseURL,
				// ProbeURL intentionally empty
				Enabled: true,
			}

			result := TargetProbeURL(tcfg)
			if result != tc.expected {
				t.Errorf("TargetProbeURL() with baseURL=%q = %q, want %q", tc.baseURL, result, tc.expected)
			}
		})
	}
}

func TestTargetProbeURL_LabConfigScenario(t *testing.T) {
	// This is the exact lab config scenario from the capture netns lab:
	// base_url = "http://10.88.76.2:8317" (for diagnostics)
	// probe_url = "http://10.88.76.2:8317/lab/probe" (for HTTP probing)
	
	tcfg := &TargetConfig{
		ID:      "lab-tovarisch",
		Name:    "Lab Tovarisch",
		BaseURL: "http://10.88.76.2:8317",
		ProbeURL: "http://10.88.76.2:8317/lab/probe",
		Enabled: true,
	}

	probeURL := TargetProbeURL(tcfg)
	expected := "http://10.88.76.2:8317/lab/probe"
	if probeURL != expected {
		t.Errorf("TargetProbeURL() = %q, want %q", probeURL, expected)
	}

	// Verify it does NOT return /lab/probe/status (the bug that was fixed)
	if probeURL == "http://10.88.76.2:8317/lab/probe/status" {
		t.Errorf("TargetProbeURL() incorrectly returned path-joined URL: %q", probeURL)
	}
}

func TestTargetStatusURL_LegacyFallbackBehavior(t *testing.T) {
	// TargetStatusURL is the LEGACY fallback for backward compatibility.
	// It appends /status to base_url, which is the exact footgun that caused
	// the lab bug (base_url = "/lab/probe" → probe = "/lab/probe/status" → 404).
	//
	// DO NOT use TargetStatusURL for explicit probe endpoints.
	// Use probe_url field + TargetProbeURL() instead.
	tests := []struct {
		baseURL  string
		expected string
	}{
		{"http://example.com", "http://example.com/status"},
		{"http://example.com:8080", "http://example.com:8080/status"},
		// This is the legacy footgun behavior - base_url with path + /status
		{"http://example.com:8317/lab/probe", "http://example.com:8317/lab/probe/status"},
	}

	for _, tc := range tests {
		result := TargetStatusURL(tc.baseURL)
		if result != tc.expected {
			t.Errorf("TargetStatusURL(%q) = %q, want %q", tc.baseURL, result, tc.expected)
		}
	}
}
