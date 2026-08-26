package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeLLM struct {
	responses []string // returned in order, one per call
	calls     []string // prompts received, in order
	err       error
}

func (f *fakeLLM) Generate(ctx context.Context, model, prompt string) (string, error) {
	f.calls = append(f.calls, prompt)
	if f.err != nil {
		return "", f.err
	}
	if len(f.calls) > len(f.responses) {
		return "", errors.New("fakeLLM: no more canned responses")
	}
	return f.responses[len(f.calls)-1], nil
}

type fakeTools struct {
	output      string
	err         error
	calls       []string // commands received, in order
	instanceIDs []string
}

func (f *fakeTools) RunToolSync(ctx context.Context, instanceID, command string) (string, error) {
	f.instanceIDs = append(f.instanceIDs, instanceID)
	f.calls = append(f.calls, command)
	if f.err != nil {
		return "", f.err
	}
	return f.output, nil
}

// linearGraph is input -> prompt -> output.
func linearGraph() registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "qwen2.5:0.5b", SystemPrompt: "Be nice."}},
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", Target: "out"},
		},
	}
}

func TestRunLinearGraph(t *testing.T) {
	llm := &fakeLLM{responses: []string{"hello there!"}}
	engine := NewEngine(llm, &fakeTools{})

	var steps []StepEvent
	reply, err := engine.Run(context.Background(), linearGraph(), nil, "hi", "", func(s StepEvent) { steps = append(steps, s) })
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "hello there!" {
		t.Errorf("Run() = %q, want %q", reply, "hello there!")
	}
	if len(steps) != 3 || steps[0].NodeType != "input" || steps[1].NodeType != "prompt" || steps[2].NodeType != "output" {
		t.Errorf("steps = %+v, want input, prompt, output in order", steps)
	}
	if len(llm.calls) != 1 || !strings.Contains(llm.calls[0], "Be nice.") || !strings.Contains(llm.calls[0], "USER: hi") {
		t.Errorf("llm.calls = %v, want a single prompt containing the system prompt and user message", llm.calls)
	}
}

func TestRunIncludesHistory(t *testing.T) {
	llm := &fakeLLM{responses: []string{"sure!"}}
	engine := NewEngine(llm, &fakeTools{})

	history := []ChatMessage{{Role: "user", Content: "remember X"}, {Role: "assistant", Content: "ok, remembered"}}
	_, err := engine.Run(context.Background(), linearGraph(), history, "what did I say?", "", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "USER: remember X") || !strings.Contains(llm.calls[0], "ASSISTANT: ok, remembered") {
		t.Errorf("llm.calls[0] = %q, want it to include prior history", llm.calls[0])
	}
}

// decisionGraph is input -> decision (yes: prompt "matched", no: prompt
// "unmatched") -> output.
func decisionGraph(keyword string) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "d1", Type: "decision", Data: registry.NodeData{Keyword: keyword}},
			{ID: "yes", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "matched branch"}},
			{ID: "no", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "unmatched branch"}},
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "d1"},
			{ID: "e2", Source: "d1", SourceHandle: "yes", Target: "yes"},
			{ID: "e3", Source: "d1", SourceHandle: "no", Target: "no"},
			{ID: "e4", Source: "yes", Target: "out"},
			{ID: "e5", Source: "no", Target: "out"},
		},
	}
}

func TestRunDecisionTakesYesBranch(t *testing.T) {
	llm := &fakeLLM{responses: []string{"took yes branch"}}
	engine := NewEngine(llm, &fakeTools{})

	reply, err := engine.Run(context.Background(), decisionGraph("weather"), nil, "what's the weather?", "", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "took yes branch" {
		t.Errorf("Run() = %q, want %q", reply, "took yes branch")
	}
	if !strings.Contains(llm.calls[0], "matched branch") {
		t.Errorf("llm.calls[0] = %q, want the yes-branch prompt node's system prompt", llm.calls[0])
	}
}

func TestRunDecisionTakesNoBranch(t *testing.T) {
	llm := &fakeLLM{responses: []string{"took no branch"}}
	engine := NewEngine(llm, &fakeTools{})

	reply, err := engine.Run(context.Background(), decisionGraph("weather"), nil, "tell me a joke", "", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "took no branch" {
		t.Errorf("Run() = %q, want %q", reply, "took no branch")
	}
	if !strings.Contains(llm.calls[0], "unmatched branch") {
		t.Errorf("llm.calls[0] = %q, want the no-branch prompt node's system prompt", llm.calls[0])
	}
}

func TestRunDecisionKeywordCaseInsensitive(t *testing.T) {
	llm := &fakeLLM{responses: []string{"took yes branch"}}
	engine := NewEngine(llm, &fakeTools{})

	_, err := engine.Run(context.Background(), decisionGraph("Weather"), nil, "WEATHER report please", "", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "matched branch") {
		t.Error("keyword match should be case-insensitive")
	}
}

func TestRunNoInputNode(t *testing.T) {
	graph := registry.Graph{Nodes: []registry.Node{{ID: "out", Type: "output"}}}
	engine := NewEngine(&fakeLLM{}, &fakeTools{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil); err == nil {
		t.Error("Run() error = nil, want an error for a graph with no input node")
	}
}

func TestRunMultipleInputNodes(t *testing.T) {
	graph := registry.Graph{Nodes: []registry.Node{{ID: "in1", Type: "input"}, {ID: "in2", Type: "input"}}}
	engine := NewEngine(&fakeLLM{}, &fakeTools{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil); err == nil {
		t.Error("Run() error = nil, want an error for a graph with more than one input node")
	}
}

func TestRunDeadEndNode(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{{ID: "in", Type: "input"}, {ID: "p1", Type: "prompt"}},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "p1"}},
	}
	engine := NewEngine(&fakeLLM{responses: []string{"reply"}}, &fakeTools{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil); err == nil {
		t.Error("Run() error = nil, want an error when a non-output node has no outgoing edge")
	}
}

func TestRunCycleHitsMaxSteps(t *testing.T) {
	// decision with no keyword always takes "no", which loops back to itself.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "d1", Type: "decision"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "d1"},
			{ID: "e2", Source: "d1", SourceHandle: "no", Target: "d1"},
		},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil); err == nil {
		t.Error("Run() error = nil, want an error when the graph cycles past the step limit")
	}
}

func TestRunPromptNodeLLMError(t *testing.T) {
	llm := &fakeLLM{err: errors.New("model runner unreachable")}
	engine := NewEngine(llm, &fakeTools{})

	if _, err := engine.Run(context.Background(), linearGraph(), nil, "hi", "", nil); err == nil {
		t.Error("Run() error = nil, want the LLM error to propagate")
	}
}

// toolGraph is input -> tool -> output.
func toolGraph(command string) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "t1", Type: "tool", Data: registry.NodeData{Command: command}},
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "t1"},
			{ID: "e2", Source: "t1", Target: "out"},
		},
	}
}

func TestRunToolNode(t *testing.T) {
	tools := &fakeTools{output: "tool output"}
	engine := NewEngine(&fakeLLM{}, tools)

	reply, err := engine.Run(context.Background(), toolGraph("echo hi"), nil, "hi", "container-1", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "tool output" {
		t.Errorf("Run() = %q, want %q", reply, "tool output")
	}
	if len(tools.calls) != 1 || tools.calls[0] != "echo hi" {
		t.Errorf("tools.calls = %v, want [echo hi]", tools.calls)
	}
	if len(tools.instanceIDs) != 1 || tools.instanceIDs[0] != "container-1" {
		t.Errorf("tools.instanceIDs = %v, want [container-1]", tools.instanceIDs)
	}
}

func TestRunToolNodeTemplatesInput(t *testing.T) {
	tools := &fakeTools{output: "done"}
	engine := NewEngine(&fakeLLM{}, tools)

	_, err := engine.Run(context.Background(), toolGraph(`curl -s "{{input}}"`), nil, "https://example.com", "container-1", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tools.calls) != 1 || tools.calls[0] != `curl -s "https://example.com"` {
		t.Errorf("tools.calls = %v, want the {{input}} placeholder replaced with the user message", tools.calls)
	}
}

func TestRunToolNodeRequiresInstance(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{})

	if _, err := engine.Run(context.Background(), toolGraph("echo hi"), nil, "hi", "", nil); err == nil {
		t.Error("Run() error = nil, want an error when a tool node runs with no Environment instance")
	}
}

func TestRunToolNodeError(t *testing.T) {
	tools := &fakeTools{err: errors.New("container not running")}
	engine := NewEngine(&fakeLLM{}, tools)

	if _, err := engine.Run(context.Background(), toolGraph("echo hi"), nil, "hi", "container-1", nil); err == nil {
		t.Error("Run() error = nil, want the tool error to propagate")
	}
}
