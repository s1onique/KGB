// test_helpers_test.go — Helper functions for tests.

package dockerlab

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

// containerJSONWithImage creates a types.ContainerJSON with the given image.
func containerJSONWithImage(image string) types.ContainerJSON {
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Image: image,
		},
		Config: &container.Config{Image: image},
	}
}

// networkSettingsWithEndpoint creates a types.NetworkSettings with the given endpoint.
func networkSettingsWithEndpoint(name, networkID string) *types.NetworkSettings {
	return &types.NetworkSettings{
		Networks: map[string]*network.EndpointSettings{
			name: {
				NetworkID: networkID,
			},
		},
	}
}
