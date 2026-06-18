package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultDiagTimeoutMs     = 1500
	DefaultDiagCooldownSeconds = 90
	DefaultDiagMaxCaptures    = 10
)

type DiagnosticsConfig struct {
	Enabled         bool           `json:"enabled"`
	CaptureOnSpike  bool           `json:"capture_on_spike"`
	TimeoutMs       int            `json:"timeout_ms"`
	CooldownSeconds int            `json:"cooldown_seconds"`
	Peers           []DiagPeerConfig `json:"peers"`
}

type DiagPeerConfig struct {
	Name     string   `json:"name"`
	BaseURL  string   `json:"base_url"`
	Targets  []string `json:"targets"`
}

func (d *DiagnosticsConfig) ApplyDefaults() {
	if d.TimeoutMs <= 0 {
		d.TimeoutMs = DefaultDiagTimeoutMs
	}
	if d.CooldownSeconds <= 0 {
		d.CooldownSeconds = DefaultDiagCooldownSeconds
	}
}

func (d *DiagnosticsConfig) Validate() error {
	if !d.Enabled {
		return nil
	}

	if len(d.Peers) == 0 {
		return fmt.Errorf("diagnostics enabled but no peers configured")
	}

	for i, peer := range d.Peers {
		if err := ValidateDiagPeerBaseURL(peer.BaseURL); err != nil {
			return fmt.Errorf("peer[%d] %s: %w", i, peer.Name, err)
		}
		if peer.Name == "" {
			return fmt.Errorf("peer[%d]: name is required", i)
		}
		if len(peer.Targets) == 0 {
			return fmt.Errorf("peer[%d] %s: at least one target is required", i, peer.Name)
		}
	}
	return nil
}

func ValidateDiagPeerBaseURL(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("base_url is required")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Require scheme
	if u.Scheme == "" {
		return fmt.Errorf("scheme is required (http or https)")
	}

	// Only allow http/https
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %s", u.Scheme)
	}

	// Require host
	if u.Host == "" {
		return fmt.Errorf("host is required")
	}

	// Reject userinfo (no username/password)
	if u.User != nil {
		return fmt.Errorf("userinfo (username/password) is not allowed")
	}

	return nil
}

func DiagPeerStatusURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/status"
}

func (d *DiagnosticsConfig) TargetToDiagPeers() map[string]*DiagPeerConfig {
	result := make(map[string]*DiagPeerConfig)
	for i := range d.Peers {
		peer := &d.Peers[i]
		for _, target := range peer.Targets {
			result[target] = peer
		}
	}
	return result
}
