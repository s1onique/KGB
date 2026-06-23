// errors.go — Error types for memory lab runner
//
// Provides typed errors for service lifecycle issues.

package main

import (
	"bytes"
	"fmt"
	"os"
)

// ServiceExitError indicates the service exited before becoming ready.
type ServiceExitError struct {
	PID        int
	Argv       []string
	ExitError  *os.ProcessState
	StdoutTail string
}

func (e *ServiceExitError) Error() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("service exited before readiness (pid %d): ", e.PID))
	if e.ExitError != nil {
		buf.WriteString(fmt.Sprintf("exit status %d", e.ExitError.ExitCode()))
	} else {
		buf.WriteString("unknown exit status")
	}
	buf.WriteString("\ncommand: ")
	for i, arg := range e.Argv {
		if i > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(arg)
	}
	if e.StdoutTail != "" {
		buf.WriteString("\nstdout/stderr tail:\n")
		buf.WriteString(e.StdoutTail)
	}
	return buf.String()
}

// ReadinessTimeoutError indicates the service didn't respond to readiness check.
type ReadinessTimeoutError struct {
	PID          int
	ReadinessURL string
	StdoutTail   string
}

func (e *ReadinessTimeoutError) Error() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("service did not become ready at %s (pid %d)", e.ReadinessURL, e.PID))
	if e.StdoutTail != "" {
		buf.WriteString("\nstdout/stderr tail:\n")
		buf.WriteString(e.StdoutTail)
	}
	return buf.String()
}
