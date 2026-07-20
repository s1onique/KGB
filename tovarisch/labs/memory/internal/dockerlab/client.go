// client.go — Docker Engine API client wrapper
//
// Uses Docker Engine Go client directly; no os/exec docker commands.
// Provides container lifecycle, network management, and inspection.
//
// Reference: kgb://doctrine/native-owned-critical-paths

package dockerlab

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// Client wraps the Docker Engine API client.
type Client struct {
	*client.Client
}

// NewClient creates a new Docker client using the default socket.
func NewClient(ctx context.Context) (*Client, error) {
	// Use default socket or DOCKER_HOST env
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	// Verify connection works
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("docker ping: %w", err)
	}

	return &Client{Client: cli}, nil
}

// Info returns Docker daemon info.
func (c *Client) Info(ctx context.Context) (types.Info, error) {
	return c.Client.Info(ctx)
}

// ServerVersion returns Docker Engine version information.
func (c *Client) ServerVersion(ctx context.Context) (types.Version, error) {
	return c.Client.ServerVersion(ctx)
}

// ClientVersion returns the API client version.
func (c *Client) ClientVersion() string {
	return c.Client.ClientVersion()
}

// ImagePull pulls an image if not present.
// Returns image ID or error.
func (c *Client) ImagePull(ctx context.Context, refStr string) (string, error) {
	// Check if image exists locally
	args := filters.NewArgs()
	args.Add("reference", refStr)
	images, err := c.Client.ImageList(ctx, types.ImageListOptions{
		Filters: args,
	})
	if err != nil {
		return "", fmt.Errorf("list images: %w", err)
	}

	if len(images) > 0 {
		return images[0].ID, nil
	}

	// Pull the image
	out, err := c.Client.ImagePull(ctx, refStr, types.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("pull image %s: %w", refStr, err)
	}
	defer out.Close()

	// Read pull result (just ensure it completes)
	buf := make([]byte, 4096)
	for {
		_, err := out.Read(buf)
		if err != nil {
			break
		}
	}

	// List again to get the ID
	args = filters.NewArgs()
	args.Add("reference", refStr)
	images, err = c.Client.ImageList(ctx, types.ImageListOptions{
		Filters: args,
	})
	if err != nil || len(images) == 0 {
		return "", fmt.Errorf("image not found after pull: %s", refStr)
	}

	return images[0].ID, nil
}

// ContainerCreate creates a new container.
func (c *Client) ContainerCreate(ctx context.Context, cfg ContainerConfig) (string, error) {
	resources := container.Resources{
		Memory:   cfg.MemoryLimit,
		CPUQuota: cfg.CPUQuota,
	}
	hostCfg := container.HostConfig{
		Resources: resources,
		// Note: Containers use Docker's default isolated PID namespace.
		// The host controller obtains container PID through Docker inspect.
		// Do NOT use PidMode: "host" for normal operations.
		// Container process /proc is visible from host at /proc/{host_pid}.
	}

	resp, err := c.Client.ContainerCreate(ctx, cfg.Config, &hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

// ContainerStart starts a container.
func (c *Client) ContainerStart(ctx context.Context, containerID string) error {
	return c.Client.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

// ContainerStop stops a container gracefully.
func (c *Client) ContainerStop(ctx context.Context, containerID string, timeout time.Duration) error {
	timeoutSec := int(timeout.Seconds())
	return c.Client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSec})
}

// ContainerRemove removes a container.
func (c *Client) ContainerRemove(ctx context.Context, containerID string, force bool) error {
	return c.Client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		Force: force,
	})
}

// ContainerInspect returns container details.
func (c *Client) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return c.Client.ContainerInspect(ctx, containerID)
}

// ContainerLogs returns container logs.
func (c *Client) ContainerLogs(ctx context.Context, containerID string, tail string) (string, error) {
	reader, err := c.Client.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}
	defer reader.Close()

	output, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}

	return string(output), nil
}

// ContainerExec creates an exec instance in a running container and returns output.
func (c *Client) ContainerExec(ctx context.Context, containerID string, cmd []string) (int, string, error) {
	execResp, err := c.Client.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return -1, "", fmt.Errorf("create exec: %w", err)
	}

	execID := execResp.ID

	// Attach to exec
	resp, err := c.Client.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return -1, "", fmt.Errorf("attach exec: %w", err)
	}
	defer resp.Close()

	// Read output
	buf := make([]byte, 32768)
	var output []byte
	for {
		n, err := resp.Reader.Read(buf)
		if n > 0 {
			output = append(output, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Wait for exec to complete
	info, err := c.Client.ContainerExecInspect(ctx, execID)
	if err != nil {
		return -1, string(output), fmt.Errorf("inspect exec: %w", err)
	}

	return info.ExitCode, string(output), nil
}

// NetworkCreate creates a new network.
func (c *Client) NetworkCreate(ctx context.Context, name string, driver string) (string, error) {
	resp, err := c.Client.NetworkCreate(ctx, name, types.NetworkCreate{
		Driver: driver,
		Labels: map[string]string{
			"kgb.dev/lab": "tovarisch-memory",
		},
		Attachable: true,
	})
	if err != nil {
		return "", fmt.Errorf("create network: %w", err)
	}
	return resp.ID, nil
}

// NetworkConnect connects a container to a network.
func (c *Client) NetworkConnect(ctx context.Context, networkID, containerID string) error {
	return c.Client.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{})
}

// NetworkDisconnect disconnects a container from a network.
func (c *Client) NetworkDisconnect(ctx context.Context, networkID, containerID string) error {
	return c.Client.NetworkDisconnect(ctx, networkID, containerID, true)
}

// NetworkRemove removes a network.
func (c *Client) NetworkRemove(ctx context.Context, networkID string) error {
	return c.Client.NetworkRemove(ctx, networkID)
}

// CleanupManager tracks resources for cleanup.
type CleanupManager struct {
	client     *Client
	runID      string
	networks   []string
	containers []string
}

// NewCleanupManager creates a new cleanup manager.
func NewCleanupManager(client *Client, runID string) *CleanupManager {
	return &CleanupManager{
		client:     client,
		runID:      runID,
		networks:   []string{},
		containers: []string{},
	}
}

// RegisterNetwork tracks a network for cleanup.
func (cm *CleanupManager) RegisterNetwork(networkID string) {
	cm.networks = append(cm.networks, networkID)
}

// RegisterContainer tracks a container for cleanup.
func (cm *CleanupManager) RegisterContainer(containerID string) {
	cm.containers = append(cm.containers, containerID)
}

// Cleanup removes all tracked resources and returns the first error encountered.
func (cm *CleanupManager) Cleanup(ctx context.Context) error {
	var lastErr error

	// Remove containers first
	for _, id := range cm.containers {
		if err := cm.client.ContainerRemove(ctx, id, true); err != nil {
			lastErr = err
		}
	}

	// Then remove networks
	for _, id := range cm.networks {
		if err := cm.client.NetworkRemove(ctx, id); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// ContainerRunner manages container execution.
type ContainerRunner struct {
	client *Client
}

// NewContainerRunner creates a new container runner.
func NewContainerRunner(client *Client) *ContainerRunner {
	return &ContainerRunner{client: client}
}

// RunOnce runs a container with the given config and returns.
func (cr *ContainerRunner) RunOnce(ctx context.Context, cfg ContainerConfig) (*types.ContainerJSON, error) {
	// Pull image
	if _, err := cr.client.ImagePull(ctx, cfg.Config.Image); err != nil {
		return nil, fmt.Errorf("pull image: %w", err)
	}

	// Create container
	id, err := cr.client.ContainerCreate(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	// Start container
	if err := cr.client.ContainerStart(ctx, id); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	// Inspect to get details
	inspect, err := cr.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	return &inspect, nil
}

// WaitForPort waits for a container port to be available.
func (cr *ContainerRunner) WaitForPort(ctx context.Context, containerID string, port string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to exec a simple command
		_, _, err := cr.client.ContainerExec(ctx, containerID, []string{"curl", "-s", "localhost:" + port})
		if err == nil {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for port %s", port)
}

// ContainerConfig wraps container configuration.
type ContainerConfig struct {
	Name        string
	Config      *container.Config
	MemoryLimit int64
	CPUQuota    int64
	Networks    []string
	AutoRemove  bool
}

// NewContainerConfig creates a basic container config.
func NewContainerConfig(name, image string) *ContainerConfig {
	return &ContainerConfig{
		Name: name,
		Config: &container.Config{
			Image: image,
		},
	}
}

// WithMemory sets memory limit.
func (c *ContainerConfig) WithMemory(bytes int64) *ContainerConfig {
	c.MemoryLimit = bytes
	return c
}

// WithCPU sets CPU quota.
func (c *ContainerConfig) WithCPU(quota int64) *ContainerConfig {
	c.CPUQuota = quota
	return c
}

// WithNetworks sets additional networks.
func (c *ContainerConfig) WithNetworks(networks ...string) *ContainerConfig {
	c.Networks = networks
	return c
}

// WithAutoRemove enables auto-removal on exit.
func (c *ContainerConfig) WithAutoRemove() *ContainerConfig {
	c.AutoRemove = true
	return c
}

// ImageBuilder builds container images.
type ImageBuilder struct {
	client *Client
}

// NewImageBuilder creates a new image builder.
func NewImageBuilder(client *Client) *ImageBuilder {
	return &ImageBuilder{client: client}
}

// BuildFromDockerfile builds an image from a Dockerfile.
// Returns image ID and binary hash.
func (ib *ImageBuilder) BuildFromDockerfile(ctx context.Context, dockerfilePath string, tag string) (string, string, error) {
	// Create tar archive with Dockerfile
	tarReader, err := createTarContext(dockerfilePath)
	if err != nil {
		return "", "", fmt.Errorf("create tar context: %w", err)
	}

	// Build the image
	resp, err := ib.client.ImageBuild(ctx, tarReader, types.ImageBuildOptions{
		Tags:       []string{tag},
		Dockerfile: "Dockerfile.canary",
		NoCache:    true,
		Remove:     true,
	})
	if err != nil {
		return "", "", fmt.Errorf("build image: %w", err)
	}
	defer resp.Body.Close()

	// Read build output to get image ID
	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read build output: %w", err)
	}

	// Extract image ID from output
	imageID := extractImageID(string(output))
	return imageID, "", nil
}

// createTarContext creates a tar archive containing the Dockerfile.
func createTarContext(dockerfilePath string) (io.Reader, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		tw := tar.NewWriter(pw)
		defer tw.Close()

		// Add Dockerfile
		dockerfile, err := os.ReadFile(dockerfilePath)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		header := &tar.Header{
			Name: filepath.Base(dockerfilePath),
			Mode: 0644,
			Size: int64(len(dockerfile)),
		}

		if err := tw.WriteHeader(header); err != nil {
			pw.CloseWithError(err)
			return
		}

		if _, err := tw.Write(dockerfile); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	return pr, nil
}

// extractImageID extracts the image ID from build output.
func extractImageID(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.Contains(line, `"stream"`) && strings.Contains(line, `Successfully built`) {
			// Parse the ID
			parts := strings.Split(line, " ")
			for _, p := range parts {
				if len(p) == 64 && strings.IndexFunc(p, func(r rune) bool {
					return r < '0' || r > '9' && r < 'a' || r > 'f'
				}) == -1 {
					return p
				}
			}
		}
	}
	return ""
}

// ContainerGetPID returns the container's host PID.
func (c *Client) ContainerGetPID(ctx context.Context, containerID string) (int, error) {
	inspect, err := c.ContainerInspect(ctx, containerID)
	if err != nil {
		return 0, fmt.Errorf("inspect container: %w", err)
	}

	if inspect.State == nil || !inspect.State.Running {
		return 0, fmt.Errorf("container not running")
	}

	return inspect.State.Pid, nil
}

// ContainerIP returns the container's IP address in the specified network.
func (c *Client) ContainerIP(ctx context.Context, containerID string, networkName string) (string, error) {
	inspect, err := c.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}

	if inspect.NetworkSettings == nil {
		return "", fmt.Errorf("no network settings")
	}

	// Find the exact network by name
	if net, ok := inspect.NetworkSettings.Networks[networkName]; ok {
		if net.IPAddress != "" {
			return net.IPAddress, nil
		}
	}

	// Also try with full network ID pattern (e.g., "network:lab")
	fullName := "network:" + networkName
	if net, ok := inspect.NetworkSettings.Networks[fullName]; ok {
		if net.IPAddress != "" {
			return net.IPAddress, nil
		}
	}

	return "", fmt.Errorf("no IP address found for network %s", networkName)
}

// ContainerStats holds container resource statistics.
type ContainerStats struct {
	MemoryUsageBytes    int64
	MemoryLimitBytes    int64
	CPUUsageNanoSeconds uint64
	MemoryPerc          float64
}

// ContainerStats returns real-time container stats.
func (c *Client) ContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	reader, err := c.Client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("get container stats: %w", err)
	}
	defer reader.Body.Close()

	// Read stats JSON
	var stats types.StatsJSON
	if err := json.NewDecoder(reader.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}

	// Extract memory stats
	memUsage := int64(stats.MemoryStats.Usage)
	memLimit := int64(stats.MemoryStats.Limit)

	// Extract CPU stats (cumulative nanoseconds)
	var cpuNano uint64
	if len(stats.CPUStats.CPUUsage.PercpuUsage) > 0 {
		cpuNano = stats.CPUStats.CPUUsage.TotalUsage
	}

	return &ContainerStats{
		MemoryUsageBytes:    memUsage,
		MemoryLimitBytes:    memLimit,
		CPUUsageNanoSeconds: cpuNano,
		MemoryPerc:          float64(memUsage) / float64(memLimit) * 100,
	}, nil
}
