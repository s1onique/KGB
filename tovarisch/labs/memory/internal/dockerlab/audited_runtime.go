// audited_runtime.go — Audited wrapper for a DockerRuntime.
//
// CORRECTION18: the audited runtime captures every operation that
// affects the qualified evidence. The wrapper is the single source
// of recorded observations; the qualified runtime MUST consume the
// audit rather than copy expected values into the observation
// object.
//
// Captured observations:
//   - pull.attempted, pull.attempt_count, pull.last_reference
//   - container_create.called, container_create.image,
//     container_create.network_name, container_create.network_id
//   - container_inspect.called, container_inspect.image,
//     container_inspect.config_image,
//     container_inspect.endpoint_network_id
//   - network.create_response_id, network.inspect_response_id
//
// The audit is concurrency-safe. The real `*client.Client` is the
// only sanctioned delegate; the qualified production path wires
// it through `NewAuditedDockerRuntime`.

package dockerlab

import (
	"context"
	"io"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// AuditedDockerRuntime wraps a DockerRuntime and instruments every
// qualified-related call.
type AuditedDockerRuntime struct {
	delegate DockerRuntime

	mu sync.Mutex

	// Pull audit.
	pullAttempted     bool
	pullAttemptCount  int
	lastPulledReference string

	// ContainerCreate audit.
	createCalled     bool
	createImage       string
	createNetName     string
	createNetID       string

	// ContainerInspect audit (response values).
	inspectCalled   bool
	inspectImage    string
	inspectConfig   string
	inspectEndpoint string

	// NetworkCreate / NetworkInspect audit (response values).
	netCreateID string
	netInspectID string
}

// NewAuditedDockerRuntime wraps the given delegate.
func NewAuditedDockerRuntime(delegate DockerRuntime) *AuditedDockerRuntime {
	return &AuditedDockerRuntime{delegate: delegate}
}

// AuditSnapshot is a read-only view of every captured observation.
type AuditSnapshot struct {
	PullAttempted     bool
	PullAttemptCount  int
	LastPulledReference string

	CreateCalled bool
	CreateImage   string
	CreateNetName string
	CreateNetID   string

	InspectCalled   bool
	InspectImage    string
	InspectConfig   string
	InspectEndpoint string

	NetCreateID string
	NetInspectID string
}

// PullAudit returns the recorded pull counters.
func (a *AuditedDockerRuntime) PullAudit() (attempted bool, count int, lastRef string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pullAttempted, a.pullAttemptCount, a.lastPulledReference
}

// Snapshot returns a copy of every recorded observation.
func (a *AuditedDockerRuntime) Snapshot() AuditSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AuditSnapshot{
		PullAttempted:       a.pullAttempted,
		PullAttemptCount:     a.pullAttemptCount,
		LastPulledReference:  a.lastPulledReference,
		CreateCalled:         a.createCalled,
		CreateImage:          a.createImage,
		CreateNetName:        a.createNetName,
		CreateNetID:          a.createNetID,
		InspectCalled:        a.inspectCalled,
		InspectImage:         a.inspectImage,
		InspectConfig:        a.inspectConfig,
		InspectEndpoint:       a.inspectEndpoint,
		NetCreateID:          a.netCreateID,
		NetInspectID:         a.netInspectID,
	}
}

// ImageInspectWithRaw forwards to the delegate.
func (a *AuditedDockerRuntime) ImageInspectWithRaw(ctx context.Context, ref string) (types.ImageInspect, []byte, error) {
	return a.delegate.ImageInspectWithRaw(ctx, ref)
}

// ContainerCreate forwards to the delegate and records the request.
func (a *AuditedDockerRuntime) ContainerCreate(
	ctx context.Context,
	cfg *container.Config,
	hostCfg *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	platform *ocispec.Platform,
	name string,
) (container.CreateResponse, error) {
	netName, netID := extractSingleNetwork(networkingConfig)
	a.mu.Lock()
	a.createCalled = true
	if cfg != nil {
		a.createImage = cfg.Image
	}
	a.createNetName = netName
	a.createNetID = netID
	a.mu.Unlock()
	return a.delegate.ContainerCreate(ctx, cfg, hostCfg, networkingConfig, platform, name)
}

// extractSingleNetwork returns the single network name and ID
// configured for create-time networking.
func extractSingleNetwork(nc *network.NetworkingConfig) (string, string) {
	if nc == nil {
		return "", ""
	}
	for name, ep := range nc.EndpointsConfig {
		if ep == nil {
			continue
		}
		return name, ep.NetworkID
	}
	return "", ""
}

// ContainerInspect forwards to the delegate and records the response.
func (a *AuditedDockerRuntime) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	resp, err := a.delegate.ContainerInspect(ctx, containerID)
	if err != nil {
		return resp, err
	}
	var image, config, endpoint string
	if resp.Config != nil {
		image = resp.Image
		config = resp.Config.Image
	}
	if resp.NetworkSettings != nil {
		for _, ep := range resp.NetworkSettings.Networks {
			if ep == nil {
				continue
			}
			endpoint = ep.NetworkID
		}
	}
	a.mu.Lock()
	a.inspectCalled = true
	a.inspectImage = image
	a.inspectConfig = config
	a.inspectEndpoint = endpoint
	a.mu.Unlock()
	return resp, nil
}

// ContainerStart forwards to the delegate.
func (a *AuditedDockerRuntime) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	return a.delegate.ContainerStart(ctx, containerID, options)
}

// NetworkConnect forwards to the delegate.
func (a *AuditedDockerRuntime) NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error {
	return a.delegate.NetworkConnect(ctx, networkID, containerID, config)
}

// ContainerRemove forwards to the delegate.
func (a *AuditedDockerRuntime) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	return a.delegate.ContainerRemove(ctx, containerID, options)
}

// NetworkRemove forwards to the delegate.
func (a *AuditedDockerRuntime) NetworkRemove(ctx context.Context, networkID string) error {
	return a.delegate.NetworkRemove(ctx, networkID)
}

// NetworkCreate forwards to the delegate and records the create-response.
func (a *AuditedDockerRuntime) NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
	resp, err := a.delegate.NetworkCreate(ctx, name, options)
	if err != nil {
		return resp, err
	}
	a.mu.Lock()
	a.netCreateID = resp.ID
	a.mu.Unlock()
	return resp, nil
}

// NetworkInspect forwards to the delegate and records the inspect-response.
func (a *AuditedDockerRuntime) NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
	resp, err := a.delegate.NetworkInspect(ctx, networkID, options)
	if err != nil {
		return resp, err
	}
	a.mu.Lock()
	a.netInspectID = resp.ID
	a.mu.Unlock()
	return resp, nil
}

// ImagePull records the attempt and returns ErrPullAttemptedSentinel.
// The real delegate is NOT called.
func (a *AuditedDockerRuntime) ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
	a.mu.Lock()
	a.pullAttempted = true
	a.pullAttemptCount++
	a.lastPulledReference = refStr
	a.mu.Unlock()
	return nil, ErrPullAttemptedSentinel
}

// Compile-time conformance.
var _ DockerRuntime = (*AuditedDockerRuntime)(nil)
