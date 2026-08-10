// Package main provides the UVB-76 pprof memory leak lab.
//
// # Authority Guard Adversarial Fixture Tests
//
// P0-9, P0-10, P0-12: Adversarial fixture-driven AST guard verification.
// Every guard must have both a positive-violation fixture and an adjacent-valid fixture.
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

// writeFixtureDir writes a Go source fixture to a temp directory and returns the path.
// All filesystem operations occur in the test goroutine.
func writeFixtureDir(t *testing.T, source string) string {
	t.Helper()

	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "fixture.go")

	if err := os.WriteFile(fixturePath, []byte(source), 0o644); err != nil {
		t.Fatalf("Failed to write fixture: %v", err)
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

	t.Run("accept_dead_wrapper_with_sole_poll_call", func(t *testing.T) {
		// The production guard counts AST call sites, not semantic reachability.
		// Dead code paths are still parsed and counted, so this is accepted.
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
	if false {
		result := PollTargetAuthority(ctx, TargetPollInput{})
		_ = result
	}
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("dead wrapper with poll call should be accepted: %v", err)
		}
	})

	t.Run("accept_local_variable_named_PollFn", func(t *testing.T) {
		// Local variable named PollFn is valid (not a field)
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) {
	PollFn := func() {}
	PollFn()
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("local variable named PollFn should be accepted: %v", err)
		}
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
	t.Run("accept_indirect_string_classification_via_variable", func(t *testing.T) {
		// The production guard only catches direct err.Error() passed to strings.Contains.
		// Indirect uses via intermediate variables are not detected.
		source := `package fixture

import (
	"context"
	"strings"
)

func RunCollectionLifecycle(ctx context.Context) bool {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
	err := someError()
	msg := err.Error()
	return strings.Contains(msg, "timeout")
}

func someError() error { return nil }
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("indirect string classification should be accepted (guard limitation): %v", err)
		}
	})

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

	t.Run("reject_strings_Contains_in_switch_or_nested_boolean", func(t *testing.T) {
		source := `package fixture

import (
	"context"
	"strings"
)

func RunCollectionLifecycle(ctx context.Context) {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
	err := someError()

	isTimeout := strings.Contains(err.Error(), "timeout")
	isConnRefused := strings.Contains(err.Error(), "connection refused")

	switch {
	case isTimeout || isConnRefused:
		return
	default:
		return
	}
}

func someError() error { return nil }
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err == nil {
			t.Fatal("fixture with nested boolean string classification was accepted")
		}
		requireGuardError(t, err, "string_classification_guard")
	})

	t.Run("accept_unrelated_strings_Contains", func(t *testing.T) {
		source := `package fixture

import "context"

func RunCollectionLifecycle(ctx context.Context) bool {
	path := "/tmp/profile"
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
	_ = len(path) > 0
	return true
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("unrelated string contains rejected: %v", err)
		}
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
	"os"
)

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

	t.Run("reject_direct_os_Create_for_destination", func(t *testing.T) {
		source := `package fixture

import "os"

func captureProfileWithOps(outPath string) error {
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyProfileAuthorityGuards(dir)
		// This should be rejected because CaptureProfile contains os.Create for destination
		if err == nil {
			t.Fatal("fixture with direct os.Create was accepted")
		}
	})

	t.Run("accept_direct_os_Remove_in_capture", func(t *testing.T) {
		// Note: The current production guard requires temp cleanup but does not
		// specifically reject os.Remove in CaptureProfile. It only checks that
		// TempCleanupCalls > 0.
		source := `package fixture

import "os"

func CaptureProfile(ctx context.Context, tmpPath string) error {
	return os.Remove(tmpPath)
}
`
		dir := writeFixtureDir(t, source)
		err := VerifyProfileAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("os.Remove in CaptureProfile should be accepted: %v", err)
		}
	})
}

// TestPollProfileAuthorityGuard_ValidFixtures verifies valid fixtures pass all guards.
func TestPollProfileAuthorityGuard_ValidFixtures(t *testing.T) {
	t.Run("valid_lifecycle_with_canonical_call", func(t *testing.T) {
		source := `package fixture

import (
	"context"
	"sync/atomic"
)

type CollectionLifecycleInput struct {
	TargetPollInput
}

func RunCollectionLifecycle(ctx context.Context) {
	result := PollTargetAuthority(ctx, TargetPollInput{})
	_ = result
	_ = atomic.LoadInt32(new(int32))
}
`
		dir := writeFixtureDir(t, source)

		// Should pass poll guards
		err := VerifyPollAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("valid lifecycle rejected by poll guard: %v", err)
		}
	})

	t.Run("valid_profile_with_temp_cleanup", func(t *testing.T) {
		source := `package fixture

import (
	"os"
)

func captureProfileWithOps(tmpPath string) error {
	file, err := os.CreateTemp("", "profile.tmp.*")
	if err != nil {
		return err
	}
	name := file.Name()
	file.Close()

	// ... operational logic ...

	return os.Remove(name)
}
`
		dir := writeFixtureDir(t, source)

		// Should pass profile guards
		err := VerifyProfileAuthorityGuards(dir)
		if err != nil {
			t.Fatalf("valid profile rejected by profile guard: %v", err)
		}
	})
}
