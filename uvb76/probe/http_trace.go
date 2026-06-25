// Package probe implements independent latency probing for UVB-76.
// Probes run on their own cadence independent from the status scraper.
package probe

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// HTTP Trace Collector — Per-Phase Timing Attribution via httptrace
// =============================================================================

// httpTraceCollector captures per-phase HTTP request timing using httptrace.
// It is safe for concurrent use by multiple goroutines performing probes.
type httpTraceCollector struct {
	mu sync.RWMutex

	// Timing basis (monotonic clock)
	startTime time.Time

	// Phase timestamps
	dnsStart          time.Time
	dnsDone           time.Time
	connectStart      time.Time
	connectDone       time.Time
	tlsStart          time.Time
	tlsDone           time.Time
	gotConnTime       time.Time
	wroteRequestTime  time.Time
	firstByteTime     time.Time
	bodyReadDone      time.Time

	// Connection info
	connectionReused bool
	wasIdle          bool
	remoteAddr       string

	// Actual connection for TCP_INFO collection
	// Set by GotConn hook; do NOT read, write, or close
	actualConn net.Conn

	// Error tracking
	dnsError        error
	connectError    error
	tlsError        error
	requestError    error
	responseError   error

	// Body read tracking
	bytesRead int64

	// Request context
	urlHost string
}

// newHTTPTraceCollector creates a new trace collector with the request start time.
func newHTTPTraceCollector(urlHost string) *httpTraceCollector {
	return &httpTraceCollector{
		startTime: time.Now(),
		urlHost:   urlHost,
	}
}

// =============================================================================
// httptrace Hooks
// =============================================================================

// getTraceHooks returns the httptrace.ClientTrace hooks for this collector.
func (c *httpTraceCollector) getTraceHooks() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		// DNS events
		DNSStart: func(info httptrace.DNSStartInfo) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.dnsDone = time.Now()
			if info.Err != nil {
				c.dnsError = info.Err
			}
		},

		// Connect events
		ConnectStart: func(network, addr string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			// Only record if we haven't started connect yet
			if c.connectStart.IsZero() {
				c.connectStart = time.Now()
			}
		},
		ConnectDone: func(network, addr string, err error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.connectDone = time.Now()
			if err != nil {
				c.connectError = err
			} else {
				// Redact the address - only keep host portion
				if host, _, splitErr := net.SplitHostPort(addr); splitErr == nil {
					c.remoteAddr = host
				} else {
					c.remoteAddr = addr
				}
			}
		},

		// TLS events
		TLSHandshakeStart: func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.tlsStart.IsZero() {
				c.tlsStart = time.Now()
			}
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.tlsDone = time.Now()
			if err != nil {
				c.tlsError = err
			}
		},

		// Connection events
		GotConn: func(info httptrace.GotConnInfo) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.gotConnTime = time.Now()
			c.connectionReused = info.Reused
			c.wasIdle = info.Reused && info.WasIdle
			// Capture the actual connection for TCP_INFO collection.
			// Per httptrace docs: do not read, write, or close this connection.
			// The transport owns the connection lifecycle.
			if c.actualConn == nil && info.Conn != nil {
				c.actualConn = info.Conn
			}
		},

		// Request events
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.wroteRequestTime = time.Now()
			if info.Err != nil {
				c.requestError = info.Err
			}
		},

		// Response events
		GotFirstResponseByte: func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.firstByteTime = time.Now()
		},
	}
}

// RecordBodyRead records a chunk of body being read.
func (c *httpTraceCollector) RecordBodyRead(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bytesRead += int64(n)
}

// Finalize is called when the request completes to mark the body read done time.
func (c *httpTraceCollector) Finalize() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bodyReadDone.IsZero() {
		c.bodyReadDone = time.Now()
	}
}

// BytesRead returns the total bytes read from the response body.
func (c *httpTraceCollector) BytesRead() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bytesRead
}

// =============================================================================
// Actual Connection Access for TCP_INFO Collection
// =============================================================================

// GetActualConn returns the actual net.Conn used by the HTTP request.
// This is used for native TCP_INFO collection from the real probe socket.
// Returns nil if no connection was captured (e.g., request failed before connect).
//
// IMPORTANT: The returned connection is owned by the http.Transport.
// Do NOT read, write, or close this connection. It is for observation only.
func (c *httpTraceCollector) GetActualConn() net.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.actualConn
}

// IsConnectionReused returns whether the connection was reused from the pool.
func (c *httpTraceCollector) IsConnectionReused() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectionReused
}

// WasIdle returns whether the connection was idle in the pool.
func (c *httpTraceCollector) WasIdle() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.wasIdle
}

// =============================================================================
// Build HTTPTrace
// =============================================================================

// BuildHTTPTrace constructs the final HTTPTrace from collected data.
// Must be called after the request completes.
func (c *httpTraceCollector) BuildHTTPTrace(totalMs float64, httpStatus int, respError error, bodyTruncated bool) *state.HTTPTrace {
	c.mu.RLock()
	defer c.mu.RUnlock()

	trace := &state.HTTPTrace{
		Kind:             "http",
		URLHost:          c.urlHost,
		RemoteAddr:       c.redactRemoteAddr(),
		ConnectionReused: c.connectionReused,
		WasIdle:          c.wasIdle,
		TotalMs:          totalMs,
		HTTPStatus:       httpStatus,
		BytesRead:        int(c.bytesRead), // Convert int64 to int
		BodyTruncated:    bodyTruncated,
	}

	// Calculate phase durations
	if !c.dnsStart.IsZero() && !c.dnsDone.IsZero() {
		trace.DNSMs = float64(c.dnsDone.Sub(c.dnsStart).Microseconds()) / 1000.0
	}
	if !c.connectStart.IsZero() && !c.connectDone.IsZero() {
		trace.TCPConnectMs = float64(c.connectDone.Sub(c.connectStart).Microseconds()) / 1000.0
	}
	if !c.tlsStart.IsZero() && !c.tlsDone.IsZero() {
		trace.TLSHandshakeMs = float64(c.tlsDone.Sub(c.tlsStart).Microseconds()) / 1000.0
	}
	if !c.gotConnTime.IsZero() {
		// GotConnMs is the time from request start to getting the connection
		trace.GotConnMs = float64(c.gotConnTime.Sub(c.startTime).Microseconds()) / 1000.0
	}
	if !c.firstByteTime.IsZero() {
		// TimeToFirstByteMs is time from request write complete to first response byte
		if !c.wroteRequestTime.IsZero() {
			trace.TimeToFirstByteMs = float64(c.firstByteTime.Sub(c.wroteRequestTime).Microseconds()) / 1000.0
		} else if !c.gotConnTime.IsZero() {
			trace.TimeToFirstByteMs = float64(c.firstByteTime.Sub(c.gotConnTime).Microseconds()) / 1000.0
		}
	}
	if !c.bodyReadDone.IsZero() {
		// BodyReadMs is time from first byte to body read complete
		if !c.firstByteTime.IsZero() {
			trace.BodyReadMs = float64(c.bodyReadDone.Sub(c.firstByteTime).Microseconds()) / 1000.0
		}
	}

	// Set error if any phase failed
	if c.dnsError != nil {
		trace.Error = sanitizeError(fmt.Sprintf("dns: %v", c.dnsError))
	} else if c.connectError != nil {
		trace.Error = sanitizeError(fmt.Sprintf("connect: %v", c.connectError))
	} else if c.tlsError != nil {
		trace.Error = sanitizeError(fmt.Sprintf("tls: %v", c.tlsError))
	} else if c.requestError != nil {
		trace.Error = sanitizeError(fmt.Sprintf("request: %v", c.requestError))
	} else if respError != nil {
		trace.Error = sanitizeError(fmt.Sprintf("response: %v", respError))
	}

	return trace
}

// redactRemoteAddr returns a privacy-safe remote address string.
func (c *httpTraceCollector) redactRemoteAddr() string {
	if c.remoteAddr == "" {
		return ""
	}
	// If we have a host:port, return just the host for privacy
	if host, _, err := net.SplitHostPort(c.remoteAddr); err == nil {
		return host
	}
	// Fallback: return as-is (may just be host)
	return c.remoteAddr
}

// sanitizeError returns a sanitized error message without sensitive data.
func sanitizeError(raw string) string {
	// Limit length
	safe := raw
	if len(safe) > 200 {
		safe = safe[:200]
	}
	// Remove newlines
	safe = strings.ReplaceAll(safe, "\n", " ")
	safe = strings.ReplaceAll(safe, "\r", "")
	return safe
}

// =============================================================================
// URL Utilities
// =============================================================================

// ExtractHost extracts and sanitizes the host from a URL string.
// Returns only the host portion, never the path or query string.
func ExtractHost(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return u.Host
}

// =============================================================================
// Body Read Tracking
// =============================================================================

// traceReadCloser wraps an io.ReadCloser and tracks bytes read for tracing.
type traceReadCloser struct {
	inner    io.ReadCloser
	collector *httpTraceCollector
}

// newTraceReadCloser creates a wrapper that tracks body reads.
func newTraceReadCloser(rc io.ReadCloser, collector *httpTraceCollector) io.ReadCloser {
	return &traceReadCloser{
		inner:     rc,
		collector: collector,
	}
}

func (t *traceReadCloser) Read(p []byte) (int, error) {
	n, err := t.inner.Read(p)
	if n > 0 {
		t.collector.RecordBodyRead(n)
	}
	return n, err
}

func (t *traceReadCloser) Close() error {
	return t.inner.Close()
}
