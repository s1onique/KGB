// identity.go — Process identity verification via procfs
//
// Verifies process identity by reading PID and starttime from /proc/<pid>/stat.
// Starttime is field 22 (0-indexed: 21) after stripping PID and comm.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package procfs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Identity captures the process identity tuple for binding.
type Identity struct {
	PID       int
	StartTime uint64
	Comm      string
}

// ReadIdentity reads process identity from /proc/<pid>/stat.
func ReadIdentity(pid int) (*Identity, error) {
	path := fmt.Sprintf("/proc/%d/stat", pid)
	file, err := os.Open(path)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "open stat", Err: err}
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	data, err := reader.ReadBytes(0) // Read until EOF or null
	if err != nil && err != io.EOF {
		return nil, &ProcError{PID: pid, Op: "read stat", Err: err}
	}

	return parseStatLine(pid, data)
}

// parseStatLine parses a /proc/<pid>/stat line.
// The comm field (field 2) may contain spaces and parentheses.
// After comm, fields 3-22 follow (21 fields after comm).
// Field 22 (starttime) is at index 21 in the slice after stripping comm.
func parseStatLine(pid int, data []byte) (*Identity, error) {
	line := strings.TrimSpace(string(data))

	// Find the last ')' which marks end of comm field
	lastParen := strings.LastIndex(line, ")")
	if lastParen == -1 {
		return nil, &ProcError{PID: pid, Op: "parse stat (no comm end)", Err: fmt.Errorf("no closing paren")}
	}

	// Extract comm (everything between first '(' and last ')')
	firstParen := strings.Index(line, "(")
	if firstParen == -1 || firstParen > lastParen {
		return nil, &ProcError{PID: pid, Op: "parse stat (no comm start)", Err: fmt.Errorf("no opening paren")}
	}
	comm := line[firstParen+1 : lastParen]

	// Fields after comm start with a space after ')'
	fieldsStr := strings.TrimSpace(line[lastParen+1:])
	if len(fieldsStr) == 0 {
		return nil, &ProcError{PID: pid, Op: "parse stat (no fields)", Err: fmt.Errorf("no fields after comm")}
	}

	fields := strings.Fields(fieldsStr)
	if len(fields) < 21 {
		return nil, &ProcError{PID: pid, Op: "parse stat (insufficient fields)", Err: fmt.Errorf("only %d fields, need 21", len(fields))}
	}

	// Field 22 (starttime) is at index 21 in fields slice (0-indexed)
	// Fields after comm: state(1), ppid(2), ... starttime(20) = index 19
	// Actually Linux /proc/<pid>/stat fields after comm:
	// 3: state (index 0 in fields)
	// 4: ppid (index 1)
	// ...
	// 22: starttime (index 19 in fields)
	starttimeStr := fields[19]
	starttime, err := strconv.ParseUint(starttimeStr, 10, 64)
	if err != nil {
		return nil, &ProcError{PID: pid, Op: "parse starttime", Err: err}
	}

	return &Identity{
		PID:       pid,
		StartTime: starttime,
		Comm:      comm,
	}, nil
}

// Equal checks if two identities refer to the same process instance.
// Process replacement is detected if PID matches but starttime differs.
func (i *Identity) Equal(other *Identity) bool {
	if i == nil || other == nil {
		return false
	}
	return i.PID == other.PID && i.StartTime == other.StartTime
}

// IsReplacedBy checks if this identity was replaced by another.
// Process replacement = same PID, different starttime.
func (i *Identity) IsReplacedBy(other *Identity) bool {
	if i == nil || other == nil {
		return false
	}
	return i.PID == other.PID && i.StartTime != other.StartTime
}

// ProcError represents a procfs operation error.
type ProcError struct {
	PID int
	Op  string
	Err error
}

func (e *ProcError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("procfs %s pid %d: %v", e.Op, e.PID, e.Err)
	}
	return fmt.Sprintf("procfs %s pid %d", e.Op, e.PID)
}

func (e *ProcError) Unwrap() error {
	return e.Err
}

// IsZombie returns true if the error indicates the process is zombie.
func (e *ProcError) IsZombie() bool {
	return strings.Contains(e.Op, "zombie") || strings.Contains(e.Op, "exited")
}

// Sentinel errors for cgroup operations
var (
	ErrNoUnifiedCgroup = fmt.Errorf("no unified cgroup v2 record found")
	ErrNoCgroup2Mount  = fmt.Errorf("cgroup2 mount not found in mountinfo")
	ErrPathTraversal   = fmt.Errorf("path traversal detected")
)
