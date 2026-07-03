// Package diagnostics is an external consumer of state.
package diagnostics

import "github.com/s1onique/KGB/uvb76/state"

// Exporter is a diagnostic exporter.
type Exporter struct{}

// BadExport demonstrates disallowed method returning raw state samples.
func (Exporter) BadExport(t *state.LatencyTracker) []state.LatencySample { // want "do not expose \\[\\]state.LatencySample outside uvb76/state; expose domain.SampleWindow or API DTOs instead"
	return nil
}
