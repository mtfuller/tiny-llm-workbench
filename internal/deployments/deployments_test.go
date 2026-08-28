package deployments

import (
	"context"
	"errors"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeDeploymentReader struct {
	deps map[string]registry.Deployment
}

func (f *fakeDeploymentReader) GetDeployment(name string) (registry.Deployment, error) {
	d, ok := f.deps[name]
	if !ok {
		return registry.Deployment{}, errors.New("not found")
	}
	return d, nil
}

type fakeWorkspaceReader struct {
	ws map[string]registry.Workspace
}

func (f *fakeWorkspaceReader) GetWorkspace(name string) (registry.Workspace, error) {
	w, ok := f.ws[name]
	if !ok {
		return registry.Workspace{}, errors.New("not found")
	}
	return w, nil
}

type fakeLauncher struct {
	instance  environments.Instance
	launchErr error
	launched  []string
	stopped   []string
}

func (f *fakeLauncher) Launch(ctx context.Context, workspaceName, instanceName string) (environments.Instance, error) {
	f.launched = append(f.launched, workspaceName)
	return f.instance, f.launchErr
}

func (f *fakeLauncher) Stop(ctx context.Context, instanceID string) error {
	f.stopped = append(f.stopped, instanceID)
	return nil
}

type fakeAgentRunner struct {
	run          *agents.Run
	startErr     error
	startedInst  []string
	reply        agents.ChatMessage
	sendErr      error
	sentMessages []string
	stoppedRuns  []string
}

func (f *fakeAgentRunner) StartRunInInstance(agentName, instanceID string) (*agents.Run, error) {
	f.startedInst = append(f.startedInst, instanceID)
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.run != nil {
		return f.run, nil
	}
	return &agents.Run{ID: "run-1", AgentName: agentName, InstanceID: instanceID}, nil
}

func (f *fakeAgentRunner) SendMessage(runID, message string) (agents.ChatMessage, error) {
	f.sentMessages = append(f.sentMessages, message)
	if f.sendErr != nil {
		return agents.ChatMessage{}, f.sendErr
	}
	return f.reply, nil
}

func (f *fakeAgentRunner) StopRun(runID string) error {
	f.stoppedRuns = append(f.stoppedRuns, runID)
	return nil
}

func newManager(deps *fakeDeploymentReader, ws *fakeWorkspaceReader, envs *fakeLauncher, ar *fakeAgentRunner) *Manager {
	return NewManager(context.Background(), deps, ws, envs, ar)
}

func TestStartLaunchesRealWorkspaceAndAgentRun(t *testing.T) {
	deps := &fakeDeploymentReader{deps: map[string]registry.Deployment{
		"prod": {Name: "prod", AgentName: "coder", WorkspaceName: "my-project"},
	}}
	ws := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"my-project": {Name: "my-project", Type: registry.WorkspaceReal, HostPath: "/home/me/project"},
	}}
	envs := &fakeLauncher{instance: environments.Instance{ID: "container-1", WorkspacePath: "/home/me/project"}}
	ar := &fakeAgentRunner{}
	m := newManager(deps, ws, envs, ar)

	s, err := m.Start("prod")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if s.InstanceID != "container-1" || s.WorkspacePath != "/home/me/project" || s.AgentName != "coder" {
		t.Errorf("Session = %+v, want it wired to the launched instance", s)
	}
	if len(envs.launched) != 1 || envs.launched[0] != "my-project" {
		t.Errorf("envs.launched = %v, want [my-project]", envs.launched)
	}
	if len(ar.startedInst) != 1 || ar.startedInst[0] != "container-1" {
		t.Errorf("ar.startedInst = %v, want [container-1]", ar.startedInst)
	}
}

func TestStartRejectsTestWorkspace(t *testing.T) {
	deps := &fakeDeploymentReader{deps: map[string]registry.Deployment{
		"prod": {Name: "prod", AgentName: "coder", WorkspaceName: "scratch"},
	}}
	ws := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"scratch": {Name: "scratch", Type: registry.WorkspaceTest},
	}}
	m := newManager(deps, ws, &fakeLauncher{}, &fakeAgentRunner{})

	if _, err := m.Start("prod"); err == nil {
		t.Error("Start() error = nil, want a deployment to reject a non-real workspace")
	}
}

func TestStartAgentRunFailureStopsTheSandbox(t *testing.T) {
	deps := &fakeDeploymentReader{deps: map[string]registry.Deployment{
		"prod": {Name: "prod", AgentName: "coder", WorkspaceName: "my-project"},
	}}
	ws := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"my-project": {Name: "my-project", Type: registry.WorkspaceReal, HostPath: "/x"},
	}}
	envs := &fakeLauncher{instance: environments.Instance{ID: "container-1"}}
	ar := &fakeAgentRunner{startErr: errors.New("no such agent")}
	m := newManager(deps, ws, envs, ar)

	if _, err := m.Start("prod"); err == nil {
		t.Fatal("Start() error = nil, want the agent-run failure to propagate")
	}
	if len(envs.stopped) != 1 || envs.stopped[0] != "container-1" {
		t.Errorf("envs.stopped = %v, want the sandbox torn down after the agent-run failure", envs.stopped)
	}
}

func TestSendMessageAppendsTranscript(t *testing.T) {
	deps := &fakeDeploymentReader{deps: map[string]registry.Deployment{
		"prod": {Name: "prod", AgentName: "coder", WorkspaceName: "my-project"},
	}}
	ws := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"my-project": {Name: "my-project", Type: registry.WorkspaceReal, HostPath: "/x"},
	}}
	envs := &fakeLauncher{instance: environments.Instance{ID: "container-1"}}
	ar := &fakeAgentRunner{reply: agents.ChatMessage{Role: "assistant", Content: "done"}}
	m := newManager(deps, ws, envs, ar)

	s, _ := m.Start("prod")
	reply, err := m.SendMessage(s.ID, "make a file")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if reply.Content != "done" {
		t.Errorf("reply = %+v, want the agent's reply", reply)
	}
	got, _ := m.Get(s.ID)
	if len(got.Messages) != 2 || got.Messages[0].Role != "user" || got.Messages[1].Role != "assistant" {
		t.Errorf("transcript = %+v, want [user, assistant]", got.Messages)
	}
}

func TestStopTearsDownRunAndSandbox(t *testing.T) {
	deps := &fakeDeploymentReader{deps: map[string]registry.Deployment{
		"prod": {Name: "prod", AgentName: "coder", WorkspaceName: "my-project"},
	}}
	ws := &fakeWorkspaceReader{ws: map[string]registry.Workspace{
		"my-project": {Name: "my-project", Type: registry.WorkspaceReal, HostPath: "/x"},
	}}
	envs := &fakeLauncher{instance: environments.Instance{ID: "container-1"}}
	ar := &fakeAgentRunner{}
	m := newManager(deps, ws, envs, ar)

	s, _ := m.Start("prod")
	if err := m.Stop(s.ID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(ar.stoppedRuns) != 1 || len(envs.stopped) != 1 {
		t.Errorf("Stop() did not tear down: runs=%v sandboxes=%v", ar.stoppedRuns, envs.stopped)
	}
	if _, ok := m.Get(s.ID); ok {
		t.Error("Get() found a session after Stop()")
	}
	if err := m.Stop(s.ID); err != nil {
		t.Errorf("Stop() twice error = %v, want nil (idempotent)", err)
	}
}
