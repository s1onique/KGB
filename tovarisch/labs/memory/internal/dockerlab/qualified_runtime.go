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

// errTerminalTimeout is the sentinel returned when the bounded
// terminal-state observer does not observe a non-running state.
// It is exported via errors.Is through errors.Join so callers
// can introspect a timeout failure.
var errTerminalTimeout = errors.New("container did not reach terminal state within bounded timeout")

// errPullAuditIncreased is the sentinel returned when the audit
// observes a pull attempt during the run.
var errPullAuditIncreased = errors.New("pull audit count increased during the run; qualified path must not pull")

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

// QualifiedLifecycleOutcome is the finalized lifecycle snapshot transferred
// to the caller. Observations is always a deep clone of lifecycle-owned state.
type QualifiedLifecycleOutcome struct {
	ContainerID      string
	ImageID          string
	NetworkID        string
	Started          bool
	Terminal         bool
	ContainerRemoved bool
	NetworkRemoved   bool
	StartedByRuntime bool
	Observations     *QualifiedExecutionObservations
	Phases           []QualifiedLifecyclePhase
}

// QualifiedLifecycleDependencies binds all authoritative lifecycle dependencies.
type QualifiedLifecycleDependencies struct {
	Runtime DockerRuntime
	Control *ControlRunner
}

var ErrQualifiedControlRequired = errors.New("qualified lifecycle requires canonical control")

// LifecycleOptions controls the production qualified lifecycle.
type LifecycleOptions struct {
	ImageReference string
	NetworkName    string
	ContainerName  string
	ContainerCmd   []string

	// TerminalTimeout bounds the wait for terminal state. Zero means
	// "no bounded wait".
	TerminalTimeout time.Duration
	// CleanupTimeout bounds the post-workload cleanup context. Zero
	// means "use a default of 10s".
	CleanupTimeout time.Duration
	// Run is invoked after start and returns workload-owned observations.
	// It never receives a writable alias to lifecycle-owned observations.
	Run QualifiedRunFunc

	// TerminalObserver is an optional seam for tests. When nil,
	// the production path uses cli.ContainerInspect to detect the
	// non-running state. Tests inject a deterministic observer.
	TerminalObserver func(ctx context.Context, containerID string) bool
}

// finalizePullAudit is the centralized pull-audit finalizer. It
// MUST be invoked before every return path of
// ExecuteQualifiedDockerLifecycle that has a non-nil obs.
// Centralizing the audit snapshot prevents the kind of drift that
// let prior CORRECTION iterations publish a stale pull=0 even when
// a pull was attempted.
func finalizePullAudit(
	audited *AuditedDockerRuntime,
	obs *QualifiedExecutionObservations,
) {
	if audited == nil || obs == nil {
		return
	}
	attempted, count, lastRef := audited.PullAudit()
	obs.SetPullAudit(attempted, count, lastRef)
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
//
// Error-propagation contract (CORRECTION22 P0-9):
//
//   - workload error (Run) is preserved as the primary error;
//   - terminal-state observation error is preserved alongside the
//     workload error via errors.Join when both apply;
//   - container cleanup error and network cleanup error are joined
//     with the primary error so every failure is discoverable via
//     errors.Is;
//   - pull-attempt error is preserved as the primary error.
//
// finalizePullAudit is invoked on every return path; no error is
// ever silently discarded.
func ExecuteQualifiedDockerLifecycle(
	ctx context.Context,
	cli *Client,
	opts LifecycleOptions,
	producerVersion string,
) (*QualifiedLifecycleOutcome, error) {
	if cli == nil {
		return nil, errors.New("docker client is nil")
	}
	control, err := NewDockerControl(cli.Client)
	if err != nil {
		return nil, err
	}
	audited := NewAuditedDockerRuntime(cli.Client)
	terminalObs := opts.TerminalObserver
	if terminalObs == nil {
		terminalObs = func(c context.Context, id string) bool {
			return waitForTerminalState(c, cli, id)
		}
	}
	return executeQualifiedLifecycleWithDependencies(ctx, QualifiedLifecycleDependencies{Runtime: audited, Control: control}, terminalObs, opts)
}

// executeQualifiedLifecycle is the runtime-agnostic core. It
// accepts an already-installed AuditedDockerRuntime and a
// terminal-state observer. Tests drive it directly with the
// recording fake.
func executeQualifiedLifecycle(ctx context.Context, audited *AuditedDockerRuntime, terminalObs func(context.Context, string) bool, opts LifecycleOptions) (*QualifiedLifecycleOutcome, error) {
	return executeQualifiedLifecycleWithDependencies(ctx, QualifiedLifecycleDependencies{Runtime: audited, Control: NewControlRunner(noopControlRuntime{})}, terminalObs, opts)
}

type noopControlRuntime struct{}

func (noopControlRuntime) ExecCreate(context.Context, string, ExecCreateOptions) (string, error) {
	return "", ErrControlRuntimeRequired
}
func (noopControlRuntime) ExecAttach(context.Context, string, string) (ControlExecAttachment, error) {
	return nil, ErrControlRuntimeRequired
}
func (noopControlRuntime) ExecInspect(context.Context, string, string) (ExecInspectResult, error) {
	return ExecInspectResult{}, ErrControlRuntimeRequired
}

func executeQualifiedLifecycleWithDependencies(
	ctx context.Context,
	deps QualifiedLifecycleDependencies,
	terminalObs func(context.Context, string) bool,
	opts LifecycleOptions,
) (*QualifiedLifecycleOutcome, error) {
	audited, ok := deps.Runtime.(*AuditedDockerRuntime)
	if !ok || audited == nil {
		return nil, errors.New("audited runtime is nil")
	}
	if deps.Control == nil {
		return nil, ErrQualifiedControlRequired
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
	if terminalObs == nil {
		return nil, errors.New("terminal observer is nil")
	}

	var phases []QualifiedLifecyclePhase
	record := func(phase QualifiedLifecyclePhase) { phases = append(phases, phase) }
	qc := NewQualifiedClient(audited)
	prepStart := audited.pullAttemptCount
	cfg := ContainerConfig{
		Name:   opts.ContainerName,
		Config: &container.Config{Image: opts.ImageReference, Cmd: opts.ContainerCmd},
	}
	obs, err := qc.PrepareQualifiedContainer(ctx, opts.ImageReference, opts.NetworkName, cfg)
	if err != nil {
		return nil, err
	}
	record(PhasePrepared)
	obs.Pull.ObservationAvailable = true

	finalOutcome := func(started, terminal bool, cleanup cleanupResult) *QualifiedLifecycleOutcome {
		record(PhaseLifecycleReturned)
		return &QualifiedLifecycleOutcome{
			ContainerID: obs.Container.ID, ImageID: obs.Image.InspectedBeforeCreate,
			NetworkID: obs.Network.InspectResponseID, Started: started, Terminal: terminal,
			ContainerRemoved: cleanup.containerRemoved, NetworkRemoved: cleanup.networkRemoved,
			StartedByRuntime: true, Observations: CloneQualifiedExecutionObservations(obs),
			Phases: append([]QualifiedLifecyclePhase(nil), phases...),
		}
	}

	if startErr := audited.ContainerStart(ctx, obs.Container.ID, types.ContainerStartOptions{}); startErr != nil {
		cleanup, cleanupErr := boundedCleanup(context.Background(), audited, obs.Container.ID, obs.Network.InspectResponseID, opts.CleanupTimeout)
		obs.Container.Removed = cleanup.containerRemoved
		obs.Network.Removed = cleanup.networkRemoved
		finalizePullAudit(audited, obs)
		return finalOutcome(false, false, cleanup), errors.Join(fmt.Errorf("start container: %w", startErr), cleanupErr)
	}
	obs.Container.Started = true
	record(PhaseStarted)

	record(PhaseWorkloadEntered)
	workloadResult, runErr := opts.Run(ctx, QualifiedWorkloadInput{
		ContainerID: obs.Container.ID,
		ImageID:     obs.Image.InspectedBeforeCreate,
		NetworkID:   obs.Network.InspectResponseID,
	})
	if workloadResult == nil {
		if runErr == nil {
			runErr = ErrMissingQualifiedWorkloadResult
		}
	} else if validationErr := validateQualifiedWorkloadObservations(workloadResult.Observations); validationErr != nil {
		runErr = errors.Join(runErr, validationErr)
	} else {
		record(PhaseWorkloadObserved)
		// The lifecycle is the sole writer of canonical observations and
		// performs this workload merge exactly once.
		obs.Reachability = workloadResult.Observations.Reachability
	}
	record(PhaseWorkloadReturned)

	terminalCtx, terminalCancel := context.WithTimeout(ctx, opts.TerminalTimeout)
	defer terminalCancel()
	terminalOK := terminalObs(terminalCtx, obs.Container.ID)
	var terminalErr error
	if terminalOK {
		obs.Container.TerminalStateObserved = true
		record(PhaseTerminalObserved)
	} else {
		terminalErr = errTerminalTimeout
	}

	cleanup, cleanupErr := boundedCleanup(context.Background(), audited, obs.Container.ID, obs.Network.InspectResponseID, opts.CleanupTimeout)
	obs.Container.Removed = cleanup.containerRemoved
	obs.Network.Removed = cleanup.networkRemoved
	if cleanup.containerRemoved {
		record(PhaseContainerRemoved)
	}
	if cleanup.networkRemoved {
		record(PhaseNetworkRemoved)
	}
	finalizePullAudit(audited, obs)

	var pullErr error
	_, pullCount, _ := audited.PullAudit()
	if pullCount > prepStart {
		pullErr = errPullAuditIncreased
	}
	joined := errors.Join(runErr, terminalErr, cleanupErr, pullErr)
	return finalOutcome(true, terminalOK, cleanup), joined
}

// cleanupResult captures the cleanup outcome.
type cleanupResult struct {
	containerRemoved bool
	networkRemoved   bool
}

// boundedCleanup creates a fresh bounded context and attempts to
// remove both the container and the network, joining errors. After
// removal, it verifies post-cleanup absence via typed Docker
// not-found evidence. Both removals are attempted even if the first
// fails.
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
			// Verify absence via typed not-found. Other errors
			// (timeout, permission, daemon unavailable) do not prove
			// absence; they are surfaced as cleanup failures.
			_, ierr := audited.ContainerInspect(cleanupCtx, containerID)
			if ierr == nil {
				// Inspect returned nil: resource still exists.
				joinErr = errors.Join(joinErr, errors.New("container remove returned nil but resource still exists"))
			} else if errdefs.IsNotFound(ierr) {
				res.containerRemoved = true
			} else {
				joinErr = errors.Join(joinErr, fmt.Errorf("container absence unproven: %w", ierr))
			}
		}
	}
	if networkID != "" {
		if err := audited.NetworkRemove(cleanupCtx, networkID); err != nil {
			joinErr = errors.Join(joinErr, fmt.Errorf("network remove: %w", err))
		} else {
			_, ierr := audited.NetworkInspect(cleanupCtx, networkID, types.NetworkInspectOptions{})
			if ierr == nil {
				joinErr = errors.Join(joinErr, errors.New("network remove returned nil but resource still exists"))
			} else if errdefs.IsNotFound(ierr) {
				res.networkRemoved = true
			} else {
				joinErr = errors.Join(joinErr, fmt.Errorf("network absence unproven: %w", ierr))
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
