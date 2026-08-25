package environments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/docker"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeDocker struct {
	launchID  string
	launchErr error
	launched  []string // environment names launched

	stopErr    error
	stoppedIDs []string

	listResult []docker.ContainerInfo
	listErr    error

	execOutput   []string
	execExit     int
	execErr      error
	execCommands [][]string
}

func (f *fakeDocker) Launch(ctx context.Context, name, environmentName, image string, mounts []docker.Mount) (string, error) {
	f.launched = append(f.launched, environmentName)
	if f.launchErr != nil {
		return "", f.launchErr
	}
	return f.launchID, nil
}

func (f *fakeDocker) Stop(ctx context.Context, containerID string) error {
	f.stoppedIDs = append(f.stoppedIDs, containerID)
	return f.stopErr
}

func (f *fakeDocker) ListManaged(ctx context.Context) ([]docker.ContainerInfo, error) {
	return f.listResult, f.listErr
}

func (f *fakeDocker) ExecStream(ctx context.Context, containerID string, cmd []string, onOutput func(string)) (int, error) {
	f.execCommands = append(f.execCommands, cmd)
	for _, chunk := range f.execOutput {
		onOutput(chunk)
	}
	return f.execExit, f.execErr
}

type fakeEnvironmentReader struct {
	envs map[string]registry.Environment
	err  error
}

func (f *fakeEnvironmentReader) GetEnvironment(name string) (registry.Environment, error) {
	if f.err != nil {
		return registry.Environment{}, f.err
	}
	env, ok := f.envs[name]
	if !ok {
		return registry.Environment{}, errors.New("not found")
	}
	return env, nil
}

func TestLaunchSuccess(t *testing.T) {
	d := &fakeDocker{launchID: "abc123"}
	envs := &fakeEnvironmentReader{envs: map[string]registry.Environment{
		"WebSearch": {Name: "WebSearch", Image: "curlimages/curl:8.10.1"},
	}}
	m := NewManager(context.Background(), d, envs, eventbus.New())

	instance, err := m.Launch(context.Background(), "WebSearch", "")
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if instance.ID != "abc123" {
		t.Errorf("Launch().ID = %q, want %q", instance.ID, "abc123")
	}
	if instance.Image != "curlimages/curl:8.10.1" {
		t.Errorf("Launch().Image = %q, want %q", instance.Image, "curlimages/curl:8.10.1")
	}
	if len(d.launched) != 1 || d.launched[0] != "WebSearch" {
		t.Errorf("d.launched = %v, want [WebSearch]", d.launched)
	}
}

func TestLaunchUnknownEnvironment(t *testing.T) {
	d := &fakeDocker{}
	envs := &fakeEnvironmentReader{envs: map[string]registry.Environment{}}
	m := NewManager(context.Background(), d, envs, eventbus.New())

	if _, err := m.Launch(context.Background(), "does-not-exist", ""); err == nil {
		t.Error("Launch() error = nil, want an error for an unknown environment")
	}
}

func TestLaunchDockerError(t *testing.T) {
	d := &fakeDocker{launchErr: errors.New("docker daemon unreachable")}
	envs := &fakeEnvironmentReader{envs: map[string]registry.Environment{
		"WebSearch": {Name: "WebSearch", Image: "curlimages/curl:8.10.1"},
	}}
	m := NewManager(context.Background(), d, envs, eventbus.New())

	if _, err := m.Launch(context.Background(), "WebSearch", ""); err == nil {
		t.Error("Launch() error = nil, want the docker error to propagate")
	}
}

func TestStopDelegatesToDocker(t *testing.T) {
	d := &fakeDocker{}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	if err := m.Stop(context.Background(), "abc123"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(d.stoppedIDs) != 1 || d.stoppedIDs[0] != "abc123" {
		t.Errorf("d.stoppedIDs = %v, want [abc123]", d.stoppedIDs)
	}
}

func TestListInstancesReflectsDocker(t *testing.T) {
	d := &fakeDocker{listResult: []docker.ContainerInfo{
		{ID: "abc123", Name: "tlw-websearch-1", Image: "curlimages/curl:8.10.1", State: "running", EnvironmentName: "WebSearch"},
	}}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	instances, err := m.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances() error = %v", err)
	}
	if len(instances) != 1 || instances[0].ID != "abc123" || instances[0].EnvironmentName != "WebSearch" {
		t.Errorf("ListInstances() = %+v, want a single abc123/WebSearch entry", instances)
	}
}

func waitForExecStatus(t *testing.T, m *Manager, id string, want ExecStatus, timeout time.Duration) *Exec {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if exec, ok := m.GetExec(id); ok && exec.Status == want {
			return exec
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("exec %s did not reach status %q within %s", id, want, timeout)
	return nil
}

func TestStartExecSuccess(t *testing.T) {
	d := &fakeDocker{execOutput: []string{"hello\n"}, execExit: 0}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	exec, err := m.StartExec("abc123", "echo hello")
	if err != nil {
		t.Fatalf("StartExec() error = %v", err)
	}
	if exec.Status != ExecRunning {
		t.Errorf("StartExec().Status = %q, want %q", exec.Status, ExecRunning)
	}

	finished := waitForExecStatus(t, m, exec.ID, ExecDone, time.Second)
	if finished.Output != "hello\n" {
		t.Errorf("finished.Output = %q, want %q", finished.Output, "hello\n")
	}
	if finished.ExitCode == nil || *finished.ExitCode != 0 {
		t.Errorf("finished.ExitCode = %v, want 0", finished.ExitCode)
	}

	if len(d.execCommands) != 1 {
		t.Fatalf("d.execCommands = %v, want 1 call", d.execCommands)
	}
	want := []string{"sh", "-c", "echo hello"}
	got := d.execCommands[0]
	if len(got) != len(want) {
		t.Fatalf("exec command = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("exec command = %v, want %v", got, want)
			break
		}
	}
}

func TestStartExecDockerError(t *testing.T) {
	d := &fakeDocker{execErr: errors.New("container not running")}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	exec, err := m.StartExec("abc123", "echo hello")
	if err != nil {
		t.Fatalf("StartExec() error = %v", err)
	}

	finished := waitForExecStatus(t, m, exec.ID, ExecFailed, time.Second)
	if finished.Error != "container not running" {
		t.Errorf("finished.Error = %q, want %q", finished.Error, "container not running")
	}
}

func TestStartExecRequiresCommand(t *testing.T) {
	m := NewManager(context.Background(), &fakeDocker{}, &fakeEnvironmentReader{}, eventbus.New())

	if _, err := m.StartExec("abc123", ""); err == nil {
		t.Error("StartExec() error = nil, want an error for an empty command")
	}
}

func TestGetExecUnknown(t *testing.T) {
	m := NewManager(context.Background(), &fakeDocker{}, &fakeEnvironmentReader{}, eventbus.New())

	if _, ok := m.GetExec("does-not-exist"); ok {
		t.Error("GetExec() ok = true, want false for an unknown exec")
	}
}

func TestRunToolSyncSuccess(t *testing.T) {
	d := &fakeDocker{execOutput: []string{"hello\n"}, execExit: 0}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	output, err := m.RunToolSync(context.Background(), "abc123", "echo hello")
	if err != nil {
		t.Fatalf("RunToolSync() error = %v", err)
	}
	if output != "hello\n" {
		t.Errorf("RunToolSync() = %q, want %q", output, "hello\n")
	}
	if len(d.execCommands) != 1 {
		t.Fatalf("d.execCommands = %v, want 1 call", d.execCommands)
	}
	want := []string{"sh", "-c", "echo hello"}
	got := d.execCommands[0]
	if len(got) != len(want) {
		t.Fatalf("exec command = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("exec command = %v, want %v", got, want)
			break
		}
	}
}

func TestRunToolSyncNonZeroExit(t *testing.T) {
	d := &fakeDocker{execOutput: []string{"boom"}, execExit: 1}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	output, err := m.RunToolSync(context.Background(), "abc123", "false")
	if err == nil {
		t.Error("RunToolSync() error = nil, want an error for a non-zero exit code")
	}
	if output != "boom" {
		t.Errorf("RunToolSync() output = %q, want %q even on failure", output, "boom")
	}
}

func TestRunToolSyncDockerError(t *testing.T) {
	d := &fakeDocker{execErr: errors.New("container not running")}
	m := NewManager(context.Background(), d, &fakeEnvironmentReader{}, eventbus.New())

	if _, err := m.RunToolSync(context.Background(), "abc123", "echo hello"); err == nil {
		t.Error("RunToolSync() error = nil, want the docker error to propagate")
	}
}

func TestRunToolSyncRequiresCommand(t *testing.T) {
	m := NewManager(context.Background(), &fakeDocker{}, &fakeEnvironmentReader{}, eventbus.New())

	if _, err := m.RunToolSync(context.Background(), "abc123", ""); err == nil {
		t.Error("RunToolSync() error = nil, want an error for an empty command")
	}
}
