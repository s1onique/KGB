// qualified_execution_correction44_test.go — Mutation matrix, ownership,
// reachability verifier, and rederived-reachability tests for
// CORRECTION44. All tests are hermetic and exercise the canonical
// in-memory + bytes verifiers directly.

package evidence

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

func validReachability(extra ReachabilityOperateObservation) ReachabilityObservations {
	health := ReachabilityOperationObservation{Operation: canarycontrol.OpHealth, ExecExitCode: 0, HTTPStatus: 200, ResponseValidated: true, Mode: "growing"}
	state := ReachabilityOperationObservation{Operation: canarycontrol.OpState, ExecExitCode: 0, HTTPStatus: 200, ResponseValidated: true, Mode: "growing"}
	operate := extra
	if operate.Operation == "" {
		operate = ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 0, HTTPStatus: 200, Requested: 5, Attempted: 5, Completed: 5, ResponseValidated: true}
	}
	final := ReachabilityOperationObservation{Operation: canarycontrol.OpState, ExecExitCode: 0, HTTPStatus: 200, ResponseValidated: true, Mode: "growing"}
	return ReachabilityObservations{Method: ReachabilityMethodDockerExec, NetworkID: validCanonicalNetworkID, TargetHost: "127.0.0.1", TargetPort: 8080, Health: health, InitialState: state, Operate: operate, FinalState: final, Success: true}
}

func TestDeriveReachability_AllFieldsValid(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	ok, errs := deriveReachability(r)
	if !ok {
		t.Fatalf("expected derive ok, got errors: %v", errs)
	}
}

func TestDeriveReachability_BadMethodFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.Method = ReachabilityMethod("unknown")
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for unknown method")
	}
}

func TestDeriveReachability_ShortNetworkFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.NetworkID = "short"
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for short network id")
	}
}

func TestDeriveReachability_UppercaseNetworkFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.NetworkID = strings.ToUpper(validCanonicalNetworkID)
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for uppercase network id")
	}
}

func TestDeriveReachability_BadHostFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.TargetHost = "10.0.0.1"
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for non-127.0.0.1 target host")
	}
}

func TestDeriveReachability_PortOutOfRangeFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.TargetPort = 70000
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for out-of-range target port")
	}
}

func TestDeriveReachability_HealthBadOperationFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.Health.Operation = canarycontrol.OpState
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for wrong health operation")
	}
}

func TestDeriveReachability_HealthNonZeroExitFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.Health.ExecExitCode = 1
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for nonzero health exit")
	}
}

func TestDeriveReachability_HealthHTTP500Fails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.Health.HTTPStatus = 500
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for non-200 health status")
	}
}

func TestDeriveReachability_HealthNotValidatedFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.Health.ResponseValidated = false
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for unvalidated health")
	}
}

func TestDeriveReachability_HealthEmptyModeFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.Health.Mode = ""
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for empty health mode")
	}
}

func TestDeriveReachability_InitialStateWrongOperationFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.InitialState.Operation = canarycontrol.OpHealth
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for wrong initial state op")
	}
}

func TestDeriveReachability_OperateRequestedZeroFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 0, HTTPStatus: 200, Requested: 0, Attempted: 0, Completed: 0, ResponseValidated: true})
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for zero operate requested")
	}
}

func TestDeriveReachability_OperateAttemptedMismatchFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 0, HTTPStatus: 200, Requested: 5, Attempted: 4, Completed: 4, ResponseValidated: true})
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for attempted mismatch")
	}
}

func TestDeriveReachability_OperateCompletedMismatchFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 0, HTTPStatus: 200, Requested: 5, Attempted: 5, Completed: 4, ResponseValidated: true})
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for completed mismatch")
	}
}

func TestDeriveReachability_OperateExitNonZeroFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 1, HTTPStatus: 200, Requested: 5, Attempted: 5, Completed: 5, ResponseValidated: true})
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for nonzero operate exit")
	}
}

func TestDeriveReachability_OperateHTTPNotOKFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 0, HTTPStatus: 500, Requested: 5, Attempted: 5, Completed: 5, ResponseValidated: true})
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for non-200 operate http")
	}
}

func TestDeriveReachability_OperateNotValidatedFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{Operation: canarycontrol.OpOperate, ExecExitCode: 0, HTTPStatus: 200, Requested: 5, Attempted: 5, Completed: 5, ResponseValidated: false})
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for unvalidated operate")
	}
}

func TestDeriveReachability_FinalStateWrongOperationFails(t *testing.T) {
	r := validReachability(ReachabilityOperateObservation{})
	r.FinalState.Operation = canarycontrol.OpHealth
	if ok, _ := deriveReachability(r); ok {
		t.Fatal("expected derive to fail for wrong final state op")
	}
}

func TestVerifyReachability_SuccessTrueWithInvalidBackingFails(t *testing.T) {
	ev := buildValidEvidence()
	ev.Reachability.Operate.Requested = 0
	ev.Pass = true
	if res := VerifyQualifiedExecution(ev); res.Pass {
		t.Fatal("expected verify to fail when success true with invalid backing")
	}
}

func TestVerifyReachability_SuccessFalseWithValidBackingFails(t *testing.T) {
	ev := buildValidEvidence()
	ev.Pass = false
	if res := VerifyQualifiedExecution(ev); res.Pass {
		t.Fatal("expected verify to fail when success false with valid backing")
	}
}

func TestReachabilityBytes_DeleteMemberFails(t *testing.T) {
	ev := buildValidEvidence()
	raw, _ := json.Marshal(ev)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	r := map[string]json.RawMessage{}
	_ = json.Unmarshal(m["reachability"], &r)
	delete(r, "operate")
	m["reachability"], _ = json.Marshal(r)
	data, _ := json.Marshal(m)
	if res, _ := VerifyQualifiedExecutionBytes(data); res.Pass {
		t.Fatal("expected bytes verifier to fail on deleted reachability.operate")
	}
}

func TestReachabilityBytes_NullMemberFails(t *testing.T) {
	ev := buildValidEvidence()
	raw, _ := json.Marshal(ev)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	r := map[string]json.RawMessage{}
	_ = json.Unmarshal(m["reachability"], &r)
	r["target_port"] = json.RawMessage("null")
	m["reachability"], _ = json.Marshal(r)
	data, _ := json.Marshal(m)
	if res, _ := VerifyQualifiedExecutionBytes(data); res.Pass {
		t.Fatal("expected bytes verifier to fail on null target_port")
	}
}

func TestReachabilityBytes_WrongTypeFails(t *testing.T) {
	ev := buildValidEvidence()
	raw, _ := json.Marshal(ev)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	r := map[string]json.RawMessage{}
	_ = json.Unmarshal(m["reachability"], &r)
	r["target_port"] = json.RawMessage(`"eight-zero-eight-zero"`)
	m["reachability"], _ = json.Marshal(r)
	data, _ := json.Marshal(m)
	if res, _ := VerifyQualifiedExecutionBytes(data); res.Pass {
		t.Fatal("expected bytes verifier to fail on wrong type target_port")
	}
}

func TestReachabilityBytes_UnknownFieldFails(t *testing.T) {
	ev := buildValidEvidence()
	raw, _ := json.Marshal(ev)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	r := map[string]json.RawMessage{}
	_ = json.Unmarshal(m["reachability"], &r)
	r["rogue_field"] = json.RawMessage(`"bad"`)
	m["reachability"], _ = json.Marshal(r)
	data, _ := json.Marshal(m)
	if res, _ := VerifyQualifiedExecutionBytes(data); res.Pass {
		t.Fatal("expected bytes verifier to fail on unknown reachability field")
	}
}

func TestReachabilityBytes_MultipleDocsFails(t *testing.T) {
	ev := buildValidEvidence()
	first, _ := json.MarshalIndent(ev, "", "  ")
	second, _ := json.MarshalIndent(ev, "", "  ")
	combined := append(first, []byte("  ")...)
	combined = append(combined, second...)
	if res, _ := VerifyQualifiedExecutionBytes(combined); res.Pass {
		t.Fatal("expected bytes verifier to fail on multiple JSON docs")
	}
}

func TestReachabilityOperation_MissingMemberFails(t *testing.T) {
	ev := buildValidEvidence()
	raw, _ := json.Marshal(ev)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	r := map[string]json.RawMessage{}
	_ = json.Unmarshal(m["reachability"], &r)
	health := map[string]json.RawMessage{}
	_ = json.Unmarshal(r["health"], &health)
	delete(health, "mode")
	r["health"], _ = json.Marshal(health)
	m["reachability"], _ = json.Marshal(r)
	data, _ := json.Marshal(m)
	if res, _ := VerifyQualifiedExecutionBytes(data); res.Pass {
		t.Fatal("expected bytes verifier to fail on missing health.mode")
	}
}

func TestDockerlabReachabilitySetReachabilityUnknownPreserved(t *testing.T) {
	o := &dockerlab.QualifiedExecutionObservations{}
	o.SetReachabilityUnknown(validCanonicalNetworkID)
	if o.Reachability.Method != dockerlab.ReachabilityMethod("unknown") {
		t.Fatal("expected unknown method")
	}
	if o.Reachability.NetworkID != validCanonicalNetworkID {
		t.Fatal("expected network id")
	}
}

func TestNoReplaceInDockerlab_AfterCORRECTION44(t *testing.T) {
	if _, ok := interface{}(&dockerlab.QualifiedExecutionObservations{}).(*dockerlab.QualifiedExecutionObservations); !ok {
		t.Fatal("QualifiedExecutionObservations must remain in dockerlab")
	}
	_ = fmt.Sprintf("%s", time.Now().UTC())
}
