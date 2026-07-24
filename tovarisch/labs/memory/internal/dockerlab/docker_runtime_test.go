// docker_runtime_test.go — Test-only recording fake for DockerRuntime interface
//
// Implements DockerRuntime with deterministic counter increments.
// Not-found errors match errdefs.IsNotFound for production parity.
// ImagePull always records an attempt and returns ErrPullAttemptedSentinel,
// so the qualified path can prove zero pull attempts.

package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// InspectOverride is a hook for per-call inspect result customization.
type InspectOverride func(containerID string) (types.ContainerJSON, error)

// recordingDockerRuntime is a fake DockerRuntime that records all calls.
type recordingDockerRuntime struct {
	// Call counters (asserted by tests, not logged)
	imageInspectCalls      int
	containerCreateCalls   int
	containerInspectCalls  int
	containerStartCalls    int
	networkConnectCalls    int
	containerRemoveCalls   int
	networkRemoveCalls     int
	networkCreateCalls     int
	networkInspectCalls    int
	imagePullCalls         int
	networkInspectOverride func(string) (types.NetworkResource, error)

	// Recorded arguments
	createdImageArgument string
	lastContainerID      string
	lastNetworkID        string
	lastPulledReference  string

	// Ordered call log for sequence assertions.
	calls []string

	// Stored data for determinism
	storedImages     map[string]types.ImageInspect
	storedContainers map[string]types.ContainerJSON
	storedNetworks   map[string]types.NetworkResource

	// Configurable errors per operation.
	inspectImageErr     error
	containerCreateErr  error
	containerInspectErr error
	containerStartErr   error
	networkConnectErr   error
	containerRemoveErr  error
	networkRemoveErr    error
	networkCreateErr    error
	networkInspectErr   error

	// Network name to attach when NetworkConnect succeeds.
	connectNetworkName string

	// InspectOverride hook for per-call inspect result customization.
	inspectOverride InspectOverride

	// ConfigMutator is an adversarial hook to simulate runtime mutation
	// attempts on the received container.Config.
	ConfigMutator func(c *container.Config)

	// Force observed image to be different from create argument (for testing).
	forceObservedImage string

	// Next network ID to return from NetworkCreate when not pre-populated.
	nextNetworkCounter int
}

func newRecordingDockerRuntime() *recordingDockerRuntime {
	return &recordingDockerRuntime{
		storedImages:     make(map[string]types.ImageInspect),
		storedContainers: make(map[string]types.ContainerJSON),
		storedNetworks:   make(map[string]types.NetworkResource),
	}
}

// addImage adds a fake image to the store.
func (f *recordingDockerRuntime) addImage(ref string, imageID string) {
	f.storedImages[ref] = types.ImageInspect{ID: imageID}
}

// addContainer adds a fake container to the store.
func (f *recordingDockerRuntime) addContainer(containerID string, image string) {
	f.storedContainers[containerID] = types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Image: image,
		},
		Config: &container.Config{Image: image},
	}
}

// addNetwork adds a fake network to the store.
func (f *recordingDockerRuntime) addNetwork(networkID, name string) {
	f.storedNetworks[networkID] = types.NetworkResource{ID: networkID, Name: name}
}

// ImageInspectWithRaw implements DockerRuntime.
func (f *recordingDockerRuntime) ImageInspectWithRaw(ctx context.Context, ref string) (types.ImageInspect, []byte, error) {
	f.imageInspectCalls++
	f.calls = append(f.calls, "ImageInspectWithRaw")
	if f.inspectImageErr != nil {
		return types.ImageInspect{}, nil, f.inspectImageErr
	}
	if img, ok := f.storedImages[ref]; ok {
		return img, nil, nil
	}
	return types.ImageInspect{}, nil, errdefs.NotFound(errors.New(fmt.Sprintf("No such image: %s", ref)))
}

// ContainerCreate implements DockerRuntime. Network state is derived from the
// actual NetworkingConfig passed in — tests must not configure successful
// network state through a side channel.
func (f *recordingDockerRuntime) ContainerCreate(
	ctx context.Context,
	cfg *container.Config,
	hostCfg *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	platform *ocispec.Platform,
	name string,
) (container.CreateResponse, error) {
	f.containerCreateCalls++
	f.calls = append(f.calls, "ContainerCreate")
	if f.containerCreateErr != nil {
		return container.CreateResponse{}, f.containerCreateErr
	}
	if cfg != nil {
		f.createdImageArgument = cfg.Image
		// Adversarial mutation hook simulating runtime behavior.
		if f.ConfigMutator != nil {
			f.ConfigMutator(cfg)
		}
	}
	containerID := "fake-container-1"
	f.lastContainerID = containerID
	// The observed image is determined by forceObservedImage (if set) or the created image.
	observedImage := f.forceObservedImage
	if observedImage == "" && cfg != nil {
		observedImage = cfg.Image
	}
	// Initialize network state from the NetworkingConfig passed in.
	networks := map[string]*network.EndpointSettings{}
	if networkingConfig != nil {
		for name, ep := range networkingConfig.EndpointsConfig {
			if ep != nil {
				networks[name] = ep
			} else {
				networks[name] = &network.EndpointSettings{}
			}
		}
	}
	// Initialize the container inspect state.
	f.storedContainers[containerID] = types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Image: observedImage,
		},
		Config:          &container.Config{Image: observedImage},
		NetworkSettings: &types.NetworkSettings{Networks: networks},
	}
	return container.CreateResponse{ID: containerID}, nil
}

// ContainerInspect implements DockerRuntime. Supports InspectOverride hook
// for deterministic per-call result customization.
func (f *recordingDockerRuntime) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	f.containerInspectCalls++
	f.calls = append(f.calls, "ContainerInspect")
	if f.containerInspectErr != nil {
		return types.ContainerJSON{}, f.containerInspectErr
	}
	if f.inspectOverride != nil {
		return f.inspectOverride(containerID)
	}
	if c, ok := f.storedContainers[containerID]; ok {
		return c, nil
	}
	return types.ContainerJSON{}, errdefs.NotFound(errors.New(fmt.Sprintf("No such container: %s", containerID)))
}

// ContainerStart implements DockerRuntime.
func (f *recordingDockerRuntime) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	f.containerStartCalls++
	f.calls = append(f.calls, "ContainerStart")
	if f.containerStartErr != nil {
		return f.containerStartErr
	}
	return nil
}

// NetworkConnect implements DockerRuntime. Mutates the inspected container state
// only after successful attachment.
func (f *recordingDockerRuntime) NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error {
	f.networkConnectCalls++
	f.calls = append(f.calls, "NetworkConnect")
	if f.networkConnectErr != nil {
		return f.networkConnectErr
	}
	if c, ok := f.storedContainers[containerID]; ok {
		if c.NetworkSettings == nil {
			c.NetworkSettings = &types.NetworkSettings{Networks: map[string]*network.EndpointSettings{}}
		}
		netName := f.connectNetworkName
		if netName == "" {
			netName = "default-network"
		}
		c.NetworkSettings.Networks[netName] = &network.EndpointSettings{
			NetworkID: networkID,
		}
		f.storedContainers[containerID] = c
	}
	return nil
}

// ContainerRemove implements DockerRuntime.
func (f *recordingDockerRuntime) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	f.containerRemoveCalls++
	f.calls = append(f.calls, "ContainerRemove")
	if f.containerRemoveErr != nil {
		return f.containerRemoveErr
	}
	delete(f.storedContainers, containerID)
	return nil
}

// NetworkRemove implements DockerRuntime.
func (f *recordingDockerRuntime) NetworkRemove(ctx context.Context, networkID string) error {
	f.networkRemoveCalls++
	f.calls = append(f.calls, "NetworkRemove")
	if f.networkRemoveErr != nil {
		return f.networkRemoveErr
	}
	delete(f.storedNetworks, networkID)
	return nil
}

// NetworkCreate implements DockerRuntime. Auto-assigns a deterministic ID when
// the test has not pre-populated the network store.
func (f *recordingDockerRuntime) NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
	f.networkCreateCalls++
	f.calls = append(f.calls, "NetworkCreate")
	if f.networkCreateErr != nil {
		return types.NetworkCreateResponse{}, f.networkCreateErr
	}
	f.nextNetworkCounter++
	counter := fmt.Sprintf("%064x", f.nextNetworkCounter)
	networkID := counter
	f.lastNetworkID = networkID
	f.storedNetworks[networkID] = types.NetworkResource{ID: networkID, Name: name}
	return types.NetworkCreateResponse{ID: networkID}, nil
}

// NetworkInspect implements DockerRuntime.
func (f *recordingDockerRuntime) NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
	f.networkInspectCalls++
	f.calls = append(f.calls, "NetworkInspect")
	if f.networkInspectErr != nil {
		return types.NetworkResource{}, f.networkInspectErr
	}
	if f.networkInspectOverride != nil {
		return f.networkInspectOverride(networkID)
	}
	if n, ok := f.storedNetworks[networkID]; ok {
		return n, nil
	}
	return types.NetworkResource{}, errdefs.NotFound(errors.New(fmt.Sprintf("No such network: %s", networkID)))
}

// ImagePull implements DockerRuntime. Always records the attempt and returns
// ErrPullAttemptedSentinel so the qualified execution path can prove that
// the production code did NOT call a pull operation.
func (f *recordingDockerRuntime) ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
	f.imagePullCalls++
	f.calls = append(f.calls, "ImagePull")
	f.lastPulledReference = refStr
	return nil, ErrPullAttemptedSentinel
}

// Compile-time conformance assertion.
var _ DockerRuntime = (*recordingDockerRuntime)(nil)

// Ensure ocispec import is used.
var _ = ocispec.Platform{}
