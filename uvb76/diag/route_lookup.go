// Package diag implements diagnostic capture for UVB-76.
// This file provides route lookup with native NETLINK_ROUTE backend.
// CLI fallback is available behind the UseCLIFallback seam.
package diag

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// UseCLIFallback is a build-tag controlled seam for CLI fallback.
// When true, the RouteCollector uses CLI composition as the primary path.
// When false (default), native NETLINK_ROUTE is the primary path with CLI fallback.
// This can be set in tests or when native netlink is unavailable.
var UseCLIFallback = false

// RouteSource indicates which backend was used for a route lookup.
type RouteSource string

const (
	RouteSourceNative   RouteSource = "native_netlink"
	RouteSourceCLIFallback RouteSource = "cli_fallback"
)

// RouteLookupParser parses `ip route get` output into structured ProbeRoute.
type RouteLookupParser struct{}

// NewRouteLookupParser creates a new route lookup parser.
func NewRouteLookupParser() *RouteLookupParser {
	return &RouteLookupParser{}
}

// ParseResult contains the result of parsing route output.
type ParseResult struct {
	RouteType string
	Interface string
	SourceIP  string
	Gateway   string
	Table     string
	MTU       *int
	UID       *int
}

// ParseRouteGetOutput parses the output of `ip route get <target>`.
// It handles various output formats and returns structured fields.
func (p *RouteLookupParser) ParseRouteGetOutput(target string, rawOutput string, redact bool) (*ParseResult, error) {
	output := strings.TrimSpace(rawOutput)
	if output == "" {
		return nil, ErrRouteNoData
	}
	result := &ParseResult{}
	normalized := normalizeSpaces(output)
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return nil, ErrRouteMalformed
	}
	firstToken := strings.ToLower(parts[0])
	switch firstToken {
	case "unreachable":
		result.RouteType = "unreachable"
		result.Interface = extractInterfaceField(normalized)
		return result, nil
	case "local":
		result.RouteType = "local"
		result.Interface = extractInterfaceField(normalized)
		result.SourceIP = extractSourceField(normalized, redact)
		return result, nil
	case "broadcast", "multicast", "throw", "blackhole", "prohibit":
		result.RouteType = firstToken
		return result, nil
	default:
		result.RouteType = "unicast"
	}
	result.Interface = extractInterfaceField(normalized)
	result.SourceIP = extractSourceField(normalized, redact)
	result.Gateway = extractGatewayField(normalized, redact)
	result.Table = extractTableField(normalized)
	result.MTU = extractMTUField(normalized)
	result.UID = extractUIDField(normalized)
	return result, nil
}

func extractInterfaceField(output string) string {
	re := regexp.MustCompile(`\bdev\s+(\S+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func extractSourceField(output string, redact bool) string {
	re := regexp.MustCompile(`\bsrc\s+(\S+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		if redact {
			return "redacted"
		}
		return matches[1]
	}
	return ""
}

func extractGatewayField(output string, redact bool) string {
	re := regexp.MustCompile(`\bvia\s+(\S+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		if redact {
			return "redacted"
		}
		return matches[1]
	}
	return ""
}

func extractTableField(output string) string {
	re := regexp.MustCompile(`\btable\s+(\S+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func extractMTUField(output string) *int {
	re := regexp.MustCompile(`\bmtu\s+(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		if mtu, err := parseInt(matches[1]); err == nil {
			return &mtu
		}
	}
	return nil
}

func extractUIDField(output string) *int {
	re := regexp.MustCompile(`\buid\s+(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		if uid, err := parseInt(matches[1]); err == nil && uid != 0 {
			return &uid
		}
	}
	return nil
}

func normalizeSpaces(s string) string {
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}

func parseInt(s string) (int, error) {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}

// Route lookup errors
var (
	ErrRouteNoData    = &RouteParseError{Kind: state.RouteLookupErrorNoData, Message: "empty route output"}
	ErrRouteMalformed = &RouteParseError{Kind: state.RouteLookupErrorParseFailed, Message: "malformed route output"}
)

// RouteParseError represents a route parsing error.
type RouteParseError struct {
	Kind    state.RouteLookupErrorKind
	Message string
}

func (e *RouteParseError) Error() string { return e.Message }

// Default timeout for route lookups.
const DefaultRouteLookupTimeout = 2 * time.Second

// RouteCollector collects route lookup evidence for diagnostic packets.
type RouteCollector struct {
	parser  *RouteLookupParser
	timeout time.Duration
}

// NewRouteCollector creates a new route collector with default settings.
func NewRouteCollector() *RouteCollector {
	return &RouteCollector{
		parser:  NewRouteLookupParser(),
		timeout: DefaultRouteLookupTimeout,
	}
}

// CollectRouteLookup performs a route lookup for the given probe kind and target.
// It uses native NETLINK_ROUTE by default, with CLI fallback behind the UseCLIFallback seam.
// This preserves the existing evidence shape while providing native performance.
func (c *RouteCollector) CollectRouteLookup(ctx context.Context, probeKind state.ProbeRouteKind, target string, resolvedIP string) *state.ProbeRoute {
	now := time.Now().UTC()
	route := &state.ProbeRoute{
		Kind:        probeKind,
		ProbeHost:   target,
		ResolvedIP:  resolvedIP,
		Ok:          false,
		CollectedAt: now.Format(time.RFC3339),
	}
	lookupTarget := resolvedIP
	if lookupTarget == "" {
		lookupTarget = target
	}
	route.LookupTarget = lookupTarget
	if !isValidRouteTarget(lookupTarget) {
		route.ErrorKind = state.RouteLookupErrorUnavailable
		route.Error = "invalid route lookup target"
		return route
	}

	// Try native NETLINK_ROUTE first (unless UseCLIFallback is set)
	if UseCLIFallback {
		return c.collectViaCLI(ctx, route, lookupTarget)
	}
	return c.collectViaNative(ctx, route, lookupTarget)
}

// collectViaNative performs route lookup using native NETLINK_ROUTE.
func (c *RouteCollector) collectViaNative(ctx context.Context, route *state.ProbeRoute, lookupTarget string) *state.ProbeRoute {
	// Check if parent context is already expired/canceled before starting native lookup
	if ctx.Err() != nil {
		route.ErrorKind = state.RouteLookupErrorTimeout
		route.Error = "route lookup context already expired"
		return route
	}
	// Derive a child timeout from the parent context
	childCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	nlResult := RouteLookupNative(childCtx, lookupTarget)
	if nlResult == nil {
		route.ErrorKind = state.RouteLookupErrorUnavailable
		route.Error = "native route lookup returned nil"
		return route
	}

	if nlResult.Error != nil {
		// Native failed - try CLI fallback if this is a recoverable error
		if c.shouldFallbackToCLI(nlResult.Error) {
			return c.collectViaCLI(ctx, route, lookupTarget)
		}
		route.ErrorKind = c.mapNetlinkErrorToKind(nlResult.Error)
		route.Error = nlResult.Error.Message
		return route
	}

	// Success - convert to ProbeRoute fields
	fields := RouteLookupNetlinkResultToProbeRoute(nlResult, true)
	route.Ok = true
	route.RouteType = fields.RouteType
	route.Interface = fields.Interface
	route.SourceIP = fields.SourceIP
	route.Gateway = fields.Gateway
	route.Table = fields.Table
	return route
}

// shouldFallbackToCLI determines if we should fall back to CLI after native failure.
func (c *RouteCollector) shouldFallbackToCLI(nlErr *NetlinkError) bool {
	// Only fallback for errors that might be platform-specific
	switch nlErr.Kind {
	case "open_failed", "permission", "no_route":
		return true
	default:
		return false
	}
}

// mapNetlinkErrorToKind maps netlink errors to RouteLookupErrorKind.
func (c *RouteCollector) mapNetlinkErrorToKind(nlErr *NetlinkError) state.RouteLookupErrorKind {
	switch nlErr.Kind {
	case "open_failed", "bind_failed", "send_failed", "receive_failed":
		return state.RouteLookupErrorUnavailable
	case "timeout":
		return state.RouteLookupErrorTimeout
	case "permission":
		return state.RouteLookupErrorUnavailable
	case "no_route":
		return state.RouteLookupErrorUnavailable
	case "malformed":
		return state.RouteLookupErrorParseFailed
	case "invalid_target":
		return state.RouteLookupErrorUnavailable
	default:
		return state.RouteLookupErrorUnavailable
	}
}

// collectViaCLI performs route lookup using CLI composition (ip route get).
// This is the legacy fallback path.
func (c *RouteCollector) collectViaCLI(ctx context.Context, route *state.ProbeRoute, lookupTarget string) *state.ProbeRoute {
	// Always derive a child timeout from the parent context to ensure bounded execution.
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result := runRouteCommand(cmdCtx, lookupTarget)
	if cmdCtx.Err() == context.DeadlineExceeded {
		route.ErrorKind = state.RouteLookupErrorTimeout
		route.Error = "route lookup timed out"
		return route
	}
	if cmdCtx.Err() == context.Canceled {
		route.ErrorKind = state.RouteLookupErrorTimeout
		route.Error = "route lookup canceled"
		return route
	}
	if result.CommandNotFound {
		route.ErrorKind = state.RouteLookupErrorCommandMissing
		route.Error = "route command not found"
		return route
	}
	if result.ExitCode != 0 {
		route.ErrorKind = state.RouteLookupErrorNonZeroExit
		stderr := strings.TrimSpace(result.Stderr)
		if strings.Contains(stderr, "Nexthop has no gateway") ||
			strings.Contains(stderr, "No route to host") ||
			strings.Contains(stderr, "Network is unreachable") {
			route.ErrorKind = state.RouteLookupErrorUnavailable
			route.Error = "no route to host"
		} else {
			route.Error = "route lookup failed"
		}
		return route
	}
	rawOutput := strings.TrimSpace(result.Stdout)
	if rawOutput == "" {
		route.ErrorKind = state.RouteLookupErrorNoData
		route.Error = "empty route output"
		return route
	}
	parseResult, err := c.parser.ParseRouteGetOutput(lookupTarget, rawOutput, true)
	if err != nil {
		if parseErr, ok := err.(*RouteParseError); ok {
			route.ErrorKind = parseErr.Kind
		} else {
			route.ErrorKind = state.RouteLookupErrorParseFailed
		}
		route.Error = err.Error()
		return route
	}
	route.Ok = true
	route.RouteType = parseResult.RouteType
	route.Interface = parseResult.Interface
	route.SourceIP = parseResult.SourceIP
	route.Gateway = parseResult.Gateway
	route.Table = parseResult.Table
	route.MTU = parseResult.MTU
	route.UID = parseResult.UID
	return route
}

type routeCommandResult struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	CommandNotFound bool
}

func runRouteCommand(ctx context.Context, target string) routeCommandResult {
	cmd := exec.CommandContext(ctx, "ip", "route", "get", target)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return routeCommandResult{ExitCode: -1, Stderr: "stdout pipe failed"}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		return routeCommandResult{ExitCode: -1, Stderr: "stderr pipe failed"}
	}
	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		if errors.Is(err, exec.ErrNotFound) {
			return routeCommandResult{ExitCode: -1, Stderr: "command not found", CommandNotFound: true}
		}
		return routeCommandResult{ExitCode: -1, Stderr: "command start failed"}
	}
	const maxStdout, maxStderr = 4096, 1024
	stdout := make([]byte, 0, maxStdout)
	stderr := make([]byte, 0, maxStderr)
	readBuf := make([]byte, 256)
	truncated := false
	for len(stdout) < maxStdout {
		n, err := stdoutPipe.Read(readBuf)
		if n > 0 {
			if len(stdout)+n > maxStdout {
				n = maxStdout - len(stdout)
				truncated = true
			}
			stdout = append(stdout, readBuf[:n]...)
			if len(stdout) >= maxStdout {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if truncated {
		for {
			if _, err := stdoutPipe.Read(readBuf); err != nil {
				break
			}
		}
	}
	for len(stderr) < maxStderr {
		n, err := stderrPipe.Read(readBuf)
		if n > 0 {
			if len(stderr)+n > maxStderr {
				n = maxStderr - len(stderr)
			}
			stderr = append(stderr, readBuf[:n]...)
			if len(stderr) >= maxStderr {
				break
			}
		}
		if err != nil {
			break
		}
	}
	stdoutPipe.Close()
	stderrPipe.Close()
	waitErr := cmd.Wait()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	return routeCommandResult{Stdout: string(stdout), Stderr: string(stderr), ExitCode: exitCode}
}

func isValidRouteTarget(target string) bool {
	if target == "" {
		return false
	}
	for _, d := range []string{";", "|", "`", "$", "(", ")", "<", ">", "&", "\n", "\r", "\t"} {
		if strings.Contains(target, d) {
			return false
		}
	}
	return len(target) <= 253
}
