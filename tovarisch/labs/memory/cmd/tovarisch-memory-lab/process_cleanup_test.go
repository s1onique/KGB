// Copyright 2025 s1onique. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractProcessIdentity_ValidCSV(t *testing.T) {
	// Create a temporary CSV file with valid process identity
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345,9876543210,1024
1234567891,12345,9876543210,2048
1234567892,12345,9876543210,3072
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	pid, startTime, err := extractProcessIdentity(samplesPath)
	if err != nil {
		t.Fatalf("extractProcessIdentity failed: %v", err)
	}
	if pid != 12345 {
		t.Errorf("expected PID 12345, got %d", pid)
	}
	if startTime != 9876543210 {
		t.Errorf("expected start time 9876543210, got %d", startTime)
	}
}

func TestExtractProcessIdentity_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for empty file, got nil")
	}
}

func TestExtractProcessIdentity_HeaderOnly(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for header-only file, got nil")
	}
}

func TestExtractProcessIdentity_MissingPIDColumn(t *testing.T) {
	content := `timestamp,process_start_time,rss_kb
1234567890,9876543210,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for missing PID column, got nil")
	}
}

func TestExtractProcessIdentity_MissingStartTimeColumn(t *testing.T) {
	content := `timestamp,process_pid,rss_kb
1234567890,12345,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for missing start time column, got nil")
	}
}

func TestExtractProcessIdentity_DuplicateColumns(t *testing.T) {
	content := `timestamp,process_pid,process_pid,process_start_time
1234567890,12345,12346,9876543210
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for duplicate PID column, got nil")
	}
}

func TestExtractProcessIdentity_EmptyPID(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,,9876543210,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for empty PID, got nil")
	}
}

func TestExtractProcessIdentity_EmptyStartTime(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345,,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for empty start time, got nil")
	}
}

func TestExtractProcessIdentity_InvalidPID(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,not_a_number,9876543210,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for invalid PID, got nil")
	}
}

func TestExtractProcessIdentity_InvalidStartTime(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345,not_a_number,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for invalid start time, got nil")
	}
}

func TestExtractProcessIdentity_InconsistentPID(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345,9876543210,1024
1234567891,12346,9876543210,2048
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for inconsistent PID, got nil")
	}
}

func TestExtractProcessIdentity_InconsistentStartTime(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345,9876543210,1024
1234567891,12345,9876543211,2048
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for inconsistent start time, got nil")
	}
}

func TestExtractProcessIdentity_ZeroPID(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,0,9876543210,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for zero PID, got nil")
	}
}

func TestExtractProcessIdentity_ZeroStartTime(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345,0,1024
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for zero start time, got nil")
	}
}

func TestExtractProcessIdentity_WrongFieldCount(t *testing.T) {
	content := `timestamp,process_pid,process_start_time,rss_kb
1234567890,12345
`
	tmpDir := t.TempDir()
	samplesPath := filepath.Join(tmpDir, "samples.csv")
	if err := os.WriteFile(samplesPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write samples.csv: %v", err)
	}

	_, _, err := extractProcessIdentity(samplesPath)
	if err == nil {
		t.Error("expected error for wrong field count, got nil")
	}
}

func TestExtractProcessIdentity_FileNotFound(t *testing.T) {
	_, _, err := extractProcessIdentity("/nonexistent/samples.csv")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestParseProcStatStartTime_ValidStat(t *testing.T) {
	// Format: pid (comm) state ppid ... starttime(jiffies after boot)
	// Field 22 (starttime) is at index 19 after ')'
	data := []byte("12345 (test-process) S 1 12345 12345 0 -1 4194304 1000 0 0 0 0 0 0 0 20 0 1 0 9876543210 12345678 256 0 0 0 0 0 0 /root/test")

	startTime, err := parseProcStatStartTime(data)
	if err != nil {
		t.Fatalf("parseProcStatStartTime failed: %v", err)
	}
	if startTime != 9876543210 {
		t.Errorf("expected start time 9876543210, got %d", startTime)
	}
}

func TestParseProcStatStartTime_CommWithSpaces(t *testing.T) {
	// Process name with spaces, parentheses
	data := []byte("12345 (my test process) S 1 12345 12345 0 -1 4194304 1000 0 0 0 0 0 0 0 20 0 1 0 5555555555 12345678 256 0 0 0 0 0 0 /root/my test")

	startTime, err := parseProcStatStartTime(data)
	if err != nil {
		t.Fatalf("parseProcStatStartTime failed: %v", err)
	}
	if startTime != 5555555555 {
		t.Errorf("expected start time 5555555555, got %d", startTime)
	}
}

func TestParseProcStatStartTime_MissingClosingParen(t *testing.T) {
	data := []byte("12345 (test-process S 1 12345")

	_, err := parseProcStatStartTime(data)
	if err == nil {
		t.Error("expected error for missing closing paren, got nil")
	}
}

func TestParseProcStatStartTime_InsufficientFields(t *testing.T) {
	// Only a few fields after the closing paren
	data := []byte("12345 (test) S 1 2 3")

	_, err := parseProcStatStartTime(data)
	if err == nil {
		t.Error("expected error for insufficient fields, got nil")
	}
}

func TestParseProcStatStartTime_InvalidStartTime(t *testing.T) {
	// Field 22 (starttime at index 19) must be at position 19 after ')'
	data := []byte("12345 (test) S 1 12345 12345 0 -1 4194304 1000 0 0 0 0 0 0 0 20 0 1 0 not_a_number 12345678 256 0 0 0 0 0 0")

	_, err := parseProcStatStartTime(data)
	if err == nil {
		t.Error("expected error for invalid start time, got nil")
	}
}

func TestInspectProcessGone_InvalidPID(t *testing.T) {
	status, err := inspectProcessGone(0, 1000)
	if err == nil {
		t.Error("expected error for zero PID, got nil")
	}
	if status != ProcessUnavailable {
		t.Errorf("expected ProcessUnavailable status, got %v", status)
	}
}

func TestInspectProcessGone_InvalidStartTime(t *testing.T) {
	status, err := inspectProcessGone(12345, 0)
	if err == nil {
		t.Error("expected error for zero start time, got nil")
	}
	if status != ProcessUnavailable {
		t.Errorf("expected ProcessUnavailable status, got %v", status)
	}
}

func TestInspectProcessGone_ProcessNotExist(t *testing.T) {
	// Use a very high PID that's extremely unlikely to exist
	status, err := inspectProcessGone(999999999, 1)
	if err != nil {
		t.Fatalf("inspectProcessGone failed: %v", err)
	}
	if status != ProcessGone {
		t.Errorf("expected ProcessGone (0), got %v (%d)", status, status)
	}
}

func TestInspectProcessGone_ProcessAlive(t *testing.T) {
	// Read PID 1's actual stat
	data, err := os.ReadFile("/proc/1/stat")
	if err != nil {
		t.Skipf("cannot read /proc/1/stat: %v", err)
	}

	actualStart, parseErr := parseProcStatStartTime(data)
	if parseErr != nil {
		t.Fatalf("failed to parse PID 1 start time: %v", parseErr)
	}

	// Check PID 1 with its actual start time - should be ProcessStillAlive
	status, err := inspectProcessGone(1, actualStart)
	if err != nil {
		t.Fatalf("inspectProcessGone failed: %v", err)
	}
	if status != ProcessStillAlive {
		t.Errorf("expected ProcessStillAlive (1) for init with matching start time, got %v (%d)", status, status)
	}
}

func TestInspectProcessGone_PIDReused(t *testing.T) {
	// Test with PID 1 but wrong start time
	status, err := inspectProcessGone(1, 999999999999)
	if err != nil {
		t.Fatalf("inspectProcessGone failed: %v", err)
	}
	// PID 1 exists with different start time = PID reused
	if status != ProcessPIDReused {
		t.Errorf("expected ProcessPIDReused for init with wrong start time, got %v", status)
	}
}

func TestIsProcessGone(t *testing.T) {
	// Process that doesn't exist
	if !isProcessGone(2, 1) {
		t.Error("expected isProcessGone(true) for nonexistent process")
	}

	// Process 1 with matching start time
	data, err := os.ReadFile("/proc/1/stat")
	if err != nil {
		t.Skipf("cannot read /proc/1/stat: %v", err)
	}
	actualStart, _ := parseProcStatStartTime(data)

	if isProcessGone(1, actualStart) {
		t.Error("expected isProcessGone(false) for init with matching start time")
	}
}
