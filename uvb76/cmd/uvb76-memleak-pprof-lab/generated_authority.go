// Package main provides the UVB-76 pprof memory leak lab.
//
// # Generated Configuration Authority
//
// This file implements P0-1: Generated lab authority bundle.
// The GeneratedLabAuthority is the sole runtime authority for target identity,
// URLs, authentication, and execution mode after configuration generation.
//
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/s1onique/KGB/uvb76/config"
)

// ExecutionMode represents the lab execution mode.
type ExecutionMode string

const (
	ExecutionModeReal ExecutionMode = "real"
	ExecutionModeFake ExecutionMode = "fake"
)

// TargetConfigBinding represents the canonical target binding extracted from
// the generated configuration.
type TargetConfigBinding struct {
	TargetID        string
	BaseURL         string
	StatusURL       string
	DiagnosticsPeer string
}

// TargetStateAuthInput represents the explicit authentication for target-state queries.
// P0-7: Authentication must be supplied explicitly, not read from environment.
type TargetStateAuthInput struct {
	SessionCookie string
}

// GeneratedLabAuthority is the sole runtime authority for the lab execution.
// P0-1: Created exactly once before process startup.
// P0-2: No package globals, no flag pointers, no ambient environment access.
type GeneratedLabAuthority struct {
	Config          *GeneratedConfig
	ConfigPath      string
	Mode            ExecutionMode
	Target          TargetConfigBinding
	UVB76APIBaseURL string
	TargetStateAuth TargetStateAuthInput
}

// GeneratedConfig represents the validated lab configuration.
// P0-4: Strict reread must prove semantic equality to this.
type GeneratedConfig config.Config

// ConfigTargetConfig is an alias for the production config type.
type ConfigTargetConfig = config.TargetConfig

// ConfigListenConfig is an alias for the production config type.
type ConfigListenConfig = config.ListenConfig

// ConfigAuthConfig is an alias for the production config type.
type ConfigAuthConfig = config.AuthConfig

// ConfigScrapeConfig is an alias for the production config type.
type ConfigScrapeConfig = config.ScrapeConfig

// ConfigLatencyConfig is an alias for the production config type.
type ConfigLatencyConfig = config.LatencyConfig

// ConfigDiagnosticsConfig is an alias for the production config type.
type ConfigDiagnosticsConfig = config.DiagnosticsConfig

// ConfigDiagPeerConfig is an alias for the production config type.
type ConfigDiagPeerConfig = config.DiagPeerConfig

// Sentinel errors for generated configuration authority.

// ErrGeneratedConfigNil is returned when the generated config is nil.
var ErrGeneratedConfigNil = errors.New("generated config is nil")

// ErrGeneratedConfigInvalid is returned when the generated config fails validation.
var ErrGeneratedConfigInvalid = errors.New("generated config invalid")

// ErrGeneratedConfigWrite is returned when config file write fails.
var ErrGeneratedConfigWrite = errors.New("generated config write failed")

// ErrGeneratedConfigRead is returned when config file read fails.
var ErrGeneratedConfigRead = errors.New("generated config read failed")

// ErrGeneratedConfigDecode is returned when config file decode fails.
var ErrGeneratedConfigDecode = errors.New("generated config decode failed")

// ErrGeneratedConfigTrailingContent is returned when config has trailing content.
var ErrGeneratedConfigTrailingContent = errors.New("generated config has trailing content")

// ErrGeneratedConfigMismatch is returned when reread config doesn't match generated.
var ErrGeneratedConfigMismatch = errors.New("generated config mismatch")

// ErrTargetConfigMissing is returned when no target is configured.
var ErrTargetConfigMissing = errors.New("target config missing")

// ErrTargetConfigAmbiguous is returned when multiple targets are selected.
var ErrTargetConfigAmbiguous = errors.New("target config ambiguous")

// ErrTargetConfigDisabled is returned when selected target is disabled.
var ErrTargetConfigDisabled = errors.New("target config disabled")

// ErrTargetConfigDuplicateID is returned when target IDs are not unique.
var ErrTargetConfigDuplicateID = errors.New("target config duplicate ID")

// ErrTargetConfigEmptyID is returned when target ID is empty.
var ErrTargetConfigEmptyID = errors.New("target config empty ID")

// ErrTargetConfigEmptyBaseURL is returned when target base URL is empty.
var ErrTargetConfigEmptyBaseURL = errors.New("target config empty base URL")

// ErrTargetPeerBindingMissing is returned when peer binding is missing.
var ErrTargetPeerBindingMissing = errors.New("target peer binding missing")

// ErrTargetPeerBindingMismatch is returned when peer doesn't match target.
var ErrTargetPeerBindingMismatch = errors.New("target peer binding mismatch")

// ErrModeTargetMismatch is returned when mode and target don't align.
var ErrModeTargetMismatch = errors.New("mode target mismatch")

// ErrTargetPollTransport is returned on transport errors during polling.
var ErrTargetPollTransport = errors.New("target poll transport error")

// ErrTargetPollUnauthorized is returned on 401 during polling.
var ErrTargetPollUnauthorized = errors.New("target poll unauthorized")

// ErrTargetPollForbidden is returned on 403 during polling.
var ErrTargetPollForbidden = errors.New("target poll forbidden")

// ErrTargetPollUnexpectedStatus is returned on unexpected HTTP status.
var ErrTargetPollUnexpectedStatus = errors.New("target poll unexpected status")

// ErrTargetPollDecode is returned when snapshot decode fails.
var ErrTargetPollDecode = errors.New("target poll decode failed")

// ErrTargetPollTrailingContent is returned when response has trailing content.
var ErrTargetPollTrailingContent = errors.New("target poll trailing content")

// ErrTargetPollTargetNotFound is returned when target doesn't exist.
var ErrTargetPollTargetNotFound = errors.New("target poll target not found")

// ErrTargetPollNoObservation is returned when no observation before deadline.
var ErrTargetPollNoObservation = errors.New("target poll no observation")

// ErrTargetPollNoCompletion is returned when no completion before deadline.
var ErrTargetPollNoCompletion = errors.New("target poll no completion")

// ErrTargetPollDeadline is returned when polling deadline expires.
var ErrTargetPollDeadline = errors.New("target poll deadline")

// ErrTargetIdentityMismatch is returned when snapshot identity doesn't match.
var ErrTargetIdentityMismatch = errors.New("target identity mismatch")

// ErrStatusFieldMissing is returned when a mandatory status field is absent.
var ErrStatusFieldMissing = errors.New("status field missing")

// ErrStatusFieldDuplicate is returned when a status field appears twice.
var ErrStatusFieldDuplicate = errors.New("status field duplicate")

// ErrStatusFieldParse is returned when a status field fails to parse.
var ErrStatusFieldParse = errors.New("status field parse error")

// ErrSmapsFieldMissing is returned when a mandatory smaps field is absent.
var ErrSmapsFieldMissing = errors.New("smaps field missing")

// ErrSmapsFieldDuplicate is returned when a smaps field appears twice.
var ErrSmapsFieldDuplicate = errors.New("smaps field duplicate")

// ErrSmapsFieldParse is returned when a smaps field fails to parse.
var ErrSmapsFieldParse = errors.New("smaps field parse error")

// ErrFDDirectoryRead is returned when FD directory cannot be read.
var ErrFDDirectoryRead = errors.New("fd directory read error")

// ErrProcessDisappeared is returned when process disappears during collection.
var ErrProcessDisappeared = errors.New("process disappeared")

// ErrGoroutineTransport is returned on transport errors fetching goroutines.
var ErrGoroutineTransport = errors.New("goroutine transport error")

// ErrGoroutineUnexpectedStatus is returned on non-200 fetching goroutines.
var ErrGoroutineUnexpectedStatus = errors.New("goroutine unexpected status")

// ErrGoroutineBodyTooLarge is returned when goroutine dump exceeds limit.
var ErrGoroutineBodyTooLarge = errors.New("goroutine body too large")

// ErrGoroutineRead is returned when goroutine dump read fails.
var ErrGoroutineRead = errors.New("goroutine read error")

// ErrGoroutineParse is returned when goroutine dump parse fails.
var ErrGoroutineParse = errors.New("goroutine parse error")

// ErrGoroutineObservationEmpty is returned when goroutine dump is empty.
var ErrGoroutineObservationEmpty = errors.New("goroutine observation empty")

// ErrAuthInputMissing is returned when authentication input is missing.
var ErrAuthInputMissing = errors.New("auth input missing")

// ErrURLInvalid is returned when a URL fails validation.
var ErrURLInvalid = errors.New("URL invalid")

// ErrURLScheme is returned when URL has wrong scheme.
var ErrURLScheme = errors.New("URL scheme invalid")

// ErrURLHost is returned when URL host is absent.
var ErrURLHost = errors.New("URL host absent")

// ErrURLPort is returned when URL port is absent.
var ErrURLPort = errors.New("URL port absent")

// ErrURLUserinfo is returned when URL contains userinfo.
var ErrURLUserinfo = errors.New("URL userinfo present")

// ErrURLQuery is returned when URL contains query.
var ErrURLQuery = errors.New("URL query present")

// ErrURLFragment is returned when URL contains fragment.
var ErrURLFragment = errors.New("URL fragment present")

// ErrURLNotLoopback is returned when URL host is not loopback.
var ErrURLNotLoopback = errors.New("URL host not loopback")

// ValidateGeneratedConfig validates the complete generated configuration.
// P0-3: Must be called before any process startup.
// P0-4: Returns typed errors compatible with errors.Is.
func ValidateGeneratedConfig(cfg *GeneratedConfig, mode ExecutionMode) error {
	if cfg == nil {
		return ErrGeneratedConfigNil
	}

	// Validate exactly one target
	if len(cfg.Targets) == 0 {
		return ErrTargetConfigMissing
	}

	// Find selected target (enabled target)
	var selectedTarget *ConfigTargetConfig
	for i, t := range cfg.Targets {
		if t.Enabled {
			if selectedTarget != nil {
				return fmt.Errorf("%w: multiple enabled targets", ErrTargetConfigAmbiguous)
			}
			selectedTarget = &cfg.Targets[i]
		}
	}

	if selectedTarget == nil {
		return fmt.Errorf("%w: no enabled target", ErrTargetConfigMissing)
	}

	// Validate target ID
	if selectedTarget.ID == "" {
		return ErrTargetConfigEmptyID
	}

	// Validate no duplicate target IDs
	seenIDs := make(map[string]bool)
	for _, t := range cfg.Targets {
		if seenIDs[t.ID] {
			return fmt.Errorf("%w: duplicate ID %q", ErrTargetConfigDuplicateID, t.ID)
		}
		seenIDs[t.ID] = true
	}

	// Validate target base URL
	if selectedTarget.BaseURL == "" {
		return fmt.Errorf("%w: target %q", ErrTargetConfigEmptyBaseURL, selectedTarget.ID)
	}

	// P0-5: Validate target URL format
	if err := ValidateLabURL(selectedTarget.BaseURL); err != nil {
		return fmt.Errorf("target base URL validation: %w", err)
	}

	// Validate mode matches target
	switch mode {
	case ExecutionModeReal:
		if selectedTarget.ID != "real-tovarisch" {
			return fmt.Errorf("%w: expected real-tovarisch, got %q", ErrModeTargetMismatch, selectedTarget.ID)
		}
	case ExecutionModeFake:
		if selectedTarget.ID != "fake-tovarisch" {
			return fmt.Errorf("%w: expected fake-tovarisch, got %q", ErrModeTargetMismatch, selectedTarget.ID)
		}
	}

	// Validate diagnostics peer binding
	if len(cfg.Diagnostics.Peers) == 0 {
		return fmt.Errorf("%w: no diagnostics peers", ErrTargetPeerBindingMissing)
	}

	// Find peer that monitors the selected target
	var foundPeer bool
	for _, peer := range cfg.Diagnostics.Peers {
		for _, targetID := range peer.Targets {
			if targetID == selectedTarget.ID {
				foundPeer = true
				// Validate peer base URL matches target base URL
				if peer.BaseURL != selectedTarget.BaseURL {
					return fmt.Errorf("%w: peer %q base URL %q != target base URL %q",
						ErrTargetPeerBindingMismatch, peer.Name, peer.BaseURL, selectedTarget.BaseURL)
				}
				break
			}
		}
	}

	if !foundPeer {
		return fmt.Errorf("%w: no peer monitors target %q", ErrTargetPeerBindingMissing, selectedTarget.ID)
	}

	return nil
}

// ExtractTargetBinding extracts the canonical target binding from validated config.
// P0-2: Target binding comes from config, not reconstructed from flags.
func ExtractTargetBinding(cfg *GeneratedConfig, mode ExecutionMode) (TargetConfigBinding, error) {
	if cfg == nil {
		return TargetConfigBinding{}, ErrGeneratedConfigNil
	}

	// Find selected target
	var selectedTarget *ConfigTargetConfig
	for i, t := range cfg.Targets {
		if t.Enabled {
			selectedTarget = &cfg.Targets[i]
			break
		}
	}

	if selectedTarget == nil {
		return TargetConfigBinding{}, ErrTargetConfigMissing
	}

	// Find the peer that monitors this target
	var peerName string
	for _, peer := range cfg.Diagnostics.Peers {
		for _, targetID := range peer.Targets {
			if targetID == selectedTarget.ID {
				peerName = peer.Name
				break
			}
		}
		if peerName != "" {
			break
		}
	}

	// P0-5: Derive status URL from canonical base URL
	baseURL := selectedTarget.BaseURL
	statusURL := strings.TrimSuffix(baseURL, "/") + "/status"

	return TargetConfigBinding{
		TargetID:        selectedTarget.ID,
		BaseURL:         baseURL,
		StatusURL:       statusURL,
		DiagnosticsPeer: peerName,
	}, nil
}

// DeriveUVB76APIBaseURL derives the canonical UVB-76 API base URL from config.
// P0-5: Uses structured URL resolution, not string concatenation.
func DeriveUVB76APIBaseURL(cfg *GeneratedConfig) (string, error) {
	if cfg == nil {
		return "", ErrGeneratedConfigNil
	}

	// Parse the listen address
	listenAddr := cfg.Listen.Addr
	if listenAddr == "" {
		return "", fmt.Errorf("listen address empty")
	}

	// Construct canonical API base URL
	// Listen.Addr is like "localhost:18444"
	apiBase := "http://" + listenAddr
	return apiBase, nil
}

// BuildSnapshotURL builds the canonical target snapshot URL from authority.
// P0-5: Uses structured URL resolution, not string concatenation.
func BuildSnapshotURL(apiBase, targetID string) string {
	return apiBase + "/api/v1/targets/" + targetID + "/snapshot"
}

// ValidateLabURL validates that a URL meets hermetic lab requirements.
// P0-5: Canonical URL must have:
// - scheme: http
// - absolute: true
// - host present
// - explicit port present
// - no userinfo
// - no query
// - no fragment
// - host is loopback or localhost
func ValidateLabURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: empty URL", ErrURLInvalid)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: parse error: %v", ErrURLInvalid, err)
	}

	// Check scheme
	if u.Scheme != "http" {
		return fmt.Errorf("%w: expected http, got %q", ErrURLScheme, u.Scheme)
	}

	// Check absolute
	if !u.IsAbs() {
		return fmt.Errorf("%w: URL is not absolute", ErrURLInvalid)
	}

	// Check host present
	if u.Host == "" {
		return fmt.Errorf("%w: host is absent", ErrURLHost)
	}

	// Check explicit port present
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrURLHost, err)
	}
	if port == "" {
		return fmt.Errorf("%w: explicit port required", ErrURLPort)
	}

	// Check no userinfo
	if u.User != nil {
		return ErrURLUserinfo
	}

	// Check no query
	if u.RawQuery != "" {
		return ErrURLQuery
	}

	// Check no fragment
	if u.Fragment != "" {
		return ErrURLFragment
	}

	// Check host is loopback
	if !isLoopback(host) {
		return fmt.Errorf("%w: host %q is not loopback", ErrURLNotLoopback, host)
	}

	return nil
}

// splitHostPort splits a host:port string.
func splitHostPort(hostPort string) (host, port string, err error) {
	// Handle IPv6 addresses in brackets
	if len(hostPort) >= 2 && hostPort[0] == '[' {
		end := strings.Index(hostPort, "]")
		if end == -1 {
			return "", "", fmt.Errorf("unclosed bracket")
		}
		host = hostPort[:end+1]
		rest := hostPort[end+1:]
		if len(rest) > 0 && rest[0] == ':' {
			port = rest[1:]
		}
		return host, port, nil
	}

	// Handle regular host:port
	for i := len(hostPort) - 1; i >= 0; i-- {
		if hostPort[i] == ':' {
			return hostPort[:i], hostPort[i+1:], nil
		}
	}
	return hostPort, "", nil
}

// isLoopback returns true if the host is a loopback address.
func isLoopback(host string) bool {
	// Handle IPv6 loopback
	if host == "::1" {
		return true
	}
	// Handle IPv4 loopback
	if host == "127.0.0.1" {
		return true
	}
	// Handle localhost
	if host == "localhost" {
		return true
	}
	// Handle localhost with port stripped
	hostOnly, _, _ := splitHostPort(host)
	if hostOnly == "localhost" || hostOnly == "127.0.0.1" || hostOnly == "::1" {
		return true
	}
	return false
}

// StrictlyReadConfig reads and decodes a config file with strict validation.
// P0-4: Rejects unknown fields, malformed JSON, multiple documents, trailing content.
func StrictlyReadConfig(path string) (*GeneratedConfig, error) {
	f, err := openForReading(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeneratedConfigRead, err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()

	var cfg GeneratedConfig
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeneratedConfigDecode, err)
	}

	// P0-4: Require exactly one JSON document
	var trailingCheck map[string]interface{}
	if err := decoder.Decode(&trailingCheck); err != io.EOF {
		if err == nil {
			return nil, ErrGeneratedConfigTrailingContent
		}
		return nil, fmt.Errorf("%w: %v", ErrGeneratedConfigTrailingContent, err)
	}

	return &cfg, nil
}

// openForReading opens a file for reading, used for testing.
var openForReading = func(path string) (interface {
	io.ReadCloser
}, error) {
	return os.Open(path)
}

// ProveConfigEquality proves that reread config equals generated config.
// P0-3: Returns error if configs are not semantically equal.
func ProveConfigEquality(generated, reread *GeneratedConfig) error {
	if generated == nil {
		return fmt.Errorf("%w: generated config is nil", ErrGeneratedConfigMismatch)
	}
	if reread == nil {
		return fmt.Errorf("%w: reread config is nil", ErrGeneratedConfigMismatch)
	}

	// Compare target count
	if len(generated.Targets) != len(reread.Targets) {
		return fmt.Errorf("%w: target count mismatch", ErrGeneratedConfigMismatch)
	}

	// Compare targets
	for i, gt := range generated.Targets {
		rt := reread.Targets[i]
		if gt.ID != rt.ID {
			return fmt.Errorf("%w: target[%d] ID mismatch", ErrGeneratedConfigMismatch, i)
		}
		if gt.BaseURL != rt.BaseURL {
			return fmt.Errorf("%w: target[%d] BaseURL mismatch", ErrGeneratedConfigMismatch, i)
		}
		if gt.Enabled != rt.Enabled {
			return fmt.Errorf("%w: target[%d] Enabled mismatch", ErrGeneratedConfigMismatch, i)
		}
	}

	// Compare listen address
	if generated.Listen.Addr != reread.Listen.Addr {
		return fmt.Errorf("%w: listen addr mismatch", ErrGeneratedConfigMismatch)
	}

	// Compare diagnostics peers
	if len(generated.Diagnostics.Peers) != len(reread.Diagnostics.Peers) {
		return fmt.Errorf("%w: peer count mismatch", ErrGeneratedConfigMismatch)
	}

	for i, gp := range generated.Diagnostics.Peers {
		rp := reread.Diagnostics.Peers[i]
		if gp.Name != rp.Name {
			return fmt.Errorf("%w: peer[%d] name mismatch", ErrGeneratedConfigMismatch, i)
		}
		if gp.BaseURL != rp.BaseURL {
			return fmt.Errorf("%w: peer[%d] BaseURL mismatch", ErrGeneratedConfigMismatch, i)
		}
	}

	return nil
}

// ProveTargetBindingEquality proves that reread binding equals generated binding.
func ProveTargetBindingEquality(generated, reread TargetConfigBinding) error {
	if generated.TargetID != reread.TargetID {
		return fmt.Errorf("%w: TargetID mismatch", ErrGeneratedConfigMismatch)
	}
	if generated.BaseURL != reread.BaseURL {
		return fmt.Errorf("%w: BaseURL mismatch", ErrGeneratedConfigMismatch)
	}
	if generated.StatusURL != reread.StatusURL {
		return fmt.Errorf("%w: StatusURL mismatch", ErrGeneratedConfigMismatch)
	}
	if generated.DiagnosticsPeer != reread.DiagnosticsPeer {
		return fmt.Errorf("%w: DiagnosticsPeer mismatch", ErrGeneratedConfigMismatch)
	}
	return nil
}
