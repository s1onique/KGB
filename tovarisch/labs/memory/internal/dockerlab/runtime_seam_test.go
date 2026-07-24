// runtime_seam_test.go — Executable tests for production methods using the injected fake.
//
// These tests invoke production methods (ClientWithRuntime) against the recording fake
// and assert exact call counts and arguments. No Docker daemon is required.

package dockerlab

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// TestContainerCreateWithImageID_HermeticZeroCalls verifies zero Docker calls on invalid input.
func TestContainerCreateWithImageID_HermeticZeroCalls(t *testing.T) {
	fake := newRecordingDockerRuntime()
	client := NewClientWithRuntime(fake)
	ctx := context.Background()

	testCases := []struct {
		name string
		cfg  ContainerConfig
	}{
		{"nil_config", ContainerConfig{Name: "test-nil"}},
		{"empty_image", ContainerConfig{Name: "test-empty", Config: &container.Config{Image: ""}}},
		{"tag_format", ContainerConfig{Name: "test-tag", Config: &container.Config{Image: "kgb:latest"}}},
		{"short_id", ContainerConfig{Name: "test-short", Config: &container.Config{Image: "sha256:abc12345"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.ContainerCreateWithImageID(ctx, tc.cfg)
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			if fake.containerCreateCalls != 0 {
				t.Errorf("expected 0 container create calls, got %d", fake.containerCreateCalls)
			}
			if fake.containerStartCalls != 0 {
				t.Errorf("expected 0 container start calls, got %d", fake.containerStartCalls)
			}
		})
	}
}

// TestContainerCreateWithImageID_HermeticOneCall verifies exactly one Docker call on valid input.
func TestContainerCreateWithImageID_HermeticOneCall(t *testing.T) {
	fake := newRecordingDockerRuntime()
	client := NewClientWithRuntime(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	cfg := ContainerConfig{
		Name:   "test-valid",
		Config: &container.Config{Image: canonicalID},
	}

	containerID, err := client.ContainerCreateWithImageID(ctx, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containerID == "" {
		t.Error("expected non-empty container ID")
	}
	if fake.containerCreateCalls != 1 {
		t.Errorf("expected 1 container create call, got %d", fake.containerCreateCalls)
	}
	if fake.createdImageArgument != canonicalID {
		t.Errorf("expected image ID %q, got %q", canonicalID, fake.createdImageArgument)
	}
}

// TestResolveImageIdentity_Hermetic verifies resolver uses runtime and returns ErrImageNotFound.
func TestResolveImageIdentity_Hermetic(t *testing.T) {
	fake := newRecordingDockerRuntime()
	client := NewClientWithRuntime(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	// Resolve existing image
	id, err := client.ResolveImageIdentity(ctx, "myimage:latest")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if id != canonicalID {
		t.Errorf("expected %q, got %q", canonicalID, id)
	}
	if fake.imageInspectCalls != 1 {
		t.Errorf("expected 1 image inspect call, got %d", fake.imageInspectCalls)
	}

	// Resolve missing image
	_, err = client.ResolveImageIdentity(ctx, "nonexistent:latest")
	if err == nil {
		t.Fatal("expected error for missing image")
	}
	if !errors.Is(err, ErrImageNotFound) {
		t.Errorf("expected ErrImageNotFound, got: %v", err)
	}
}

// TestQualifiedRun_HermeticCallOrder verifies the qualified execution path produces the expected sequence of calls.
func TestQualifiedRun_HermeticCallOrder(t *testing.T) {
	fake := newRecordingDockerRuntime()
	client := NewClientWithRuntime(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	// Step 1: Resolve exact local image
	resolvedID, err := client.ResolveImageIdentity(ctx, "myimage:latest")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Step 2: Strict exact-ID create
	cfg := ContainerConfig{
		Name:   "qualified-run",
		Config: &container.Config{Image: resolvedID},
	}
	containerID, err := client.ContainerCreateWithImageID(ctx, cfg)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Step 3: Inspect actual container image
	insp, err := client.InspectContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	// Step 4: Verify expected/actual image equality
	if insp.Config == nil || insp.Config.Image != resolvedID {
		actual := ""
		if insp.Config != nil {
			actual = insp.Config.Image
		}
		t.Fatalf("expected %q, got %q", resolvedID, actual)
	}

	// Step 5: Start container
	if err := client.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Assert call counts
	if fake.imageInspectCalls != 1 {
		t.Errorf("expected 1 image inspect call, got %d", fake.imageInspectCalls)
	}
	if fake.containerCreateCalls != 1 {
		t.Errorf("expected 1 container create call, got %d", fake.containerCreateCalls)
	}
	if fake.containerInspectCalls != 1 {
		t.Errorf("expected 1 container inspect call, got %d", fake.containerInspectCalls)
	}
	if fake.containerStartCalls != 1 {
		t.Errorf("expected 1 container start call, got %d", fake.containerStartCalls)
	}
	if fake.createdImageArgument != canonicalID {
		t.Errorf("expected image ID %q, got %q", canonicalID, fake.createdImageArgument)
	}
}
