// audited_runtime.go — Audited wrapper for a DockerRuntime.
//
// CORRECTION17: the live Docker smoke must construct the qualified
// client with this audited runtime so the recorded pull counters
// are observable, not just implied by source-code review. Any
// qualified production path that wants a real-Docker audit can
// also install this wrapper.
//
//   - All normal operations forward to the real delegate.
//   - ImagePull increments pullAttemptCount, captures the
//     reference, and returns ErrPullAttemptedSentinel without
//     touching the delegate. The sentinel is a fail-closed
//     observation marker; a real Docker pull must not occur in
//     the qualified path.
//   - The wrapper is concurrency-safe; pull counters are recorded
//     under a mutex.

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
// pull-related call. The audit counters are observable via the
// PullObservations helper.
type AuditedDockerRuntime struct {
	delegate DockerRuntime

	mu                  sync.Mutex
	pullAttemptCount    int
	lastPulledReference string
}

// NewAuditedDockerRuntime wraps the given delegate.
func NewAuditedDockerRuntime(delegate DockerRuntime) *AuditedDockerRuntime {
	return &AuditedDockerRuntime{delegate: delegate}
}

// PullAttemptCount returns the number of times ImagePull was invoked.
func (a *AuditedDockerRuntime) PullAttemptCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pullAttemptCount
}

// LastPulledReference returns the most recent reference passed to
// ImagePull, or "" if no pull was attempted.
func (a *AuditedDockerRuntime) LastPulledReference() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastPulledReference
}

// PullAudit returns the audit snapshot as separate values.
func (a *AuditedDockerRuntime) PullAudit() (attempted bool, count int, lastRef string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pullAttemptCount > 0, a.pullAttemptCount, a.lastPulledReference
}

// ImageInspectWithRaw forwards to the delegate.
func (a *AuditedDockerRuntime) ImageInspectWithRaw(ctx context.Context, ref string) (types.ImageInspect, []byte, error) {
	return a.delegate.ImageInspectWithRaw(ctx, ref)
}

// ContainerCreate forwards to the delegate.
func (a *AuditedDockerRuntime) ContainerCreate(
	ctx context.Context,
	cfg *container.Config,
	hostCfg *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	platform *ocispec.Platform,
	name string,
) (container.CreateResponse, error) {
	return a.delegate.ContainerCreate(ctx, cfg, hostCfg, networkingConfig, platform, name)
}

// ContainerInspect forwards to the delegate.
func (a *AuditedDockerRuntime) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return a.delegate.ContainerInspect(ctx, containerID)
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

// NetworkCreate forwards to the delegate.
func (a *AuditedDockerRuntime) NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
	return a.delegate.NetworkCreate(ctx, name, options)
}

// NetworkInspect forwards to the delegate.
func (a *AuditedDockerRuntime) NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
	return a.delegate.NetworkInspect(ctx, networkID, options)
}

// ImagePull records the attempt and returns ErrPullAttemptedSentinel.
// The real delegate is NOT called. The live test asserts the recorded
// counters via PullAudit.
func (a *AuditedDockerRuntime) ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
	a.mu.Lock()
	a.pullAttemptCount++
	a.lastPulledReference = refStr
	a.mu.Unlock()
	return nil, ErrPullAttemptedSentinel
}

// Compile-time conformance.
var _ DockerRuntime = (*AuditedDockerRuntime)(nil)
