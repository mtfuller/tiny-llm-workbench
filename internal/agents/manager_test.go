package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeAgentReader struct {
	agents map[string]registry.Agent
	err    error
	// models maps a registry model name -> the path/repo id ResolveModelRef
	// returns for it. A ref not present here passes through unchanged.
	models map[string]string
}

func (f *fakeAgentReader) GetAgent(name string) (registry.Agent, error) {
	if f.err != nil {
		return registry.Agent{}, f.err
	}
	agent, ok := f.agents[name]
	if !ok {
		return registry.Agent{}, errors.New("not found")
	}
	return agent, nil
}

func (f *fakeAgentReader) ResolveModelRef(ref string) string {
	if resolved, ok := f.models[ref]; ok {
		return resolved
	}
	return ref
}

type fakeWorkspaceRunner struct {
	launchResult environments.Instance
	launchErr    error
	launched     []string

	stopErr    error
	stoppedIDs []string

	toolOutput string
	toolErr    error
}

func (f *fakeWorkspaceRunner) Launch(ctx context.Context, workspaceName, instanceName string) (environments.Instance, error) {
	f.launched = append(f.launched, workspaceName)
	return f.launchResult, f.launchErr
}

func (f *fakeWorkspaceRunner) Stop(ctx context.Context, instanceID string) error {
	f.stoppedIDs = append(f.stoppedIDs, instanceID)
	return f.stopErr
}

func (f *fakeWorkspaceRunner) RunToolSync(ctx context.Context, instanceID, command string) (string, error) {
	return f.toolOutput, f.toolErr
}

// fakeToolReader resolves the Tool catalog entries an agent's Tools set
// names, keyed by tool name.
type fakeToolReader struct {
	tools map[string]registry.Tool
}

func (f *fakeToolReader) GetTool(name string) (registry.Tool, error) {
	tool, ok := f.tools[name]
	if !ok {
		return registry.Tool{}, errors.New("not found")
	}
	return tool, nil
}

func newManager(t *testing.T, agents *fakeAgentReader, llm *fakeLLM, envs *fakeWorkspaceRunner, tools *fakeToolReader) *Manager {
	t.Helper()
	if envs == nil {
		envs = &fakeWorkspaceRunner{}
	}
	if tools == nil {
		tools = &fakeToolReader{}
	}
	return NewManager(context.Background(), agents, llm, envs, tools, &fakeKnowledgeReader{}, eventbus.New())
}

func TestStartRunSuccess(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	m := newManager(t, agents, &fakeLLM{}, nil, nil)

	run, err := m.StartRun("greeter", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.AgentName != "greeter" {
		t.Errorf("run.AgentName = %q, want %q", run.AgentName, "greeter")
	}
	if run.Messages == nil {
		t.Error("run.Messages = nil, want an initialized empty slice")
	}
	if run.InstanceID != "" {
		t.Errorf("run.InstanceID = %q, want empty for an agent with no workspace", run.InstanceID)
	}
}

func TestStartRunUnknownAgent(t *testing.T) {
	m := newManager(t, &fakeAgentReader{agents: map[string]registry.Agent{}}, &fakeLLM{}, nil, nil)

	if _, err := m.StartRun("does-not-exist", ""); err == nil {
		t.Error("StartRun() error = nil, want an error for an unknown agent")
	}
}

func TestStartRunLaunchesAgentWorkspace(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Workspace: "scratch", Graph: linearGraph()},
	}}
	envs := &fakeWorkspaceRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	run, err := m.StartRun("researcher", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.InstanceID != "container-1" {
		t.Errorf("run.InstanceID = %q, want %q", run.InstanceID, "container-1")
	}
	if len(envs.launched) != 1 || envs.launched[0] != "scratch" {
		t.Errorf("envs.launched = %v, want [scratch]", envs.launched)
	}
}

func TestStartRunWorkspaceOverrideWins(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Workspace: "default-ws", Graph: linearGraph()},
	}}
	envs := &fakeWorkspaceRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	if _, err := m.StartRun("researcher", "picked-ws"); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if len(envs.launched) != 1 || envs.launched[0] != "picked-ws" {
		t.Errorf("envs.launched = %v, want the per-run override [picked-ws]", envs.launched)
	}
}

func TestStartRunWorkspaceLaunchFailure(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Workspace: "scratch", Graph: linearGraph()},
	}}
	envs := &fakeWorkspaceRunner{launchErr: errors.New("docker daemon unreachable")}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	if _, err := m.StartRun("researcher", ""); err == nil {
		t.Error("StartRun() error = nil, want the launch error to propagate")
	}
}

func TestStopRunStopsWorkspace(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Workspace: "scratch", Graph: linearGraph()},
	}}
	envs := &fakeWorkspaceRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	run, err := m.StartRun("researcher", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if err := m.StopRun(run.ID); err != nil {
		t.Fatalf("StopRun() error = %v", err)
	}
	if len(envs.stoppedIDs) != 1 || envs.stoppedIDs[0] != "container-1" {
		t.Errorf("envs.stoppedIDs = %v, want [container-1]", envs.stoppedIDs)
	}
	if _, ok := m.GetRun(run.ID); ok {
		t.Error("GetRun() found a run after StopRun(), want it removed")
	}
}

func TestStopRunWithoutWorkspaceDoesNothing(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	envs := &fakeWorkspaceRunner{}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	run, err := m.StartRun("greeter", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if err := m.StopRun(run.ID); err != nil {
		t.Fatalf("StopRun() error = %v", err)
	}
	if len(envs.stoppedIDs) != 0 {
		t.Errorf("envs.stoppedIDs = %v, want none stopped for a run with no workspace", envs.stoppedIDs)
	}
}

func TestStopRunUnknownRunIsNotAnError(t *testing.T) {
	m := newManager(t, &fakeAgentReader{}, &fakeLLM{}, nil, nil)

	if err := m.StopRun("does-not-exist"); err != nil {
		t.Errorf("StopRun() error = %v, want nil for an unknown run (idempotent cleanup)", err)
	}
}

func TestStartRunInInstanceReusesGivenInstance(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	envs := &fakeWorkspaceRunner{}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	run, err := m.StartRunInInstance("greeter", "eval-container-1")
	if err != nil {
		t.Fatalf("StartRunInInstance() error = %v", err)
	}
	if run.InstanceID != "eval-container-1" {
		t.Errorf("run.InstanceID = %q, want %q", run.InstanceID, "eval-container-1")
	}
	if len(envs.launched) != 0 {
		t.Errorf("envs.launched = %v, want none — StartRunInInstance must not launch its own instance", envs.launched)
	}
}

func TestStartRunInInstanceUnknownAgent(t *testing.T) {
	m := newManager(t, &fakeAgentReader{agents: map[string]registry.Agent{}}, &fakeLLM{}, nil, nil)

	if _, err := m.StartRunInInstance("does-not-exist", "container-1"); err == nil {
		t.Error("StartRunInInstance() error = nil, want an error for an unknown agent")
	}
}

func TestStopRunDoesNotStopAnInstanceItDidNotLaunch(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	envs := &fakeWorkspaceRunner{}
	m := newManager(t, agents, &fakeLLM{}, envs, nil)

	run, err := m.StartRunInInstance("greeter", "eval-container-1")
	if err != nil {
		t.Fatalf("StartRunInInstance() error = %v", err)
	}

	if err := m.StopRun(run.ID); err != nil {
		t.Fatalf("StopRun() error = %v", err)
	}
	if len(envs.stoppedIDs) != 0 {
		t.Errorf("envs.stoppedIDs = %v, want none stopped — the caller owns this instance's lifecycle", envs.stoppedIDs)
	}
	if _, ok := m.GetRun(run.ID); ok {
		t.Error("GetRun() found a run after StopRun(), want it removed from the in-memory run map regardless")
	}
}

// A prompt node whose Model is a registry model name is resolved to that
// model's path / repo id before the engine calls the runner — otherwise
// mlx_lm.server gets an org-less name it can't fetch (the "looks hung" bug).
func TestSendMessageResolvesPromptNodeModel(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "Llama-3.2-1B-Instruct-4bit"}},
		},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "p1"}},
	}
	agents := &fakeAgentReader{
		agents: map[string]registry.Agent{"a": {Name: "a", Graph: graph}},
		models: map[string]string{"Llama-3.2-1B-Instruct-4bit": "mlx-community/Llama-3.2-1B-Instruct-4bit"},
	}
	llm := &fakeLLM{responses: []string{"hi"}}
	m := newManager(t, agents, llm, nil, nil)

	run, _ := m.StartRun("a", "")
	if _, err := m.SendMessage(run.ID, "hi"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if len(llm.models) != 1 || llm.models[0] != "mlx-community/Llama-3.2-1B-Instruct-4bit" {
		t.Errorf("llm.models = %v, want the resolved repo id", llm.models)
	}
}

func TestSendMessageSuccess(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	llm := &fakeLLM{responses: []string{"hello there!"}}
	m := newManager(t, agents, llm, nil, nil)

	run, err := m.StartRun("greeter", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	reply, err := m.SendMessage(run.ID, "hi")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if reply.Role != "assistant" || reply.Content != "hello there!" {
		t.Errorf("SendMessage() = %+v, want assistant/%q", reply, "hello there!")
	}

	got, ok := m.GetRun(run.ID)
	if !ok {
		t.Fatal("GetRun() not found after SendMessage()")
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "user" || got.Messages[1].Role != "assistant" {
		t.Errorf("got.Messages = %+v, want [user, assistant]", got.Messages)
	}
}

// A say node's messages are published on the eventbus (agent.message) as the
// turn runs; a say node marked final is what SendMessage returns, and only
// that final answer is kept in the run's conversation history — progress
// messages are display-only.
func TestSendMessageStreamsSayMessages(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "s1", Type: "say", Data: registry.NodeData{Name: "Progress", SayTemplate: "looking into it"}},
			{ID: "s2", Type: "say", Data: registry.NodeData{Name: "Answer", SayTemplate: "done: {{Input}}", SayFinal: true}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "s1"},
			{ID: "e2", Source: "s1", Target: "s2"},
		},
	}
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"narrator": {Name: "narrator", Graph: graph}}}
	bus := eventbus.New()
	m := NewManager(context.Background(), agents, &fakeLLM{}, &fakeWorkspaceRunner{}, &fakeToolReader{}, &fakeKnowledgeReader{}, bus)

	run, err := m.StartRun("narrator", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	events, unsub := bus.Subscribe()
	defer unsub()

	reply, err := m.SendMessage(run.ID, "my task")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if reply.Content != "done: my task" {
		t.Errorf("SendMessage() = %q, want the say-final message", reply.Content)
	}

	var messageEvents []string
	drain := true
	for drain {
		select {
		case e := <-events:
			if e.Type == MessageEventType {
				messageEvents = append(messageEvents, e.Data)
			}
		default:
			drain = false
		}
	}
	if len(messageEvents) != 2 {
		t.Fatalf("agent.message events = %d, want 2 (one progress, one final); got %v", len(messageEvents), messageEvents)
	}
	if !strings.Contains(messageEvents[0], `"kind":"progress"`) || !strings.Contains(messageEvents[0], "looking into it") {
		t.Errorf("first event = %q, want the progress message", messageEvents[0])
	}
	if !strings.Contains(messageEvents[1], `"kind":"final"`) || !strings.Contains(messageEvents[1], "done: my task") {
		t.Errorf("second event = %q, want the final message", messageEvents[1])
	}

	got, _ := m.GetRun(run.ID)
	if len(got.Messages) != 2 || got.Messages[1].Content != "done: my task" {
		t.Errorf("history = %+v, want just [user, final-assistant] (progress messages not persisted)", got.Messages)
	}
}

func TestSendMessageUsesRunInstanceForToolNodes(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {
			Name:      "researcher",
			Workspace: "scratch",
			Tools:     []string{"web_search"},
			Graph:     toolGraph(registry.NodeData{ToolName: "web_search", ToolArgs: map[string]string{"query": "{{Input}}"}}),
		},
	}}
	envs := &fakeWorkspaceRunner{launchResult: environments.Instance{ID: "container-1"}, toolOutput: "search results"}
	toolStore := &fakeToolReader{tools: map[string]registry.Tool{
		"web_search": {
			Name:       "web_search",
			Command:    "curl -s {{query}}",
			Parameters: []registry.ToolParameter{{Name: "query", Type: registry.ToolParamString, Required: true}},
		},
	}}
	m := newManager(t, agents, &fakeLLM{}, envs, toolStore)

	run, err := m.StartRun("researcher", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	reply, err := m.SendMessage(run.ID, "search for cats")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if reply.Content != "search results" {
		t.Errorf("SendMessage() = %+v, want the tool node's output", reply)
	}
}

// A tool name in the agent's set that's since been removed from the catalog
// is skipped when resolving (not an error) — the engine's own "tool not
// found" surfaces only if a node actually tries to use it.
func TestSendMessageSkipsUnknownToolInSet(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"greeter": {Name: "greeter", Tools: []string{"deleted_tool"}, Graph: linearGraph()},
	}}
	llm := &fakeLLM{responses: []string{"hi"}}
	m := newManager(t, agents, llm, nil, &fakeToolReader{tools: map[string]registry.Tool{}})

	run, _ := m.StartRun("greeter", "")
	if _, err := m.SendMessage(run.ID, "hi"); err != nil {
		t.Errorf("SendMessage() error = %v, want a missing tool in the set to be tolerated", err)
	}
}

func TestSendMessageUnknownRun(t *testing.T) {
	m := newManager(t, &fakeAgentReader{}, &fakeLLM{}, nil, nil)

	if _, err := m.SendMessage("does-not-exist", "hi"); err == nil {
		t.Error("SendMessage() error = nil, want an error for an unknown run")
	}
}

func TestSendMessageRequiresMessage(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	m := newManager(t, agents, &fakeLLM{}, nil, nil)

	run, err := m.StartRun("greeter", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if _, err := m.SendMessage(run.ID, ""); err == nil {
		t.Error("SendMessage() error = nil, want an error for an empty message")
	}
}

func TestSendMessageEngineErrorRecordsUserMessageOnly(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	llm := &fakeLLM{err: errors.New("model runner unreachable")}
	m := newManager(t, agents, llm, nil, nil)

	run, err := m.StartRun("greeter", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if _, err := m.SendMessage(run.ID, "hi"); err == nil {
		t.Fatal("SendMessage() error = nil, want the engine error to propagate")
	}

	got, ok := m.GetRun(run.ID)
	if !ok {
		t.Fatal("GetRun() not found")
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Errorf("got.Messages = %+v, want just the user message recorded on failure", got.Messages)
	}
}

func TestSendMessageIncludesPriorHistory(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	llm := &fakeLLM{responses: []string{"first reply", "second reply"}}
	m := newManager(t, agents, llm, nil, nil)

	run, err := m.StartRun("greeter", "")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if _, err := m.SendMessage(run.ID, "first message"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if _, err := m.SendMessage(run.ID, "second message"); err != nil {
		t.Fatalf("SendMessage() (second) error = %v", err)
	}

	if len(llm.calls) != 2 {
		t.Fatalf("llm.calls = %v, want 2 calls", llm.calls)
	}
	if !strings.Contains(llm.calls[1], "first message") || !strings.Contains(llm.calls[1], "first reply") {
		t.Errorf("second call prompt = %q, want it to include the first turn's history", llm.calls[1])
	}
}
