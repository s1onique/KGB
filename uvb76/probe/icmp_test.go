package probe

import (
	"testing"
)

func TestParsePingTime(t *testing.T) {
	for _, fixture := range ParsePingFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			ms, err := ParsePingTime(fixture.output)
			if fixture.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", fixture.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if ms != fixture.wantMs {
				t.Errorf("want %f ms, got %f ms", fixture.wantMs, ms)
			}
		})
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		wantHost string
	}{
		{
			name:     "http with IP and port",
			baseURL:  "http://10.149.149.1:8317/status",
			wantHost: "10.149.149.1",
		},
		{
			name:     "https with domain and port",
			baseURL:  "https://example.com:8443/status",
			wantHost: "example.com",
		},
		{
			name:     "http localhost",
			baseURL:  "http://localhost:8080/api",
			wantHost: "localhost",
		},
		{
			name:     "https with subdomain",
			baseURL:  "https://api.vps.example.com:9443/health",
			wantHost: "api.vps.example.com",
		},
		{
			name:     "no port",
			baseURL:  "http://192.168.1.1/status",
			wantHost: "192.168.1.1",
		},
		{
			name:     "empty URL",
			baseURL:  "",
			wantHost: "",
		},
		{
			name:     "invalid URL",
			baseURL:  "not-a-url",
			wantHost: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHost(tt.baseURL)
			if got != tt.wantHost {
				t.Errorf("extractHost(%q) = %q, want %q", tt.baseURL, got, tt.wantHost)
			}
		})
	}
}
