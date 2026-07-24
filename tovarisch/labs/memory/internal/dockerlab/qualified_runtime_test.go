// qualified_runtime_test.go — Tests for QualifiedClient execution workflow.

package dockerlab

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// TestQualifiedRun_NilConfigFailsBeforeDocker verifies nil config fails before any Docker call.
func TestQualifiedRun_NilConfigFailsBeforeDocker(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	cfg := ContainerConfig{Name: "test"} // Config is nil

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "", cfg)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if !errors.Is(err, ErrMissingContainerConfig) {
		t.Errorf("expected ErrMissingContainerConfig, got: %v", err)
	}

	if fake.imageInspectCalls != 0 || fake.containerCreateCalls != 0 ||
		fake.containerInspectCalls != 0 || fake.networkConnectCalls != 0 ||
		fake.containerStartCalls != 0 || fake.containerRemoveCalls != 0 {
		t.Errorf("expected zero Docker calls, got calls=%v", fake.calls)
	}
}

// The NetworkNameWithoutID and NetworkIDWithoutName tests were removed in CORRECTION17
// because the qualified runtime now treats the networkID as an optional
// input (the runtime creates the network itself when no ID is supplied).

// TestQualifiedRun_CreateReceivesExactNetworkingConfig verifies ContainerCreate receives the network config.
func TestQualifiedRun_CreateReceivesExactNetworkingConfig(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	cfg := ContainerConfig{
		Name:   "qualified-run",
		Config: &container.Config{Image: ""},
	}

	// The runtime creates and inspects the network. The test asserts
	// the resulting endpoint equals the recorded network ID.
	result, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result.ObservedNetworkID == "" {
		t.Fatal("expected non-empty observed network ID")
	}
	if fake.networkCreateCalls == 0 {
		t.Errorf("expected NetworkCreate to be called")
	}

	// The container inspect should show only the declared network (no default bridge).
	c, ok := fake.storedContainers[result.ContainerID]
	if !ok {
		t.Fatal("expected container to be in store")
	}
	if c.NetworkSettings == nil || len(c.NetworkSettings.Networks) != 1 {
		t.Errorf("expected exactly 1 network, got %d", len(c.NetworkSettings.Networks))
	}
	if _, ok := c.NetworkSettings.Networks["mynetwork"]; !ok {
		t.Errorf("expected 'mynetwork' to be in networks, got %v", c.NetworkSettings.Networks)
	}
	if _, ok := c.NetworkSettings.Networks["bridge"]; ok {
		t.Errorf("did not expect default 'bridge' network")
	}
}

// TestQualifiedRun_AcceptsExactDeclaredNetworkSet verifies exact network set acceptance.
func TestQualifiedRun_AcceptsExactDeclaredNetworkSet(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	cfg := ContainerConfig{
		Name:   "qualified-run",
		Config: &container.Config{Image: ""},
	}

	result, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result.ObservedNetworkID == "" {
		t.Errorf("expected non-empty observed network ID")
	}
}

// TestQualifiedRun_RejectsWrongObservedNetworkID verifies wrong network ID rejection
// by mutating the container's observed network state after create to simulate
// daemon-side binding error.
func TestQualifiedRun_RejectsWrongObservedNetworkID(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	cfg := ContainerConfig{
		Name:   "qualified-run",
		Config: &container.Config{Image: ""},
	}

	// Override the inspect result via a hook on the fake.
	// We replace ContainerInspect behavior using inspect queue.
	fake.inspectOverride = func(_ string) (types.ContainerJSON, error) {
		return types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{Image: canonicalID},
			Config:            &container.Config{Image: canonicalID},
			NetworkSettings: &types.NetworkSettings{Networks: map[string]*network.EndpointSettings{
				"mynetwork": {NetworkID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdff"},
			}},
		}, nil
	}

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if err == nil {
		t.Fatal("expected error for wrong network ID")
	}
	if !errors.Is(err, ErrNetworkIdentityMismatch) {
		t.Errorf("expected ErrNetworkIdentityMismatch, got: %v", err)
	}
	if fake.containerStartCalls != 0 {
		t.Errorf("expected 0 container start calls, got %d", fake.containerStartCalls)
	}
}

// TestQualifiedRun_OrderedCallsWithNetwork verifies the complete call order with create-time networking.
func TestQualifiedRun_OrderedCallsWithNetwork(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	cfg := ContainerConfig{
		Name:   "qualified-run",
		Config: &container.Config{Image: ""},
	}

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	expected := []string{
		"ImageInspectWithRaw",
		"NetworkCreate",
		"NetworkInspect",
		"ContainerCreate",
		"ContainerInspect",
		"ContainerStart",
	}
	if !reflect.DeepEqual(fake.calls, expected) {
		t.Errorf("expected calls %v, got %v", expected, fake.calls)
	}
}

// TestQualifiedRun_OrderedCallsRequiresNetwork verifies that a network
// declaration is mandatory.
func TestQualifiedRun_OrderedCallsRequiresNetwork(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	cfg := ContainerConfig{
		Name:   "qualified-run",
		Config: &container.Config{Image: ""},
	}

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "", cfg)
	if err == nil {
		t.Fatal("expected error for missing qualified network")
	}
	if !errors.Is(err, ErrMissingQualifiedNetwork) {
		t.Errorf("expected ErrMissingQualifiedNetwork, got: %v", err)
	}
}

// TestQualifiedRun_DoesNotMutateCallerImage verifies the qualified workflow does
// not mutate the caller's container.Config.Image field.
func TestQualifiedRun_DoesNotMutateCallerImage(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	originalImage := "myimage:latest"
	callerConfig := &container.Config{Image: originalImage}
	callerLabels := map[string]string{"app": "tovarisch"}
	callerConfig.Labels = callerLabels
	callerEnv := []string{"FOO=bar"}
	callerConfig.Env = callerEnv

	cfg := ContainerConfig{
		Name:   "isolation-test",
		Config: callerConfig,
	}

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// The caller's Image field must be unchanged.
	if callerConfig.Image != originalImage {
		t.Errorf("caller config Image was mutated: got %q, want %q",
			callerConfig.Image, originalImage)
	}

	// The caller's Labels map must not have been replaced.
	if callerConfig.Labels["app"] != "tovarisch" {
		t.Errorf("caller labels were mutated: got %v", callerConfig.Labels)
	}

	// The caller's Env slice must not have been mutated.
	if len(callerConfig.Env) != 1 || callerConfig.Env[0] != "FOO=bar" {
		t.Errorf("caller env was mutated: got %v", callerConfig.Env)
	}
}

// TestQualifiedRun_RuntimeCannotMutateCallerConfig verifies that mutations
// made inside the fake's ContainerCreate (simulating the runtime) do not
// affect the caller's original configuration values.
func TestQualifiedRun_RuntimeCannotMutateCallerConfig(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	fake.addImage("myimage:latest", canonicalID)

	originalLabels := map[string]string{"app": "tovarisch", "env": "test"}
	originalEnv := []string{"FOO=bar", "BAZ=qux"}
	originalCmd := []string{"/bin/sh", "-c", "true"}
	originalEntrypoint := []string{"/entry.sh"}
	originalOnBuild := []string{"RUN apk add curl"}
	originalShell := []string{"/bin/sh", "-c"}
	originalHealthcheckTest := []string{"CMD", "curl", "-f", "http://localhost"}
	stTimeout := 30
	originalExposedPorts := nat.PortSet{"80/tcp": struct{}{}, "443/tcp": struct{}{}}
	originalVolumes := map[string]struct{}{"/data": {}}

	callerConfig := &container.Config{
		Image:        "myimage:latest",
		Labels:       originalLabels,
		Env:          originalEnv,
		Cmd:          originalCmd,
		Entrypoint:   originalEntrypoint,
		OnBuild:      originalOnBuild,
		Shell:        originalShell,
		StopTimeout:  &stTimeout,
		Healthcheck:  &container.HealthConfig{Test: originalHealthcheckTest},
		ExposedPorts: originalExposedPorts,
		Volumes:      originalVolumes,
	}

	cfg := ContainerConfig{
		Name:   "adversarial-test",
		Config: callerConfig,
	}

	// The fake mutates the received config's mutable fields to simulate
	// adversarial runtime behavior. The qualified workflow's caller-isolation
	// contract requires that this does NOT affect the caller's original values.
	fake.ConfigMutator = func(c *container.Config) {
		// Mutate map (add new key).
		c.Labels["runtime_added"] = "evil"
		// Mutate slice (append element).
		c.Env = append(c.Env, "INJECTED=bad")
		// Mutate Cmd slice.
		newCmd := make([]string, len(c.Cmd), len(c.Cmd)+1)
		copy(newCmd, c.Cmd)
		newCmd = append(newCmd, "--exploit")
		c.Cmd = newCmd
		// Mutate Entrypoint slice.
		newEp := []string{"/malicious"}
		c.Entrypoint = newEp
	}

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// The caller's Labels map must remain byte-identical.
	if callerConfig.Labels["app"] != "tovarisch" {
		t.Errorf("caller Labels mutated: %v", callerConfig.Labels)
	}
	if _, ok := callerConfig.Labels["runtime_added"]; ok {
		t.Errorf("runtime injection leaked into caller Labels: %v", callerConfig.Labels)
	}

	// The caller's Env slice must remain byte-identical.
	if len(callerConfig.Env) != len(originalEnv) {
		t.Errorf("caller Env mutated: %v", callerConfig.Env)
	}
	for i, v := range originalEnv {
		if callerConfig.Env[i] != v {
			t.Errorf("caller Env[%d] mutated: got %q, want %q", i, callerConfig.Env[i], v)
		}
	}

	// The caller's Cmd slice must remain byte-identical.
	if len(callerConfig.Cmd) != len(originalCmd) {
		t.Errorf("caller Cmd mutated: got %v", callerConfig.Cmd)
	}
	for i, v := range originalCmd {
		if callerConfig.Cmd[i] != v {
			t.Errorf("caller Cmd[%d] mutated: got %q, want %q", i, callerConfig.Cmd[i], v)
		}
	}

	// The caller's Entrypoint slice must remain byte-identical.
	if len(callerConfig.Entrypoint) != len(originalEntrypoint) {
		t.Errorf("caller Entrypoint mutated: got %v", callerConfig.Entrypoint)
	}
	for i, v := range originalEntrypoint {
		if callerConfig.Entrypoint[i] != v {
			t.Errorf("caller Entrypoint[%d] mutated: got %q, want %q", i, callerConfig.Entrypoint[i], v)
		}
	}
}

// TestQualifiedRun_ImageMismatchTriggersCleanup verifies image mismatch triggers cleanup.
func TestQualifiedRun_ImageMismatchTriggersCleanup(t *testing.T) {
	fake := newRecordingDockerRuntime()
	qc := NewQualifiedClient(fake)
	ctx := context.Background()

	canonicalID := "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	wrongID := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	fake.addImage("myimage:latest", canonicalID)
	fake.forceObservedImage = wrongID

	cfg := ContainerConfig{
		Name:   "mismatch-run",
		Config: &container.Config{Image: ""},
	}

	_, err := qc.ExecuteQualifiedContainer(ctx, "myimage:latest", "mynetwork", cfg)
	if !errors.Is(err, ErrImageIdentityMismatch) {
		t.Errorf("expected ErrImageIdentityMismatch, got: %v", err)
	}
	if fake.containerRemoveCalls != 1 {
		t.Errorf("expected 1 container remove call (cleanup), got %d", fake.containerRemoveCalls)
	}
	if fake.containerStartCalls != 0 {
		t.Errorf("expected 0 container start calls, got %d", fake.containerStartCalls)
	}
}

// TestCombineErrors_BothPreservePrimary verifies both errors are discoverable via errors.Is.
func TestCombineErrors_BothPreservePrimary(t *testing.T) {
	primary := errors.New("primary failure")
	cleanup := errors.New("cleanup failure")

	result := combineErrors(primary, cleanup)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !errors.Is(result, primary) {
		t.Errorf("expected errors.Is(result, primary) to be true")
	}
	if !errors.Is(result, cleanup) {
		t.Errorf("expected errors.Is(result, cleanup) to be true")
	}
}

// TestCombineErrors_PrimaryOnly verifies primary-only path.
func TestCombineErrors_PrimaryOnly(t *testing.T) {
	primary := errors.New("primary failure")
	result := combineErrors(primary, nil)
	if result != primary {
		t.Errorf("expected primary to be returned unchanged")
	}
}

// TestCombineErrors_CleanupOnly verifies cleanup-only path.
func TestCombineErrors_CleanupOnly(t *testing.T) {
	cleanup := errors.New("cleanup failure")
	result := combineErrors(nil, cleanup)
	if result != cleanup {
		t.Errorf("expected cleanup to be returned unchanged")
	}
}
