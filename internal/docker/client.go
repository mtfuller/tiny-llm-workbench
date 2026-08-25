// Package docker wraps the Docker Engine API (via the official Docker SDK
// for Go) for launching, stopping, and exec'ing into Environment containers.
package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// ManagedLabel marks every container tlw creates, so ListManaged can find
// them again (e.g. after a `tlw serve` restart) without keeping its own
// separate bookkeeping — the container itself is the source of truth.
const ManagedLabel = "tlw.managed"

// EnvironmentLabel records which registry.Environment definition a
// container was launched from.
const EnvironmentLabel = "tlw.environment"

// execStopTimeoutSeconds bounds how long Stop waits for a graceful
// container shutdown before the container is force-removed anyway.
const execStopTimeoutSeconds = 5

// Client talks to the local Docker daemon.
type Client struct {
	cli *client.Client
}

// New builds a Client using the same connection Docker's own CLI would use:
// $DOCKER_HOST if set, otherwise Docker Desktop's default socket on macOS
// (the Docker Go SDK's client.FromEnv doesn't know about Docker Desktop's
// context system and would otherwise default to /var/run/docker.sock, which
// Docker Desktop doesn't use), otherwise the SDK's own built-in default.
func New() (*Client, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if os.Getenv("DOCKER_HOST") == "" {
		if host, ok := dockerDesktopHost(); ok {
			opts = append(opts, client.WithHost(host))
		}
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// dockerDesktopHost returns Docker Desktop's default socket path if it
// exists on disk, so New() can connect without requiring $DOCKER_HOST to be
// set (which the `docker` CLI itself doesn't require either, since it
// resolves the socket from its own context config instead).
func dockerDesktopHost() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	sock := filepath.Join(home, ".docker", "run", "docker.sock")
	if _, err := os.Stat(sock); err != nil {
		return "", false
	}

	return "unix://" + sock, true
}

// Ping checks that the Docker daemon is reachable, returning a clear error
// if it isn't (e.g. Docker Desktop not running) rather than letting later
// calls fail with an opaque connection error.
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.cli.Ping(ctx); err != nil {
		return fmt.Errorf("docker daemon unreachable (is Docker running?): %w", err)
	}
	return nil
}

// Mount is a host↔container bind mount.
type Mount struct {
	HostPath      string
	ContainerPath string
}

// pullIfMissing pulls image if it isn't already present locally.
func (c *Client) pullIfMissing(ctx context.Context, image string) error {
	if _, _, err := c.cli.ImageInspectWithRaw(ctx, image); err == nil {
		return nil // already present
	}

	reader, err := c.cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}
	defer reader.Close()

	// Pull progress is newline-delimited JSON; we only need to wait for it
	// to finish; discard the body.
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}

	return nil
}

// Launch creates and starts a new container from image, labeled so it's
// recognized by ListManaged, and returns its container ID (used as the
// Environment instance's ID). image is pulled first if not already present
// locally — unlike the `docker` CLI, the raw container-create API doesn't
// do this automatically.
func (c *Client) Launch(ctx context.Context, name, environmentName, image string, mounts []Mount) (string, error) {
	if err := c.pullIfMissing(ctx, image); err != nil {
		return "", err
	}

	mountSpecs := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		mountSpecs = append(mountSpecs, mount.Mount{
			Type:   mount.TypeBind,
			Source: m.HostPath,
			Target: m.ContainerPath,
		})
	}

	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Tty:   false,
		// Keeps the container alive with no foreground process of its own
		// so it's ready to exec into; every image we launch (alpine,
		// debian-slim, python-slim, curlimages/curl) provides `sleep`.
		Cmd: []string{"sleep", "infinity"},
		Labels: map[string]string{
			ManagedLabel:     "true",
			EnvironmentLabel: environmentName,
		},
	}, &container.HostConfig{
		Mounts: mountSpecs,
	}, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	return resp.ID, nil
}

// Stop stops and removes containerID.
func (c *Client) Stop(ctx context.Context, containerID string) error {
	timeout := execStopTimeoutSeconds
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	if err := c.cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

// ContainerInfo is a tlw-managed container's current state.
type ContainerInfo struct {
	ID              string
	Name            string
	Image           string
	State           string // "running", "exited", ...
	EnvironmentName string
	CreatedAt       time.Time
}

// ListManaged returns every container tlw has launched (running or not),
// identified by ManagedLabel.
func (c *Client) ListManaged(ctx context.Context) ([]ContainerInfo, error) {
	listFilters := filters.NewArgs(filters.Arg("label", ManagedLabel+"=true"))
	containers, err := c.cli.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: listFilters})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	infos := make([]ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}
		infos = append(infos, ContainerInfo{
			ID:              ctr.ID,
			Name:            name,
			Image:           ctr.Image,
			State:           ctr.State,
			EnvironmentName: ctr.Labels[EnvironmentLabel],
			CreatedAt:       time.Unix(ctr.Created, 0).UTC(),
		})
	}

	return infos, nil
}

// chunkWriter adapts a callback to an io.Writer, so stdcopy.StdCopy can
// stream demuxed exec output straight to onChunk one write at a time.
type chunkWriter func(chunk string)

func (w chunkWriter) Write(p []byte) (int, error) {
	w(string(p))
	return len(p), nil
}

// ExecStream runs cmd inside containerID, streaming combined stdout/stderr
// to onOutput as it arrives, and returns the command's exit code once it
// finishes.
func (c *Client) ExecStream(ctx context.Context, containerID string, cmd []string, onOutput func(chunk string)) (int, error) {
	created, err := c.cli.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return 0, fmt.Errorf("create exec: %w", err)
	}

	attach, err := c.cli.ContainerExecAttach(ctx, created.ID, types.ExecStartCheck{})
	if err != nil {
		return 0, fmt.Errorf("attach exec: %w", err)
	}
	defer attach.Close()

	sink := chunkWriter(onOutput)
	if _, err := stdcopy.StdCopy(sink, sink, attach.Reader); err != nil {
		return 0, fmt.Errorf("read exec output: %w", err)
	}

	inspect, err := c.cli.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return 0, fmt.Errorf("inspect exec: %w", err)
	}

	return inspect.ExitCode, nil
}
