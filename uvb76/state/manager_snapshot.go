package state

// GetICMPSnapshot returns an immutable snapshot of ICMP latency tracker state for a target.
// This is the preferred single primitive for latency series handlers - it provides
// both samples and metadata in one locked operation.
//
// Thread-safe: holds Manager read lock and tracker read lock for entire snapshot.
func (m *Manager) GetICMPSnapshot(targetID string, limit int) *LatencySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.icmpTrackers[targetID]
	if tracker == nil {
		buckets := make([]int64, len(m.icmpBuckets))
		copy(buckets, m.icmpBuckets)
		return &LatencySnapshot{
			TargetID: targetID,
			Buckets:  buckets,
		}
	}

	// Clamp limit under Manager lock
	if limit <= 0 {
		limit = m.icmpMaxSamples
	} else if limit > m.icmpMaxSamples {
		limit = m.icmpMaxSamples
	}

	return tracker.Snapshot(targetID, limit)
}

// GetHTTPSnapshot returns an immutable snapshot of HTTP latency tracker state for a target.
// This is the preferred single primitive for latency series handlers - it provides
// both samples and metadata in one locked operation.
//
// Thread-safe: holds Manager read lock and tracker read lock for entire snapshot.
func (m *Manager) GetHTTPSnapshot(targetID string, limit int) *LatencySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.httpTrackers[targetID]
	if tracker == nil {
		buckets := make([]int64, len(m.buckets))
		copy(buckets, m.buckets)
		return &LatencySnapshot{
			TargetID: targetID,
			Buckets:  buckets,
		}
	}

	// Clamp limit under Manager lock
	if limit <= 0 {
		limit = m.maxSamples
	} else if limit > m.maxSamples {
		limit = m.maxSamples
	}

	return tracker.Snapshot(targetID, limit)
}
