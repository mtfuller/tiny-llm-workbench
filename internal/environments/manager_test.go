package environments

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/docker"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeDocker struct {
	launchID  string
	launchErr error
	launched  []string         // workspace names launched
	mounts    [][]docker.Mount // mounts passed per launch

	stopErr    error
	stoppedIDs []string

	listResult []docker.ContainerInfo
	listErr    error

	execOutput   []string
	execExit     int
	execErr      error
	execCommands [][]string
}

func (f *fakeDocker) Launch(ctx context.Context, name, workspaceName, image string, mounts []docker.Mount) (string, error) {
	f.launched = append(f.launched, workspaceName)
	f.mounts = append(f.mounts, mounts)
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

type fakeWorkspaceReader struct {
	ws  map[string]registry.Workspace
	err error
}

func (f *fakeWorkspaceReader) GetWorkspace(name string) (registry.Workspace, error) {
	if f.err != nil {
		return registry.Workspace{}, f.err
	}
	w, ok := f.ws[name]
	if !ok {
		return registry.Workspace{}, errors.New("not found")
	}
	return w, nil
}

func newTestManager(t *testing.T, d dockerClient, wr workspaceReader) *Manager {
	t.Helper()
	return NewManager(context.Background(), d, wr, eventbus.New(), t.TempDir())
}

func TestLaunchRealWorkspaceBindMountsDirectly(t *testing.T) {
	realDir := t.TempDir()
	d := &fakeDocker{launchID: "abc123"}
	wr := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"my-project": {Name: "my-project", Type: registry.WorkspaceReal, HostPath: realDir},
	}}
	m := newTestManager(t, d, wr)

	instance, err := m.Launch(context.Background(), "my-project", "")
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if instance.ID != "abc123" {
		t.Errorf("Launch().ID = %q, want %q", instance.ID, "abc123")
	}
	if instance.Image != docker.DefaultSandboxImage {
		t.Errorf("Launch().Image = %q, want the fixed default %q", instance.Image, docker.DefaultSandboxImage)
	}
	if instance.WorkspaceName != "my-project" || instance.WorkspaceType != "real" {
		t.Errorf("Launch() workspace fields = %q/%q, want my-project/real", instance.WorkspaceName, instance.WorkspaceType)
	}
	if instance.WorkspacePath != realDir {
		t.Errorf("Launch().WorkspacePath = %q, want the real dir %q (no copy)", instance.WorkspacePath, realDir)
	}
	if len(d.mounts) != 1 || len(d.mounts[0]) != 1 || d.mounts[0][0].HostPath != realDir || d.mounts[0][0].ContainerPath != docker.ContainerWorkdir {
		t.Errorf("d.mounts = %+v, want a single %s -> %s bind mount", d.mounts, realDir, docker.ContainerWorkdir)
	}
}

func TestLaunchTestWorkspaceStagesAThrowawayCopy(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "notes.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	d := &fakeDocker{launchID: "abc123"}
	wr := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"scratch": {Name: "scratch", Type: registry.WorkspaceTest, HostPath: src},
	}}
	m := newTestManager(t, d, wr)

	instance, err := m.Launch(context.Background(), "scratch", "inst-1")
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if instance.WorkspacePath == src {
		t.Fatalf("Launch().WorkspacePath = %q, want a staged copy, not the source", instance.WorkspacePath)
	}
	staged := filepath.Join(instance.WorkspacePath, "notes.txt")
	got, err := os.ReadFile(staged)
	if err != nil || string(got) != "seed" {
		t.Fatalf("staged copy missing seed file: content=%q err=%v", got, err)
	}
	// Editing the staged copy must not touch the source.
	if err := os.WriteFile(staged, []byte("changed"), 0o644); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	src0, _ := os.ReadFile(filepath.Join(src, "notes.txt"))
	if string(src0) != "seed" {
		t.Errorf("source file changed to %q, want it untouched at %q", src0, "seed")
	}
}

func TestLaunchUnknownWorkspace(t *testing.T) {
	m := newTestManager(t, &fakeDocker{}, &fakeWorkspaceReader{ws: map[string]registry.Workspace{}})

	if _, err := m.Launch(context.Background(), "does-not-exist", ""); err == nil {
		t.Error("Launch() error = nil, want an error for an unknown workspace")
	}
}

func TestLaunchDockerError(t *testing.T) {
	realDir := t.TempDir()
	d := &fakeDocker{launchErr: errors.New("docker daemon unreachable")}
	wr := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"my-project": {Name: "my-project", Type: registry.WorkspaceReal, HostPath: realDir},
	}}
	m := newTestManager(t, d, wr)

	if _, err := m.Launch(context.Background(), "my-project", ""); err == nil {
		t.Error("Launch() error = nil, want the docker error to propagate")
	}
}

func TestStopDelegatesToDocker(t *testing.T) {
	d := &fakeDocker{}
	m := newTestManager(t, d, &fakeWorkspaceReader{})

	if err := m.Stop(context.Background(), "abc123"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(d.stoppedIDs) != 1 || d.stoppedIDs[0] != "abc123" {
		t.Errorf("d.stoppedIDs = %v, want [abc123]", d.stoppedIDs)
	}
}

func TestListInstancesReflectsDocker(t *testing.T) {
	d := &fakeDocker{listResult: []docker.ContainerInfo{
		{ID: "abc123", Name: "tlw-scratch-1", Image: docker.DefaultSandboxImage, State: "running", WorkspaceName: "scratch"},
	}}
	m := newTestManager(t, d, &fakeWorkspaceReader{})

	instances, err := m.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances() error = %v", err)
	}
	if len(instances) != 1 || instances[0].ID != "abc123" || instances[0].WorkspaceName != "scratch" {
		t.Errorf("ListInstances() = %+v, want a single abc123/scratch entry", instances)
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
	m := newTestManager(t, d, &fakeWorkspaceReader{})

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
	m := newTestManager(t, d, &fakeWorkspaceReader{})

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
	m := newTestManager(t, &fakeDocker{}, &fakeWorkspaceReader{})

	if _, err := m.StartExec("abc123", ""); err == nil {
		t.Error("StartExec() error = nil, want an error for an empty command")
	}
}

func TestGetExecUnknown(t *testing.T) {
	m := newTestManager(t, &fakeDocker{}, &fakeWorkspaceReader{})

	if _, ok := m.GetExec("does-not-exist"); ok {
		t.Error("GetExec() ok = true, want false for an unknown exec")
	}
}

func TestRunToolSyncSuccess(t *testing.T) {
	d := &fakeDocker{execOutput: []string{"hello\n"}, execExit: 0}
	m := newTestManager(t, d, &fakeWorkspaceReader{})

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
	m := newTestManager(t, d, &fakeWorkspaceReader{})

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
	m := newTestManager(t, d, &fakeWorkspaceReader{})

	if _, err := m.RunToolSync(context.Background(), "abc123", "echo hello"); err == nil {
		t.Error("RunToolSync() error = nil, want the docker error to propagate")
	}
}

func TestRunToolSyncRequiresCommand(t *testing.T) {
	m := newTestManager(t, &fakeDocker{}, &fakeWorkspaceReader{})

	if _, err := m.RunToolSync(context.Background(), "abc123", ""); err == nil {
		t.Error("RunToolSync() error = nil, want an error for an empty command")
	}
}

func TestRenderToolCommandSubstitutesAndQuotes(t *testing.T) {
	tool := registry.Tool{
		Command:    "cat {{path}}",
		Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}},
	}

	got, err := RenderToolCommand(tool, map[string]string{"path": "/some path/with spaces.txt"})
	if err != nil {
		t.Fatalf("RenderToolCommand() error = %v", err)
	}
	want := `cat '/some path/with spaces.txt'`
	if got != want {
		t.Errorf("RenderToolCommand() = %q, want %q", got, want)
	}
}

func TestRenderToolCommandEscapesEmbeddedSingleQuotes(t *testing.T) {
	tool := registry.Tool{
		Command:    "printf '%s' {{content}} > {{path}}",
		Parameters: []registry.ToolParameter{{Name: "content", Type: registry.ToolParamString}, {Name: "path", Type: registry.ToolParamString}},
	}

	got, err := RenderToolCommand(tool, map[string]string{"content": "it's a test", "path": "/tmp/out.txt"})
	if err != nil {
		t.Fatalf("RenderToolCommand() error = %v", err)
	}
	want := `printf '%s' 'it'\''s a test' > '/tmp/out.txt'`
	if got != want {
		t.Errorf("RenderToolCommand() = %q, want %q", got, want)
	}
}

func TestRenderToolCommandMissingRequiredParameter(t *testing.T) {
	tool := registry.Tool{
		Command:    "cat {{path}}",
		Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}},
	}

	if _, err := RenderToolCommand(tool, map[string]string{}); err == nil {
		t.Error("RenderToolCommand() error = nil, want an error for a missing required parameter")
	}
}

func TestRenderToolCommandOptionalParameterOmitted(t *testing.T) {
	tool := registry.Tool{
		Command:    "ls {{flags}}{{path}}",
		Parameters: []registry.ToolParameter{{Name: "flags", Type: registry.ToolParamString}, {Name: "path", Type: registry.ToolParamString, Required: true}},
	}

	got, err := RenderToolCommand(tool, map[string]string{"path": "/tmp"})
	if err != nil {
		t.Fatalf("RenderToolCommand() error = %v", err)
	}
	want := "ls {{flags}}'/tmp'"
	if got != want {
		t.Errorf("RenderToolCommand() = %q, want %q (an omitted optional parameter's placeholder is left as-is)", got, want)
	}
}

func TestRenderToolCommandValidatesNumberType(t *testing.T) {
	tool := registry.Tool{
		Command:    "seq {{count}}",
		Parameters: []registry.ToolParameter{{Name: "count", Type: registry.ToolParamNumber, Required: true}},
	}

	if _, err := RenderToolCommand(tool, map[string]string{"count": "not-a-number"}); err == nil {
		t.Error("RenderToolCommand() error = nil, want an error for a non-numeric value on a number parameter")
	}

	got, err := RenderToolCommand(tool, map[string]string{"count": "5"})
	if err != nil {
		t.Fatalf("RenderToolCommand() error = %v", err)
	}
	if got != "seq '5'" {
		t.Errorf("RenderToolCommand() = %q, want %q", got, "seq '5'")
	}
}

func TestRenderToolCommandValidatesBooleanType(t *testing.T) {
	tool := registry.Tool{
		Command:    "echo {{verbose}}",
		Parameters: []registry.ToolParameter{{Name: "verbose", Type: registry.ToolParamBoolean, Required: true}},
	}

	if _, err := RenderToolCommand(tool, map[string]string{"verbose": "yes"}); err == nil {
		t.Error("RenderToolCommand() error = nil, want an error for a non-boolean value on a boolean parameter")
	}
}

func TestTryToolStartsExecWithRenderedCommand(t *testing.T) {
	d := &fakeDocker{execOutput: []string{"file contents\n"}, execExit: 0}
	m := newTestManager(t, d, &fakeWorkspaceReader{})

	tool := registry.Tool{
		Command:    "cat {{path}}",
		Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}},
	}

	exec, err := m.TryTool("abc123", tool, map[string]string{"path": "/etc/hostname"})
	if err != nil {
		t.Fatalf("TryTool() error = %v", err)
	}

	finished := waitForExecStatus(t, m, exec.ID, ExecDone, time.Second)
	if finished.Output != "file contents\n" {
		t.Errorf("finished.Output = %q, want %q", finished.Output, "file contents\n")
	}
	if len(d.execCommands) != 1 || d.execCommands[0][2] != "cat '/etc/hostname'" {
		t.Errorf("d.execCommands = %v, want the rendered command", d.execCommands)
	}
}

func TestTryToolValidationErrorNeverStartsExec(t *testing.T) {
	d := &fakeDocker{}
	m := newTestManager(t, d, &fakeWorkspaceReader{})

	tool := registry.Tool{
		Command:    "cat {{path}}",
		Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}},
	}

	if _, err := m.TryTool("abc123", tool, map[string]string{}); err == nil {
		t.Error("TryTool() error = nil, want an error for a missing required parameter")
	}
	if len(d.execCommands) != 0 {
		t.Errorf("d.execCommands = %v, want no exec started when validation fails", d.execCommands)
	}
}
