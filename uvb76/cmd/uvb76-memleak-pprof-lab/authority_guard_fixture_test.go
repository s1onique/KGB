// Package main provides the UVB-76 pprof memory leak lab.
//
// # Authority Guard Adversarial Fixture Tests
//
// P0-9, P0-10, P0-12: Adversarial fixture-driven guard verification.
// Every guard has positive-violation fixtures and adjacent-valid fixtures.
//
// These tests write fixture source to temp directories and execute the REAL production
// guards (VerifyPollAuthorityGuards, VerifyProfileAuthorityGuards), NOT a parallel inspector.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeFixtureDir writes a Go source fixture to a temp directory with a go.mod
// and returns the path. This ensures go/packages can load the fixture.
func writeFixtureDir(t *testing.T, source string) string {
	t.Helper()

	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "fixture.go")

	if err := os.WriteFile(fixturePath, []byte(source), 0o644); err != nil {
		t.Fatalf("Failed to write fixture: %v", err)
	}

	// Add go.mod so go/packages can load the fixture
	goMod := `module example.com/fixture

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	return tmpDir
}

// requireGuardError asserts that err is an ErrAuthorityGuardFailed with the expected guard name.
func requireGuardError(t *testing.T, err error, expectedGuard string) {
	t.Helper()

	var guardErr *ErrAuthorityGuardFailed
	if !errors.As(err, &guardErr) {
		t.Fatalf("expected ErrAuthorityGuardFailed, got: %v", err)
	}
	if guardErr.Guard != expectedGuard {
		t.Fatalf("expected guard %q, got %q", expectedGuard, guardErr.Guard)
	}
}

// TestPollAuthorityGuard_Adversarial_PollCanonicalCall tests the canonical poll authority rule.
func TestPollAuthorityGuard_Adversarial_PollCanonicalCall(t *testing.T) {
	t.Run("accept_exactly_one_canonical_call", func(t *testing.T) {
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("valid fixture rejected: %v", err)
		}
	})

	t.Run("reject_zero_canonical_calls", func(t *testing.T) {
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with zero canonical calls was accepted")
		}
		requireGuardError(t, err, "lifecycle_poll_call_count")
	})

	t.Run("reject_two_canonical_calls", func(t *testing.T) {
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
	result1 := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result1
	result2 := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result2
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with two canonical calls was accepted")
		}
		requireGuardError(t, err, "lifecycle_poll_call_count")
	})

	t.Run("reject_call_from_runLab", func(t *testing.T) {
		source := `package fixture

import "context"

func runLab(ctx context.Context) {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with direct runner poll call was accepted")
		}
		requireGuardError(t, err, "direct_runner_poll_guard")
	})

	t.Run("reject_call_from_arbitrary_helper", func(t *testing.T) {
		source := `package fixture

import "context"

func someHelper(ctx context.Context) {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with arbitrary helper poll call was accepted")
		}
		requireGuardError(t, err, "unexpected_poll_call_guard")
	})

	t.Run("reject_PollFn_field_in_CollectionLifecycleInput", func(t *testing.T) {
		source := `package fixture

import "context"

type CollectionLifecycleInput struct {
	PollFn func(context.Context) error
}

func RunCollectionLifecycle(input CollectionLifecycleInput) {
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with PollFn field was accepted")
		}
		requireGuardError(t, err, "generic_poll_callback_guard")
	})
}

// TestPollAuthorityGuard_Adversarial_LossyPollSend tests lossy poll send detection.
func TestPollAuthorityGuard_Adversarial_LossyPollSend(t *testing.T) {
	t.Run("reject_poll_send_with_default_in_select", func(t *testing.T) {
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
	pollResultCh := make(chan int, 1)
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result

	select {
	case pollResultCh <- 42:
	default:
	}
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with lossy send was accepted")
		}
		requireGuardError(t, err, "lossy_poll_send_guard")
	})

	t.Run("accept_unrelated_select_with_default", func(t *testing.T) {
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
	done := make(chan struct{})

	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result

	select {
	case <-done:
	default:
	}
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("unrelated select rejected: %v", err)
		}
	})
}

// TestPollAuthorityGuard_Adversarial_StringErrorClassification tests string-based error classification.
func TestPollAuthorityGuard_Adversarial_StringErrorClassification(t *testing.T) {
	t.Run("reject_strings_Contains_in_if", func(t *testing.T) {
		source := `package fixture

import (
	"context"
	"strings"
)

func RunCollectionLifecycle(ctx context.Context) {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
	err := someError()
	if strings.Contains(err.Error(), "timeout") {
		return
	}
}

func someError() error { return nil }
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with string classification in if was accepted")
		}
		requireGuardError(t, err, "string_classification_guard")
	})

	t.Run("reject_strings_Contains_in_return", func(t *testing.T) {
		source := `package fixture

import (
	"context"
	"strings"
)

func RunCollectionLifecycle(ctx context.Context) bool {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
	err := someError()
	return strings.Contains(err.Error(), "timeout")
}

func someError() error { return nil }
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with string classification was accepted")
		}
		requireGuardError(t, err, "string_classification_guard")
	})
}

// TestProfileAuthorityGuard_Adversarial tests profile publication guards.
func TestProfileAuthorityGuard_Adversarial(t *testing.T) {
	t.Run("reject_direct_client_Get_in_capture", func(t *testing.T) {
		source := `package fixture

import (
	"context"
	"net/http"
)

func CaptureProfile(ctx context.Context, client *http.Client) error {
	resp, err := client.Get("http://example.com/profile")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyProfileAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with direct client.Get was accepted")
		}
		requireGuardError(t, err, "client_get_guard")
	})

	t.Run("accept_canonical_op_factory", func(t *testing.T) {
		source := `package fixture

import (
	"context"
	"net/http"
	"os"
)

func CaptureProfile(ctx context.Context, tmpPath string) error {
	file, err := os.CreateTemp("", "profile.tmp.*")
	if err != nil {
		return err
	}
	file.Close()
	return os.Remove(file.Name())
}

func captureProfilesWithValidation(ctx context.Context) error {
	return CaptureProfile(ctx, "tmp")
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyProfileAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("canonical op factory rejected: %v", err)
		}
	})

	t.Run("reject_no_temp_cleanup", func(t *testing.T) {
		source := `package fixture

import (
	"os"
)

func CaptureProfile(ctx context.Context) error {
	file, err := os.CreateTemp("", "profile.tmp.*")
	if err != nil {
		return err
	}
	file.Close()
	return nil
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyProfileAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture without temp cleanup was accepted")
		}
		requireGuardError(t, err, "temp_cleanup_guard")
	})

	t.Run("reject_zero_authority_calls", func(t *testing.T) {
		source := `package fixture

import "os"

func captureProfileWithOps(tmpPath string) error {
	file, err := os.CreateTemp("", "profile.tmp.*")
	if err != nil {
		return err
	}
	file.Close()
	return os.Remove(file.Name())
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyProfileAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with zero authority calls was accepted")
		}
		requireGuardError(t, err, "capture_profile_call_count")
	})
}

// TestRunnerProfileOrchestrationGuard_Adversarial tests the runner topology-bound guard.
// P0-11: All spoof patterns MUST be rejected with hard requireGuardError assertions.
func TestRunnerProfileOrchestrationGuard_Adversarial(t *testing.T) {
	// Common type definition - using interface{} for maximum compatibility in test fixtures.
	// Both the field type and the function literals use the same func(interface{}) error signature.
	canonicalTypeDef := `
// Canonical authority type for profile orchestration
type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
}
`

	t.Run("accept_canonical_direct_orchestration", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err != nil {
			t.Fatalf("canonical orchestration rejected: %v", err)
		}
	})

	t.Run("reject_missing_orchestration", func(t *testing.T) {
		// Use canonical type definition with OtherFn as an additional valid field
		// so the fixture is valid Go (OtherFn is a real field)
		typeDefWithOther := `
// Extended type with additional field - valid Go
type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
	OtherFn          func(ctx interface{}) error
}
`
		source := `package fixture

func runLab() {
	// Only set OtherFn, not CaptureProfilesFn - this is a valid Go struct literal
	_ = CollectionLifecycleInput{
		OtherFn: func(ctx interface{}) error {
			return nil
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + typeDefWithOther
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("missing orchestration was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_helper_bypass", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return helperProfile(ctx)
		},
	}
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("helper bypass was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_alias_bypass", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			capture := CaptureProfile
			return capture(ctx, "out", "heap")
		},
	}
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("alias bypass was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("accept_captureProfilesWithValidation_with_extra_args", func(t *testing.T) {
		source := `package fixture

var extraArg1, extraArg2 interface{}

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx, extraArg1, extraArg2)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}, arg1, arg2 interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err != nil {
			t.Fatalf("captureProfilesWithValidation with args rejected: %v", err)
		}
	})

	t.Run("reject_function_field_bypass", func(t *testing.T) {
		source := `package fixture

type holder struct {
	ProfileFn func(ctx interface{}) error
}

func runLab() {
	h := holder{
		ProfileFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: h.ProfileFn,
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("function field bypass was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_dead_canonical_call_plus_real_bypass", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			if false {
				return captureProfilesWithValidation(ctx)
			}
			return helperProfile(ctx)
		},
	}
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("dead canonical call + real bypass was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_canonical_plus_bypass_binding", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return helperProfile(ctx)
		},
	}

	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("canonical + bypass bindings were accepted (masking not prevented)")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("accept_multiple_canonical_bindings", func(t *testing.T) {
		source := `package fixture

var extraArg interface{}

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}

	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx, extraArg)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}, args ...interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err != nil {
			t.Fatalf("multiple canonical bindings rejected: %v", err)
		}
	})

	t.Run("reject_canonical_assignment_plus_real_bypass", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			err := captureProfilesWithValidation(ctx)
			_ = err
			return helperProfile(ctx)
		},
	}
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("canonical assignment + real bypass was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_conditional_bypass_plus_canonical_fallback", func(t *testing.T) {
		source := `package fixture

var alternate bool

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			if alternate {
				return helperProfile(ctx)
			}
			return captureProfilesWithValidation(ctx)
		},
	}
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("conditional bypass + canonical fallback was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_decoy_struct_canonical_binding", func(t *testing.T) {
		// Decoy struct has canonical CaptureProfilesFn, but real type is missing
		// Guard should REJECT because there are no bindings to the real CollectionLifecycleInput
		source := `package fixture

type Decoy struct {
	CaptureProfilesFn func(ctx interface{}) error
}

func runLab() {
	_ = Decoy{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("decoy struct canonical binding was accepted (should reject - no real CollectionLifecycleInput)")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("accept_decoy_noncanonical_plus_real_canonical", func(t *testing.T) {
		// Decoy struct has noncanonical binding, real CollectionLifecycleInput has canonical
		// Guard should ACCEPT because the real type's binding is canonical
		// Decoy is a different types.TypeName and must be ignored
		source := `package fixture

type Decoy struct {
	CaptureProfilesFn func(ctx interface{}) error
}

func runLab() {
	_ = Decoy{
		CaptureProfilesFn: func(ctx interface{}) error {
			return helperProfile(ctx)
		},
	}

	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err != nil {
			t.Fatalf("decoy noncanonical + real canonical was rejected: %v (should accept - decoy ignored)", err)
		}
	})

	t.Run("reject_selector_name_spoof", func(t *testing.T) {
		source := `package fixture

type Impostor struct{}

func (i Impostor) captureProfilesWithValidation(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return impostor.captureProfilesWithValidation(ctx)
		},
	}
}

var impostor Impostor

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("selector name spoof was accepted (selector bypass)")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("accept_real_collection_lifecycle_input", func(t *testing.T) {
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err != nil {
			t.Fatalf("real CollectionLifecycleInput was rejected: %v", err)
		}
	})

	t.Run("reject_function_valued_variable_name_spoof", func(t *testing.T) {
		// Function-valued variable with same name as canonical function
		source := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

var captureProfilesWithValidation = func(ctx interface{}) error {
	return helperProfile(ctx)
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("function-valued variable spoof was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_method_decl_plus_variable_spoof", func(t *testing.T) {
		// Method with same name + package-level variable
		source := `package fixture

type impostor struct{}

func (impostor) captureProfilesWithValidation(ctx interface{}) error {
	return nil
}

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

var captureProfilesWithValidation = func(ctx interface{}) error {
	return helperProfile(ctx)
}

func helperProfile(ctx interface{}) error {
	return CaptureProfile(ctx, "out", "heap")
}

func CaptureProfile(ctx interface{}, url, profileType string) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("method + variable spoof was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("reject_local_type_shadow_spoof", func(t *testing.T) {
		// Local type shadows package-level type
		source := `package fixture

func runLab() {
	type CollectionLifecycleInput struct {
		CaptureProfilesFn func(ctx interface{}) error
	}

	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
` + "\n" + canonicalTypeDef
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err == nil {
			t.Fatal("local type shadow spoof was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("accept_package_level_type_with_local_binding", func(t *testing.T) {
		source := `package fixture

type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
}

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyRunnerProfileOrchestrationGuard(dir)
		if err != nil {
			t.Fatalf("package-level type with local binding was rejected: %v", err)
		}
	})

	t.Run("reject_selector_type_name_spoof", func(t *testing.T) {
		// Create a valid multi-package fixture with proper module structure.
		// Layout:
		//   tmp/
		//     go.mod              (module example.com/fixture)
		//     fixture.go          (imports example.com/fixture/impostor)
		//     impostor/
		//       impostor.go       (defines impostor.CollectionLifecycleInput)
		//
		// This creates two distinct TypeName objects with the same identifier.
		// The guard uses semantic identity (types.TypeName pointer comparison)
		// to reject the impostor type.
		tmpDir := t.TempDir()

		// Create impostor package directory
		impostorDir := filepath.Join(tmpDir, "impostor")
		if err := os.Mkdir(impostorDir, 0o755); err != nil {
			t.Fatalf("Failed to create impostor dir: %v", err)
		}

		// Create impostor package
		impostorSrc := `package impostor

type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
}
`
		if err := os.WriteFile(filepath.Join(impostorDir, "impostor.go"), []byte(impostorSrc), 0o644); err != nil {
			t.Fatalf("Failed to write impostor.go: %v", err)
		}

		// Create root go.mod for the fixture module
		rootMod := `module example.com/fixture

go 1.21
`
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(rootMod), 0o644); err != nil {
			t.Fatalf("Failed to write root go.mod: %v", err)
		}

		// Create fixture package with proper module import
		fixtureSrc := `package fixture

import "example.com/fixture/impostor"

func runLab() {
	// Use impostor.CollectionLifecycleInput - different package, different type object
	_ = impostor.CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}

type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "fixture.go"), []byte(fixtureSrc), 0o644); err != nil {
			t.Fatalf("Failed to write fixture.go: %v", err)
		}

		err := VerifyRunnerProfileOrchestrationGuard(tmpDir)
		if err == nil {
			t.Fatal("selector type name spoof was accepted")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})

	t.Run("accept_function_decl_in_different_file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Add go.mod so go/packages can load the fixture
		rootMod := `module example.com/fixture

go 1.21
`
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(rootMod), 0o644); err != nil {
			t.Fatalf("Failed to write go.mod: %v", err)
		}

		runnerSrc := `package fixture

func runLab() {
	_ = CollectionLifecycleInput{
		CaptureProfilesFn: func(ctx interface{}) error {
			return captureProfilesWithValidation(ctx)
		},
	}
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "runner.go"), []byte(runnerSrc), 0o644); err != nil {
			t.Fatalf("Failed to write runner.go: %v", err)
		}

		profileSrc := `package fixture

func captureProfilesWithValidation(ctx interface{}) error {
	return nil
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "profile.go"), []byte(profileSrc), 0o644); err != nil {
			t.Fatalf("Failed to write profile.go: %v", err)
		}

		typeSrc := `package fixture

type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
}
`
		if err := os.WriteFile(filepath.Join(tmpDir, "types.go"), []byte(typeSrc), 0o644); err != nil {
			t.Fatalf("Failed to write types.go: %v", err)
		}

		err := VerifyRunnerProfileOrchestrationGuard(tmpDir)
		if err != nil {
			t.Fatalf("declaration in different file was rejected: %v", err)
		}
	})

	t.Run("reject_loader_failure", func(t *testing.T) {
		// P0-11: Dedicated loader-failure test - verifies fail-closed on package errors.
		// The guard must reject any loader/package/type error, not silently fall back.
		tmpDir := t.TempDir()

		// Create a package with intentional type errors
		brokenSrc := `package fixture

// Intentional syntax error: missing closing brace
type CollectionLifecycleInput struct {
	CaptureProfilesFn func(ctx interface{}) error
	// Missing closing }
`
		if err := os.WriteFile(filepath.Join(tmpDir, "fixture.go"), []byte(brokenSrc), 0o644); err != nil {
			t.Fatalf("Failed to write fixture: %v", err)
		}

		// Add a valid go.mod so go/packages tries to load it
		rootMod := `module example.com/fixture

go 1.21
`
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(rootMod), 0o644); err != nil {
			t.Fatalf("Failed to write go.mod: %v", err)
		}

		err := VerifyRunnerProfileOrchestrationGuard(tmpDir)
		if err == nil {
			t.Fatal("loader failure was accepted (should reject)")
		}
		requireGuardError(t, err, "runner_profile_orchestration_guard")
	})
}
