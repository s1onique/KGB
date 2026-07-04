package state

import (
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// LatencySeriesSampleMeta provides minimal metadata about a retained sample.
// This is a DTO that exposes timestamp and success/failure without raw latency values,
// preserving the domain boundary while enabling exact per-window error counts.
type LatencySeriesSampleMeta struct {
	At time.Time
	OK bool
}

// LatencySeriesSnapshot combines a domain.SampleWindow with state-owned metadata
// needed by the latency series API. Raw []state.LatencySample is never exposed
// outside this package.
//
// This snapshot type provides:
// - domain.SampleWindow for percentile math (successful samples only)
// - Samples metadata (timestamps + OK/failed) for exact per-window error counts
// - ErrorCount for total failed samples in the retained window
// - Oldest/Newest timestamps for the full retained window
// - RetainedSampleCount and Capacity
type LatencySeriesSnapshot struct {
	Window                 domain.SampleWindow
	Samples                []LatencySeriesSampleMeta
	RetainedSampleCount    int
	RetainedSampleCapacity int
	ErrorCount             int
	OldestSampleTs         time.Time
	NewestSampleTs         time.Time
	ProbeKind              domain.ProbeKind
}

// GetHTTPSeriesSnapshot returns a series snapshot for HTTP latency data.
func (m *Manager) GetHTTPSeriesSnapshot(targetID string, limit int) LatencySeriesSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.httpTrackers[targetID]
	if tracker == nil {
		return LatencySeriesSnapshot{ProbeKind: domain.ProbeKindHTTP}
	}

	return buildSeriesSnapshot(tracker, limit, domain.ProbeKindHTTP)
}

// GetICMPSeriesSnapshot returns a series snapshot for ICMP latency data.
func (m *Manager) GetICMPSeriesSnapshot(targetID string, limit int) LatencySeriesSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.icmpTrackers[targetID]
	if tracker == nil {
		return LatencySeriesSnapshot{ProbeKind: domain.ProbeKindICMP}
	}

	return buildSeriesSnapshot(tracker, limit, domain.ProbeKindICMP)
}

// GetSeriesSnapshot dispatches to the appropriate probe kind snapshot method.
func (m *Manager) GetSeriesSnapshot(targetID string, probeKind domain.ProbeKind, limit int) LatencySeriesSnapshot {
	switch probeKind {
	case domain.ProbeKindHTTP:
		return m.GetHTTPSeriesSnapshot(targetID, limit)
	case domain.ProbeKindICMP:
		return m.GetICMPSeriesSnapshot(targetID, limit)
	default:
		return LatencySeriesSnapshot{ProbeKind: probeKind}
	}
}

// buildSeriesSnapshot constructs a LatencySeriesSnapshot from a LatencyTracker.
// This is called while holding the Manager read lock.
func buildSeriesSnapshot(tracker *LatencyTracker, limit int, kind domain.ProbeKind) LatencySeriesSnapshot {
	// Get snapshot from tracker (holds its own read lock internally)
	snap := tracker.Snapshot("", limit)
	if snap == nil {
		return LatencySeriesSnapshot{ProbeKind: kind}
	}

	// Convert state samples to domain samples for the window
	domainSamples := make([]domain.Sample, 0, len(snap.Samples))
	sampleMetas := make([]LatencySeriesSampleMeta, 0, len(snap.Samples))

	for _, s := range snap.Samples {
		domainSamples = append(domainSamples, LatencySampleToDomainSampleWithKind(s, kind))
		sampleMetas = append(sampleMetas, LatencySeriesSampleMeta{
			At: s.Timestamp,
			OK: s.Reachable,
		})
	}

	// Build the domain window (filters failed samples automatically)
	window := domain.NewSampleWindow(domainSamples)

	snapshot := LatencySeriesSnapshot{
		Window:                 window,
		Samples:                sampleMetas,
		RetainedSampleCount:    snap.Count,
		RetainedSampleCapacity: snap.Capacity,
		ErrorCount:             0,
		ProbeKind:             kind,
	}

	// Count failed samples and find timestamps
	var oldestTS, newestTS time.Time
	haveOldest, haveNewest := false, false

	for _, s := range snap.Samples {
		if !s.Reachable {
			snapshot.ErrorCount++
		}
		// Track timestamps for all samples (successful + failed)
		if !haveOldest || s.Timestamp.Before(oldestTS) {
			oldestTS = s.Timestamp
			haveOldest = true
		}
		if !haveNewest || s.Timestamp.After(newestTS) {
			newestTS = s.Timestamp
			haveNewest = true
		}
	}

	if haveOldest {
		snapshot.OldestSampleTs = oldestTS
	}
	if haveNewest {
		snapshot.NewestSampleTs = newestTS
	}

	return snapshot
}
