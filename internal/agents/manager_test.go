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

type fakeEnvironmentRunner struct {
	launchResult environments.Instance
	launchErr    error
	launched     []string

	stopErr    error
	stoppedIDs []string

	toolOutput string
	toolErr    error
}

func (f *fakeEnvironmentRunner) Launch(ctx context.Context, environmentName, instanceName string) (environments.Instance, error) {
	f.launched = append(f.launched, environmentName)
	return f.launchResult, f.launchErr
}

func (f *fakeEnvironmentRunner) Stop(ctx context.Context, instanceID string) error {
	f.stoppedIDs = append(f.stoppedIDs, instanceID)
	return f.stopErr
}

func (f *fakeEnvironmentRunner) RunToolSync(ctx context.Context, instanceID, command string) (string, error) {
	return f.toolOutput, f.toolErr
}

// fakeEnvironmentReader resolves an Environment's Tool definitions for
// Manager's environmentReader dependency, keyed by environment name.
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

// fakeToolReader resolves the Tool catalog entries an Environment's Tools
// list names, keyed by tool name.
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

func TestStartRunSuccess(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("greeter")
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
		t.Errorf("run.InstanceID = %q, want empty for an agent with no Environment", run.InstanceID)
	}
}

func TestStartRunUnknownAgent(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	if _, err := m.StartRun("does-not-exist"); err == nil {
		t.Error("StartRun() error = nil, want an error for an unknown agent")
	}
}

func TestStartRunLaunchesEnvironment(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Environment: "WebSearch", Graph: linearGraph()},
	}}
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("researcher")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.InstanceID != "container-1" {
		t.Errorf("run.InstanceID = %q, want %q", run.InstanceID, "container-1")
	}
	if len(envs.launched) != 1 || envs.launched[0] != "WebSearch" {
		t.Errorf("envs.launched = %v, want [WebSearch]", envs.launched)
	}
}

func TestStartRunEnvironmentLaunchFailure(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Environment: "WebSearch", Graph: linearGraph()},
	}}
	envs := &fakeEnvironmentRunner{launchErr: errors.New("docker daemon unreachable")}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	if _, err := m.StartRun("researcher"); err == nil {
		t.Error("StartRun() error = nil, want the launch error to propagate")
	}
}

func TestStopRunStopsEnvironment(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Environment: "WebSearch", Graph: linearGraph()},
	}}
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("researcher")
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

func TestStopRunWithoutEnvironmentDoesNothing(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	envs := &fakeEnvironmentRunner{}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("greeter")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if err := m.StopRun(run.ID); err != nil {
		t.Fatalf("StopRun() error = %v", err)
	}
	if len(envs.stoppedIDs) != 0 {
		t.Errorf("envs.stoppedIDs = %v, want none stopped for a run with no Environment", envs.stoppedIDs)
	}
}

func TestStopRunUnknownRunIsNotAnError(t *testing.T) {
	m := NewManager(context.Background(), &fakeAgentReader{}, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	if err := m.StopRun("does-not-exist"); err != nil {
		t.Errorf("StopRun() error = %v, want nil for an unknown run (idempotent cleanup)", err)
	}
}

func TestStartRunInInstanceReusesGivenInstance(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	envs := &fakeEnvironmentRunner{}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

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
	agents := &fakeAgentReader{agents: map[string]registry.Agent{}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	if _, err := m.StartRunInInstance("does-not-exist", "container-1"); err == nil {
		t.Error("StartRunInInstance() error = nil, want an error for an unknown agent")
	}
}

func TestStopRunDoesNotStopAnInstanceItDidNotLaunch(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	envs := &fakeEnvironmentRunner{}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRunInInstance("greeter", "eval-container-1")
	if err != nil {
		t.Fatalf("StartRunInInstance() error = %v", err)
	}

	if err := m.StopRun(run.ID); err != nil {
		t.Fatalf("StopRun() error = %v", err)
	}
	if len(envs.stoppedIDs) != 0 {
		t.Errorf("envs.stoppedIDs = %v, want none stopped — the caller (Evaluations) owns this instance's lifecycle", envs.stoppedIDs)
	}
	if _, ok := m.GetRun(run.ID); ok {
		t.Error("GetRun() found a run after StopRun(), want it removed from the in-memory run map regardless")
	}
}

func TestSendMessageSuccess(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	llm := &fakeLLM{responses: []string{"hello there!"}}
	m := NewManager(context.Background(), agents, llm, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("greeter")
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
	m := NewManager(context.Background(), agents, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, bus)

	run, err := m.StartRun("narrator")
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
			Name:        "researcher",
			Environment: "WebSearch",
			Graph:       toolGraph(registry.NodeData{ToolName: "web_search", ToolArgs: map[string]string{"query": "{{Input}}"}}),
		},
	}}
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}, toolOutput: "search results"}
	envReader := &fakeEnvironmentReader{envs: map[string]registry.Environment{
		"WebSearch": {Name: "WebSearch", Tools: []string{"web_search"}},
	}}
	toolStore := &fakeToolReader{tools: map[string]registry.Tool{
		"web_search": {
			Name:       "web_search",
			Command:    "curl -s {{query}}",
			Parameters: []registry.ToolParameter{{Name: "query", Type: registry.ToolParamString, Required: true}},
		},
	}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, envReader, toolStore, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("researcher")
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

func TestSendMessageUnknownEnvironment(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{
		"researcher": {Name: "researcher", Environment: "does-not-exist", Graph: linearGraph()},
	}}
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, envs, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("researcher")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if _, err := m.SendMessage(run.ID, "hi"); err == nil {
		t.Error("SendMessage() error = nil, want an error when the agent's Environment can't be resolved")
	}
}

func TestSendMessageUnknownRun(t *testing.T) {
	m := NewManager(context.Background(), &fakeAgentReader{}, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	if _, err := m.SendMessage("does-not-exist", "hi"); err == nil {
		t.Error("SendMessage() error = nil, want an error for an unknown run")
	}
}

func TestSendMessageRequiresMessage(t *testing.T) {
	agents := &fakeAgentReader{agents: map[string]registry.Agent{"greeter": {Name: "greeter", Graph: linearGraph()}}}
	m := NewManager(context.Background(), agents, &fakeLLM{}, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("greeter")
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
	m := NewManager(context.Background(), agents, llm, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("greeter")
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
	m := NewManager(context.Background(), agents, llm, &fakeEnvironmentRunner{}, &fakeEnvironmentReader{}, &fakeToolReader{}, &fakeKnowledgeReader{}, eventbus.New())

	run, err := m.StartRun("greeter")
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
