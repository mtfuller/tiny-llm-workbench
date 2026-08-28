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

func newDebugTestManager(llm *fakeLLM, envs environmentRunner, tools toolReader) *Manager {
	envReader := &fakeEnvironmentReader{envs: map[string]registry.Environment{
		"WebSearch": {Name: "WebSearch", Tools: []string{"echo_tool"}},
	}}
	return NewManager(context.Background(), &fakeAgentReader{}, llm, envs, envReader, tools, &fakeKnowledgeReader{}, eventbus.New())
}

func TestStartDebugRunNoEnvironment(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})

	state, err := m.StartDebugRun("greeter", linearGraph(), "")
	if err != nil {
		t.Fatalf("StartDebugRun() error = %v", err)
	}
	if state.InstanceID != "" {
		t.Errorf("state.InstanceID = %q, want empty for no Environment", state.InstanceID)
	}
	if state.Finished || state.PendingNodeID != "" {
		t.Errorf("state = %+v, want idle (no turn started yet)", state)
	}
}

func TestStartDebugRunLaunchesEnvironment(t *testing.T) {
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := newDebugTestManager(&fakeLLM{}, envs, &fakeToolReader{})

	state, err := m.StartDebugRun("researcher", linearGraph(), "WebSearch")
	if err != nil {
		t.Fatalf("StartDebugRun() error = %v", err)
	}
	if state.InstanceID != "container-1" {
		t.Errorf("state.InstanceID = %q, want %q", state.InstanceID, "container-1")
	}
	if len(envs.launched) != 1 || envs.launched[0] != "WebSearch" {
		t.Errorf("envs.launched = %v, want [WebSearch]", envs.launched)
	}
}

func TestStartDebugRunInvalidGraph(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})

	graph := registry.Graph{Nodes: []registry.Node{{ID: "p1", Type: "prompt"}}}
	if _, err := m.StartDebugRun("greeter", graph, ""); err == nil {
		t.Error("StartDebugRun() error = nil, want an error for a graph with no input node")
	}
}

func TestSendDebugMessageSetsPendingToInput(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")

	state, err := m.SendDebugMessage(started.ID, "hi")
	if err != nil {
		t.Fatalf("SendDebugMessage() error = %v", err)
	}
	if state.PendingNodeID != "in" || state.PendingNodeType != "input" {
		t.Errorf("state = %+v, want pending at the input node", state)
	}
	if state.Finished {
		t.Error("state.Finished = true, want false — nothing has been stepped yet")
	}
}

func TestSendDebugMessageRequiresMessage(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")

	if _, err := m.SendDebugMessage(started.ID, ""); err == nil {
		t.Error("SendDebugMessage() error = nil, want an error for an empty message")
	}
}

func TestSendDebugMessageUnknownRun(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})

	if _, err := m.SendDebugMessage("does-not-exist", "hi"); err == nil {
		t.Error("SendDebugMessage() error = nil, want an error for an unknown debug run")
	}
}

func TestSendDebugMessageMidTurnErrors(t *testing.T) {
	llm := &fakeLLM{responses: []string{"hello!"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")
	m.SendDebugMessage(started.ID, "hi")
	m.StepDebugRun(started.ID) // steps input, pending now = prompt node

	if _, err := m.SendDebugMessage(started.ID, "another message"); err == nil {
		t.Error("SendDebugMessage() error = nil, want an error while a turn is still in progress")
	}
}

func TestStepDebugRunWalksToCompletionAndRecordsMessages(t *testing.T) {
	llm := &fakeLLM{responses: []string{"hello there!"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")
	m.SendDebugMessage(started.ID, "hi")

	// Step 1: input node — output equals the raw message.
	s1, err := m.StepDebugRun(started.ID)
	if err != nil {
		t.Fatalf("StepDebugRun() (1st) error = %v", err)
	}
	if s1.LastStep == nil || s1.LastStep.NodeType != "input" || s1.LastStep.Output != "hi" {
		t.Errorf("s1.LastStep = %+v, want the input node's own output (%q)", s1.LastStep, "hi")
	}
	if s1.Finished {
		t.Error("s1.Finished = true, want false — the prompt node hasn't run yet")
	}
	if s1.PendingNodeID != "p1" {
		t.Errorf("s1.PendingNodeID = %q, want %q", s1.PendingNodeID, "p1")
	}

	// Step 2: prompt node — a dead end, so the turn finishes here.
	s2, err := m.StepDebugRun(started.ID)
	if err != nil {
		t.Fatalf("StepDebugRun() (2nd) error = %v", err)
	}
	if s2.LastStep == nil || s2.LastStep.NodeType != "prompt" || s2.LastStep.Output != "hello there!" {
		t.Errorf("s2.LastStep = %+v, want the prompt node's own reply", s2.LastStep)
	}
	if !s2.Finished || s2.PendingNodeID != "" {
		t.Errorf("s2 = %+v, want finished with nothing pending", s2)
	}
	if len(s2.Messages) != 2 || s2.Messages[0].Role != "user" || s2.Messages[1].Role != "assistant" || s2.Messages[1].Content != "hello there!" {
		t.Errorf("s2.Messages = %+v, want [user hi, assistant \"hello there!\"]", s2.Messages)
	}
}

// Stepping a prompt node whose OutputSchema rejects the reply follows its
// "fail" handle in the debugger, exactly as Run does — no hard error.
func TestStepDebugRunFollowsSchemaFailHandle(t *testing.T) {
	schema := `{"type":"object","required":["city"]}`
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Classifier", Model: "m", OutputSchema: schema}},
			{ID: "p2", Type: "prompt", Data: registry.NodeData{Name: "Recover", Model: "m", PromptTemplate: "recovered"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", SourceHandle: "fail", Target: "p2"},
		},
	}
	llm := &fakeLLM{responses: []string{"not json", "recovered reply"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", graph, "")
	m.SendDebugMessage(started.ID, "hi")

	m.StepDebugRun(started.ID) // input
	s2, err := m.StepDebugRun(started.ID)
	if err != nil {
		t.Fatalf("StepDebugRun() (schema-failing prompt) error = %v, want the fail handle followed", err)
	}
	if s2.PendingNodeID != "p2" {
		t.Errorf("s2.PendingNodeID = %q, want %q (routed down the fail handle)", s2.PendingNodeID, "p2")
	}
}

func TestStepDebugRunNothingPendingErrors(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")

	if _, err := m.StepDebugRun(started.ID); err == nil {
		t.Error("StepDebugRun() error = nil, want an error when no message has been sent yet")
	}
}

func TestStepDebugRunFailureLeavesSessionUnchanged(t *testing.T) {
	llm := &fakeLLM{err: errors.New("model runner unreachable")}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")
	m.SendDebugMessage(started.ID, "hi")
	m.StepDebugRun(started.ID) // input node succeeds regardless

	if _, err := m.StepDebugRun(started.ID); err == nil {
		t.Fatal("StepDebugRun() error = nil, want the LLM error to propagate")
	}

	state, ok := m.GetDebugRun(started.ID)
	if !ok {
		t.Fatal("GetDebugRun() ok = false, want the session to still exist after a failed step")
	}
	if state.PendingNodeID != "p1" || state.Finished {
		t.Errorf("state = %+v, want still pending at the prompt node, not finished", state)
	}
}

func TestRetryDebugRunGetsAFreshResult(t *testing.T) {
	llm := &fakeLLM{responses: []string{"first reply", "second reply"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")
	m.SendDebugMessage(started.ID, "hi")
	m.StepDebugRun(started.ID) // input
	first, err := m.StepDebugRun(started.ID)
	if err != nil {
		t.Fatalf("StepDebugRun() error = %v", err)
	}
	if first.LastStep.Output != "first reply" || !first.Finished {
		t.Fatalf("first = %+v, want finished with %q", first, "first reply")
	}

	retried, err := m.RetryDebugRun(started.ID)
	if err != nil {
		t.Fatalf("RetryDebugRun() error = %v", err)
	}
	if retried.LastStep.Output != "second reply" {
		t.Errorf("retried.LastStep.Output = %q, want %q", retried.LastStep.Output, "second reply")
	}
	if !retried.Finished {
		t.Error("retried.Finished = false, want true — retrying a dead-end node still finishes the turn")
	}
	// The finished turn's assistant message should reflect the retried
	// (second) reply, not the discarded first one.
	last := retried.Messages[len(retried.Messages)-1]
	if last.Content != "second reply" {
		t.Errorf("final assistant message = %q, want the retried reply %q", last.Content, "second reply")
	}
}

func TestRetryDebugRunNothingToRetryErrors(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("greeter", linearGraph(), "")
	m.SendDebugMessage(started.ID, "hi")

	if _, err := m.RetryDebugRun(started.ID); err == nil {
		t.Error("RetryDebugRun() error = nil, want an error before any node has been stepped")
	}
}

func TestRetryDebugRunDiscardsStaleDownstreamReference(t *testing.T) {
	// Classifier (OutputSchema requiring "city") -> Responder, referencing
	// {{Classifier.city}}. Retrying Classifier with a different reply must
	// make Responder see the NEW city, not the first attempt's.
	schema := `{"type":"object","required":["city"],"properties":{"city":{"type":"string"}}}`
	graph := schemaChainGraph(schema, "please help with {{Classifier.city}}")
	llm := &fakeLLM{responses: []string{`{"city": "Paris"}`, `{"city": "Tokyo"}`, "final reply"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})

	started, _ := m.StartDebugRun("planner", graph, "")
	m.SendDebugMessage(started.ID, "where should I go?")
	m.StepDebugRun(started.ID) // input
	m.StepDebugRun(started.ID) // Classifier -> {"city": "Paris"}

	if _, err := m.RetryDebugRun(started.ID); err != nil {
		t.Fatalf("RetryDebugRun() error = %v", err)
	}

	final, err := m.StepDebugRun(started.ID) // Responder, should see Tokyo now
	if err != nil {
		t.Fatalf("StepDebugRun() (Responder) error = %v", err)
	}
	if !final.Finished {
		t.Fatalf("final = %+v, want finished", final)
	}
	if len(llm.calls) != 3 || !strings.Contains(llm.calls[2], "please help with Tokyo") {
		t.Errorf("llm.calls[2] = %q, want it to reference the retried city (Tokyo), not the discarded Paris", llm.calls[2])
	}
}

func TestStopDebugRunStopsOwnedInstance(t *testing.T) {
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}}
	m := newDebugTestManager(&fakeLLM{}, envs, &fakeToolReader{})
	started, _ := m.StartDebugRun("researcher", linearGraph(), "WebSearch")

	if err := m.StopDebugRun(started.ID); err != nil {
		t.Fatalf("StopDebugRun() error = %v", err)
	}
	if len(envs.stoppedIDs) != 1 || envs.stoppedIDs[0] != "container-1" {
		t.Errorf("envs.stoppedIDs = %v, want [container-1]", envs.stoppedIDs)
	}
	if _, ok := m.GetDebugRun(started.ID); ok {
		t.Error("GetDebugRun() found a session after StopDebugRun(), want it removed")
	}
}

func TestStopDebugRunUnknownRunIsNotAnError(t *testing.T) {
	m := newDebugTestManager(&fakeLLM{}, &fakeEnvironmentRunner{}, &fakeToolReader{})

	if err := m.StopDebugRun("does-not-exist"); err != nil {
		t.Errorf("StopDebugRun() error = %v, want nil for an unknown session (idempotent cleanup)", err)
	}
}

func TestDebugRunToolNodeUsesSessionInstance(t *testing.T) {
	envs := &fakeEnvironmentRunner{launchResult: environments.Instance{ID: "container-1"}, toolOutput: "tool output"}
	toolStore := &fakeToolReader{tools: map[string]registry.Tool{"echo_tool": {Name: "echo_tool", Command: "echo hi"}}}
	m := newDebugTestManager(&fakeLLM{}, envs, toolStore)

	graph := toolGraph(registry.NodeData{ToolName: "echo_tool"})
	started, _ := m.StartDebugRun("worker", graph, "WebSearch")
	m.SendDebugMessage(started.ID, "hi")
	m.StepDebugRun(started.ID) // input

	final, err := m.StepDebugRun(started.ID) // tool
	if err != nil {
		t.Fatalf("StepDebugRun() error = %v", err)
	}
	if final.LastStep.Output != "tool output" || !final.Finished {
		t.Errorf("final = %+v, want finished with %q", final, "tool output")
	}
}

func TestRetryDebugRunLoopNodeDoesNotDoubleIncrement(t *testing.T) {
	// input -> L(loop_start, name L) --body--> P(prompt, template
	// "{{L.iteration}}"). Stepping L once sets iteration=1; retrying L must
	// re-run it from the pre-increment snapshot, yielding iteration=1 again,
	// not 2.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 5}},
			{ID: "p", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "iter {{L.iteration}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "p"},
		},
	}
	llm := &fakeLLM{responses: []string{"p reply"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("looper", graph, "")
	m.SendDebugMessage(started.ID, "go")
	m.StepDebugRun(started.ID) // input -> pending L
	m.StepDebugRun(started.ID) // L: iteration 1 -> pending P

	if _, err := m.RetryDebugRun(started.ID); err != nil { // re-run L
		t.Fatalf("RetryDebugRun() error = %v", err)
	}
	if _, err := m.StepDebugRun(started.ID); err != nil { // P
		t.Fatalf("StepDebugRun() (P) error = %v", err)
	}
	if len(llm.calls) != 1 || !strings.Contains(llm.calls[0], "iter 1") {
		t.Errorf("llm.calls[0] = %q, want it to reference iteration 1 (retry must not double-count)", llm.calls[0])
	}
}

func TestRetryDebugRunAgentNodeReRunsInternalLoop(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "a1", Type: "agent", Data: registry.NodeData{Model: "m", AgentInstructions: "answer"}},
		},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "a1"}},
	}
	llm := &fakeLLM{responses: []string{"FINAL: first", "FINAL: second"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("assistant", graph, "")
	m.SendDebugMessage(started.ID, "go")
	m.StepDebugRun(started.ID) // input

	first, err := m.StepDebugRun(started.ID) // agent
	if err != nil {
		t.Fatalf("StepDebugRun() (agent) error = %v", err)
	}
	if first.LastStep.Output != "first" || !first.Finished {
		t.Fatalf("first = %+v, want finished with %q", first, "first")
	}

	retried, err := m.RetryDebugRun(started.ID)
	if err != nil {
		t.Fatalf("RetryDebugRun() error = %v", err)
	}
	if retried.LastStep.Output != "second" {
		t.Errorf("retried.LastStep.Output = %q, want %q (agent node's internal loop re-ran)", retried.LastStep.Output, "second")
	}
	if last := retried.Messages[len(retried.Messages)-1]; last.Content != "second" {
		t.Errorf("final assistant message = %q, want %q", last.Content, "second")
	}
}

func TestStepDebugRunFollowsLoopEndBackToStart(t *testing.T) {
	// input -> L(loop_start, max 3) --body--> W(prompt) -> LE(loop_end -> L);
	// L --done--> F(prompt). Stepping should visit W three times (the walk
	// jumping LE -> L each time) before L routes "done" to F.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 3}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "W", Model: "m", SystemPrompt: "w"}},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "f", Type: "prompt", Data: registry.NodeData{Name: "F", Model: "m", SystemPrompt: "f"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "w"},
			{ID: "e3", Source: "w", Target: "le"},
			{ID: "e4", Source: "l", SourceHandle: "done", Target: "f"},
		},
	}
	llm := &fakeLLM{responses: []string{"w1", "w2", "w3", "f reply"}}
	m := newDebugTestManager(llm, &fakeEnvironmentRunner{}, &fakeToolReader{})
	started, _ := m.StartDebugRun("looper", graph, "")
	m.SendDebugMessage(started.ID, "go")

	var visited []string
	for i := 0; i < 20; i++ {
		s, err := m.StepDebugRun(started.ID)
		if err != nil {
			t.Fatalf("StepDebugRun() error = %v", err)
		}
		visited = append(visited, s.LastStep.NodeType)
		if s.Finished {
			break
		}
	}

	prompts := 0
	for _, v := range visited {
		if v == "prompt" {
			prompts++
		}
	}
	if prompts != 4 {
		t.Errorf("prompt steps = %d (sequence %v), want 4 (W three times + F once)", prompts, visited)
	}
	if visited[len(visited)-1] != "prompt" {
		t.Errorf("last step = %q, want the F prompt node", visited[len(visited)-1])
	}
}
