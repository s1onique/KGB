// qualified_runtime.go — Shared interface-backed qualified execution path.
//
// CORRECTION17: this file is the single production implementation
// used by:
//   - the real CLI run path (via the production client.Client);
//   - the live Docker smoke (via the AuditedDockerRuntime wrapper);
//   - hermetic DockerRuntime tests (via the recordingDockerRuntime).
//
// The path exposes a single shared Prepare operation that returns
// raw authoritative observations. The caller owns start, workload,
// stop and cleanup.

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

// ErrContainerAlreadyStarted is returned when the caller attempts to
// start a container that the qualified runtime has already started.
var ErrContainerAlreadyStarted = errors.New("container is already started by the qualified runtime")

// QualifiedExecutionObservationsSchemaVersion is the canonical
// schema version used by the qualified execution observation object.
// The runtime does not import the evidence package to keep the
// import graph acyclic; the constant is duplicated in evidence.
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
	Removed           bool
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
//     ImageInspectWithRaw. Record the observation.
//  3. Create the isolated lab network via NetworkCreate. Record
//     the create-response ID.
//  4. Inspect the network via NetworkInspect. Record the
//     inspect-response ID.
//  5. Create the container with the exact image ID and the
//     create-time networking config.
//  6. Post-create inspect and complete validation (P0-4).
//
// PrepareQualifiedContainer does NOT start the container. The
// caller owns the start, workload, stop and cleanup.
func (q *QualifiedClient) PrepareQualifiedContainer(
	ctx context.Context,
	imageReference string,
	networkName string,
	networkID string,
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
	if networkID != "" {
		if err := ValidateCanonicalNetworkIDLenient(networkID); err != nil {
			return nil, fmt.Errorf("requested network ID is not canonical: %w", err)
		}
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
	// create-time networking. Deep-copy the caller's container.Config
	// so runtime mutations cannot leak (CORRECTION15).
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
// PrepareQualifiedContainer + the runtime start/remove helpers.
// The observations are filled in from real Docker operations.
//
// Deprecated: callers should use PrepareQualifiedContainer and
// own the lifecycle explicitly so the observation fields are
// fully populated.
func (q *QualifiedClient) ExecuteQualifiedContainer(
	ctx context.Context,
	imageReference string,
	networkName string,
	networkID string,
	cfg ContainerConfig,
) (QualifiedContainerResult, error) {
	obs, err := q.PrepareQualifiedContainer(ctx, imageReference, networkName, networkID, cfg)
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

// CleanupContainerAndNetwork is a bounded helper that the caller can
// use after the workload completes. It always tries to remove both
// resources and joins the diagnostics.
func (q *QualifiedClient) CleanupContainerAndNetwork(ctx context.Context, containerID, networkID string) error {
	cleanErr := error(nil)
	if containerID != "" {
		if err := q.runtime.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}); err != nil {
			cleanErr = errors.Join(cleanErr, fmt.Errorf("container remove: %w", err))
		}
	}
	if networkID != "" {
		if err := q.runtime.NetworkRemove(ctx, networkID); err != nil {
			cleanErr = errors.Join(cleanErr, fmt.Errorf("network remove: %w", err))
		}
	}
	return cleanErr
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
