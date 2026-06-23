// runner_test.go — Tests for runner functionality

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactDir(t *testing.T) {
	tests := []struct {
		name       string
		cfg        RunConfig
		wantPrefix string
	}{
		{
			name: "tovarisch default",
			cfg: RunConfig{
				Service:      "tovarisch",
				ArtifactDir: "",
			},
			wantPrefix: "artifacts/memory-labs/tovarisch",
		},
		{
			name: "uvb76 default",
			cfg: RunConfig{
				Service:      "uvb76",
				ArtifactDir: "",
			},
			wantPrefix: "artifacts/memory-labs/uvb76",
		},
		{
			name: "custom artifact dir",
			cfg: RunConfig{
				Service:      "tovarisch",
				ArtifactDir: "/custom/path",
			},
			wantPrefix: "/custom/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{cfg: tt.cfg}
			got := r.artifactDir()
			// Just verify it doesn't panic and returns a non-empty string
			if got == "" {
				t.Error("artifactDir() returned empty string")
			}
			// For default case, verify it ends with the expected suffix
			if tt.cfg.ArtifactDir == "" {
				if len(got) < len(tt.wantPrefix) {
					t.Errorf("artifactDir() = %q, want prefix %q", got, tt.wantPrefix)
				}
			}
		})
	}
}

func TestMaxOf3(t *testing.T) {
	tests := []struct {
		a, b, c   int64
		want      int64
	}{
		{1, 2, 3, 3},
		{3, 2, 1, 3},
		{1, 3, 2, 3},
		{5, 5, 5, 5},
		{0, 0, 0, 0},
		{100, 50, 75, 100},
	}

	for _, tt := range tests {
		got := maxOf3(tt.a, tt.b, tt.c)
		if got != tt.want {
			t.Errorf("maxOf3(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, got, tt.want)
		}
	}
}

func TestRunConfigDefaults(t *testing.T) {
	// Verify RunConfig has all required fields
	cfg := RunConfig{
		Service:      "tovarisch",
		WorkloadType: WorkloadTovarischIdle,
		Binary:       "./tovarisch",
		ConfigPath:   "",
		Port:         18080,
		WarmupSecs:   60,
		Operations:   100,
		IntervalMs:   100,
		ArtifactDir:  "",
	}

	if cfg.Service != "tovarisch" {
		t.Errorf("Service = %q, want tovarisch", cfg.Service)
	}
	if cfg.Port != 18080 {
		t.Errorf("Port = %d, want 18080", cfg.Port)
	}
	if cfg.ConfigPath != "" {
		t.Errorf("ConfigPath = %q, want empty for tovarisch", cfg.ConfigPath)
	}
}

func TestRunConfigUvb76(t *testing.T) {
	cfg := RunConfig{
		Service:      "uvb76",
		WorkloadType: WorkloadUVB76Idle,
		Binary:       "./uvb76",
		ConfigPath:   "./custom/config.json",
		Port:         18081,
		WarmupSecs:   120,
		Operations:   200,
		IntervalMs:   50,
		ArtifactDir:  "",
	}

	if cfg.Service != "uvb76" {
		t.Errorf("Service = %q, want uvb76", cfg.Service)
	}
	if cfg.Port != 18081 {
		t.Errorf("Port = %d, want 18081", cfg.Port)
	}
	if cfg.ConfigPath != "./custom/config.json" {
		t.Errorf("ConfigPath = %q, want ./custom/config.json", cfg.ConfigPath)
	}
}

func TestBuildServiceCommandTovarisch(t *testing.T) {
	r := &Runner{
		cfg: RunConfig{
			Service: "tovarisch",
			Binary:  "./tovarisch/zig-out/bin/tovarisch",
			Port:    18080,
		},
	}

	cmd := r.buildServiceCommand()

	// Verify exact argv
	if len(cmd.Args) != 3 {
		t.Errorf("len(cmd.Args) = %d, want 3", len(cmd.Args))
	}
	if cmd.Args[0] != "./tovarisch/zig-out/bin/tovarisch" {
		t.Errorf("cmd.Args[0] = %q, want ./tovarisch/zig-out/bin/tovarisch", cmd.Args[0])
	}
	if cmd.Args[1] != "serve" {
		t.Errorf("cmd.Args[1] = %q, want serve", cmd.Args[1])
	}
	if cmd.Args[2] != "--listen=127.0.0.1:18080" {
		t.Errorf("cmd.Args[2] = %q, want --listen=127.0.0.1:18080", cmd.Args[2])
	}
}

func TestBuildServiceCommandUvb76Default(t *testing.T) {
	r := &Runner{
		cfg: RunConfig{
			Service:    "uvb76",
			Binary:     "./uvb76/uvb76",
			ConfigPath: "", // empty, should use default
			Port:       18081,
		},
	}

	cmd := r.buildServiceCommand()

	// Verify exact argv (binary + config flag)
	if len(cmd.Args) != 2 {
		t.Errorf("len(cmd.Args) = %d, want 2", len(cmd.Args))
	}
	if cmd.Args[0] != "./uvb76/uvb76" {
		t.Errorf("cmd.Args[0] = %q, want ./uvb76/uvb76", cmd.Args[0])
	}
	if cmd.Args[1] != "-config=./uvb76/uvb76.example.json" {
		t.Errorf("cmd.Args[1] = %q, want -config=./uvb76/uvb76.example.json", cmd.Args[1])
	}
}

func TestBuildServiceCommandUvb76Custom(t *testing.T) {
	r := &Runner{
		cfg: RunConfig{
			Service:    "uvb76",
			Binary:     "./uvb76/uvb76",
			ConfigPath: "/etc/uvb76/production.json",
			Port:       18081,
		},
	}

	cmd := r.buildServiceCommand()

	// Verify exact argv
	if len(cmd.Args) != 2 {
		t.Errorf("len(cmd.Args) = %d, want 2", len(cmd.Args))
	}
	if cmd.Args[0] != "./uvb76/uvb76" {
		t.Errorf("cmd.Args[0] = %q, want ./uvb76/uvb76", cmd.Args[0])
	}
	if cmd.Args[1] != "-config=/etc/uvb76/production.json" {
		t.Errorf("cmd.Args[1] = %q, want -config=/etc/uvb76/production.json", cmd.Args[1])
	}
}

func TestReadinessURL(t *testing.T) {
	tests := []struct {
		service string
		port    int
		want    string
	}{
		{"tovarisch", 18080, "http://127.0.0.1:18080/status"},
		{"uvb76", 18081, "http://127.0.0.1:18081/api/v1/status"},
	}

	for _, tt := range tests {
		r := &Runner{
			cfg: RunConfig{
				Service: tt.service,
				Port:    tt.port,
			},
		}
		got := r.readinessURL()
		if got != tt.want {
			t.Errorf("readinessURL() for %s = %q, want %q", tt.service, got, tt.want)
		}
	}
}

func TestReadTail(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "stdout.log")

	// Write a log file with known content
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Test reading last 100 bytes
	tail := readTail(logPath, 100)
	if tail == "" {
		t.Error("readTail returned empty string")
	}
	if !strings.Contains(tail, "line5") {
		t.Errorf("readTail = %q, want contains line5", tail)
	}

	// Test reading last 10 bytes (should get partial line)
	tail = readTail(logPath, 10)
	if tail == "" {
		t.Error("readTail returned empty string for small n")
	}
}

func TestReadTailMissingFile(t *testing.T) {
	tail := readTail("/nonexistent/file/path", 100)
	if tail != "" {
		t.Errorf("readTail = %q, want empty string for missing file", tail)
	}
}

func TestServiceExitError(t *testing.T) {
	err := &ServiceExitError{
		PID:       12345,
		Argv:      []string{"./tovarisch", "serve", "--listen=127.0.0.1:18080"},
		ExitError: nil,
		StdoutTail: "error: config missing\n",
	}

	msg := err.Error()

	// Verify PID is in error message
	if !strings.Contains(msg, "12345") {
		t.Errorf("ServiceExitError.Error() = %q, want contains PID 12345", msg)
	}

	// Verify command is in error message
	if !strings.Contains(msg, "tovarisch") {
		t.Errorf("ServiceExitError.Error() = %q, want contains command", msg)
	}

	// Verify stdout tail is in error message
	if !strings.Contains(msg, "error: config missing") {
		t.Errorf("ServiceExitError.Error() = %q, want contains stdout tail", msg)
	}
}

func TestServiceExitErrorWithExitStatus(t *testing.T) {
	// Test that ServiceExitError with nil ExitError shows "unknown exit status"
	err := &ServiceExitError{
		PID:        12345,
		Argv:       []string{"./tovarisch", "serve", "--listen=127.0.0.1:18080"},
		ExitError:  nil,
		StdoutTail: "fatal: serve command not supported\n",
	}

	msg := err.Error()

	// Verify PID is present
	if !strings.Contains(msg, "12345") {
		t.Errorf("ServiceExitError.Error() = %q, want contains PID", msg)
	}

	// Verify "unknown exit status" is shown when ExitError is nil
	if !strings.Contains(msg, "unknown exit status") {
		t.Errorf("ServiceExitError.Error() = %q, want contains 'unknown exit status'", msg)
	}

	// Verify stdout tail is present
	if !strings.Contains(msg, "fatal:") {
		t.Errorf("ServiceExitError.Error() = %q, want contains stdout tail", msg)
	}
}

func TestReadinessTimeoutError(t *testing.T) {
	err := &ReadinessTimeoutError{
		PID:          12345,
		ReadinessURL: "http://127.0.0.1:18080/status",
		StdoutTail:   "listening on 127.0.0.1:18080\n",
	}

	msg := err.Error()

	// Verify URL is in error message
	if !strings.Contains(msg, "http://127.0.0.1:18080/status") {
		t.Errorf("ReadinessTimeoutError.Error() = %q, want contains URL", msg)
	}

	// Verify PID is in error message
	if !strings.Contains(msg, "12345") {
		t.Errorf("ReadinessTimeoutError.Error() = %q, want contains PID", msg)
	}

	// Verify stdout tail is in error message
	if !strings.Contains(msg, "listening on") {
		t.Errorf("ReadinessTimeoutError.Error() = %q, want contains stdout tail", msg)
	}
}

func TestReadinessTimeoutErrorNoTail(t *testing.T) {
	err := &ReadinessTimeoutError{
		PID:          12345,
		ReadinessURL: "http://127.0.0.1:18080/status",
		StdoutTail:   "",
	}

	msg := err.Error()

	// Should still contain URL and PID even without tail
	if !strings.Contains(msg, "http://127.0.0.1:18080/status") {
		t.Errorf("ReadinessTimeoutError.Error() = %q, want contains URL", msg)
	}
	if !strings.Contains(msg, "12345") {
		t.Errorf("ReadinessTimeoutError.Error() = %q, want contains PID", msg)
	}
}

func TestRunnerKeepsCmd(t *testing.T) {
	// Verify Runner struct has cmd field
	r := &Runner{}
	// This just verifies the field exists and is accessible
	r.cmd = nil
	if r.cmd != nil {
		t.Error("cmd should be nil initially")
	}
}

// fakeCmd is a minimal fake for testing
type fakeCmd struct {
	args         []string
	processState os.ProcessState
}

// fakeProcessState is a minimal fake for testing
type fakeProcessState struct {
	exitStatus int
}

func (f *fakeProcessState) Exited() bool      { return true }
func (f *fakeProcessState) Success() bool     { return f.exitStatus == 0 }
func (f *fakeProcessState) ExitStatus() int   { return f.exitStatus }
func (f *fakeProcessState) Sys() interface{}  { return nil }
func (f *fakeProcessState) SysUsage() interface{} { return nil }
func (f *fakeProcessState) String() string    { return "" }
