// client.go — Docker Engine API client wrapper
//
// Uses Docker Engine Go client directly; no os/exec docker commands.
// Provides container lifecycle, network management, and inspection.
//
// Reference: kgb://doctrine/native-owned-critical-paths
//
// CORRECTION02: ImageRepoDigests / ImageLabels / ContainerImageID
// provide canary image provenance (image ID, repository digests,
// OCI labels).

package dockerlab

import (
	"archive/tar"
	"context"
	"encoding/json"
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
// Allows tests to inject a recording fake. This is the CORRECTION06 seam
// used by the qualified memory-lab execution path.
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
// The exact supplied ID reaches Docker exactly once when validation passes.
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
// Returns nil if valid, ErrEmptyImageID if empty, or error otherwise.
func ValidateExactImageID(imageID string) error {
	if imageID == "" {
		return ErrEmptyImageID
	}

	// Check canonical form using the pre-compiled regex
	if !canonicalImageIDPattern.MatchString(imageID) {
		// Provide helpful error messages for common issues
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
// image ID using ONLY local inspection via ImageInspectWithRaw. This is the
// P0-10 canonical approach:
// - locally present image: resolve full canonical ID
// - locally absent image: fail closed (ErrImageNotFound)
// - registry access: never attempted
// - automatic pull: never attempted
// - fallback reference: never attempted
func (c *Client) ResolveImageIdentity(ctx context.Context, reference string) (string, error) {
	// Use direct image inspection - single reference, no list
	inspect, _, err := c.Client.ImageInspectWithRaw(ctx, reference)
	if err != nil {
		// Use structured Docker not-found classification
		if errdefs.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s", ErrImageNotFound, reference)
		}
		// Daemon failures are preserved, not misclassified
		return "", fmt.Errorf("inspect local image %q: %w", reference, err)
	}

	// Validate the returned ID is canonical
	if err := ValidateExactImageID(inspect.ID); err != nil {
		return "", fmt.Errorf("resolved image %q has invalid ID: %w", reference, err)
	}

	return inspect.ID, nil
}

// ImagePull pulls an image if not present.
// P0-10: This is retained ONLY for explicitly pull-owning workflows.
// Qualified execution paths MUST use ResolveImageIdentity instead.
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

// ImageRepoDigests returns the repository digests for a referenced
// image. CORRECTION02 §7. Returns an empty slice when the image
// has no repository digests (typical for a local-only image built
// from a Dockerfile without tagging a remote registry).
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
// CORRECTION02 §7: OCI labels (org.opencontainers.image.revision
// and kgb.dev/* labels) bind the canary image to the source tree
// and binary identity.
func (c *Client) ImageLabels(ctx context.Context, imageID string) (map[string]string, error) {
	inspect, _, err := c.Client.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, fmt.Errorf("inspect image: %w", err)
	}
	return inspect.Config.Labels, nil
}

// ContainerImageID returns the image ID for a running container.
// CORRECTION02 §7: used to verify the container inspect's image
// matches the verified image ID.
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
// This is the P0-10 canonical approach: use the full immutable image ID
// instead of a mutable tag, which guarantees no pull behavior.
//
// REQUIRED: The image ID must be in canonical sha256:<64-hex> format.
// This method fails closed on any validation error before calling Docker.
func (c *Client) ContainerCreateWithImageID(ctx context.Context, cfg ContainerConfig) (string, error) {
	// Phase 2: Fail closed on nil config
	if cfg.Config == nil {
		return "", ErrMissingContainerConfig
	}

	// Phase 2: Fail closed on empty image
	if cfg.Config.Image == "" {
		return "", ErrEmptyImageID
	}

	// Phase 2: Fail closed on invalid canonical image ID
	// This rejects tags, short IDs, uppercase, and malformed IDs
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
// and returns (exit_code, output, error). Used by the canary
// binary hash verification path (CORRECTION02 §7).
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

// ContainerExtractFile extracts a single file from a running
// (or stopped) container via `docker cp` and returns its
// contents as a byte slice. Used by CORRECTION03 to hash
// /app/canary inside the built image.
func (c *Client) ContainerExtractFile(ctx context.Context, containerID, path string) ([]byte, error) {
	rc, _, err := c.Client.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}
	defer rc.Close()
	// Untar: the docker cp protocol emits a tar stream
	// containing the file. We extract the first regular file.
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

// ContainerCreateReadOnly creates a read-only container from
// the given image so the binary can be extracted without
// running the canary. Returns the container ID.
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
// Returns image ID and binary hash.
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

// CanaryHealthCheckViaExec performs a health check on a canary container
// using docker exec instead of direct HTTP. This addresses CORRECTION27:
// Docker bridge networking issues can prevent direct IP access, but
// `docker exec` always works because it uses the container's own network
// namespace.
func (c *Client) CanaryHealthCheckViaExec(ctx context.Context, containerID string, port int, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		default:
		}
		// Use curl from inside the container to hit localhost:port
		exitCode, _, err := c.ContainerExec(ctx, containerID, []string{
			"sh", "-c",
			fmt.Sprintf("curl -sf http://localhost:%d/health || curl -sf http://127.0.0.1:%d/health || exit 1", port, port),
		})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if exitCode == 0 {
			return 0, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return -1, fmt.Errorf("timeout waiting for canary health via docker exec")
}

// CanaryStateViaExec fetches canary state using docker exec and
// the container's localhost networking. CORRECTION27.
func (c *Client) CanaryStateViaExec(ctx context.Context, containerID string, port int) (*CanaryStateFromExec, error) {
	exitCode, output, err := c.ContainerExec(ctx, containerID, []string{
		"sh", "-c",
		fmt.Sprintf("curl -sf http://localhost:%d/state || curl -sf http://127.0.0.1:%d/state", port, port),
	})
	if err != nil {
		return nil, fmt.Errorf("exec state: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("state check failed: exit code %d", exitCode)
	}
	var state CanaryStateFromExec
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		return nil, fmt.Errorf("parse state JSON: %w", err)
	}
	return &state, nil
}

// CanaryStateFromExec represents the canary state when fetched via exec.
type CanaryStateFromExec struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int64   `json:"operation_count"`
	FDCount        int     `json:"fd_count"`
	Ready          bool    `json:"ready"`
}
