package state

import (
	"sync"
)

// spikeTracker tracks spike events for a single target/probe kind combination.
type spikeTracker struct {
	mu        sync.Mutex
	events    []SpikeEvent // ring buffer of spike events
	maxEvents int          // max events to retain
	config    SpikeConfig
	kind      string // "http" or "icmp"
}

// newSpikeTracker creates a new spike tracker with the given configuration.
func newSpikeTracker(kind string, config SpikeConfig) *spikeTracker {
	return &spikeTracker{
		events:    make([]SpikeEvent, 0, config.MaxEventsPerTracker),
		maxEvents: config.MaxEventsPerTracker,
		config:    config,
		kind:      kind,
	}
}

// recordSpike records a spike event, evicting oldest if at capacity.
// EVICTION POLICY: Capture-aware spike retention
// - Protected spikes (with captures or in-flight) are NEVER evicted
// - Only purge-eligible spikes count against the uncaptured cap
// - maxEvents limits total storage, but protected spikes don't count toward it
func (st *spikeTracker) recordSpike(event SpikeEvent, getProtectionInfo func(eventID string) (isProtected bool, hasArtifact bool)) {
	st.mu.Lock()
	defer st.mu.Unlock()

	// Count purge-eligible (uncaptured) spikes
	purgeableCount := 0
	for _, ev := range st.events {
		isProtected, _ := getProtectionInfo(ev.EventID)
		if !isProtected {
			purgeableCount++
		}
	}

	// Evict oldest purge-eligible spike if we're at the uncaptured cap
	if purgeableCount >= st.config.MaxEventsPerTracker {
		// Find and remove oldest purge-eligible spike
		for i := 0; i < len(st.events); i++ {
			isProtected, _ := getProtectionInfo(st.events[i].EventID)
			if !isProtected {
				// Remove this purge-eligible spike
				st.events = append(st.events[:i], st.events[i+1:]...)
				break
			}
		}
	}

	st.events = append(st.events, event)
}

// getEvents returns all spike events (newest first for display).
func (st *spikeTracker) getEvents(limit int) []SpikeEvent {
	st.mu.Lock()
	defer st.mu.Unlock()

	if limit <= 0 || limit > len(st.events) {
		limit = len(st.events)
	}

	// Return newest first (reverse order)
	result := make([]SpikeEvent, limit)
	for i := 0; i < limit; i++ {
		result[i] = st.events[len(st.events)-1-i]
	}
	return result
}

// count returns the number of spike events.
func (st *spikeTracker) count() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.events)
}
