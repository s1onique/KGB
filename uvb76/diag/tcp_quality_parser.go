// Package diag implements diagnostic capture for UVB-76.
// This file provides ss output parsing for TCP quality evidence collection.
package diag

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// =============================================================================
// ss Output Parsing
// =============================================================================

// ssSocket represents a parsed TCP socket from ss output.
type ssSocket struct {
	State              string
	Local              string
	Remote             string
	// Queue fields are pointers so that zero (observed) is distinguishable
	// from absent. When a socket line is parsed, these are always populated.
	SendQueueBytes     *int64
	RecvQueueBytes     *int64
	RTTUs              *int64
	RTTVarUs           *int64
	RetransmitsCurrent *int64
	RetransmitsTotal   *int64
	Unacked            *int64
	Lost               *int64
	Sacked             *int64
	Reordering         *int64
	SndCwnd            *int32
	Ssthresh           *int32
	DeliveryRateBps    *int64
}

// parseSsOutput parses ss -tin output into a list of sockets.
func parseSsOutput(output string) ([]ssSocket, error) {
	var sockets []ssSocket
	lines := strings.Split(output, "\n")

	var currentSocket *ssSocket

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is a header line
		if strings.HasPrefix(line, "State") || strings.HasPrefix(line, "Recv-Q") || strings.HasPrefix(line, "Netid") {
			continue
		}

		// Check if this is a new socket line (starts with state)
		if isSocketStartLine(line) {
			// Save previous socket if exists
			if currentSocket != nil {
				sockets = append(sockets, *currentSocket)
			}
			// Start new socket
			currentSocket = parseSocketLine(line)
			continue
		}

		// This is a continuation line - parse tokens
		if currentSocket != nil {
			parseContinuationLine(line, currentSocket)
		}
	}

	// Don't forget the last socket
	if currentSocket != nil {
		sockets = append(sockets, *currentSocket)
	}

	return sockets, nil
}

// isSocketStartLine checks if a line starts a new socket entry.
func isSocketStartLine(line string) bool {
	states := []string{"ESTAB", "LISTEN", "SYN-SENT", "SYN-RECV", "FIN-WAIT-1", "FIN-WAIT-2",
		"TIME-WAIT", "CLOSE", "CLOSE-WAIT", "LAST-ACK", "CLOSING", "UNCONN"}
	for _, state := range states {
		if strings.HasPrefix(line, state) {
			return true
		}
	}
	return false
}

// parseSocketLine parses the first line of a socket entry.
// Format: "State Recv-Q Send-Q LocalAddress:Port PeerAddress:Port"
func parseSocketLine(line string) *ssSocket {
	socket := &ssSocket{}

	// Common format: "ESTAB 0 0 local:port remote:port"
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return socket
	}

	socket.State = parts[0]

	// In ss output: column 1 (parts[1]) is Recv-Q, column 2 (parts[2]) is Send-Q
	// Queue fields are always populated when the socket line is parsed, even if zero.
	if len(parts) > 1 {
		if q, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			socket.RecvQueueBytes = &q
		}
	}
	if len(parts) > 2 {
		if q, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			socket.SendQueueBytes = &q
		}
	}

	// Look for local and remote addresses
	for i, part := range parts {
		if strings.Contains(part, ":") {
			// Try to identify local vs remote
			if i < len(parts)-1 && strings.Contains(parts[i+1], ":") {
				socket.Local = part
				socket.Remote = parts[i+1]
				break
			}
		}
	}

	return socket
}

// parseContinuationLine parses a continuation line containing TCP metrics.
func parseContinuationLine(line string, socket *ssSocket) {
	tokens := strings.Fields(line)

	for _, token := range tokens {
		// RTT: format "rtt:113.5/12.0" or "rtt:113000/12000" (in us)
		if strings.HasPrefix(token, "rtt:") {
			parseRTTToken(token, socket)
			continue
		}

		// Retrans: format "retrans:0/3" (current/total)
		if strings.HasPrefix(token, "retrans:") || strings.HasPrefix(token, "retrans:0/") {
			parts := strings.Split(strings.TrimPrefix(token, "retrans:"), "/")
			if len(parts) == 2 {
				if cur, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					socket.RetransmitsCurrent = &cur
				}
				if tot, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					socket.RetransmitsTotal = &tot
				}
			}
			continue
		}

		// cwnd: congestion window
		if strings.HasPrefix(token, "cwnd:") {
			val := strings.TrimPrefix(token, "cwnd:")
			if cwnd, err := strconv.ParseInt(val, 10, 64); err == nil {
				cwnd32 := int32(cwnd)
				socket.SndCwnd = &cwnd32
			}
			continue
		}

		// ssthresh: slow start threshold
		if strings.HasPrefix(token, "ssthresh:") {
			val := strings.TrimPrefix(token, "ssthresh:")
			if val != "" && val != "any" {
				if ssthresh, err := strconv.ParseInt(val, 10, 64); err == nil {
					ssthresh32 := int32(ssthresh)
					socket.Ssthresh = &ssthresh32
				}
			}
			continue
		}

		// unacked
		if strings.HasPrefix(token, "unacked:") {
			val := strings.TrimPrefix(token, "unacked:")
			if parts := strings.Split(val, "/"); len(parts) >= 1 {
				if v, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					socket.Unacked = &v
				}
			}
			continue
		}

		// lost
		if strings.HasPrefix(token, "lost:") {
			val := strings.TrimPrefix(token, "lost:")
			if parts := strings.Split(val, "/"); len(parts) >= 1 {
				if v, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					socket.Lost = &v
				}
			}
			continue
		}

		// sacked
		if strings.HasPrefix(token, "sacked:") {
			val := strings.TrimPrefix(token, "sacked:")
			if parts := strings.Split(val, "/"); len(parts) >= 1 {
				if v, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					socket.Sacked = &v
				}
			}
			continue
		}

		// reordering
		if strings.HasPrefix(token, "reordering:") {
			val := strings.TrimPrefix(token, "reordering:")
			if parts := strings.Split(val, "/"); len(parts) >= 1 {
				if v, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					socket.Reordering = &v
				}
			}
			continue
		}

		// delivery_rate
		if strings.HasPrefix(token, "delivery_rate:") || strings.HasPrefix(token, "pacing_rate:") {
			val := strings.TrimPrefix(token, "delivery_rate:")
			val = strings.TrimPrefix(val, "pacing_rate:")
			if parts := strings.Split(val, "/"); len(parts) >= 1 {
				// Extract bps value
				re := regexp.MustCompile(`(\d+)bps`)
				if matches := re.FindStringSubmatch(parts[0]); len(matches) >= 2 {
					if rate, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
						socket.DeliveryRateBps = &rate
					}
				}
			}
			continue
		}
	}
}

// parseRTTToken parses RTT token. ss displays RTT in milliseconds when unitless,
// so we interpret unitless values as ms and convert to microseconds.
func parseRTTToken(token string, socket *ssSocket) {
	parts := strings.Split(strings.TrimPrefix(token, "rtt:"), "/")
	if len(parts) < 2 {
		return
	}

	// Parse RTT value (interpret as milliseconds, convert to microseconds)
	rttStr := parts[0]
	rttUs := parseSSMilliseconds(rttStr)
	if rttUs > 0 {
		socket.RTTUs = &rttUs
	}

	// Parse RTT variance (interpret as milliseconds, convert to microseconds)
	rttvarStr := parts[1]
	rttvarUs := parseSSMilliseconds(rttvarStr)
	if rttvarUs > 0 {
		socket.RTTVarUs = &rttvarUs
	}
}

// parseSSMilliseconds parses a value from ss output as milliseconds and returns microseconds.
// ss typically displays RTT values in milliseconds when no unit is specified (e.g., "113.5" means 113.5ms).
func parseSSMilliseconds(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Check for explicit units
	var multiplier float64 = 1000.0 // Default: ms to us (1ms = 1000us)
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "ms") {
		s = strings.TrimSuffix(s, "ms")
		multiplier = 1000.0
	} else if strings.HasSuffix(lower, "us") || strings.HasSuffix(lower, "µs") {
		s = strings.TrimSuffix(s, "us")
		s = strings.TrimSuffix(s, "µs")
		multiplier = 1.0
	} else if strings.HasSuffix(lower, "s") {
		s = strings.TrimSuffix(s, "s")
		multiplier = 1000000.0
	} else if strings.HasSuffix(lower, "mdev") {
		// mdev is RTT variance, still in ms
		s = strings.TrimSuffix(s, "mdev")
		multiplier = 1000.0
	}
	// No explicit unit: ss typically shows ms for RTT values, so multiplier stays at 1000.0

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return int64(val * multiplier)
}

// =============================================================================
// Socket Matching
// =============================================================================

// findMatchingSocket finds the best matching socket for the target IP.
// If multiple sockets match, returns the best match deterministically.
func findMatchingSocket(sockets []ssSocket, targetIP string) (*ssSocket, int) {
	var matches []ssSocket

	for _, socket := range sockets {
		if socket.State != "ESTAB" {
			continue
		}

		// Extract IP from remote address
		remoteIP := extractIP(socket.Remote)
		if remoteIP == "" {
			continue
		}

		// Check if remote IP matches target
		if remoteIP == targetIP {
			matches = append(matches, socket)
		}
	}

	if len(matches) == 0 {
		return nil, 0
	}

	// Return the first match (deterministic: same input order = same output)
	return &matches[0], len(matches)
}

// extractIP extracts the IP address from a socket address string.
// Handles formats like "10.0.0.1:8080", "[::1]:8080", etc.
func extractIP(addr string) string {
	if addr == "" {
		return ""
	}

	// Try net.SplitHostPort first for proper parsing
	// Handle bracketed IPv6: [::1]:8080 or [2001:db8::1]:443
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Fallback: manually handle common cases
		return extractIPManual(addr)
	}
	return host
}

// extractIPManual handles address extraction for malformed input without panicking.
func extractIPManual(addr string) string {
	// Remove brackets for IPv6
	if strings.HasPrefix(addr, "[") {
		// Find closing bracket
		bracketEnd := strings.Index(addr, "]")
		if bracketEnd > 0 {
			return addr[1:bracketEnd]
		}
	}

	// For IPv4 or plain host:port
	// Split on colon and take the host part
	parts := strings.Split(addr, ":")
	if len(parts) >= 2 {
		// First part is the host/IP
		return parts[0]
	}

	// No port found, return as-is
	return addr
}

// redactAddress redacts an address for privacy.
// Returns "redacted" or "redacted:port" depending on format.
func redactAddress(addr string) string {
	if addr == "" {
		return ""
	}

	// Extract host and port properly
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Fallback for malformed addresses
		return "redacted"
	}

	// Redact the host, preserve port if present
	if port != "" {
		return "redacted:" + port
	}
	return "redacted"
}
