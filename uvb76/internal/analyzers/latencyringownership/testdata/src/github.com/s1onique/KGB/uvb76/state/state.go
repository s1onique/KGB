// Package state is the latency ring owner package.
package state

import (
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// LatencySample represents a single latency measurement.
type LatencySample struct {
	Timestamp time.Time
	LatencyMs float64
	Reachable bool
}

// LatencyTracker tracks latency data for a single target.
type LatencyTracker struct {
	mu            sync.RWMutex
	recentSamples []LatencySample
	maxSamples    int
	head          int
	count         int
}

// GetRecentSamples returns the most recent `limit` latency samples.
func (lt *LatencyTracker) GetRecentSamples(limit int) []LatencySample {
	return nil
}

// GetSampleWindow returns an immutable analysis snapshot.
func (lt *LatencyTracker) GetSampleWindow(limit int) domain.SampleWindow {
	return domain.SampleWindow{}
}

type Manager struct{}

func (m *Manager) GetHTTPSampleWindow(targetID string, limit int) domain.SampleWindow {
	return domain.SampleWindow{}
}

func (m *Manager) GetICMPSampleWindow(targetID string, limit int) domain.SampleWindow {
	return domain.SampleWindow{}
}

// OwnerPackageMayUseRawSamples demonstrates that the state package is allowed
// to call GetRecentSamples directly.
func OwnerPackageMayUseRawSamples(t *LatencyTracker) int {
	return len(t.GetRecentSamples(100))
}
