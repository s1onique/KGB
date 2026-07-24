// docker_runtime.go — Minimal Docker API interface for exact image ID boundary
//
// Defines the smallest interface required by the qualified execution path.
// Production methods use this interface; tests inject a recording fake.
//
// CORRECTION16: the interface now covers network create/inspect and a
// pull-attempt sentinel so the qualified path can prove zero pulls
// without depending on source-code grep.

package dockerlab

import (
	"context"
	"errors"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ErrPullAttemptedSentinel is returned by ImagePull on the recording
// fake whenever any image-pull path is invoked. Production code
// routes pull operations through this seam so the smoke can prove
// zero pull attempts in the qualified execution path.
var ErrPullAttemptedSentinel = errors.New("pull attempted: not allowed in qualified execution path")

// DockerRuntime is the minimal Docker API interface for qualified execution paths.
// It MUST match the signature of *client.Client so that the real Docker client
// satisfies it (enforced by a compile-time assertion below).
type DockerRuntime interface {
	ImageInspectWithRaw(ctx context.Context, ref string) (types.ImageInspect, []byte, error)
	ContainerCreate(
		ctx context.Context,
		cfg *container.Config,
		hostCfg *container.HostConfig,
		networkingConfig *network.NetworkingConfig,
		platform *ocispec.Platform,
		name string,
	) (container.CreateResponse, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	NetworkRemove(ctx context.Context, networkID string) error

	// CORRECTION16: network create/inspect and pull-attempt seam. These
	// signatures mirror the real client.Client methods exactly so the
	// production path can use the real Docker SDK without adapter code.
	NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error)
	NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error)
	// ImagePull matches the real client.Client signature; the recording
	// fake discards the reader and always returns ErrPullAttemptedSentinel.
	ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error)
}

// Compile-time conformance assertion: the real Docker client must implement DockerRuntime.
// If the pinned dependency changes signature, this line fails to compile.
var _ DockerRuntime = (*client.Client)(nil)
