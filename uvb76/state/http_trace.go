package state

// HTTPTrace represents per-phase timing attribution for HTTP requests.
// This data is included in spike events/bundles for latency diagnostics.
//
// Privacy: No headers, cookies, auth tokens, full URLs, or response bodies are captured.
// The remote address is redacted to host-only (port stripped).
// Error messages are sanitized (newlines removed, truncated to 200 chars).
type HTTPTrace struct {
	// Kind is always "http" for HTTP traces
	Kind string `json:"kind"`
	
	// URLHost is the extracted host:port from the probe URL (no path, no auth)
	// Example: "example.com:8080" or "192.0.2.1:443"
	URLHost string `json:"url_host"`
	
	// RemoteAddr is the server's address with port stripped (host/IP only)
	// Privacy: Full IP addresses are not considered PII in infrastructure context
	RemoteAddr string `json:"remote_addr"`
	
	// DNS resolution time in milliseconds (0 if not applicable, e.g., reused connection)
	DNSMs float64 `json:"dns_ms,omitempty"`
	
	// TCP connection time in milliseconds
	TCPConnectMs float64 `json:"tcp_connect_ms,omitempty"`
	
	// TLS handshake time in milliseconds
	TLSHandshakeMs float64 `json:"tls_handshake_ms,omitempty"`
	
	// Time from connection established to connection ready (GotConn callback)
	// This includes any protocol negotiation or authentication
	GotConnMs float64 `json:"got_conn_ms,omitempty"`
	
	// Time to first byte of response headers in milliseconds
	// This is the primary latency metric for spike attribution
	TimeToFirstByteMs float64 `json:"time_to_first_byte_ms"`
	
	// Time to read response body in milliseconds (0 if no body or immediate)
	BodyReadMs float64 `json:"body_read_ms,omitempty"`
	
	// Total time from request start to body read complete
	// Note: This is latency_ms plus body read time for non-error responses
	TotalMs float64 `json:"total_ms"`
	
	// Whether the connection was reused from the pool
	ConnectionReused bool `json:"connection_reused"`
	
	// Whether the connection was idle in the pool before use
	WasIdle bool `json:"was_idle,omitempty"`
	
	// HTTP status code (0 if request failed before receiving headers)
	HTTPStatus int `json:"http_status"`
	
	// Total bytes read from response body
	BytesRead int `json:"bytes_read"`
	
	// BodyTruncated indicates the response body exceeded the bounded read limit.
	// When true, body_read_ms attribution is capped at the limit.
	BodyTruncated bool `json:"body_truncated,omitempty"`
	
	// Sanitized error message (if any)
	// Newlines removed, truncated to 200 chars
	// No sensitive data (tokens, paths, query params) included
	Error string `json:"error,omitempty"`
}
