package probe

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// FakeHTTPBackend provides deterministic fake HTTP responses for testing.
// No unit test may dial real DNS or external endpoints.
type FakeHTTPBackend struct {
	Server     *httptest.Server
	StatusCode int
	LatencyMs  float64
	CloseAfter int // Number of requests before closing (0 = never)
	requestCount int
}

// NewFakeHTTPBackend200 creates a backend that returns HTTP 200.
func NewFakeHTTPBackend200() *FakeHTTPBackend {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	return &FakeHTTPBackend{Server: server, StatusCode: 200}
}

// NewFakeHTTPBackend500 creates a backend that returns HTTP 500.
func NewFakeHTTPBackend500() *FakeHTTPBackend {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	return &FakeHTTPBackend{Server: server, StatusCode: 500}
}

// NewFakeHTTPBackendTimeout creates a backend that times out.
// Uses a bounded delay that exceeds reasonable probe timeouts.
func NewFakeHTTPBackendTimeout() *FakeHTTPBackend {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Context-aware handler - respects client cancellation
		select {
		case <-r.Context().Done():
			return // Client cancelled, don't send response
		case <-time.After(5 * time.Second): // Bounded delay, exceeds probe timeout
			w.WriteHeader(http.StatusOK)
		}
	}))
	return &FakeHTTPBackend{Server: server, StatusCode: 200}
}

// NewFakeHTTPBackend503 creates a backend that returns HTTP 503.
func NewFakeHTTPBackend503() *FakeHTTPBackend {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	return &FakeHTTPBackend{Server: server, StatusCode: 503}
}

// NewFakeHTTPBackendConnectionRefused creates a server that is immediately closed.
// All requests will fail with connection refused.
func NewFakeHTTPBackendConnectionRefused() *FakeHTTPBackend {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately - all requests will get connection refused
	return &FakeHTTPBackend{Server: server, StatusCode: 0}
}

// Close closes the fake backend server.
func (f *FakeHTTPBackend) Close() {
	if f.Server != nil {
		f.Server.Close()
	}
}

// URL returns the server URL.
func (f *FakeHTTPBackend) URL() string {
	if f.Server == nil {
		return ""
	}
	return f.Server.URL
}

// NewFakeHTTPBackendContextCanceled creates a server that responds after delay,
// simulating a client timeout scenario. Uses bounded delay.
func NewFakeHTTPBackendContextCanceled() *FakeHTTPBackend {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bounded handler - simulates delayed response for context-canceled test cases
		select {
		case <-r.Context().Done():
			return // Client cancelled - don't send response
		case <-time.After(2 * time.Second): // Bounded delay
			w.WriteHeader(http.StatusOK)
		}
	}))
	return &FakeHTTPBackend{Server: server, StatusCode: 200}
}

// FakeICMPResult provides deterministic fake ICMP results for testing.
type FakeICMPResult struct {
	Success     bool
	LatencyMs   float64
	Degraded    bool
	ErrorKind   string
	ErrorText   string
}

// NewFakeICMPSuccess creates a successful ICMP result.
func NewFakeICMPSuccess(latencyMs float64) FakeICMPResult {
	return FakeICMPResult{
		Success:   true,
		LatencyMs: latencyMs,
		Degraded:  false,
	}
}

// NewFakeICMPTimeout creates an ICMP timeout result.
func NewFakeICMPTimeout() FakeICMPResult {
	return FakeICMPResult{
		Success:   false,
		LatencyMs: 0,
		Degraded:  false,
		ErrorKind: "icmp_timeout",
		ErrorText: "ICMP ping timed out",
	}
}

// NewFakeICMPPermissionUnavailable creates an ICMP permission unavailable result.
func NewFakeICMPPermissionUnavailable() FakeICMPResult {
	return FakeICMPResult{
		Success:   false,
		LatencyMs: 0,
		Degraded:  false,
		ErrorKind: "icmp_permission",
		ErrorText: "ICMP socket: operation not permitted",
	}
}

// NewFakeICMPDisabled creates a result indicating ICMP is disabled.
func NewFakeICMPDisabled() FakeICMPResult {
	return FakeICMPResult{
		Success:   false,
		LatencyMs: 0,
		Degraded:  false,
		ErrorKind: "icmp_disabled",
		ErrorText: "ICMP probing is disabled",
	}
}

// NewFakeICMPDegraded creates a degraded ICMP result.
func NewFakeICMPDegraded(latencyMs float64) FakeICMPResult {
	return FakeICMPResult{
		Success:   true,
		LatencyMs: latencyMs,
		Degraded:  true,
	}
}

// ToProbeEvidence converts a FakeICMPResult to ProbeEvidence.
func (f FakeICMPResult) ToProbeEvidence() ProbeEvidence {
	return ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   f.Success,
		Degraded:  f.Degraded,
		Timestamp: time.Now(),
		ErrorKind: f.ErrorKind,
		ErrorText: f.ErrorText,
	}
}

// ToProbeEvidenceWithTimestamp converts with a specific timestamp.
func (f FakeICMPResult) ToProbeEvidenceWithTimestamp(ts time.Time) ProbeEvidence {
	return ProbeEvidence{
		Kind:      domain.ProbeKindICMP,
		Seen:      true,
		Success:   f.Success,
		Degraded:  f.Degraded,
		Timestamp: ts,
		ErrorKind: f.ErrorKind,
		ErrorText: f.ErrorText,
	}
}

// FakeHTTPResult provides deterministic fake HTTP results for testing.
type FakeHTTPResult struct {
	Success    bool
	StatusCode int
	LatencyMs  float64
	Degraded   bool
	ErrorKind  string
	ErrorText  string
}

// NewFakeHTTPSuccess creates a successful HTTP result.
func NewFakeHTTPSuccess(statusCode int, latencyMs float64) FakeHTTPResult {
	return FakeHTTPResult{
		Success:    true,
		StatusCode: statusCode,
		LatencyMs:  latencyMs,
		Degraded:   false,
	}
}

// NewFakeHTTPTimeout creates an HTTP timeout result.
func NewFakeHTTPTimeout() FakeHTTPResult {
	return FakeHTTPResult{
		Success:   false,
		LatencyMs: 0,
		ErrorKind: "http_timeout",
		ErrorText: "HTTP request timed out",
	}
}

// NewFakeHTTPConnectionRefused creates an HTTP connection refused result.
func NewFakeHTTPConnectionRefused() FakeHTTPResult {
	return FakeHTTPResult{
		Success:   false,
		LatencyMs: 0,
		ErrorKind: "http_connection_refused",
		ErrorText: "Connection refused",
	}
}

// NewFakeHTTPContextCanceled creates an HTTP context canceled result.
func NewFakeHTTPContextCanceled() FakeHTTPResult {
	return FakeHTTPResult{
		Success:   false,
		LatencyMs: 0,
		ErrorKind: "http_context_canceled",
		ErrorText: "Context canceled",
	}
}

// NewFakeHTTP5xx creates an HTTP 5xx error result.
func NewFakeHTTP5xx(statusCode int) FakeHTTPResult {
	return FakeHTTPResult{
		Success:    false,
		StatusCode: statusCode,
		LatencyMs:  0,
		ErrorKind:  "http_probe_5xx",
		ErrorText:  "HTTP server error",
	}
}

// NewFakeHTTPDegraded creates a degraded HTTP result.
func NewFakeHTTPDegraded(statusCode int, latencyMs float64) FakeHTTPResult {
	return FakeHTTPResult{
		Success:    true,
		StatusCode: statusCode,
		LatencyMs:  latencyMs,
		Degraded:   true,
	}
}

// ToProbeEvidence converts a FakeHTTPResult to ProbeEvidence.
func (f FakeHTTPResult) ToProbeEvidence() ProbeEvidence {
	return ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   f.Success,
		Degraded:  f.Degraded,
		Timestamp: time.Now(),
		ErrorKind: f.ErrorKind,
		ErrorText: f.ErrorText,
	}
}

// ToProbeEvidenceWithTimestamp converts with a specific timestamp.
func (f FakeHTTPResult) ToProbeEvidenceWithTimestamp(ts time.Time) ProbeEvidence {
	return ProbeEvidence{
		Kind:      domain.ProbeKindHTTP,
		Seen:      true,
		Success:   f.Success,
		Degraded:  f.Degraded,
		Timestamp: ts,
		ErrorKind: f.ErrorKind,
		ErrorText: f.ErrorText,
	}
}

// FakeBackendSet provides a complete set of fake backends for testing.
type FakeBackendSet struct {
	HTTP200              *FakeHTTPBackend
	HTTP500              *FakeHTTPBackend
	HTTP503              *FakeHTTPBackend
	HTTPTimeout          *FakeHTTPBackend
	HTTPConnectionRefused *FakeHTTPBackend
	HTTPContextCanceled  *FakeHTTPBackend
	ICMPSuccess         FakeICMPResult
	ICMPTimeout         FakeICMPResult
	ICMPPermission      FakeICMPResult
	ICMPDisabled        FakeICMPResult
	ICMPDegraded        FakeICMPResult
}

// NewFakeBackendSet creates a complete set of fake backends.
func NewFakeBackendSet() *FakeBackendSet {
	return &FakeBackendSet{
		HTTP200:              NewFakeHTTPBackend200(),
		HTTP500:              NewFakeHTTPBackend500(),
		HTTP503:              NewFakeHTTPBackend503(),
		HTTPTimeout:          NewFakeHTTPBackendTimeout(),
		HTTPConnectionRefused: NewFakeHTTPBackendConnectionRefused(),
		HTTPContextCanceled:  NewFakeHTTPBackendContextCanceled(),
		ICMPSuccess:         NewFakeICMPSuccess(30.0),
		ICMPTimeout:         NewFakeICMPTimeout(),
		ICMPPermission:      NewFakeICMPPermissionUnavailable(),
		ICMPDisabled:        NewFakeICMPDisabled(),
		ICMPDegraded:        NewFakeICMPDegraded(500.0),
	}
}

// Close closes all HTTP backend servers.
func (f *FakeBackendSet) Close() {
	if f.HTTP200 != nil {
		f.HTTP200.Close()
	}
	if f.HTTP500 != nil {
		f.HTTP500.Close()
	}
	if f.HTTP503 != nil {
		f.HTTP503.Close()
	}
	if f.HTTPTimeout != nil {
		f.HTTPTimeout.Close()
	}
	if f.HTTPConnectionRefused != nil {
		f.HTTPConnectionRefused.Close()
	}
	if f.HTTPContextCanceled != nil {
		f.HTTPContextCanceled.Close()
	}
}

// GetHTTPBackend returns the appropriate HTTP backend for a result type.
func (f *FakeBackendSet) GetHTTPBackend(result FakeHTTPResult) *FakeHTTPBackend {
	switch result.ErrorKind {
	case "http_timeout":
		return f.HTTPTimeout
	case "http_connection_refused":
		return f.HTTPConnectionRefused
	case "http_context_canceled":
		return f.HTTPContextCanceled
	case "http_probe_5xx":
		return f.HTTP503
	default:
		if result.StatusCode >= 500 {
			return f.HTTP500
		}
		return f.HTTP200
	}
}

// HTTP200 returns ProbeEvidence for HTTP 200.
func (f *FakeBackendSet) HTTP200Evidence() ProbeEvidence {
	return FakeHTTPResult{
		Success:    true,
		StatusCode: 200,
		LatencyMs:  50.0,
	}.ToProbeEvidence()
}

// HTTP500Evidence returns ProbeEvidence for HTTP 500.
func (f *FakeBackendSet) HTTP500Evidence() ProbeEvidence {
	return FakeHTTPResult{
		Success:    false,
		StatusCode: 500,
		ErrorKind:  "http_probe_5xx",
		ErrorText:  "HTTP 500 Internal Server Error",
	}.ToProbeEvidence()
}

// HTTPTimeoutEvidence returns ProbeEvidence for HTTP timeout.
func (f *FakeBackendSet) HTTPTimeoutEvidence() ProbeEvidence {
	return FakeHTTPResult{
		Success:   false,
		ErrorKind: "http_timeout",
		ErrorText: "HTTP request timed out",
	}.ToProbeEvidence()
}

// ConnectionRefusedEvidence returns ProbeEvidence for connection refused.
func (f *FakeBackendSet) ConnectionRefusedEvidence() ProbeEvidence {
	return FakeHTTPResult{
		Success:   false,
		ErrorKind: "http_connection_refused",
		ErrorText: "Connection refused",
	}.ToProbeEvidence()
}

// ContextCanceledEvidence returns ProbeEvidence for context canceled.
func (f *FakeBackendSet) ContextCanceledEvidence() ProbeEvidence {
	return FakeHTTPResult{
		Success:   false,
		ErrorKind: "http_context_canceled",
		ErrorText: "Context canceled",
	}.ToProbeEvidence()
}

// ICMPSuccessEvidence returns ProbeEvidence for ICMP success.
func (f *FakeBackendSet) ICMPSuccessEvidence() ProbeEvidence {
	return f.ICMPSuccess.ToProbeEvidence()
}

// ICMPTimeoutEvidence returns ProbeEvidence for ICMP timeout.
func (f *FakeBackendSet) ICMPTimeoutEvidence() ProbeEvidence {
	return f.ICMPTimeout.ToProbeEvidence()
}

// ICMPPermissionEvidence returns ProbeEvidence for ICMP permission unavailable.
func (f *FakeBackendSet) ICMPPermissionEvidence() ProbeEvidence {
	return f.ICMPPermission.ToProbeEvidence()
}

// ICMPDisabledEvidence returns ProbeEvidence for ICMP disabled.
func (f *FakeBackendSet) ICMPDisabledEvidence() ProbeEvidence {
	return f.ICMPDisabled.ToProbeEvidence()
}

// ICMPDegradedEvidence returns ProbeEvidence for ICMP degraded.
func (f *FakeBackendSet) ICMPDegradedEvidence() ProbeEvidence {
	return f.ICMPDegraded.ToProbeEvidence()
}
