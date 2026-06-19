// Package config provides configuration loading and validation for UVB-76.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// TargetStatusURL returns the status diagnostic URL for a target base URL.
// This is the URL used for diagnostic capture, NOT for latency probing.
// The actual status endpoint fetched by diagnostics is /status (without .json suffix).
// UVB-76 probe uses TargetProbeURL() instead for latency measurements.
func TargetStatusURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/status"
}

// TargetProbeURL returns the effective HTTP probe URL for a target.
// Priority:
// 1. If TargetConfig.ProbeURL is set and non-empty, use it directly (explicit probe URL)
// 2. Otherwise, fall back to TargetStatusURL(baseURL) which appends /status
//
// This allows labs to probe /lab/probe (returns 503 during defect) while
// diagnostics still fetch /status (always healthy).
func TargetProbeURL(t *TargetConfig) string {
	if t.ProbeURL != "" {
		return t.ProbeURL
	}
	return TargetStatusURL(t.BaseURL)
}

// Config represents the full configuration for UVB-76.
type Config struct {
	Listen       ListenConfig        `json:"listen"`
	Auth         AuthConfig         `json:"auth"`
	Scrape       ScrapeConfig       `json:"scrape"`
	Latency      LatencyConfig      `json:"latency"`
	Targets      []TargetConfig     `json:"targets"`
	Diagnostics  DiagnosticsConfig   `json:"diagnostics"`
}

// ListenConfig holds HTTP server settings.
type ListenConfig struct {
	Addr        string `json:"addr"`
	TLSCertFile string `json:"tls_cert_file"`
	TLSKeyFile  string `json:"tls_key_file"`
}

// AuthConfig holds authentication credentials.
type AuthConfig struct {
	Username      string `json:"username"`
	PasswordSHA256 string `json:"password_sha256"`
}

// ScrapeConfig holds scraper timing settings.
type ScrapeConfig struct {
	IntervalSeconds     int `json:"interval_seconds"`
	TimeoutMilliseconds int `json:"timeout_milliseconds"`
}

// TargetConfig represents a single tovarisch target.
type TargetConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	// ProbeURL is the explicit HTTP probe endpoint for this target.
	// If set, this URL is used directly for HTTP probing (latency measurement).
	// If empty, the probe uses base_url + "/status" (backward compatible default).
	// This allows probing a different path than diagnostics (/status).
	// Example: probe_url = "http://host:8317/lab/probe" while diagnostics uses base_url.
	ProbeURL string `json:"probe_url,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// Validation errors.
var (
	ErrEmptyListenAddr            = errors.New("listen.addr is required")
	ErrEmptyTLSCert               = errors.New("listen.tls_cert_file is required")
	ErrEmptyTLSKey                = errors.New("listen.tls_key_file is required")
	ErrEmptyUsername              = errors.New("auth.username is required")
	ErrEmptyPasswordSHA256        = errors.New("auth.password_sha256 is required")
	ErrEmptyPasswordFormat        = errors.New("auth.password_sha256 must be in format sha256:<salt>:<hex>")
	ErrInvalidPasswordFormat      = errors.New("auth.password_sha256 format must be sha256:<salt>:<hex>")
	ErrInvalidSaltLength          = errors.New("auth.password_sha256 salt must be 16 bytes (32 hex chars)")
	ErrInvalidHashLength          = errors.New("auth.password_sha256 hash must be 32 bytes (64 hex chars)")
	ErrInvalidSaltHex             = errors.New("auth.password_sha256 salt must be valid hex")
	ErrInvalidHashHex             = errors.New("auth.password_sha256 hash must be valid hex")
	ErrInvalidInterval            = errors.New("scrape.interval_seconds must be > 0")
	ErrInvalidTimeout             = errors.New("scrape.timeout_milliseconds must be > 0")
	ErrEmptyTargetID              = errors.New("target.id is required")
	ErrEmptyTargetName            = errors.New("target.name is required")
	ErrEmptyTargetBaseURL         = errors.New("target.base_url is required")
	ErrInvalidTargetBaseURLScheme = errors.New("target.base_url must use http:// or https:// scheme")
	ErrDuplicateTargetID          = errors.New("duplicate target.id found")
)

// ValidationOptions controls validation behavior.
type ValidationOptions struct {
	AllowMissingTLS bool // Allow empty TLS cert/key (dev mode)
}

// Load reads a JSON config file and validates it.
func Load(path string) (*Config, error) {
	return LoadWithOptions(path, ValidationOptions{})
}

// LoadWithOptions reads a JSON config file and validates it with options.
func LoadWithOptions(path string, opts ValidationOptions) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if err := cfg.Validate(opts); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks that all required fields are present and valid.
func (c *Config) Validate(opts ValidationOptions) error {
	// Listen validation
	if c.Listen.Addr == "" {
		return ErrEmptyListenAddr
	}
	// TLS validation (skip in dev mode)
	if !opts.AllowMissingTLS {
		if c.Listen.TLSCertFile == "" {
			return ErrEmptyTLSCert
		}
		if c.Listen.TLSKeyFile == "" {
			return ErrEmptyTLSKey
		}
	}

	// Auth validation - reject empty auth config (fail-closed)
	if c.Auth.Username == "" {
		return ErrEmptyUsername
	}
	if c.Auth.PasswordSHA256 == "" {
		return ErrEmptyPasswordSHA256
	}
	if err := ValidatePasswordHashFormat(c.Auth.PasswordSHA256); err != nil {
		return err
	}

	// Scrape validation
	if c.Scrape.IntervalSeconds <= 0 {
		return ErrInvalidInterval
	}
	if c.Scrape.TimeoutMilliseconds <= 0 {
		return ErrInvalidTimeout
	}

	// Target validation
	seenIDs := make(map[string]bool)
	for i, t := range c.Targets {
		if t.ID == "" {
			return fmt.Errorf("%w: index %d", ErrEmptyTargetID, i)
		}
		if t.Name == "" {
			return fmt.Errorf("%w: index %d", ErrEmptyTargetName, i)
		}
		if t.BaseURL == "" {
			return fmt.Errorf("%w: index %d", ErrEmptyTargetBaseURL, i)
		}
		if err := ValidateTargetBaseURLScheme(t.BaseURL); err != nil {
			return fmt.Errorf("%w: index %d: %v", ErrInvalidTargetBaseURLScheme, i, err)
		}
		if seenIDs[t.ID] {
			return fmt.Errorf("%w: %s", ErrDuplicateTargetID, t.ID)
		}
		seenIDs[t.ID] = true
	}

	// Apply diagnostics defaults and validate
	c.Diagnostics.ApplyDefaults()
	if err := c.Diagnostics.Validate(); err != nil {
		return fmt.Errorf("diagnostics validation failed: %w", err)
	}

	return nil
}

// ValidateTargetBaseURLScheme validates that a target base_url uses http:// or https:// scheme.
// Returns an error describing the issue if the scheme is invalid or missing.
func ValidateTargetBaseURLScheme(baseURL string) error {
	// Check for empty baseURL first (handled elsewhere, but defensive)
	if baseURL == "" {
		return errors.New("base_url is empty")
	}

	// Check for missing scheme (no "://" in URL)
	if !strings.Contains(baseURL, "://") {
		return errors.New("missing scheme (must include ://)")
	}

	// Extract scheme (everything before "://")
	schemeEnd := strings.Index(baseURL, "://")
	scheme := strings.ToLower(baseURL[:schemeEnd])

	// Accept only http and https
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (must use http:// or https://)", scheme)
	}

	return nil
}

// ValidatePasswordHashFormat checks that a password hash matches sha256:<32-hex-salt>:<64-hex-hash>.
func ValidatePasswordHashFormat(hash string) error {
	if hash == "" {
		return ErrEmptyPasswordFormat
	}
	prefix := "sha256:"
	if len(hash) < len(prefix)+3 {
		return ErrInvalidPasswordFormat
	}
	if hash[:len(prefix)] != prefix {
		return ErrInvalidPasswordFormat
	}

	// Parse parts: sha256:<salt-hex>:<hash-hex>
	remaining := hash[len(prefix):]
	parts := strings.SplitN(remaining, ":", 2)
	if len(parts) != 2 {
		return ErrInvalidPasswordFormat
	}
	saltHex := parts[0]
	hashHex := parts[1]

	// Salt must be exactly 32 hex chars (16 bytes)
	if len(saltHex) != 32 {
		return ErrInvalidSaltLength
	}
	// Hash must be exactly 64 hex chars (32 bytes)
	if len(hashHex) != 64 {
		return ErrInvalidHashLength
	}

	// Validate hex encoding for salt
	if _, err := hexDecode(saltHex); err != nil {
		return ErrInvalidSaltHex
	}
	// Validate hex encoding for hash
	if _, err := hexDecode(hashHex); err != nil {
		return ErrInvalidHashHex
	}

	return nil
}

// hexDecode validates and decodes hex string. Returns error if invalid.
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("odd length")
	}
	decoded := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi := hexChar(s[i])
		lo := hexChar(s[i+1])
		if hi < 0 || lo < 0 {
			return nil, errors.New("invalid hex")
		}
		decoded[i/2] = byte(hi<<4) | byte(lo)
	}
	return decoded, nil
}

// hexChar converts a hex char to its numeric value. Returns -1 if invalid.
func hexChar(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c - 'a' + 10)
	case c >= 'A' && c <= 'F':
		return int(c - 'A' + 10)
	default:
		return -1
	}
}
