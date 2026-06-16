// Package state provides bounded in-memory state management for UVB-76.
package state

import (
	"sync"
	"time"
)

// TargetSnapshot represents the latest status received from a tovarisch target.
type TargetSnapshot struct {
	TargetID    string            `json:"target_id"`
	ScrapedAt   time.Time         `json:"scraped_at"`
	Reachable   bool              `json:"reachable"`
	Status      string            `json:"status,omitempty"`
	Version     string            `json:"version,omitempty"`
	NodeID      string            `json:"node_id,omitempty"`
	Checks      []CheckResult     `json:"checks,omitempty"`
	Error       string            `json:"error,omitempty"`
	RawResponse string            `json:"raw_response,omitempty"`
}

// CheckResult represents a single health check result.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Manager manages the bounded state for all targets.
type Manager struct {
	mu       sync.RWMutex
	snapshots map[string]*TargetSnapshot // keyed by target ID
}

// NewManager creates a new state manager with bounded capacity.
func NewManager() *Manager {
	return &Manager{
		snapshots: make(map[string]*TargetSnapshot),
	}
}

// UpdateSnapshot stores the latest snapshot for a target.
func (m *Manager) UpdateSnapshot(targetID string, snap *TargetSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots[targetID] = snap
}

// GetSnapshot returns the latest snapshot for a target.
func (m *Manager) GetSnapshot(targetID string) *TargetSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshots[targetID]
}

// GetAllSnapshots returns all stored snapshots.
func (m *Manager) GetAllSnapshots() map[string]*TargetSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*TargetSnapshot, len(m.snapshots))
	for k, v := range m.snapshots {
		result[k] = v
	}
	return result
}

// GetSnapshotCount returns the number of stored snapshots.
func (m *Manager) GetSnapshotCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.snapshots)
}
