package lab

import (
	"context"
	"sync"
)

// CleanupStep represents a named cleanup action.
type CleanupStep struct {
	Name string
	Fn   func(context.Context) error
}

// CleanupStack manages a stack of cleanup actions that run in reverse order.
type CleanupStack struct {
	mu    sync.Mutex
	steps []CleanupStep
}

// NewCleanupStack creates a new cleanup stack.
func NewCleanupStack() *CleanupStack {
	return &CleanupStack{}
}

// Add registers a cleanup step.
func (c *CleanupStack) Add(name string, fn func(context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.steps = append(c.steps, CleanupStep{Name: name, Fn: fn})
}

// Run executes all cleanup steps in reverse order.
// Returns any errors encountered during cleanup.
func (c *CleanupStack) Run(ctx context.Context) []error {
	c.mu.Lock()
	// Reverse the slice for LIFO order
	steps := make([]CleanupStep, len(c.steps))
	copy(steps, c.steps)
	// Clear original slice
	c.steps = nil
	c.mu.Unlock()

	var errors []error
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if err := step.Fn(ctx); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

