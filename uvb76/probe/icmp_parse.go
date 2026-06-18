// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrPingTimeout indicates the ping timed out.
	ErrPingTimeout = errors.New("ping timeout")
	// ErrPingUnreachable indicates the host is unreachable.
	ErrPingUnreachable = errors.New("host unreachable")
	// ErrPingParseError indicates failure to parse ping output.
	ErrPingParseError = errors.New("failed to parse ping output")

	// Patterns for parsing ping output from various implementations.
	// iputils/Linux: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=1.23 ms"
	// BusyBox: "64 bytes from 1.2.3.4: seq=1 ttl=64 time=1.23 ms"
	// macOS: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=1.234 ms"
	// Some variants: "PING 1.2.3.4 (1.2.3.4): 56 data bytes"
	pingTimeRegex = regexp.MustCompile(`time[=<]?\s*([0-9.]+)\s*ms`)
)

// ParsePingTime extracts the round-trip time from ping output line.
// Returns the time in milliseconds, or an error if not found.
func ParsePingTime(line string) (float64, error) {
	matches := pingTimeRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return 0, ErrPingParseError
	}
	timeStr := matches[1]
	ms, err := strconv.ParseFloat(timeStr, 64)
	if err != nil {
		return 0, ErrPingParseError
	}
	return ms, nil
}

// PingOS runs a single ping using the platform's ping command and returns the RTT.
// It extracts the hostname/IP from the target base URL and pings just the host.
func PingOS(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	// Build the ping command with one packet and appropriate timeout
	// Use -c 1 for one packet, -W for timeout in seconds (Linux/BusyBox)
	timeoutSecs := int(timeout.Seconds())
	if timeoutSecs < 1 {
		timeoutSecs = 1
	}

	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSecs), host)

	// Run with the system ping command
	output, err := cmd.Output()
	if err != nil {
		// Check if it was a context cancellation (timeout)
		if ctx.Err() == context.DeadlineExceeded {
			return 0, ErrPingTimeout
		}
		// Check for common error messages in stderr or combined output
		outputStr := string(output)
		if strings.Contains(outputStr, "Destination Host Unreachable") ||
			strings.Contains(outputStr, "Request timeout") ||
			strings.Contains(outputStr, "100% packet loss") {
			return 0, ErrPingUnreachable
		}
		// Other errors (no ping binary, permission denied, etc.)
		return 0, err
	}

	// Parse the output to extract RTT
	return parsePingOutput(string(output))
}

// parsePingOutput extracts the round-trip time from ping command output.
func parsePingOutput(output string) (time.Duration, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		ms, err := ParsePingTime(line)
		if err == nil {
			// Use float multiplication to preserve sub-millisecond precision
			return time.Duration(ms * float64(time.Millisecond)), nil
		}
	}
	return 0, ErrPingParseError
}

// ParsePingFixtures provides test cases for ping output parsing.
var ParsePingFixtures = []struct {
	name    string
	output  string
	wantMs  float64
	wantErr error
}{
	{
		name:   "iputils Linux",
		output: "PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.\n64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=13.5 ms\n\n--- 8.8.8.8 ping statistics ---\n1 packets transmitted, 1 received, 0% packet loss, time 0ms\n",
		wantMs:  13.5,
		wantErr: nil,
	},
	{
		name:   "BusyBox",
		output: "PING 192.168.1.1 (192.168.1.1): 56 data bytes\n64 bytes from 192.168.1.1: seq=1 ttl=64 time=0.823 ms\n\n--- 192.168.1.1 ping statistics ---\n1 packets transmitted, 1 packets received, 0% packet loss\n",
		wantMs:  0.823,
		wantErr: nil,
	},
	{
		name:   "macOS",
		output: "PING 1.1.1.1 (1.1.1.1): 56 data bytes, 64 bytes from 1.1.1.1: icmp_seq=0 ttl=55 time=9.836 ms\n64 bytes from 1.1.1.1: icmp_seq=0 ttl=55 time=9.836 ms\n\n--- 1.1.1.1 ping statistics ---\n1 packets transmitted, 1 packets received, 0.0% packet loss, round-trip min/avg/max/stddev = 9.836/9.836/9.836/0.000 ms\n",
		wantMs:  9.836,
		wantErr: nil,
	},
	{
		name:   "timeout no response",
		output: "PING 10.255.255.1 (10.255.255.1) 56(84) bytes of data.\n\n--- 10.255.255.1 ping statistics ---\n1 packets transmitted, 0 packets received, 100% packet loss\n",
		wantMs:  0,
		wantErr: ErrPingUnreachable,
	},
	{
		name:   "invalid output",
		output: "some random text\nwithout ping data\n",
		wantMs:  0,
		wantErr: ErrPingParseError,
	},
	{
		name:   "DNS resolution failure",
		output: "ping: unknown host nonexistent.invalid\n",
		wantMs:  0,
		wantErr: ErrPingUnreachable,
	},
	{
		name:   "float milliseconds with many decimals",
		output: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=1.234567 ms\n",
		wantMs:  1.234567,
		wantErr: nil,
	},
	{
		name:   "time= with equals sign",
		output: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=5.0ms\n",
		wantMs:  5.0,
		wantErr: nil,
	},
	{
		name:   "trailing spaces",
		output: "64 bytes from 1.2.3.4: icmp_seq=1 ttl=64 time=7.5 ms   \n",
		wantMs:  7.5,
		wantErr: nil,
	},
}
