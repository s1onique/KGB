// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"sync/atomic"
)

// OSExecSemaphore bounds concurrent OS ping executions.
// Default to 1 for router safety; can be configured higher for lab environments.
type OSExecSemaphore struct {
	sem     chan struct{}
	started uint64
}

// NewOSExecSemaphore creates a new semaphore with the given concurrency limit.
func NewOSExecSemaphore(maxConcurrent int) *OSExecSemaphore {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &OSExecSemaphore{
		sem: make(chan struct{}, maxConcurrent),
	}
}

// Acquire acquires a slot in the semaphore.
func (s *OSExecSemaphore) Acquire() {
	atomic.AddUint64(&s.started, 1)
	s.sem <- struct{}{}
}

// Release releases a slot in the semaphore.
func (s *OSExecSemaphore) Release() {
	<-s.sem
}

// SemaphoreGuard provides RAII-style semaphore acquire/release.
type SemaphoreGuard struct {
	sem *OSExecSemaphore
}

// AcquireSemaphore creates a guard that acquires the semaphore.
func AcquireSemaphore(sem *OSExecSemaphore) *SemaphoreGuard {
	sem.Acquire()
	return &SemaphoreGuard{sem: sem}
}

// Release releases the semaphore (safe to call multiple times).
func (g *SemaphoreGuard) Release() {
	if g.sem != nil {
		g.sem.Release()
		g.sem = nil
	}
}
