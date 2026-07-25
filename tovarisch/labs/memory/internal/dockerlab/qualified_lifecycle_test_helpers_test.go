package dockerlab

import (
	"context"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

func validQualifiedWorkloadResult(input QualifiedWorkloadInput) *QualifiedWorkloadResult {
	return &QualifiedWorkloadResult{Observations: QualifiedWorkloadObservations{Reachability: ReachabilityObservations{
		Method: ReachabilityMethodDockerExec, NetworkID: input.NetworkID,
		TargetHost: "127.0.0.1", TargetPort: 8080, Success: true,
		Health:       ReachabilityOperationObservation{Operation: canarycontrol.OpHealth, HTTPStatus: 200, ResponseValidated: true, Mode: "bounded"},
		InitialState: ReachabilityOperationObservation{Operation: canarycontrol.OpState, HTTPStatus: 200, ResponseValidated: true, Mode: "bounded"},
		Operate:      ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, HTTPStatus: 200, Requested: 1, Attempted: 1, Completed: 1, ResponseValidated: true},
		FinalState:   ReachabilityOperationObservation{Operation: canarycontrol.OpState, HTTPStatus: 200, ResponseValidated: true, Mode: "bounded"},
	}}}
}

func successfulQualifiedWorkload(_ context.Context, input QualifiedWorkloadInput) (*QualifiedWorkloadResult, error) {
	return validQualifiedWorkloadResult(input), nil
}
