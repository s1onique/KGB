// util.go — Utility functions for memory lab runner
//
// Provides helper functions for file reading and command building.

package main

import (
	"io"
	"os"
	"os/exec"
)

// readTail reads the last n bytes from a file.
func readTail(path string, n int) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return ""
	}

	var start int64
	if info.Size() > int64(n) {
		start = info.Size() - int64(n)
	}

	_, err = file.Seek(start, io.SeekStart)
	if err != nil {
		return ""
	}

	data := make([]byte, n)
	read, _ := file.Read(data)
	return string(data[:read])
}

// findRepoRootOrCWD returns the repo root or current directory.
func findRepoRootOrCWD() string {
	if root, err := findRepoRoot(); err == nil {
		return root
	}
	cwd, _ := os.Getwd()
	return cwd
}

// getBinaryVersion returns the version string of a binary.
func getBinaryVersion(binary string) string {
	out, err := exec.Command(binary, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return trimNL(string(out))
}

// getGitCommit returns the short git commit hash.
func getGitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return trimNL(string(out))
}

// getKernelVersion returns the kernel version string.
func getKernelVersion() string {
	out, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	return trimNL(string(out))
}

// trimNL removes trailing newline characters.
func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// describeWorkload returns a human-readable description of the workload.
func describeWorkload(service string, wt WorkloadType, warmupSecs int) string {
	switch wt {
	case WorkloadTovarischStatusJSON, WorkloadUVB76StatusAPIPolling:
		return "Repeated status calls after " + itoa(warmupSecs) + "s warmup"
	case WorkloadTovarischStatusJSONNetDiag, WorkloadUVB76DiagnosticCaptureLoop:
		return "Repeated status with network_diag after " + itoa(warmupSecs) + "s warmup"
	default:
		return "Idle memory footprint after " + itoa(warmupSecs) + "s warmup"
	}
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// maxOf3 returns the maximum of three int64 values.
func maxOf3(a, b, c int64) int64 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}
