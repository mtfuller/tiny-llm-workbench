package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/docker"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// dockerClient is the subset of docker.Client Manager needs.
type dockerClient interface {
	Launch(ctx context.Context, name, environmentName, image string, mounts []docker.Mount) (string, error)
	Stop(ctx context.Context, containerID string) error
	ListManaged(ctx context.Context) ([]docker.ContainerInfo, error)
	ExecStream(ctx context.Context, containerID string, cmd []string, onOutput func(chunk string)) (int, error)
}

// environmentReader is the subset of registry.Registry Manager needs to
// look up an Environment definition when launching it.
type environmentReader interface {
	GetEnvironment(name string) (registry.Environment, error)
}

// Manager launches, stops, and execs into Environment instances.
type Manager struct {
	ctx    context.Context
	docker dockerClient
	envs   environmentReader
	bus    *eventbus.Bus

	mu    sync.Mutex
	execs map[string]*Exec
}

// NewManager builds a Manager. ctx bounds the lifetime of any exec it
// starts (the server's shutdown context), not any single HTTP request's
// context, since an exec continues streaming after StartExec's caller gets
// its response.
func NewManager(ctx context.Context, dockerClient dockerClient, envs environmentReader, bus *eventbus.Bus) *Manager {
	return &Manager{
		ctx:    ctx,
		docker: dockerClient,
		envs:   envs,
		bus:    bus,
		execs:  make(map[string]*Exec),
	}
}

// Launch starts a new instance of the named Environment definition. If
// instanceName is empty, one is generated.
func (m *Manager) Launch(ctx context.Context, environmentName, instanceName string) (Instance, error) {
	env, err := m.envs.GetEnvironment(environmentName)
	if err != nil {
		return Instance{}, fmt.Errorf("look up environment %q: %w", environmentName, err)
	}

	if instanceName == "" {
		instanceName = fmt.Sprintf("tlw-%s-%d", environmentName, time.Now().UnixNano())
	}

	mounts := make([]docker.Mount, len(env.Mounts))
	for i, mnt := range env.Mounts {
		mounts[i] = docker.Mount{HostPath: mnt.HostPath, ContainerPath: mnt.ContainerPath}
	}

	containerID, err := m.docker.Launch(ctx, instanceName, environmentName, env.Image, mounts)
	if err != nil {
		return Instance{}, fmt.Errorf("launch environment %q: %w", environmentName, err)
	}

	return Instance{
		ID:              containerID,
		Name:            instanceName,
		EnvironmentName: environmentName,
		Image:           env.Image,
		State:           "running",
		CreatedAt:       time.Now().UTC(),
	}, nil
}

// Stop stops and removes the instance with the given container ID.
func (m *Manager) Stop(ctx context.Context, instanceID string) error {
	return m.docker.Stop(ctx, instanceID)
}

// ListInstances reflects Docker's live state directly — there's no
// in-process bookkeeping to fall out of sync, so this is correct even
// immediately after a `tlw serve` restart.
func (m *Manager) ListInstances(ctx context.Context) ([]Instance, error) {
	containers, err := m.docker.ListManaged(ctx)
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, len(containers))
	for i, c := range containers {
		instances[i] = Instance{
			ID:              c.ID,
			Name:            c.Name,
			EnvironmentName: c.EnvironmentName,
			Image:           c.Image,
			State:           c.State,
			CreatedAt:       c.CreatedAt,
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

// RunToolSync runs command inside instanceID and blocks until it finishes,
// returning its combined output. Unlike StartExec, this doesn't stream
// progress over the event bus or track an Exec — it's for callers (an
// agent's Tool node) that need the result before they can continue, not for
// live observation.
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
