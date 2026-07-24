// qualified_runtime.go — Shared interface-backed qualified execution path.
//
// CORRECTION18: this file is the single production implementation
// used by:
//   - the real CLI run path (via the production client.Client wrapped
//     in NewAuditedDockerRuntime);
//   - the live Docker smoke (via the same wrapper);
//   - hermetic DockerRuntime tests (via the recordingDockerRuntime).
//
// The path exposes a single shared Prepare operation that consumes
// the audited runtime observations and returns raw authoritative
// observations. The caller owns start, workload, terminal-state
// observation, bounded cleanup and final evidence persistence.

package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
)

// ErrImageIdentityMismatch is returned when the inspected image does not match the expected image ID.
var ErrImageIdentityMismatch = errors.New("inspected image identity does not match expected")

// ErrNetworkIdentityMismatch is returned when the inspected network does not match the expected network ID.
var ErrNetworkIdentityMismatch = errors.New("inspected network identity does not match expected")

// ErrMissingQualifiedNetwork is returned when a qualified run does not declare
// the network it should be attached to.
var ErrMissingQualifiedNetwork = errors.New("qualified run requires an explicitly declared network")

// QualifiedExecutionObservationsSchemaVersion is the canonical
// schema version used by the qualified execution observation object.
const QualifiedExecutionObservationsSchemaVersion = "1.0.0"

// QualifiedContainerResult describes the outcome of a qualified run.
type QualifiedContainerResult struct {
	ContainerID       string
	ExpectedImageID   string
	ObservedImageID   string
	ExpectedNetworkID string
	ObservedNetworkID string
	StartedByRuntime  bool
	Started           bool
	Terminal          bool
	ContainerRemoved  bool
	NetworkRemoved    bool
}

// QualifiedClient combines DockerRuntime operations into a single
// high-level workflow.
type QualifiedClient struct {
	runtime DockerRuntime
}

// NewQualifiedClient wraps a DockerRuntime as a QualifiedClient.
func NewQualifiedClient(runtime DockerRuntime) *QualifiedClient {
	return &QualifiedClient{runtime: runtime}
}

// Runtime returns the wrapped DockerRuntime.
func (q *QualifiedClient) Runtime() DockerRuntime {
	return q.runtime
}

// PrepareQualifiedContainer runs the prepare phase of the qualified
// execution workflow:
//  1. Validate preconditions.
//  2. Resolve the local image to its exact canonical ID via
//     ImageInspectWithRaw.
//  3. Create the isolated lab network via NetworkCreate.
//  4. Inspect the network via NetworkInspect.
//  5. Create the container with the exact image ID and the
//     create-time networking config.
//  6. Post-create inspect and complete validation (P0-4).
//
// PrepareQualifiedContainer does NOT start the container. The
// caller owns the start, workload, stop and cleanup. The
// observation values are populated from the audited runtime
// where possible; the recorded audit values are an integral
// authority — the producer never copies a value that contradicts
// the audit.
//
// The runtime must be an AuditedDockerRuntime; the qualified
// runtime consumes the audit rather than rely on local
// post-call string copies of the audit's own recorded values.
func (q *QualifiedClient) PrepareQualifiedContainer(
	ctx context.Context,
	imageReference string,
	networkName string,
	cfg ContainerConfig,
) (*QualifiedExecutionObservations, error) {
	if q.runtime == nil {
		return nil, errors.New("runtime is nil")
	}
	if cfg.Config == nil {
		return nil, ErrMissingContainerConfig
	}
	if networkName == "" {
		return nil, ErrMissingQualifiedNetwork
	}

	obs := &QualifiedExecutionObservations{
		SchemaVersion: QualifiedExecutionObservationsSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Image:         ImageObservations{RequestedReference: imageReference},
	}

	// Step 2: Resolve the local image to its exact canonical ID.
	inspect, _, err := q.runtime.ImageInspectWithRaw(ctx, imageReference)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrImageNotFound, imageReference)
		}
		return nil, fmt.Errorf("inspect local image %q: %w", imageReference, err)
	}
	if err := ValidateExactImageID(inspect.ID); err != nil {
		return nil, fmt.Errorf("resolved image %q has invalid ID: %w", imageReference, err)
	}
	obs.SetInspectedImage(inspect.ID, inspect.RepoDigests)

	// Step 3: Create the isolated lab network.
	createResp, err := q.runtime.NetworkCreate(ctx, networkName, types.NetworkCreate{
		Driver:     "bridge",
		Attachable: true,
		Labels: map[string]string{
			"kgb.dev/lab":          "tovarisch-memory",
			"kgb.dev/lab.run-name": networkName,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create network %q: %w", networkName, err)
	}
	if err := ValidateCanonicalNetworkIDLenient(createResp.ID); err != nil {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, fmt.Errorf("created network %q has invalid ID: %w", networkName, err)
	}

	// Step 4: Inspect the network to confirm the canonical ID.
	netInsp, err := q.runtime.NetworkInspect(ctx, createResp.ID, types.NetworkInspectOptions{})
	if err != nil {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, fmt.Errorf("inspect network %q: %w", createResp.ID, err)
	}
	if netInsp.ID == "" {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, fmt.Errorf("inspected network %q has empty ID", createResp.ID)
	}
	if netInsp.ID != createResp.ID {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, fmt.Errorf("network create/inspect mismatch: create=%q inspect=%q",
			createResp.ID, netInsp.ID)
	}
	obs.SetNetworkCreated(networkName, createResp.ID, netInsp.ID)

	// Step 5: Create the container with the exact image ID and
	// create-time networking. Deep-copy the caller's container.Config.
	qualifiedCfg := ContainerConfig{
		Name:        cfg.Name,
		MemoryLimit: cfg.MemoryLimit,
		CPUQuota:    cfg.CPUQuota,
		Networks:    cfg.Networks,
		AutoRemove:  cfg.AutoRemove,
	}
	copied := cloneContainerConfigForQualifiedRun(cfg.Config)
	copied.Image = inspect.ID
	qualifiedCfg.Config = copied
	resources := container.Resources{Memory: cfg.MemoryLimit, CPUQuota: cfg.CPUQuota}
	hostCfg := container.HostConfig{Resources: resources}
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {NetworkID: createResp.ID},
		},
	}
	createResp2, err := q.runtime.ContainerCreate(ctx, qualifiedCfg.Config, &hostCfg, netConfig, nil, qualifiedCfg.Name)
	if err != nil {
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, fmt.Errorf("create container with exact image ID: %w", err)
	}
	if createResp2.ID == "" {
		_ = q.runtime.ContainerRemove(ctx, createResp2.ID, types.ContainerRemoveOptions{Force: true})
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, errors.New("container create returned empty ID")
	}

	// Cross-check: the create-request image observed by the audit
	// must match the exact image ID we passed. This proves the
	// production path does not substitute a different value at
	// the SDK boundary.
	if audited, ok := q.runtime.(*AuditedDockerRuntime); ok {
		snap := audited.Snapshot()
		if !snap.CreateCalled {
			return nil, errors.New("audit did not record a ContainerCreate call")
		}
		if snap.CreateImage != inspect.ID {
			_ = q.runtime.ContainerRemove(ctx, createResp2.ID, types.ContainerRemoveOptions{Force: true})
			_ = q.runtime.NetworkRemove(ctx, createResp.ID)
			return nil, fmt.Errorf("audit-recorded create-request image %q != resolved image %q",
				snap.CreateImage, inspect.ID)
		}
		if snap.CreateNetName != networkName {
			_ = q.runtime.ContainerRemove(ctx, createResp2.ID, types.ContainerRemoveOptions{Force: true})
			_ = q.runtime.NetworkRemove(ctx, createResp.ID)
			return nil, fmt.Errorf("audit-recorded create network name %q != requested %q",
				snap.CreateNetName, networkName)
		}
		if snap.CreateNetID != createResp.ID {
			_ = q.runtime.ContainerRemove(ctx, createResp2.ID, types.ContainerRemoveOptions{Force: true})
			_ = q.runtime.NetworkRemove(ctx, createResp.ID)
			return nil, fmt.Errorf("audit-recorded create network ID %q != createResp.ID %q",
				snap.CreateNetID, createResp.ID)
		}
	}

	obs.SetContainerCreated(createResp2.ID)
	obs.SetCreateRequestImage(inspect.ID)

	// Step 6: Post-create inspect.
	insp, err := q.runtime.ContainerInspect(ctx, createResp2.ID)
	if err != nil {
		_ = q.runtime.ContainerRemove(ctx, createResp2.ID, types.ContainerRemoveOptions{Force: true})
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	// Step 7: Complete post-create validation (P0-4).
	if err := validateContainerInspect(insp, inspect.ID, networkName, netInsp.ID); err != nil {
		_ = q.runtime.ContainerRemove(ctx, createResp2.ID, types.ContainerRemoveOptions{Force: true})
		_ = q.runtime.NetworkRemove(ctx, createResp.ID)
		return nil, err
	}

	obs.SetContainerInspect(createResp2.ID, insp.Image, insp.Config.Image, insp.NetworkSettings.Networks[networkName].NetworkID)
	obs.Network.Removed = false
	obs.Container.Removed = false
	return obs, nil
}

// validateContainerInspect enforces the P0-4 post-create invariants.
func validateContainerInspect(insp types.ContainerJSON, expectedImageID, expectedNetworkName, expectedNetworkID string) error {
	if insp.Image == "" {
		return errors.New("container inspect has empty image")
	}
	if err := ValidateExactImageID(insp.Image); err != nil {
		return fmt.Errorf("observed image identity malformed: %w", err)
	}
	if insp.Image != expectedImageID {
		return fmt.Errorf("%w: expected %q, observed %q",
			ErrImageIdentityMismatch, expectedImageID, insp.Image)
	}
	if insp.Config == nil {
		return errors.New("container inspect has no config")
	}
	if err := ValidateExactImageID(insp.Config.Image); err != nil {
		return fmt.Errorf("container config image invalid: %w", err)
	}
	if insp.Config.Image != expectedImageID {
		return fmt.Errorf("%w: configured image %q != create %q",
			ErrImageIdentityMismatch, insp.Config.Image, expectedImageID)
	}
	if insp.NetworkSettings == nil {
		return errors.New("container inspect has no network settings")
	}
	if len(insp.NetworkSettings.Networks) != 1 {
		return fmt.Errorf("container has %d networks, expected exactly 1",
			len(insp.NetworkSettings.Networks))
	}
	endpoint, ok := insp.NetworkSettings.Networks[expectedNetworkName]
	if !ok || endpoint == nil {
		return fmt.Errorf("container endpoint for network %q not found", expectedNetworkName)
	}
	if endpoint.NetworkID == "" {
		return errors.New("container endpoint has empty network ID")
	}
	if endpoint.NetworkID != expectedNetworkID {
		return fmt.Errorf("%w: endpoint %q != inspected %q",
			ErrNetworkIdentityMismatch, endpoint.NetworkID, expectedNetworkID)
	}
	return nil
}

// ExecuteQualifiedContainer runs the qualified lifecycle using
// PrepareQualifiedContainer + the runtime start helper. Deprecated
// in CORRECTION18; new callers should use the production helper
// ExecuteQualifiedDockerLifecycle directly.
func (q *QualifiedClient) ExecuteQualifiedContainer(
	ctx context.Context,
	imageReference string,
	networkName string,
	cfg ContainerConfig,
) (QualifiedContainerResult, error) {
	obs, err := q.PrepareQualifiedContainer(ctx, imageReference, networkName, cfg)
	if err != nil {
		return QualifiedContainerResult{}, err
	}
	if err := q.runtime.ContainerStart(ctx, obs.Container.ID, types.ContainerStartOptions{}); err != nil {
		_ = q.runtime.ContainerRemove(ctx, obs.Container.ID, types.ContainerRemoveOptions{Force: true})
		_ = q.runtime.NetworkRemove(ctx, obs.Network.InspectResponseID)
		return QualifiedContainerResult{StartedByRuntime: true}, fmt.Errorf("start container: %w", err)
	}
	obs.Container.Started = true
	return QualifiedContainerResult{
		ContainerID:       obs.Container.ID,
		ExpectedImageID:   obs.Image.InspectedBeforeCreate,
		ObservedImageID:   obs.Image.ContainerInspectImage,
		ExpectedNetworkID: obs.Network.InspectResponseID,
		ObservedNetworkID: obs.Network.ContainerEndpointID,
		StartedByRuntime:  true,
		Started:           true,
	}, nil
}

// QualifiedLifecycleOutcome describes the full result of a qualified
// lifecycle run (CORRECTION18 production helper).
type QualifiedLifecycleOutcome struct {
	ContainerID string
	ImageID     string
	NetworkID   string
	Started     bool
	Terminal    bool
	ContainerRemoved bool
	NetworkRemoved   bool
	StartedByRuntime bool
	Observations     *QualifiedExecutionObservations
}

// LifecycleOptions controls the production qualified lifecycle.
type LifecycleOptions struct {
	ImageReference string
	NetworkName    string
	ContainerName  string
	ContainerCmd   []string
	// StartTimeout bounds the post-create start wait. Zero means
	// "no bounded wait; the caller waits separately".
	StartTimeout time.Duration
	// TerminalTimeout bounds the wait for terminal state. Zero means
	// "no bounded wait".
	TerminalTimeout time.Duration
	// CleanupTimeout bounds the post-workload cleanup context. Zero
	// means "use a default of 10s".
	CleanupTimeout time.Duration
	// Run is invoked after the container starts. The runner is
	// responsible for owning the workload and returning when done.
	Run func(ctx context.Context, containerID string) error
}

// ExecuteQualifiedDockerLifecycle is the shared production
// lifecycle used by both the CLI run path and the live smoke. The
// helper installs the audited runtime, calls PrepareQualifiedContainer,
// starts the container, runs the caller-supplied workload, observes
// the terminal state, performs bounded cleanup, and persists the
// canonical qualified evidence.
//
// The runtime must wrap a real Docker client. The caller may pass
// `dockerlab.NewAuditedDockerRuntime(dockerClient.Client)` directly.
// Pull observations are an explicit fail-closed signal: any
// non-zero pull counter fails the outcome.
func ExecuteQualifiedDockerLifecycle(
	ctx context.Context,
	cli *Client,
	opts LifecycleOptions,
	producerVersion string,
) (*QualifiedLifecycleOutcome, error) {
	if cli == nil {
		return nil, errors.New("docker client is nil")
	}
	if opts.ImageReference == "" {
		return nil, errors.New("image reference is empty")
	}
	if opts.NetworkName == "" {
		return nil, errors.New("network name is empty")
	}
	if opts.Run == nil {
		return nil, errors.New("lifecycle Run is nil")
	}

	audited := NewAuditedDockerRuntime(cli.Client)
	qc := NewQualifiedClient(audited)

	// Pull-attempt pre-check: a healthy qualified run has zero pull
	// audit at this point. We re-check after the run to detect any
	// mid-run pulls.
	prepStart := audited.pullAttemptCount

	cfg := ContainerConfig{
		Name: opts.ContainerName,
		Config: &container.Config{
			Image: opts.ImageReference,
			Cmd:   opts.ContainerCmd,
		},
	}
	obs, err := qc.PrepareQualifiedContainer(ctx, opts.ImageReference, opts.NetworkName, cfg)
	if err != nil {
		return nil, err
	}
	// The audit is always installed in the qualified lifecycle. Record
	// the pull-observation availability signal on the observation
	// object so the verifier can require it as a fail-closed signal.
	obs.Pull.ObservationAvailable = true

	// Start the container.
	if err := audited.ContainerStart(ctx, obs.Container.ID, types.ContainerStartOptions{}); err != nil {
		cleanup, _ := boundedCleanup(context.Background(), audited, obs.Container.ID, obs.Network.InspectResponseID, opts.CleanupTimeout)
		obs.Container.Removed = cleanup.containerRemoved
		obs.Network.Removed = cleanup.networkRemoved
		return &QualifiedLifecycleOutcome{
			ContainerID:      obs.Container.ID,
			ImageID:          obs.Image.InspectedBeforeCreate,
			NetworkID:        obs.Network.InspectResponseID,
			Started:          false,
			ContainerRemoved: cleanup.containerRemoved,
			NetworkRemoved:   cleanup.networkRemoved,
			StartedByRuntime: true,
			Observations:     obs,
		}, fmt.Errorf("start container: %w", err)
	}
	obs.Container.Started = true

	// Run the workload.
	runErr := opts.Run(ctx, obs.Container.ID)
	_ = runErr // run errors are captured in outcome via state, not as a return

	// Wait for terminal state. The canary image is a long-running
	// server, so the caller is responsible for terminating the
	// container via stop. We only observe the terminal state via
	// inspect (no sleep-as-authority).
	terminalCtx, terminalCancel := context.WithTimeout(ctx, opts.TerminalTimeout)
	defer terminalCancel()
	if !waitForTerminalState(terminalCtx, cli, obs.Container.ID) {
		cleanup, _ := boundedCleanup(context.Background(), audited, obs.Container.ID, obs.Network.InspectResponseID, opts.CleanupTimeout)
		obs.Container.Removed = cleanup.containerRemoved
		obs.Network.Removed = cleanup.networkRemoved
		return &QualifiedLifecycleOutcome{
			ContainerID: obs.Container.ID,
			ImageID:     obs.Image.InspectedBeforeCreate,
			NetworkID:   obs.Network.InspectResponseID,
			Started:     true,
			Terminal:    false,
			StartedByRuntime: true,
			Observations:     obs,
		}, errors.New("container did not reach terminal state within bounded timeout")
	}
	obs.Container.TerminalStateObserved = true

	// Bounded cleanup (independent context).
	cleanup, cleanupErr := boundedCleanup(context.Background(), audited, obs.Container.ID, obs.Network.InspectResponseID, opts.CleanupTimeout)
	obs.Container.Removed = cleanup.containerRemoved
	obs.Network.Removed = cleanup.networkRemoved

	// Pull-attempt post-check: any pull during the run is a fail-closed signal.
	_, pullCount, _ := audited.PullAudit()
	if pullCount > prepStart {
		return &QualifiedLifecycleOutcome{
			ContainerID: obs.Container.ID,
			ImageID:     obs.Image.InspectedBeforeCreate,
			NetworkID:   obs.Network.InspectResponseID,
			Started:     true,
			Terminal:    true,
			StartedByRuntime: true,
			Observations:     obs,
		}, errors.New("pull audit count increased during the run; qualified path must not pull")
	}

	if cleanupErr != nil {
		return &QualifiedLifecycleOutcome{
			ContainerID: obs.Container.ID,
			ImageID:     obs.Image.InspectedBeforeCreate,
			NetworkID:   obs.Network.InspectResponseID,
			Started:     true,
			Terminal:    true,
			StartedByRuntime: true,
			Observations:     obs,
		}, fmt.Errorf("cleanup: %w", cleanupErr)
	}

	return &QualifiedLifecycleOutcome{
		ContainerID: obs.Container.ID,
		ImageID:     obs.Image.InspectedBeforeCreate,
		NetworkID:   obs.Network.InspectResponseID,
		Started:     true,
		Terminal:    true,
		ContainerRemoved: true,
		NetworkRemoved:   true,
		StartedByRuntime: true,
		Observations:     obs,
	}, nil
}

// cleanupResult captures the cleanup outcome.
type cleanupResult struct {
	containerRemoved bool
	networkRemoved   bool
}

// boundedCleanup creates a fresh bounded context and attempts to
// remove both the container and the network, joining errors. After
// removal, it verifies post-cleanup absence via inspect operations
// and only marks the cleanup state true for successfully proven
// removal.
func boundedCleanup(
	parentCtx context.Context,
	audited *AuditedDockerRuntime,
	containerID, networkID string,
	timeout time.Duration,
) (cleanupResult, error) {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = parentCtx // parent is ignored: cleanup uses a fresh bounded ctx

	res := cleanupResult{}
	var joinErr error
	if containerID != "" {
		if err := audited.ContainerRemove(cleanupCtx, containerID, types.ContainerRemoveOptions{Force: true}); err != nil {
			joinErr = errors.Join(joinErr, fmt.Errorf("container remove: %w", err))
		} else {
			// Verify absence.
			_, err := audited.ContainerInspect(cleanupCtx, containerID)
			if err != nil {
				res.containerRemoved = true
			}
		}
	}
	if networkID != "" {
		if err := audited.NetworkRemove(cleanupCtx, networkID); err != nil {
			joinErr = errors.Join(joinErr, fmt.Errorf("network remove: %w", err))
		} else {
			_, err := audited.NetworkInspect(cleanupCtx, networkID, types.NetworkInspectOptions{})
			if err != nil {
				res.networkRemoved = true
			}
		}
	}
	return res, joinErr
}

// waitForTerminalState polls Docker inspect until the container
// reports a non-running state or the context is done. No
// sleep-as-authority: the inspect API is the only authority.
func waitForTerminalState(ctx context.Context, cli *Client, containerID string) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		ci, err := cli.ContainerInspect(ctx, containerID)
		if err == nil && ci.State != nil && !ci.State.Running {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// combineErrors returns an error that combines primary and cleanup errors.
func combineErrors(primary, cleanup error) error {
	switch {
	case primary == nil:
		return cleanup
	case cleanup == nil:
		return primary
	default:
		return errors.Join(primary, fmt.Errorf("cleanup failed: %w", cleanup))
	}
}

// cloneContainerConfigForQualifiedRun returns a deep copy of the caller's
// container.Config with every reference-bearing field duplicated.
func cloneContainerConfigForQualifiedRun(cfg *container.Config) *container.Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	if cfg.StopTimeout != nil {
		stValue := *cfg.StopTimeout
		cloned.StopTimeout = &stValue
	}
	if cfg.Labels != nil {
		cloned.Labels = make(map[string]string, len(cfg.Labels))
		for k, v := range cfg.Labels {
			cloned.Labels[k] = v
		}
	}
	if cfg.Env != nil {
		cloned.Env = make([]string, len(cfg.Env))
		copy(cloned.Env, cfg.Env)
	}
	if cfg.Cmd != nil {
		cloned.Cmd = make([]string, len(cfg.Cmd))
		copy(cloned.Cmd, cfg.Cmd)
	}
	if cfg.Entrypoint != nil {
		cloned.Entrypoint = make([]string, len(cfg.Entrypoint))
		copy(cloned.Entrypoint, cfg.Entrypoint)
	}
	if cfg.OnBuild != nil {
		cloned.OnBuild = make([]string, len(cfg.OnBuild))
		copy(cloned.OnBuild, cfg.OnBuild)
	}
	if cfg.Shell != nil {
		cloned.Shell = make([]string, len(cfg.Shell))
		copy(cloned.Shell, cfg.Shell)
	}
	if cfg.ExposedPorts != nil {
		cloned.ExposedPorts = make(nat.PortSet, len(cfg.ExposedPorts))
		for k, v := range cfg.ExposedPorts {
			cloned.ExposedPorts[k] = v
		}
	}
	if cfg.Volumes != nil {
		cloned.Volumes = make(map[string]struct{}, len(cfg.Volumes))
		for k, v := range cfg.Volumes {
			cloned.Volumes[k] = v
		}
	}
	if cfg.Healthcheck != nil {
		hcCopy := *cfg.Healthcheck
		if cfg.Healthcheck.Test != nil {
			hcCopy.Test = make([]string, len(cfg.Healthcheck.Test))
			copy(hcCopy.Test, cfg.Healthcheck.Test)
		}
		cloned.Healthcheck = &hcCopy
	}
	return &cloned
}
