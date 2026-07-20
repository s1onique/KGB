// counts.go — Process resource count utilities
//
// Provides utilities for counting VMAs, file descriptors, sockets, and threads.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package procfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadVMACount reads the number of virtual memory areas for a process.
func ReadVMACount(pid int) (int, error) {
	// Count lines in /proc/<pid>/maps
	path := fmt.Sprintf("/proc/%d/maps", pid)
	file, err := os.Open(path)
	if err != nil {
		return 0, &ProcError{PID: pid, Op: "open maps", Err: err}
	}
	defer file.Close()

	count := 0
	buf := make([]byte, 0, 4096)
	for {
		n, err := file.Read(buf[:cap(buf)])
		if n > 0 {
			// Count newlines
			for _, b := range buf[:n] {
				if b == '\n' {
					count++
				}
			}
		}
		if err != nil {
			break
		}
	}
	return count, nil
}

// FDInfo holds file descriptor statistics.
type FDInfo struct {
	Total   int
	Socket  int
	pipe    int
	eventfd int
	anon    int
}

// ReadFDCounts reads file descriptor counts for a process.
func ReadFDCounts(pid int) (*FDInfo, error) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "read fd dir", Err: err}
	}

	info := &FDInfo{}
	for _, entry := range entries {
		info.Total++

		// Read the symlink target to determine type
		linkPath := filepath.Join(fdDir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			info.anon++
			continue
		}

		if strings.HasPrefix(target, "socket:[") {
			info.Socket++
		} else if strings.HasPrefix(target, "pipe:[") {
			info.pipe++
		} else if strings.HasPrefix(target, "anon_inode:[eventpoll]") ||
			strings.HasPrefix(target, "anon_inode:[eventfd]") {
			info.eventfd++
		} else if strings.HasPrefix(target, "anon_inode:") {
			info.anon++
		}
	}

	return info, nil
}

// ReadThreadCount reads the number of threads for a process.
func ReadThreadCount(pid int) (int, error) {
	// Count entries in /proc/<pid>/task
	taskDir := fmt.Sprintf("/proc/%d/task", pid)
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return 0, &ProcError{PID: pid, Op: "read task dir", Err: err}
	}
	return len(entries), nil
}

// ReadOOMCount reads OOM score for a process.
func ReadOOMScore(pid int) (int, error) {
	path := fmt.Sprintf("/proc/%d/oom_score", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, &ProcError{PID: pid, Op: "read oom_score", Err: err}
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
