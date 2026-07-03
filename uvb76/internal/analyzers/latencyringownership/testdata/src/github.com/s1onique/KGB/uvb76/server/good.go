// Package server is an external consumer of state.
package server

import "github.com/s1onique/KGB/uvb76/state"

// GoodWindow demonstrates allowed use of GetSampleWindow.
func GoodWindow(t *state.LatencyTracker) int {
	window := t.GetSampleWindow(100)
	return window.Len()
}

// GoodManagerWindow demonstrates allowed use of manager snapshot APIs.
func GoodManagerWindow(m *state.Manager, targetID string) int {
	httpWindow := m.GetHTTPSampleWindow(targetID, 100)
	icmpWindow := m.GetICMPSampleWindow(targetID, 100)
	return httpWindow.Len() + icmpWindow.Len()
}
