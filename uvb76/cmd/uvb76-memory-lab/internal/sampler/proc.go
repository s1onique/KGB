// Package sampler provides Linux /proc/<pid> RSS and goroutine sampling.
package sampler

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProcSample captures process metrics from /proc/<pid>.
type ProcSample struct {
	Timestamp time.Time
	PID       int
	RSSBytes  uint64
	VSZBytes  uint64
	Threads   int
	FDCount   int
}

// ParseProcStatusReader parses /proc/<pid>/status from any io.Reader.
// Returns error if VmRSS, VmSize, or Threads are missing.
func ParseProcStatusReader(r io.Reader) (rss, vsz uint64, threads int, err error) {
	scanner := bufio.NewScanner(r)
	var foundRSS, foundVSZ, foundThreads bool

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "VmRSS":
			rss, err = parseKBytes(value)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parse VmRSS: %w", err)
			}
			foundRSS = true
		case "VmSize":
			vsz, err = parseKBytes(value)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parse VmSize: %w", err)
			}
			foundVSZ = true
		case "Threads":
			threads, err = strconv.Atoi(value)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parse Threads: %w", err)
			}
			foundThreads = true
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, 0, 0, fmt.Errorf("scan status: %w", err)
	}

	if !foundRSS {
		return 0, 0, 0, fmt.Errorf("VmRSS not found in /proc/<pid>/status")
	}
	if !foundVSZ {
		return 0, 0, 0, fmt.Errorf("VmSize not found in /proc/<pid>/status")
	}
	if !foundThreads {
		return 0, 0, 0, fmt.Errorf("Threads not found in /proc/<pid>/status")
	}

	// VmRSS is in kB, convert to bytes
	rss *= 1024
	vsz *= 1024

	return rss, vsz, threads, nil
}

// ParseProcStatus parses /proc/<pid>/status and returns RSS, VSZ, and thread count.
// Returns error if the file cannot be opened or required fields are missing.
func ParseProcStatus(pid int) (rss, vsz uint64, threads int, err error) {
	path := fmt.Sprintf("/proc/%d/status", pid)
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	return ParseProcStatusReader(file)
}

// parseKBytes parses a value like "123456 kB" and returns the value in bytes.
func parseKBytes(s string) (uint64, error) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty value")
	}
	val, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// CountFDs counts the number of file descriptors for a process.
func CountFDs(pid int) (int, error) {
	fdPath := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", fdPath, err)
	}
	return len(entries), nil
}

// SampleProc samples RSS, VSZ, threads, and FD count for a process.
func SampleProc(pid int) (*ProcSample, error) {
	rss, vsz, threads, err := ParseProcStatus(pid)
	if err != nil {
		return nil, err
	}

	fdCount, err := CountFDs(pid)
	if err != nil {
		// FD count is best-effort; log but don't fail
		fdCount = -1
	}

	return &ProcSample{
		Timestamp: time.Now(),
		PID:       pid,
		RSSBytes:  rss,
		VSZBytes:  vsz,
		Threads:   threads,
		FDCount:   fdCount,
	}, nil
}

// ProcSampleCSVHeader returns the CSV header for ProcSample series.
func ProcSampleCSVHeader() string {
	return "elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count"
}

// ProcSampleToCSV writes a sample to CSV format.
func ProcSampleToCSV(sample *ProcSample, start time.Time) string {
	elapsed := sample.Timestamp.Sub(start).Seconds()
	return fmt.Sprintf("%.1f,%d,%d,%d,%d,%d",
		elapsed, sample.PID, sample.RSSBytes, sample.VSZBytes, sample.Threads, sample.FDCount)
}
