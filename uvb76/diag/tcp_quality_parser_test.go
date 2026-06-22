package diag

import (
	"testing"
)

func TestParseSsOutput_EstablishedSocketWithQueues(t *testing.T) {
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		uid:1000 ino:12345 sk:abc123
		rtt:113.5/12.0 rttc:0-0-0 send 10Mbps rcv_space:29200
		cwnd:10
		ssthresh:2147483647
		retrans:0/3
		unacked:0
		lost:0
		sacked:0
		reordering:3`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	sock := sockets[0]

	if sock.State != "ESTAB" {
		t.Errorf("expected state 'ESTAB', got '%s'", sock.State)
	}
	if sock.SendQueueBytes == nil {
		t.Error("expected send queue to be set (pointer)")
	} else if *sock.SendQueueBytes != 0 {
		t.Errorf("expected send queue 0, got %d", *sock.SendQueueBytes)
	}
	if sock.RecvQueueBytes == nil {
		t.Error("expected recv queue to be set (pointer)")
	} else if *sock.RecvQueueBytes != 0 {
		t.Errorf("expected recv queue 0, got %d", *sock.RecvQueueBytes)
	}
	if sock.Remote != "10.0.0.5:8080" {
		t.Errorf("expected remote '10.0.0.5:8080', got '%s'", sock.Remote)
	}

	// RTT should be ~113500us (113.5ms)
	if sock.RTTUs == nil {
		t.Error("expected RTT to be set")
	} else if *sock.RTTUs < 110000 || *sock.RTTUs > 120000 {
		t.Errorf("expected RTT ~113500us, got %d", *sock.RTTUs)
	}

	// RTT variance should be ~12000us (12ms)
	if sock.RTTVarUs == nil {
		t.Error("expected RTT variance to be set")
	} else if *sock.RTTVarUs < 10000 || *sock.RTTVarUs > 15000 {
		t.Errorf("expected RTT variance ~12000us, got %d", *sock.RTTVarUs)
	}

	// Retransmits
	if sock.RetransmitsCurrent == nil || *sock.RetransmitsCurrent != 0 {
		t.Errorf("expected retransmits current 0, got %v", sock.RetransmitsCurrent)
	}
	if sock.RetransmitsTotal == nil || *sock.RetransmitsTotal != 3 {
		t.Errorf("expected retransmits total 3, got %v", sock.RetransmitsTotal)
	}

	// Congestion window
	if sock.SndCwnd == nil || *sock.SndCwnd != 10 {
		t.Errorf("expected cwnd 10, got %v", sock.SndCwnd)
	}

	// Reordering
	if sock.Reordering == nil || *sock.Reordering != 3 {
		t.Errorf("expected reordering 3, got %v", sock.Reordering)
	}
}

func TestParseSsOutput_RTTParsingMilliseconds(t *testing.T) {
	// Some ss versions output RTT in milliseconds
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		rtt:50/10
		cwnd:10`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	sock := sockets[0]

	// RTT should be 50000us (50ms)
	if sock.RTTUs == nil {
		t.Error("expected RTT to be set")
	} else if *sock.RTTUs != 50000 {
		t.Errorf("expected RTT 50000us, got %d", *sock.RTTUs)
	}

	// RTT variance should be 10000us (10ms)
	if sock.RTTVarUs == nil {
		t.Error("expected RTT variance to be set")
	} else if *sock.RTTVarUs != 10000 {
		t.Errorf("expected RTT variance 10000us, got %d", *sock.RTTVarUs)
	}
}

func TestParseSsOutput_IPv6Socket(t *testing.T) {
	input := `ESTAB 0 0 [::1]:45678 [2001:db8::1]:8080
		rtt:100/20
		cwnd:15`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	sock := sockets[0]

	if sock.State != "ESTAB" {
		t.Errorf("expected state 'ESTAB', got '%s'", sock.State)
	}
	if sock.Remote != "[2001:db8::1]:8080" {
		t.Errorf("expected remote '[2001:db8::1]:8080', got '%s'", sock.Remote)
	}
}

func TestParseSsOutput_DeliveryRate(t *testing.T) {
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		delivery_rate:10000000bps 0 0
		cwnd:10`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	sock := sockets[0]

	if sock.DeliveryRateBps == nil {
		t.Error("expected delivery rate to be set")
	} else if *sock.DeliveryRateBps != 10000000 {
		t.Errorf("expected delivery rate 10000000bps, got %d", *sock.DeliveryRateBps)
	}
}

func TestParseSsOutput_MultipleSockets(t *testing.T) {
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		rtt:100/10
		cwnd:10
ESTAB 0 0 10.0.0.1:45679 10.0.0.6:9090
		rtt:200/20
		cwnd:20
TIME-WAIT 0 0 10.0.0.1:45680 10.0.0.7:7070
		cwnd:5`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 3 {
		t.Fatalf("expected 3 sockets, got %d", len(sockets))
	}

	// First socket
	if sockets[0].Remote != "10.0.0.5:8080" {
		t.Errorf("expected first socket remote '10.0.0.5:8080', got '%s'", sockets[0].Remote)
	}
	if sockets[0].State != "ESTAB" {
		t.Errorf("expected first socket state 'ESTAB', got '%s'", sockets[0].State)
	}

	// Second socket
	if sockets[1].Remote != "10.0.0.6:9090" {
		t.Errorf("expected second socket remote '10.0.0.6:9090', got '%s'", sockets[1].Remote)
	}

	// Third socket is TIME-WAIT
	if sockets[2].State != "TIME-WAIT" {
		t.Errorf("expected third socket state 'TIME-WAIT', got '%s'", sockets[2].State)
	}
}

func TestParseSsOutput_EmptyOutput(t *testing.T) {
	sockets, err := parseSsOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sockets) != 0 {
		t.Errorf("expected 0 sockets, got %d", len(sockets))
	}
}

func TestParseSsOutput_HeaderLineSkipped(t *testing.T) {
	input := `State Rec-Q Send-Q Local Address:Port Peer Address:Port Process
ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		cwnd:10`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Errorf("expected 1 socket, got %d", len(sockets))
	}
}

func TestParseSsOutput_MalformedContinuationLine(t *testing.T) {
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		completely invalid token that should be ignored
		rtt:100/10
		unknown_field:garbage
		cwnd:10`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	// Should still have parsed the valid fields
	sock := sockets[0]
	if sock.SndCwnd == nil || *sock.SndCwnd != 10 {
		t.Errorf("expected cwnd 10, got %v", sock.SndCwnd)
	}
	if sock.RTTUs == nil || *sock.RTTUs != 100000 {
		t.Errorf("expected RTT 100000us, got %v", sock.RTTUs)
	}
}

func TestParseSsOutput_SsthreshParsing(t *testing.T) {
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		ssthresh:512
		cwnd:10`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	sock := sockets[0]
	if sock.Ssthresh == nil {
		t.Error("expected ssthresh to be set")
	} else if *sock.Ssthresh != 512 {
		t.Errorf("expected ssthresh 512, got %d", *sock.Ssthresh)
	}
}

func TestParseSsOutput_LossSignals(t *testing.T) {
	input := `ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080
		unacked:5/10
		lost:2/8
		sacked:3/12
		reordering:5`

	sockets, err := parseSsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sockets) != 1 {
		t.Fatalf("expected 1 socket, got %d", len(sockets))
	}

	sock := sockets[0]

	if sock.Unacked == nil || *sock.Unacked != 5 {
		t.Errorf("expected unacked 5, got %v", sock.Unacked)
	}
	if sock.Lost == nil || *sock.Lost != 2 {
		t.Errorf("expected lost 2, got %v", sock.Lost)
	}
	if sock.Sacked == nil || *sock.Sacked != 3 {
		t.Errorf("expected sacked 3, got %v", sock.Sacked)
	}
	if sock.Reordering == nil || *sock.Reordering != 5 {
		t.Errorf("expected reordering 5, got %v", sock.Reordering)
	}
}

func TestFindMatchingSocket(t *testing.T) {
	sockets := []ssSocket{
		{State: "ESTAB", Remote: "10.0.0.5:8080"},
		{State: "ESTAB", Remote: "10.0.0.6:9090"},
		{State: "TIME-WAIT", Remote: "10.0.0.5:8080"},
		{State: "ESTAB", Remote: "10.0.0.7:7070"},
	}

	// Match first established socket to 10.0.0.5
	matched, count := findMatchingSocket(sockets, "10.0.0.5")
	if matched == nil {
		t.Fatal("expected match")
	}
	if matched.Remote != "10.0.0.5:8080" {
		t.Errorf("expected remote '10.0.0.5:8080', got '%s'", matched.Remote)
	}
	if count != 1 { // Only one ESTAB socket to 10.0.0.5 (TIME-WAIT is skipped)
		t.Errorf("expected match count 1 (1 ESTAB to IP), got %d", count)
	}

	// No match for non-existent IP
	matched, count = findMatchingSocket(sockets, "192.0.2.1")
	if matched != nil {
		t.Error("expected no match")
	}
	if count != 0 {
		t.Errorf("expected match count 0, got %d", count)
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10.0.0.1:8080", "10.0.0.1"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"192.168.1.100:3000", "192.168.1.100"},
	}

	for _, tt := range tests {
		result := extractIP(tt.input)
		if result != tt.expected {
			t.Errorf("extractIP(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestRedactAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10.0.0.1:8080", "redacted:8080"},
		{"[::1]:8080", "redacted:8080"},
		{"192.168.1.100:443", "redacted:443"},
		{"10.0.0.1", "redacted"},
		{"", ""},
	}

	for _, tt := range tests {
		result := redactAddress(tt.input)
		if result != tt.expected {
			t.Errorf("redactAddress(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestParseSSMilliseconds(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		// Explicit milliseconds -> microseconds
		{"100ms", 100000},
		{"50.5ms", 50500},
		// Explicit microseconds -> microseconds (no conversion)
		{"100us", 100},
		{"100µs", 100},
		// Seconds -> microseconds
		{"1s", 1000000},
		{"0.5s", 500000},
		// Without unit: ss typically shows ms, so "100" means 100ms -> 100000us
		{"100", 100000},
		// RTT variance style: "12.0" means 12ms -> 12000us
		{"12.0", 12000},
		// mdev suffix: still in ms
		{"10mdev", 10000},
		// Empty string
		{"", 0},
	}

	for _, tt := range tests {
		result := parseSSMilliseconds(tt.input)
		if result != tt.expected {
			t.Errorf("parseSSMilliseconds(%q): expected %d, got %d", tt.input, tt.expected, result)
		}
	}
}

func TestIsSocketStartLine(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ESTAB 0 0 10.0.0.1:45678 10.0.0.5:8080", true},
		{"LISTEN 0 128 0.0.0.0:8080 0.0.0.0:0", true},
		{"TIME-WAIT 0 0 10.0.0.1:45678 10.0.0.5:8080", true},
		{"State Rec-Q Send-Q Local Address:Port", false},
		{"rtt:100/10", false},
		{"completely invalid line", false},
	}

	for _, tt := range tests {
		result := isSocketStartLine(tt.input)
		if result != tt.expected {
			t.Errorf("isSocketStartLine(%q): expected %v, got %v", tt.input, tt.expected, result)
		}
	}
}
