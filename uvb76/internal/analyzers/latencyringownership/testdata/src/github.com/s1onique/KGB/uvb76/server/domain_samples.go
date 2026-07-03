// Package server is an external consumer of state.
package server

import "github.com/s1onique/KGB/uvb76/internal/uvb76/domain"

// GoodDomainSamples demonstrates allowed use of domain.SampleWindow.Samples().
func GoodDomainSamples(w domain.SampleWindow) int {
	return len(w.Samples())
}
