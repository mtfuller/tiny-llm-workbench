package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/docker"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// dockerClient is the subset of docker.Client Manager needs.
type dockerClient interface {
	Launch(ctx context.Context, name, workspaceName, image string, mounts []docker.Mount) (string, error)
	Stop(ctx context.Context, containerID string) error
	ListManaged(ctx context.Context) ([]docker.ContainerInfo, error)
	ExecStream(ctx context.Context, containerID string, cmd []string, onOutput func(chunk string)) (int, error)
}

// workspaceReader is the subset of registry.Registry Manager needs to look
// up a Workspace when launching a sandbox for it.
type workspaceReader interface {
	GetWorkspace(name string) (registry.Workspace, error)
}

// Manager launches, stops, and execs into workspace sandbox instances.
type Manager struct {
	ctx        context.Context
	docker     dockerClient
	workspaces workspaceReader
	bus        *eventbus.Bus
	runsDir    string // where a test workspace's throwaway per-run copy is made

	mu    sync.Mutex
	execs map[string]*Exec
}

// NewManager builds a Manager. ctx bounds the lifetime of any exec it
// starts (the server's shutdown context), not any single HTTP request's
// context, since an exec continues streaming after StartExec's caller gets
// its response. runsDir is where a test workspace's files are copied per
// launch — a discoverable location under the registry root the user can
// open in an editor to watch an agent work.
func NewManager(ctx context.Context, dockerClient dockerClient, workspaces workspaceReader, bus *eventbus.Bus, runsDir string) *Manager {
	return &Manager{
		ctx:        ctx,
		docker:     dockerClient,
		workspaces: workspaces,
		bus:        bus,
		runsDir:    runsDir,
		execs:      make(map[string]*Exec),
	}
}

// Launch starts a new sandbox instance for the named Workspace. If
// instanceName is empty, one is generated. For a test workspace the
// workspace's files/ directory is copied to a fresh per-run directory under
// runsDir and *that* is mounted (so the agent's changes never touch the
// source); for a real workspace the workspace's own directory is
// bind-mounted directly (changes persist). Either way the directory lands
// at /workspace inside the container.
func (m *Manager) Launch(ctx context.Context, workspaceName, instanceName string) (Instance, error) {
	ws, err := m.workspaces.GetWorkspace(workspaceName)
	if err != nil {
		return Instance{}, fmt.Errorf("look up workspace %q: %w", workspaceName, err)
	}

	if instanceName == "" {
		instanceName = fmt.Sprintf("tlw-%s-%d", workspaceName, time.Now().UnixNano())
	}

	hostDir := ws.HostPath
	if ws.Type == registry.WorkspaceTest {
		hostDir = filepath.Join(m.runsDir, instanceName)
		if err := copyDir(ws.HostPath, hostDir); err != nil {
			return Instance{}, fmt.Errorf("stage test workspace %q: %w", workspaceName, err)
		}
	}
	if hostDir == "" {
		return Instance{}, fmt.Errorf("workspace %q has no directory configured", workspaceName)
	}

	mounts := []docker.Mount{{HostPath: hostDir, ContainerPath: docker.ContainerWorkdir}}

	containerID, err := m.docker.Launch(ctx, instanceName, workspaceName, docker.DefaultSandboxImage, mounts)
	if err != nil {
		return Instance{}, fmt.Errorf("launch sandbox for workspace %q: %w", workspaceName, err)
	}

	return Instance{
		ID:            containerID,
		Name:          instanceName,
		WorkspaceName: workspaceName,
		WorkspaceType: string(ws.Type),
		WorkspacePath: hostDir,
		Image:         docker.DefaultSandboxImage,
		State:         "running",
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// Stop stops and removes the instance with the given container ID. It does
// not delete a test workspace's staged copy under runsDir — that's left in
// place on purpose so the user can inspect what the agent did.
func (m *Manager) Stop(ctx context.Context, instanceID string) error {
	return m.docker.Stop(ctx, instanceID)
}

// ListInstances reflects Docker's live state directly — there's no
// in-process bookkeeping to fall out of sync, so this is correct even
// immediately after a `tlw serve` restart. WorkspacePath/WorkspaceType
// aren't recoverable from Docker, so a reconciled instance leaves them
// empty.
func (m *Manager) ListInstances(ctx context.Context) ([]Instance, error) {
	containers, err := m.docker.ListManaged(ctx)
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, len(containers))
	for i, c := range containers {
		instances[i] = Instance{
			ID:            c.ID,
			Name:          c.Name,
			WorkspaceName: c.WorkspaceName,
			Image:         c.Image,
			State:         c.State,
			CreatedAt:     c.CreatedAt,
		}
	}

	return instances, nil
}

// StartExec runs command (a shell command line, e.g. "pip install requests")
// inside instanceID in the background, streaming its output over the event
// bus (ExecOutputEvent/ExecStatusEvent) as it runs. It returns immediately
// with the new Exec in the "running" state — poll GetExec or listen for
// ExecStatusEvent to observe completion.
func (m *Manager) StartExec(instanceID, command string) (*Exec, error) {
	if command == "" {
		return nil, errors.New("command is required")
	}

	exec := &Exec{
		ID:         newExecID(),
		InstanceID: instanceID,
		Command:    command,
		Status:     ExecRunning,
		StartedAt:  time.Now().UTC(),
	}

	m.mu.Lock()
	m.execs[exec.ID] = exec
	m.mu.Unlock()

	m.publishExecStatus(exec)

	go m.runExec(exec)

	return exec, nil
}

// GetExec returns the exec with the given ID, if any.
func (m *Manager) GetExec(id string) (*Exec, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	exec, ok := m.execs[id]
	return exec, ok
}

// TryTool renders tool's command with args (see RenderToolCommand) and
// starts it inside instanceID in the background, streaming output the same
// way StartExec does — the Tools page's "Playground" uses this so trying a
// tool against a test workspace behaves exactly like running a plain ad hoc
// command, just with its arguments collected as a form.
func (m *Manager) TryTool(instanceID string, tool registry.Tool, args map[string]string) (*Exec, error) {
	command, err := RenderToolCommand(tool, args)
	if err != nil {
		return nil, err
	}
	return m.StartExec(instanceID, command)
}

// RunToolSync runs command inside instanceID and blocks until it finishes,
// returning its combined output. Unlike StartExec, this doesn't stream
// progress over the event bus or track an Exec — it's for callers (an
// agent's Tool node, an evaluation's Verify step) that need the result
// before they can continue, not for live observation.
func (m *Manager) RunToolSync(ctx context.Context, instanceID, command string) (string, error) {
	if command == "" {
		return "", errors.New("command is required")
	}

	var output strings.Builder
	exitCode, err := m.docker.ExecStream(ctx, instanceID, []string{"sh", "-c", command}, func(chunk string) {
		output.WriteString(chunk)
	})
	if err != nil {
		return "", fmt.Errorf("run tool: %w", err)
	}
	if exitCode != 0 {
		return output.String(), fmt.Errorf("command exited with status %d", exitCode)
	}

	return output.String(), nil
}

func (m *Manager) runExec(exec *Exec) {
	exitCode, err := m.docker.ExecStream(m.ctx, exec.InstanceID, []string{"sh", "-c", exec.Command}, func(chunk string) {
		m.mu.Lock()
		exec.Output += chunk
		m.mu.Unlock()

		m.bus.Publish(eventbus.Event{Type: ExecOutputEvent, Data: mustMarshal(struct {
			ExecID string `json:"execId"`
			Chunk  string `json:"chunk"`
		}{ExecID: exec.ID, Chunk: chunk})})
	})

	m.mu.Lock()
	now := time.Now().UTC()
	exec.FinishedAt = &now
	if err != nil {
		exec.Status = ExecFailed
		exec.Error = err.Error()
	} else {
		exec.Status = ExecDone
		exec.ExitCode = &exitCode
	}
	m.mu.Unlock()

	m.publishExecStatus(exec)
}

func (m *Manager) publishExecStatus(exec *Exec) {
	m.mu.Lock()
	data := mustMarshal(exec)
	m.mu.Unlock()
	m.bus.Publish(eventbus.Event{Type: ExecStatusEvent, Data: data})
}

func mustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func newExecID() string {
	return fmt.Sprintf("exec-%d", time.Now().UnixNano())
}
