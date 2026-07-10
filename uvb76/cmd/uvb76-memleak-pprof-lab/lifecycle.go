package main

import (
	"sync"
	"syscall"
)

// LifecycleState represents the current state of the lab harness.
type LifecycleState int

const (
	StateSetup LifecycleState = iota
	StateLaunching
	StateRunning
	StateReady
	StateCollecting
	StateCollected
	StateVerified
	StateShutdown

	// Failure states
	StateFailedStartup
	StateFailedReadiness
	StateFailedCollection
	StateFailedVerification
)

func (s LifecycleState) String() string {
	switch s {
	case StateSetup:
		return "SETUP"
	case StateLaunching:
		return "LAUNCHING"
	case StateRunning:
		return "RUNNING"
	case StateReady:
		return "READY"
	case StateCollecting:
		return "COLLECTING"
	case StateCollected:
		return "COLLECTED"
	case StateVerified:
		return "VERIFIED"
	case StateShutdown:
		return "SHUTDOWN"
	case StateFailedStartup:
		return "FAILED_STARTUP"
	case StateFailedReadiness:
		return "FAILED_READINESS"
	case StateFailedCollection:
		return "FAILED_COLLECTION"
	case StateFailedVerification:
		return "FAILED_VERIFICATION"
	default:
		return "UNKNOWN"
	}
}

// ProcessState holds shared process state for the monitored child.
type ProcessState struct {
	mu       sync.RWMutex
	running  bool
	exited   bool
	exitCode int
	signal   syscall.Signal
	done     chan struct{} // Closed when Wait() completes
}

// Running returns true if the process is still running.
func (ps *ProcessState) Running() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.running
}

// Exited returns true if the process has exited.
func (ps *ProcessState) Exited() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.exited
}

// ExitInfo returns the exit code and signal if the process has exited.
func (ps *ProcessState) ExitInfo() (exitCode int, signal syscall.Signal) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.exitCode, ps.signal
}

// SetExited records that the process has exited.
func (ps *ProcessState) SetExited(code int, sig syscall.Signal) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.exited = true
	ps.running = false
	ps.exitCode = code
	ps.signal = sig
}
