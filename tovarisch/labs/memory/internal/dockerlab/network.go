// network.go — Docker network and container helpers
//
// Reference: kgb://factory/workflow

package dockerlab

import (
	"context"
	"fmt"
)

// Network represents a Docker network.
type Network struct {
	ID   string
	Name string
}

// ResourceLimits defines container resource constraints.
type ResourceLimits struct {
	MemoryLimit string
	CPUPeriod   int64
	CPUQuota    int64
	PidsLimit   int64
	MemorySwap  string
}

// DefaultResourceLimits returns default container resource limits.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MemoryLimit: "128m",
		CPUPeriod:   100000,
		CPUQuota:    50000,
		PidsLimit:   64,
		MemorySwap:  "128m",
	}
}

// CreateNetwork creates a new lab network with proper labeling.
func CreateNetwork(ctx context.Context, c *Client, cleanup *CleanupManager, runID, suffix string) (*Network, error) {
	name := fmt.Sprintf("kgb-lab-%s-%s", runID, suffix)

	resp, err := c.NetworkCreate(ctx, name, "bridge")
	if err != nil {
		return nil, fmt.Errorf("create network %s: %w", name, err)
	}

	cleanup.RegisterNetwork(resp)

	return &Network{
		ID:   resp,
		Name: name,
	}, nil
}

// ContainerInfo holds container information from inspect.
type ContainerInfo struct {
	ID       string
	Name     string
	PID      int
	Image    string
	Status   string
	Networks []string
}

// CreateContainer creates a container with the given config and returns its ID.
func (c *Client) CreateContainer(ctx context.Context, name, image string, cmd []string, labels map[string]string) (string, error) {
	resp, err := c.Client.ContainerCreate(ctx, nil, nil, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

// Container represents a Docker container.
type Container struct {
	ID     string
	Name   string
	Status string
	PID    int
}
