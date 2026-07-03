// Package server is an external consumer of state.
package server

// LatencyPoint is a DTO for latency data.
type LatencyPoint struct {
	LatencyMs float64
}

// GoodDTO demonstrates allowed use of DTO types.
func GoodDTO() []LatencyPoint {
	return nil
}
