package lab

import (
	"sync"
	"time"
)

// PhaseTracker tracks the current phase cursor.
type PhaseTracker struct {
	mu      sync.RWMutex
	cursor  string
	lastSet time.Time
}

// NewPhaseTracker creates a new phase tracker.
func NewPhaseTracker() *PhaseTracker {
	return &PhaseTracker{}
}

// SetCursor sets the current phase cursor.
func (t *PhaseTracker) SetCursor(phase PhaseName) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cursor = string(phase)
	t.lastSet = time.Now()
}

// GetCursor returns the current phase cursor.
func (t *PhaseTracker) GetCursor() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cursor
}

// GetLastSet returns when the cursor was last set.
func (t *PhaseTracker) GetLastSet() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastSet
}
