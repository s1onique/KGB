// client.go — Docker Engine API client wrapper
//
// Uses Docker Engine Go client directly; no os/exec docker commands.
// Provides container lifecycle, network management, and inspection.
//
// Reference: kgb://doctrine/native-owned-critical-paths
//
// CORRECTION30: Presence-aware control protocol with strict validation.

package dockerlab

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// Client wraps the Docker Engine API client.
type Client struct {
	*client.Client
}

// NewClient creates a new Docker client using the default socket.
func NewClient(ctx context.Context) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("docker ping: %w", err)
	}
	return &Client{Client: cli}, nil
}

// ClientWithRuntime wraps a DockerRuntime for the qualified execution path.
// CORRECTION06 seam used by the qualified memory-lab execution path.
type ClientWithRuntime struct {
	runtime DockerRuntime
}

// NewClientWithRuntime creates a ClientWithRuntime backed by the given DockerRuntime.
func NewClientWithRuntime(runtime DockerRuntime) *ClientWithRuntime {
	return &ClientWithRuntime{runtime: runtime}
}

// ResolveImageIdentity uses the injected runtime to resolve a local image reference
// to its exact canonical ID. Returns ErrImageNotFound when the image is absent.
func (c *ClientWithRuntime) ResolveImageIdentity(ctx context.Context, reference string) (string, error) {
	inspect, _, err := c.runtime.ImageInspectWithRaw(ctx, reference)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s", ErrImageNotFound, reference)
		}
		return "", fmt.Errorf("inspect local image %q: %w", reference, err)
	}
	if err := ValidateExactImageID(inspect.ID); err != nil {
		return "", fmt.Errorf("resolved image %q has invalid ID: %w", reference, err)
	}
	return inspect.ID, nil
}

// ContainerCreateWithImageID validates and creates a container via the injected runtime.
func (c *ClientWithRuntime) ContainerCreateWithImageID(ctx context.Context, cfg ContainerConfig) (string, error) {
	if cfg.Config == nil {
		return "", ErrMissingContainerConfig
	}
	if cfg.Config.Image == "" {
		return "", ErrEmptyImageID
	}
	if err := ValidateExactImageID(cfg.Config.Image); err != nil {
		return "", fmt.Errorf("image ID validation for container create: %w", err)
	}
	resources := container.Resources{
		Memory:   cfg.MemoryLimit,
		CPUQuota: cfg.CPUQuota,
	}
	hostCfg := container.HostConfig{Resources: resources}
	resp, err := c.runtime.ContainerCreate(ctx, cfg.Config, &hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("create container with exact image ID: %w", err)
	}
	return resp.ID, nil
}

// InspectContainer returns the container inspection from the injected runtime.
func (c *ClientWithRuntime) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return c.runtime.ContainerInspect(ctx, containerID)
}

// StartContainer starts a container via the injected runtime.
func (c *ClientWithRuntime) StartContainer(ctx context.Context, containerID string) error {
	return c.runtime.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

// ConnectNetwork connects a container to a network via the injected runtime.
func (c *ClientWithRuntime) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	return c.runtime.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{})
}

// RemoveContainer removes a container via the injected runtime.
func (c *ClientWithRuntime) RemoveContainer(ctx context.Context, containerID string) error {
	return c.runtime.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
}

// RemoveNetwork removes a network via the injected runtime.
func (c *ClientWithRuntime) RemoveNetwork(ctx context.Context, networkID string) error {
	return c.runtime.NetworkRemove(ctx, networkID)
}

func (c *Client) Info(ctx context.Context) (types.Info, error) {
	return c.Client.Info(ctx)
}

func (c *Client) ServerVersion(ctx context.Context) (types.Version, error) {
	return c.Client.ServerVersion(ctx)
}

func (c *Client) ClientVersion() string {
	return c.Client.ClientVersion()
}

// ErrImageNotFound is returned when the image is not present locally.
var ErrImageNotFound = fmt.Errorf("image not found locally")

// ErrEmptyImageID is returned when the image ID is empty.
var ErrEmptyImageID = fmt.Errorf("image ID is empty")

// canonicalImageIDPattern matches the only accepted image ID form.
var canonicalImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ValidateExactImageID validates that the given string is a canonical image ID.
func ValidateExactImageID(imageID string) error {
	if imageID == "" {
		return ErrEmptyImageID
	}
	if !canonicalImageIDPattern.MatchString(imageID) {
		if strings.HasPrefix(imageID, "sha256:") {
			suffix := strings.TrimPrefix(imageID, "sha256:")
			if len(suffix) != 64 {
				return fmt.Errorf("sha256 suffix must be exactly 64 hex chars, got %d", len(suffix))
			}
			if suffix != strings.ToLower(suffix) {
				return fmt.Errorf("sha256 suffix must be lowercase hexadecimal")
			}
			return fmt.Errorf("sha256 suffix contains non-hex characters")
		}
		if strings.Contains(imageID, ":") && !strings.HasPrefix(imageID, "sha256:") {
			return fmt.Errorf("image ID must use sha256: prefix, not tag format")
		}
		return fmt.Errorf("image ID must match sha256:<64-lowercase-hex>")
	}
	return nil
}

// ResolveImageIdentity resolves a descriptive reference to its exact canonical
// image ID using ONLY local inspection via ImageInspectWithRaw.
func (c *Client) ResolveImageIdentity(ctx context.Context, reference string) (string, error) {
	inspect, _, err := c.Client.ImageInspectWithRaw(ctx, reference)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s", ErrImageNotFound, reference)
		}
		return "", fmt.Errorf("inspect local image %q: %w", reference, err)
	}
	if err := ValidateExactImageID(inspect.ID); err != nil {
		return "", fmt.Errorf("resolved image %q has invalid ID: %w", reference, err)
	}
	return inspect.ID, nil
}

// ImagePull pulls an image if not present.
func (c *Client) ImagePull(ctx context.Context, refStr string) (string, error) {
	args := filters.NewArgs()
	args.Add("reference", refStr)
	images, err := c.Client.ImageList(ctx, types.ImageListOptions{Filters: args})
	if err != nil {
		return "", fmt.Errorf("list images: %w", err)
	}
	if len(images) > 0 {
		return images[0].ID, nil
	}
	out, err := c.Client.ImagePull(ctx, refStr, types.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("pull image %s: %w", refStr, err)
	}
	defer out.Close()
	buf := make([]byte, 4096)
	for {
		_, err := out.Read(buf)
		if err != nil {
			break
		}
	}
	args = filters.NewArgs()
	args.Add("reference", refStr)
	images, err = c.Client.ImageList(ctx, types.ImageListOptions{Filters: args})
	if err != nil || len(images) == 0 {
		return "", fmt.Errorf("image not found after pull: %s", refStr)
	}
	return images[0].ID, nil
}

// ImageRepoDigests returns the repository digests for a referenced image.
func (c *Client) ImageRepoDigests(ctx context.Context, refStr string) ([]string, error) {
	args := filters.NewArgs()
	args.Add("reference", refStr)
	images, err := c.Client.ImageList(ctx, types.ImageListOptions{Filters: args})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("image not found: %s", refStr)
	}
	return images[0].RepoDigests, nil
}

// ImageLabels returns the labels for a specific image ID.
func (c *Client) ImageLabels(ctx context.Context, imageID string) (map[string]string, error) {
	inspect, _, err := c.Client.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, fmt.Errorf("inspect image: %w", err)
	}
	return inspect.Config.Labels, nil
}

// ContainerImageID returns the image ID for a running container.
func (c *Client) ContainerImageID(ctx context.Context, containerID string) (string, error) {
	inspect, err := c.Client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}
	return inspect.Image, nil
}

// ContainerCreate creates a new container.
func (c *Client) ContainerCreate(ctx context.Context, cfg ContainerConfig) (string, error) {
	resources := container.Resources{
		Memory:   cfg.MemoryLimit,
		CPUQuota: cfg.CPUQuota,
	}
	hostCfg := container.HostConfig{Resources: resources}
	resp, err := c.Client.ContainerCreate(ctx, cfg.Config, &hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

// ErrMissingContainerConfig is returned when the container config is nil.
var ErrMissingContainerConfig = fmt.Errorf("container config is required for exact image ID creation")

// ContainerCreateWithImageID creates a container using the exact image ID.
func (c *Client) ContainerCreateWithImageID(ctx context.Context, cfg ContainerConfig) (string, error) {
	if cfg.Config == nil {
		return "", ErrMissingContainerConfig
	}
	if cfg.Config.Image == "" {
		return "", ErrEmptyImageID
	}
	if err := ValidateExactImageID(cfg.Config.Image); err != nil {
		return "", fmt.Errorf("image ID validation for container create: %w", err)
	}
	resources := container.Resources{
		Memory:   cfg.MemoryLimit,
		CPUQuota: cfg.CPUQuota,
	}
	hostCfg := container.HostConfig{Resources: resources}
	resp, err := c.Client.ContainerCreate(ctx, cfg.Config, &hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("create container with exact image ID: %w", err)
	}
	return resp.ID, nil
}

func (c *Client) ContainerStart(ctx context.Context, containerID string) error {
	return c.Client.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

func (c *Client) ContainerStop(ctx context.Context, containerID string, timeout time.Duration) error {
	timeoutSec := int(timeout.Seconds())
	return c.Client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSec})
}

func (c *Client) ContainerRemove(ctx context.Context, containerID string, force bool) error {
	return c.Client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: force})
}

func (c *Client) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return c.Client.ContainerInspect(ctx, containerID)
}

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

// ContainerExec creates an exec instance in a running container
// and returns (exit_code, output, error).
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
	resp, err := c.Client.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return -1, "", fmt.Errorf("attach exec: %w", err)
	}
	defer resp.Close()
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
	info, err := c.Client.ContainerExecInspect(ctx, execID)
	if err != nil {
		return -1, string(output), fmt.Errorf("inspect exec: %w", err)
	}
	return info.ExitCode, string(output), nil
}

// ContainerExtractFile extracts a single file from a running container.
func (c *Client) ContainerExtractFile(ctx context.Context, containerID, path string) ([]byte, error) {
	rc, _, err := c.Client.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("no file in tar stream for %s", path)
		}
		if err != nil {
			return nil, fmt.Errorf("tar next: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read file: %w", err)
			}
			return data, nil
		}
	}
}

// ContainerCreateReadOnly creates a read-only container.
func (c *Client) ContainerCreateReadOnly(ctx context.Context, imageID string) (string, error) {
	resp, err := c.Client.ContainerCreate(ctx,
		&container.Config{Image: imageID, Cmd: []string{"/bin/sh"}},
		&container.HostConfig{AutoRemove: false},
		nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("create read-only container: %w", err)
	}
	return resp.ID, nil
}

func (c *Client) NetworkCreate(ctx context.Context, name string, driver string) (string, error) {
	resp, err := c.Client.NetworkCreate(ctx, name, types.NetworkCreate{
		Driver:     driver,
		Labels:     map[string]string{"kgb.dev/lab": "tovarisch-memory"},
		Attachable: true,
	})
	if err != nil {
		return "", fmt.Errorf("create network: %w", err)
	}
	return resp.ID, nil
}

func (c *Client) NetworkConnect(ctx context.Context, networkID, containerID string) error {
	return c.Client.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{})
}

func (c *Client) NetworkDisconnect(ctx context.Context, networkID, containerID string) error {
	return c.Client.NetworkDisconnect(ctx, networkID, containerID, true)
}

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

func NewCleanupManager(client *Client, runID string) *CleanupManager {
	return &CleanupManager{client: client, runID: runID}
}

func (cm *CleanupManager) RegisterNetwork(networkID string) {
	cm.networks = append(cm.networks, networkID)
}

func (cm *CleanupManager) RegisterContainer(containerID string) {
	cm.containers = append(cm.containers, containerID)
}

func (cm *CleanupManager) Cleanup(ctx context.Context) error {
	var lastErr error
	for _, id := range cm.containers {
		if err := cm.client.ContainerRemove(ctx, id, true); err != nil {
			lastErr = err
		}
	}
	for _, id := range cm.networks {
		if err := cm.client.NetworkRemove(ctx, id); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

type ContainerRunner struct {
	client *Client
}

func NewContainerRunner(client *Client) *ContainerRunner {
	return &ContainerRunner{client: client}
}

func (cr *ContainerRunner) RunOnce(ctx context.Context, cfg ContainerConfig) (*types.ContainerJSON, error) {
	if _, err := cr.client.ImagePull(ctx, cfg.Config.Image); err != nil {
		return nil, fmt.Errorf("pull image: %w", err)
	}
	id, err := cr.client.ContainerCreate(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	if err := cr.client.ContainerStart(ctx, id); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	inspect, err := cr.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}
	return &inspect, nil
}

func (cr *ContainerRunner) WaitForPort(ctx context.Context, containerID string, port string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, _, err := cr.client.ContainerExec(ctx, containerID, []string{"curl", "-s", "localhost:" + port})
		if err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %s", port)
}

type ContainerConfig struct {
	Name        string
	Config      *container.Config
	MemoryLimit int64
	CPUQuota    int64
	Networks    []string
	AutoRemove  bool
}

func NewContainerConfig(name, image string) *ContainerConfig {
	return &ContainerConfig{
		Name: name,
		Config: &container.Config{
			Image: image,
		},
	}
}

func (c *ContainerConfig) WithMemory(bytes int64) *ContainerConfig {
	c.MemoryLimit = bytes
	return c
}

func (c *ContainerConfig) WithCPU(quota int64) *ContainerConfig {
	c.CPUQuota = quota
	return c
}

func (c *ContainerConfig) WithNetworks(networks ...string) *ContainerConfig {
	c.Networks = networks
	return c
}

func (c *ContainerConfig) WithAutoRemove() *ContainerConfig {
	c.AutoRemove = true
	return c
}

type ImageBuilder struct {
	client *Client
}

func NewImageBuilder(client *Client) *ImageBuilder {
	return &ImageBuilder{client: client}
}

// BuildFromDockerfile builds an image from a Dockerfile.
func (ib *ImageBuilder) BuildFromDockerfile(ctx context.Context, dockerfilePath string, tag string) (string, string, error) {
	tarReader, err := createTarContext(dockerfilePath)
	if err != nil {
		return "", "", fmt.Errorf("create tar context: %w", err)
	}
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
	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read build output: %w", err)
	}
	imageID := extractImageID(string(output))
	return imageID, "", nil
}

func createTarContext(dockerfilePath string) (io.Reader, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()
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

func extractImageID(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.Contains(line, `"stream"`) && strings.Contains(line, `Successfully built`) {
			parts := strings.Split(line, " ")
			for _, p := range parts {
				if len(p) == 64 {
					return p
				}
			}
		}
	}
	return ""
}

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

func (c *Client) ContainerIP(ctx context.Context, containerID string, networkName string) (string, error) {
	inspect, err := c.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}
	if inspect.NetworkSettings == nil {
		return "", fmt.Errorf("no network settings")
	}
	if net, ok := inspect.NetworkSettings.Networks[networkName]; ok {
		if net.IPAddress != "" {
			return net.IPAddress, nil
		}
	}
	fullName := "network:" + networkName
	if net, ok := inspect.NetworkSettings.Networks[fullName]; ok {
		if net.IPAddress != "" {
			return net.IPAddress, nil
		}
	}
	return "", fmt.Errorf("no IP address found for network %s", networkName)
}

type ContainerStats struct {
	MemoryUsageBytes    int64
	MemoryLimitBytes    int64
	CPUUsageNanoSeconds uint64
	MemoryPerc          float64
}

func (c *Client) ContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	reader, err := c.Client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	defer reader.Body.Close()
	var stats types.StatsJSON
	if err := json.NewDecoder(reader.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	memUsage := int64(stats.MemoryStats.Usage)
	memLimit := int64(stats.MemoryStats.Limit)
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

// CanaryControlExecResult represents the result of a canary-control exec operation.
// CORRECTION30: Typed protocol errors with presence-aware validation.
type CanaryControlExecResult struct {
	ExitCode      int
	Stdout        string
	Stderr        string
	HealthValid   bool
	StateValid    bool
	WorkloadValid bool
	State         *CanaryStateFromExec
	Attempted     int
	Completed     int
	Error         error
}

// CanaryHealthCheckViaExec performs a health check on a canary container
// using the image-owned canary-control binary via docker exec.
func (c *Client) CanaryHealthCheckViaExec(ctx context.Context, containerID string, port int, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		default:
		}
		// CORRECTION30 P0-7: Typed methods own argv construction
		result := c.canaryControlExec(ctx, containerID, "health", []string{
			"/app/canary", "control", "health",
			"--port", fmt.Sprintf("%d", port),
			"--timeout", "5s",
		})
		// CORRECTION30 P0-6: Protocol errors are typed, stop immediately
		if result.Error != nil {
			return -1, result.Error
		}
		if result.ExitCode == 0 && result.HealthValid {
			return 0, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return -1, fmt.Errorf("timeout waiting for canary health via docker exec")
}

// CanaryStateViaExec fetches canary state using the image-owned canary-control binary.
func (c *Client) CanaryStateViaExec(ctx context.Context, containerID string, port int) (*CanaryStateFromExec, error) {
	result := c.canaryControlExec(ctx, containerID, "state", []string{
		"/app/canary", "control", "state",
		"--port", fmt.Sprintf("%d", port),
		"--timeout", "5s",
	})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("state check failed: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if result.State == nil {
		return nil, fmt.Errorf("state check succeeded but state is nil")
	}
	return result.State, nil
}

// CanaryOperateViaExec performs N operations using the image-owned canary-control binary.
func (c *Client) CanaryOperateViaExec(ctx context.Context, containerID string, port int, count int, timeout time.Duration) (*CanaryWorkloadResult, error) {
	result := c.canaryControlExec(ctx, containerID, "operate", []string{
		"/app/canary", "control", "operate",
		"--port", fmt.Sprintf("%d", port),
		"--count", fmt.Sprintf("%d", count),
		"--timeout", fmt.Sprintf("%ds", int(timeout.Seconds())),
	})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("operate failed: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if !result.WorkloadValid {
		return nil, fmt.Errorf("operate response validation failed")
	}
	return &CanaryWorkloadResult{
		Attempted: result.Attempted,
		Completed: result.Completed,
	}, nil
}

// CanaryWorkloadResult represents the result of an operate command.
type CanaryWorkloadResult struct {
	Attempted int `json:"attempted"`
	Completed int `json:"completed"`
}

// canaryControlExec runs the canary-control binary inside the container.
// CORRECTION30 P0-7: Private method with validated argv.
// CORRECTION30 P0-6: Protocol failures populate typed errors.
func (c *Client) canaryControlExec(
	ctx context.Context,
	containerID string,
	expectedOperation string,
	argv []string,
) CanaryControlExecResult {
	result := CanaryControlExecResult{ExitCode: -1}

	// CORRECTION30 P0-8: Validate exact argv before execution
	if len(argv) < 3 {
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "invalid_arguments",
			Message:   "argv too short",
		}
		result.Stderr = "argv too short"
		return result
	}
	if argv[0] != "/app/canary" {
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "invalid_arguments",
			Message:   "must start with /app/canary",
		}
		result.Stderr = "must start with /app/canary"
		return result
	}
	if argv[1] != "control" {
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "invalid_arguments",
			Message:   "must be control subcommand",
		}
		result.Stderr = "must be control subcommand"
		return result
	}
	if argv[2] != expectedOperation {
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "invalid_arguments",
			Message:   "operation mismatch",
		}
		result.Stderr = fmt.Sprintf("operation mismatch: expected %s, got %s", expectedOperation, argv[2])
		return result
	}

	// Use ContainerExec to run the exact command
	exitCode, stdout, err := c.ContainerExec(ctx, containerID, argv)
	result.ExitCode = exitCode
	result.Stdout = stdout
	result.Error = err

	// Handle exec errors
	if err != nil {
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "connection_failed",
			Message:   err.Error(),
		}
		result.Stderr = err.Error()
		return result
	}

	// Parse canonical envelope - must be exactly one JSON document
	if stdout == "" {
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "malformed_json",
			Message:   "empty stdout",
		}
		result.Stderr = "empty stdout"
		return result
	}

	// CORRECTION30 P0-1: Strict decoding with correct trailing rejection
	envelope, parseErr := strictParseEnvelope(stdout)
	if parseErr != nil {
		var parseErrTyped *ParseError
		if errors.As(parseErr, &parseErrTyped) {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  parseErrTyped.ErrClass,
				Message:   parseErrTyped.Message,
			}
			result.Stderr = parseErrTyped.Message
		} else {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "malformed_json",
				Message:   parseErr.Error(),
			}
			result.Stderr = parseErr.Error()
		}
		return result
	}

	// CORRECTION30 P0-4: Validate envelope consistency
	validErr := validateControlEnvelope(envelope, expectedOperation, exitCode)
	if validErr != nil {
		var validErrTyped *ParseError
		if errors.As(validErr, &validErrTyped) {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  validErrTyped.ErrClass,
				Message:   validErrTyped.Message,
			}
			result.Stderr = validErrTyped.Message
		} else {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "malformed_json",
				Message:   validErr.Error(),
			}
			result.Stderr = validErr.Error()
		}
		return result
	}

	// Handle failure envelopes - CORRECTION30 P0-4
	if !envelope.Success {
		result.Error = &ProtocolError{
			Operation:    expectedOperation,
			ErrClass:     string(envelope.ErrorClass),
			HTTPStatus:   envelope.HTTPStatus,
			ExecExitCode: exitCode,
			Message:      fmt.Sprintf("control error: %s", envelope.ErrorClass),
		}
		result.Stderr = fmt.Sprintf("control error: %s", envelope.ErrorClass)
		return result
	}

	// Validate operation-specific payload
	switch expectedOperation {
	case "health":
		if envelope.Health == nil {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "missing_required_field",
				Message:   "health operation missing health payload",
			}
			result.Stderr = "health operation missing health payload"
			return result
		}
		if !envelope.Health.Ready {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "health_not_ready",
				Message:   "health not ready",
			}
			result.Stderr = "health not ready"
			return result
		}
		result.HealthValid = true

	case "state":
		if envelope.State == nil {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "missing_required_field",
				Message:   "state operation missing state payload",
			}
			result.Stderr = "state operation missing state payload"
			return result
		}
		if envelope.State.Mode == "" {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "missing_required_field",
				Message:   "state missing mode",
			}
			result.Stderr = "state missing mode"
			return result
		}
		result.StateValid = true
		result.State = &CanaryStateFromExec{
			Mode:           envelope.State.Mode,
			RetainedBlocks: envelope.State.RetainedBlocks,
			RetainedBytes:  envelope.State.RetainedBytes,
			OperationCount: envelope.State.OperationCount,
			FDCount:        envelope.State.FDCount,
			Ready:          envelope.State.Ready,
		}

	case "operate":
		if envelope.Workload == nil {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "missing_required_field",
				Message:   "operate operation missing workload payload",
			}
			result.Stderr = "operate operation missing workload payload"
			return result
		}
		if envelope.Workload.Completed > envelope.Workload.Attempted {
			result.Error = &ProtocolError{
				Operation: expectedOperation,
				ErrClass:  "workload_count_mismatch",
				Message:   "completed exceeds attempted",
			}
			result.Stderr = "completed exceeds attempted"
			return result
		}
		result.WorkloadValid = true
		result.Attempted = envelope.Workload.Attempted
		result.Completed = envelope.Workload.Completed

	default:
		result.Error = &ProtocolError{
			Operation: expectedOperation,
			ErrClass:  "invalid_arguments",
			Message:   fmt.Sprintf("unknown operation: %s", expectedOperation),
		}
		result.Stderr = fmt.Sprintf("unknown operation: %s", expectedOperation)
		return result
	}

	return result
}

// ProtocolError represents a typed protocol error.
// CORRECTION30 P0-6: Typed errors for proper error propagation.
type ProtocolError struct {
	Operation    string
	ErrClass     string
	HTTPStatus   int
	ExecExitCode int
	Message      string
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error [%s]: %s: %s", e.Operation, e.ErrClass, e.Message)
}

// IsProtocolNonRetryable returns true if this error should not be retried.
func IsProtocolNonRetryable(err error) bool {
	var protoErr *ProtocolError
	if errors.As(err, &protoErr) {
		switch protoErr.ErrClass {
		case "invalid_arguments", "malformed_json", "unknown_json_field",
			"missing_required_field", "trailing_json", "connection_failed":
			return false // These are retryable
		case "request_timeout", "unexpected_http_status", "health_not_ready",
			"state_invalid", "workload_count_mismatch":
			return true // These are non-retryable
		}
	}
	return false
}

// ControlEnvelope is the canonical protocol envelope.
type ControlEnvelope struct {
	SchemaVersion string           `json:"schema_version"`
	Operation     string           `json:"operation"`
	Success       bool             `json:"success"`
	HTTPStatus    int              `json:"http_status"`
	Health        *HealthPayload   `json:"health,omitempty"`
	State         *StatePayload    `json:"state,omitempty"`
	Workload      *WorkloadPayload `json:"workload,omitempty"`
	ErrorClass    string           `json:"error_class,omitempty"`
}

// HealthPayload represents health check result.
type HealthPayload struct {
	Ready bool   `json:"ready"`
	Mode  string `json:"mode,omitempty"`
}

// StatePayload represents state result.
type StatePayload struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int64  `json:"operation_count"`
	FDCount        int    `json:"fd_count"`
	Ready          bool   `json:"ready"`
}

// WorkloadPayload represents operate result.
type WorkloadPayload struct {
	Requested int `json:"requested"`
	Attempted int `json:"attempted"`
	Completed int `json:"completed"`
}

// ParseError represents a parsing error with classification.
type ParseError struct {
	ErrClass string
	Message  string
}

func (e *ParseError) Error() string {
	return e.Message
}

// strictParseEnvelope parses exactly one JSON envelope from stdout.
// CORRECTION30 P0-1: Correct trailing rejection - requires io.EOF.
func strictParseEnvelope(stdout string) (*ControlEnvelope, error) {
	if stdout == "" {
		return nil, &ParseError{ErrClass: "malformed_json", Message: "empty stdout"}
	}

	// First pass: check required envelope fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return nil, &ParseError{ErrClass: "malformed_json", Message: "invalid JSON"}
	}

	// Check required envelope fields are present
	requiredEnvelopeFields := []string{"schema_version", "operation", "success", "http_status"}
	for _, field := range requiredEnvelopeFields {
		if _, ok := raw[field]; !ok {
			return nil, &ParseError{ErrClass: "missing_required_field", Message: fmt.Sprintf("missing %s", field)}
		}
	}

	// Second pass: strict decode with DisallowUnknownFields
	dec := json.NewDecoder(bytes.NewReader([]byte(stdout)))
	dec.DisallowUnknownFields()

	var env ControlEnvelope
	if err := dec.Decode(&env); err != nil {
		// Check for unknown field error by error message
		if strings.Contains(err.Error(), "unknown field") {
			return nil, &ParseError{ErrClass: "unknown_json_field", Message: err.Error()}
		}
		return nil, &ParseError{ErrClass: "malformed_json", Message: err.Error()}
	}

	// Third pass: require exactly one value - CORRECTION30 P0-1
	// The key fix: require io.EOF, not just any error
	var extra json.RawMessage
	decodeErr := dec.Decode(&extra)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		// Malformed trailing data
		return nil, &ParseError{ErrClass: "trailing_json", Message: "trailing data after envelope"}
	}
	if decodeErr == nil && len(extra) > 0 {
		// Second valid JSON value
		return nil, &ParseError{ErrClass: "trailing_json", Message: "second JSON value after envelope"}
	}

	return &env, nil
}

// validateControlEnvelope validates envelope consistency.
// CORRECTION30 P0-4: Validates success/failure variants.
func validateControlEnvelope(env *ControlEnvelope, expectedOperation string, exitCode int) error {
	// Validate schema version
	if env.SchemaVersion != "canary-control/v1" {
		return &ParseError{ErrClass: "invalid_arguments", Message: fmt.Sprintf("unexpected schema version: %s", env.SchemaVersion)}
	}

	// Validate operation matches argv
	if env.Operation != expectedOperation {
		return &ParseError{ErrClass: "invalid_arguments", Message: fmt.Sprintf("operation mismatch: expected %s, got %s", expectedOperation, env.Operation)}
	}

	// Validate success matches exit code
	if env.Success != (exitCode == 0) {
		return &ParseError{ErrClass: "invalid_arguments", Message: fmt.Sprintf("success/exit-code mismatch: success=%v, exit=%d", env.Success, exitCode)}
	}

	// CORRECTION30 P0-4: Validate envelope variant consistency
	if env.Success {
		// Success envelope must not contain error_class
		if env.ErrorClass != "" {
			return &ParseError{ErrClass: "invalid_arguments", Message: "success envelope must not contain error_class"}
		}
		// Success envelope must have HTTP 200
		if env.HTTPStatus != 200 {
			return &ParseError{ErrClass: "unexpected_http_status", Message: fmt.Sprintf("success envelope must have HTTP 200, got %d", env.HTTPStatus)}
		}
	} else {
		// Failure envelope must contain non-empty error_class
		if env.ErrorClass == "" {
			return &ParseError{ErrClass: "missing_required_field", Message: "failure envelope must contain error_class"}
		}
	}

	return nil
}

// CanaryStateFromExec represents the canary state when fetched via exec.
type CanaryStateFromExec struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int64  `json:"operation_count"`
	FDCount        int    `json:"fd_count"`
	Ready          bool   `json:"ready"`
}
