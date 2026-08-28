package agents

import (
	"context"
	"errors"
	"fmt"
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

// fakeKnowledgeReader resolves registry.KnowledgeBase lookups for tests,
// keyed by name.
type fakeKnowledgeReader struct {
	bases map[string]registry.KnowledgeBase
}

func (f *fakeKnowledgeReader) GetKnowledgeBase(name string) (registry.KnowledgeBase, error) {
	kb, ok := f.bases[name]
	if !ok {
		return registry.KnowledgeBase{}, errors.New("not found")
	}
	return kb, nil
}

// linearGraph is input -> prompt, with the prompt node left as a dead
// end — there's no dedicated "output" node type; a node with no outgoing
// edge is simply where the turn ends.
func linearGraph() registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "qwen2.5:0.5b", SystemPrompt: "Be nice."}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
		},
	}
}

func TestRunLinearGraph(t *testing.T) {
	llm := &fakeLLM{responses: []string{"hello there!"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	var steps []StepEvent
	reply, err := engine.Run(context.Background(), linearGraph(), nil, "hi", "", nil, &RunHooks{OnStep: func(s StepEvent) { steps = append(steps, s) }})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "hello there!" {
		t.Errorf("Run() = %q, want %q", reply, "hello there!")
	}
	if len(steps) != 2 || steps[0].NodeType != "input" || steps[1].NodeType != "prompt" {
		t.Errorf("steps = %+v, want input, prompt in order", steps)
	}
	if steps[1].Output != "hello there!" {
		t.Errorf("steps[1].Output = %q, want the prompt node's own reply (%q), not the input passed into it", steps[1].Output, "hello there!")
	}
	if len(llm.calls) != 1 || !strings.Contains(llm.calls[0], "Be nice.") || !strings.Contains(llm.calls[0], "USER: hi") {
		t.Errorf("llm.calls = %v, want a single prompt containing the system prompt and user message", llm.calls)
	}
}

func TestRunIncludesHistory(t *testing.T) {
	llm := &fakeLLM{responses: []string{"sure!"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	history := []ChatMessage{{Role: "user", Content: "remember X"}, {Role: "assistant", Content: "ok, remembered"}}
	_, err := engine.Run(context.Background(), linearGraph(), history, "what did I say?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "USER: remember X") || !strings.Contains(llm.calls[0], "ASSISTANT: ok, remembered") {
		t.Errorf("llm.calls[0] = %q, want it to include prior history", llm.calls[0])
	}
}

// conditionGraph is input -> condition (pass: prompt "matched", fail: prompt
// "unmatched"), with each branch's prompt node left as a dead end. The
// condition is a case-insensitive "contains value" check.
func conditionGraph(value string) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "c1", Type: "condition", Data: registry.NodeData{ConditionType: "contains", ConditionValue: value}},
			{ID: "yes", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "matched branch"}},
			{ID: "no", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "unmatched branch"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "c1"},
			{ID: "e2", Source: "c1", SourceHandle: "pass", Target: "yes"},
			{ID: "e3", Source: "c1", SourceHandle: "fail", Target: "no"},
		},
	}
}

// switchGraph: input -> switch(match cases) with a prompt per handle plus a
// default prompt, each a dead end.
func switchGraph(cases []registry.SwitchCase, matchTemplate string) registry.Graph {
	nodes := []registry.Node{
		{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
		{ID: "sw", Type: "switch", Data: registry.NodeData{Name: "Router", SwitchCases: cases, MatchTemplate: matchTemplate}},
		{ID: "def", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "default branch"}},
	}
	edges := []registry.Edge{
		{ID: "e-in", Source: "in", Target: "sw"},
		{ID: "e-def", Source: "sw", SourceHandle: "default", Target: "def"},
	}
	for i, c := range cases {
		id := fmt.Sprintf("p%d", i)
		nodes = append(nodes, registry.Node{ID: id, Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "branch " + c.Value}})
		edges = append(edges, registry.Edge{ID: "e" + id, Source: "sw", SourceHandle: c.Value, Target: id})
	}
	return registry.Graph{Nodes: nodes, Edges: edges}
}

func TestRunSwitchTakesFirstMatchingCase(t *testing.T) {
	llm := &fakeLLM{responses: []string{"routed"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})
	graph := switchGraph([]registry.SwitchCase{{Value: "billing"}, {Value: "shipping"}}, "")

	if _, err := engine.Run(context.Background(), graph, nil, "I have a SHIPPING question", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "branch shipping") {
		t.Errorf("llm.calls[0] = %q, want the shipping-case branch (case-insensitive substring)", llm.calls[0])
	}
}

func TestRunSwitchFallsThroughToDefault(t *testing.T) {
	llm := &fakeLLM{responses: []string{"routed"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})
	graph := switchGraph([]registry.SwitchCase{{Value: "billing"}, {Value: "shipping"}}, "")

	if _, err := engine.Run(context.Background(), graph, nil, "something unrelated", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "default branch") {
		t.Errorf("llm.calls[0] = %q, want the default branch", llm.calls[0])
	}
}

func TestRunSwitchMatchTemplate(t *testing.T) {
	llm := &fakeLLM{responses: []string{"routed"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})
	// Match against the Input node's output via a template rather than the
	// (here identical) inbound value, to exercise rendering.
	graph := switchGraph([]registry.SwitchCase{{Value: "urgent"}}, "priority: {{Input}}")

	if _, err := engine.Run(context.Background(), graph, nil, "urgent", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "branch urgent") {
		t.Errorf("llm.calls[0] = %q, want the urgent branch matched via the template", llm.calls[0])
	}
}

// A say node streams a user-facing progress message via hooks.OnMessage,
// then passes its text on; the turn's reply is still the terminal node's
// output (no say node was marked final).
func TestRunSayNodeEmitsProgressMessage(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "s1", Type: "say", Data: registry.NodeData{Name: "Status", SayTemplate: "working on: {{Input}}"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "answer {{Input}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "s1"},
			{ID: "e2", Source: "s1", Target: "p1"},
		},
	}
	llm := &fakeLLM{responses: []string{"the final answer"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	var msgs []TurnMessage
	reply, err := engine.Run(context.Background(), graph, nil, "the task", "", nil, &RunHooks{
		OnMessage: func(m TurnMessage) { msgs = append(msgs, m) },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "the final answer" {
		t.Errorf("Run() = %q, want the terminal prompt's output", reply)
	}
	if len(msgs) != 1 || msgs[0].Kind != "progress" || msgs[0].Content != "working on: the task" {
		t.Errorf("msgs = %+v, want one progress message %q", msgs, "working on: the task")
	}
}

// A say node marked final becomes the turn's reply even when the walk
// continues past it to end on a different node.
func TestRunSayNodeFinalOverridesTerminalOutput(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "s1", Type: "say", Data: registry.NodeData{Name: "Answer", SayTemplate: "here is your answer", SayFinal: true}},
			{ID: "s2", Type: "say", Data: registry.NodeData{Name: "Cleanup", SayTemplate: "tidying up"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "s1"},
			{ID: "e2", Source: "s1", Target: "s2"},
		},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	var msgs []TurnMessage
	reply, err := engine.Run(context.Background(), graph, nil, "go", "", nil, &RunHooks{
		OnMessage: func(m TurnMessage) { msgs = append(msgs, m) },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "here is your answer" {
		t.Errorf("Run() = %q, want the say-final message, not the terminal node's output", reply)
	}
	if len(msgs) != 2 || msgs[0].Kind != "final" || msgs[1].Kind != "progress" {
		t.Errorf("msgs = %+v, want [final, progress]", msgs)
	}
}

// An empty say template falls back to the inbound value (same convention as
// promptTemplate / matchTemplate).
func TestRunSayNodeEmptyTemplateFallsBackToInput(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "s1", Type: "say", Data: registry.NodeData{Name: "Echo", SayFinal: true}},
		},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "s1"}},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "just echo me", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "just echo me" {
		t.Errorf("Run() = %q, want the inbound value echoed", reply)
	}
}

func TestRunConditionTakesPassBranch(t *testing.T) {
	llm := &fakeLLM{responses: []string{"took pass branch"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), conditionGraph("weather"), nil, "what's the weather?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "took pass branch" {
		t.Errorf("Run() = %q, want %q", reply, "took pass branch")
	}
	if !strings.Contains(llm.calls[0], "matched branch") {
		t.Errorf("llm.calls[0] = %q, want the pass-branch prompt node's system prompt", llm.calls[0])
	}
}

func TestRunConditionTakesFailBranch(t *testing.T) {
	llm := &fakeLLM{responses: []string{"took fail branch"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), conditionGraph("weather"), nil, "tell me a joke", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "took fail branch" {
		t.Errorf("Run() = %q, want %q", reply, "took fail branch")
	}
	if !strings.Contains(llm.calls[0], "unmatched branch") {
		t.Errorf("llm.calls[0] = %q, want the fail-branch prompt node's system prompt", llm.calls[0])
	}
}

func TestRunConditionValueCaseInsensitive(t *testing.T) {
	llm := &fakeLLM{responses: []string{"took pass branch"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	_, err := engine.Run(context.Background(), conditionGraph("Weather"), nil, "WEATHER report please", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "matched branch") {
		t.Error("contains check should be case-insensitive")
	}
}

func TestRunConditionRegexMode(t *testing.T) {
	llm := &fakeLLM{responses: []string{"matched"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "c1", Type: "condition", Data: registry.NodeData{ConditionType: "regex", ConditionValue: `\bGOOD\b`}},
			{ID: "yes", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "matched branch"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "c1"},
			{ID: "e2", Source: "c1", SourceHandle: "pass", Target: "yes"},
		},
	}
	if _, err := engine.Run(context.Background(), graph, nil, "the plan looks GOOD to me", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "matched branch") {
		t.Error("regex condition should have routed to the pass branch")
	}
}

func TestRunConditionInvalidTypeErrors(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "c1", Type: "condition", Data: registry.NodeData{}}, // no ConditionType configured
		},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "c1"}},
	}
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error for a condition node with no check type set")
	}
}

func TestRunNoInputNode(t *testing.T) {
	graph := registry.Graph{Nodes: []registry.Node{{ID: "p1", Type: "prompt"}}}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error for a graph with no input node")
	}
}

func TestRunMultipleInputNodes(t *testing.T) {
	graph := registry.Graph{Nodes: []registry.Node{{ID: "in1", Type: "input"}, {ID: "in2", Type: "input"}}}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error for a graph with more than one input node")
	}
}

func TestRunDeadEndNodeIsAValidTerminal(t *testing.T) {
	// There's no dedicated "output" node type — any node with no outgoing
	// edge is simply where the turn ends, and its own output is the reply.
	graph := registry.Graph{
		Nodes: []registry.Node{{ID: "in", Type: "input"}, {ID: "p1", Type: "prompt"}},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "p1"}},
	}
	engine := NewEngine(&fakeLLM{responses: []string{"reply"}}, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want a dead-end node to terminate the turn successfully", err)
	}
	if reply != "reply" {
		t.Errorf("Run() = %q, want %q", reply, "reply")
	}
}

func TestRunUnboundedCycleHitsMaxSteps(t *testing.T) {
	// A condition that can never pass ("contains zzz" against "hi"), with its
	// fail handle wired back to itself and no loop node capping it.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "c1", Type: "condition", Data: registry.NodeData{ConditionType: "contains", ConditionValue: "zzz"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "c1"},
			{ID: "e2", Source: "c1", SourceHandle: "fail", Target: "c1"},
		},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when an unbounded cycle passes the step limit")
	}
}

func TestRunPromptNodeLLMError(t *testing.T) {
	llm := &fakeLLM{err: errors.New("model runner unreachable")}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), linearGraph(), nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want the LLM error to propagate")
	}
}

func TestRunDuplicateNodeNames(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Step"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Step", Model: "m"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
		},
	}
	engine := NewEngine(&fakeLLM{responses: []string{"reply"}}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when two nodes share a name")
	}
}

// toolGraph is input(named "Input") -> tool, with the tool node configured
// by data and left as a dead end.
func toolGraph(data registry.NodeData) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "t1", Type: "tool", Data: data},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "t1"},
		},
	}
}

func TestRunToolNode(t *testing.T) {
	tools := &fakeTools{output: "tool output"}
	toolDefs := []registry.Tool{{Name: "echo_tool", Command: "echo hi"}}
	engine := NewEngine(&fakeLLM{}, tools, &fakeKnowledgeReader{})

	graph := toolGraph(registry.NodeData{ToolName: "echo_tool"})
	reply, err := engine.Run(context.Background(), graph, nil, "hi", "container-1", toolDefs, nil)
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

// A tool node whose output is (or contains) JSON exposes its properties
// downstream as {{Name.property}}, without needing a schema.
func TestRunToolNodeJSONOutputExposesPropertyDownstream(t *testing.T) {
	tools := &fakeTools{output: `here is the result: {"status": "ok", "count": 3}`}
	toolDefs := []registry.Tool{{Name: "api", Command: "call"}}
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "t1", Type: "tool", Data: registry.NodeData{Name: "Api", ToolName: "api"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "status was {{Api.status}} with {{Api.count}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "t1"},
			{ID: "e2", Source: "t1", Target: "p1"},
		},
	}
	llm := &fakeLLM{responses: []string{"done"}}
	engine := NewEngine(llm, tools, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "go", "container-1", toolDefs, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(llm.calls) != 1 || !strings.Contains(llm.calls[0], "status was ok with 3") {
		t.Errorf("llm.calls[0] = %q, want the tool's JSON properties resolved", llm.calls[0])
	}
}

// A plain-text tool output keeps parsed == nil — {{Name.property}} still
// errors, only {{Name}} works.
func TestRunToolNodePlainTextOutputHasNoProperties(t *testing.T) {
	tools := &fakeTools{output: "just some text"}
	toolDefs := []registry.Tool{{Name: "api", Command: "call"}}
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "t1", Type: "tool", Data: registry.NodeData{Name: "Api", ToolName: "api"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "{{Api.status}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "t1"},
			{ID: "e2", Source: "t1", Target: "p1"},
		},
	}
	engine := NewEngine(&fakeLLM{responses: []string{"x"}}, tools, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "go", "container-1", toolDefs, nil); err == nil {
		t.Error("Run() error = nil, want an error resolving a property on a plain-text tool output")
	}
}

func TestRunToolNodeTemplateReferencesInputNode(t *testing.T) {
	tools := &fakeTools{output: "done"}
	toolDefs := []registry.Tool{
		{
			Name:       "fetch",
			Command:    "curl -s {{url}}",
			Parameters: []registry.ToolParameter{{Name: "url", Type: registry.ToolParamString, Required: true}},
		},
	}
	engine := NewEngine(&fakeLLM{}, tools, &fakeKnowledgeReader{})

	graph := toolGraph(registry.NodeData{ToolName: "fetch", ToolArgs: map[string]string{"url": "{{Input}}"}})
	_, err := engine.Run(context.Background(), graph, nil, "https://example.com", "container-1", toolDefs, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "curl -s 'https://example.com'"
	if len(tools.calls) != 1 || tools.calls[0] != want {
		t.Errorf("tools.calls = %v, want [%s]", tools.calls, want)
	}
}

func TestRunToolNodeStaticAndTemplatedArgsTogether(t *testing.T) {
	tools := &fakeTools{output: "done"}
	toolDefs := []registry.Tool{
		{
			Name:    "write_file",
			Command: "printf '%s' {{content}} > {{path}}",
			Parameters: []registry.ToolParameter{
				{Name: "path", Type: registry.ToolParamString, Required: true},
				{Name: "content", Type: registry.ToolParamString, Required: true},
			},
		},
	}
	engine := NewEngine(&fakeLLM{}, tools, &fakeKnowledgeReader{})

	graph := toolGraph(registry.NodeData{
		ToolName: "write_file",
		ToolArgs: map[string]string{"path": "/tmp/out.txt", "content": "{{Input}}"},
	})
	_, err := engine.Run(context.Background(), graph, nil, "hello world", "container-1", toolDefs, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "printf '%s' 'hello world' > '/tmp/out.txt'"
	if len(tools.calls) != 1 || tools.calls[0] != want {
		t.Errorf("tools.calls = %v, want [%s]", tools.calls, want)
	}
}

func TestRunToolNodeUnresolvedTemplateReference(t *testing.T) {
	tools := &fakeTools{output: "done"}
	toolDefs := []registry.Tool{
		{Name: "fetch", Command: "curl -s {{url}}", Parameters: []registry.ToolParameter{{Name: "url", Type: registry.ToolParamString, Required: true}}},
	}
	engine := NewEngine(&fakeLLM{}, tools, &fakeKnowledgeReader{})

	graph := toolGraph(registry.NodeData{ToolName: "fetch", ToolArgs: map[string]string{"url": "{{NoSuchNode}}"}})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "container-1", toolDefs, nil); err == nil {
		t.Error("Run() error = nil, want an error for a template referencing an unknown node")
	}
}

func TestRunToolNodeRequiresInstance(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})
	toolDefs := []registry.Tool{{Name: "echo_tool", Command: "echo hi"}}

	graph := toolGraph(registry.NodeData{ToolName: "echo_tool"})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", toolDefs, nil); err == nil {
		t.Error("Run() error = nil, want an error when a tool node runs with no Environment instance")
	}
}

func TestRunToolNodeNoToolSelected(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	graph := toolGraph(registry.NodeData{})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "container-1", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when a tool node has no tool selected")
	}
}

func TestRunToolNodeUnknownTool(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})
	toolDefs := []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}

	graph := toolGraph(registry.NodeData{ToolName: "does-not-exist"})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "container-1", toolDefs, nil); err == nil {
		t.Error("Run() error = nil, want an error when the named tool isn't on the bound environment")
	}
}

func TestRunToolNodeMissingRequiredParameter(t *testing.T) {
	tools := &fakeTools{output: "done"}
	toolDefs := []registry.Tool{
		{
			Name:       "read_file",
			Command:    "cat {{path}}",
			Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}},
		},
	}
	engine := NewEngine(&fakeLLM{}, tools, &fakeKnowledgeReader{})

	// No ToolArgs at all — "path" is required but never supplied.
	graph := toolGraph(registry.NodeData{ToolName: "read_file"})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "container-1", toolDefs, nil); err == nil {
		t.Error("Run() error = nil, want the missing-required-parameter error to propagate")
	}
	if len(tools.calls) != 0 {
		t.Errorf("tools.calls = %v, want no command run when validation fails", tools.calls)
	}
}

func TestRunToolNodeError(t *testing.T) {
	tools := &fakeTools{err: errors.New("container not running")}
	toolDefs := []registry.Tool{{Name: "echo_tool", Command: "echo hi"}}
	engine := NewEngine(&fakeLLM{}, tools, &fakeKnowledgeReader{})

	graph := toolGraph(registry.NodeData{ToolName: "echo_tool"})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "container-1", toolDefs, nil); err == nil {
		t.Error("Run() error = nil, want the tool error to propagate")
	}
}

// schemaChainGraph is input(named "Input") -> prompt(named "Classifier",
// OutputSchema requiring a "city" string) -> prompt(named "Responder",
// PromptTemplate referencing {{Classifier.city}}), with Responder left as a
// dead end.
func schemaChainGraph(outputSchema, promptTemplate string) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Classifier", Model: "m", OutputSchema: outputSchema}},
			{ID: "p2", Type: "prompt", Data: registry.NodeData{Name: "Responder", Model: "m", PromptTemplate: promptTemplate}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", Target: "p2"},
		},
	}
}

func TestRunPromptNodeOutputSchemaExposesPropertyDownstream(t *testing.T) {
	schema := `{"type":"object","required":["city"],"properties":{"city":{"type":"string"}}}`
	llm := &fakeLLM{responses: []string{`{"city": "Paris"}`, "final reply"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := schemaChainGraph(schema, "please solve user problem: {{Classifier.city}}")
	reply, err := engine.Run(context.Background(), graph, nil, "where should I go?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "final reply" {
		t.Errorf("Run() = %q, want %q", reply, "final reply")
	}
	if len(llm.calls) != 2 || !strings.Contains(llm.calls[1], "USER: please solve user problem: Paris") {
		t.Errorf("llm.calls[1] = %q, want it to include the resolved city property", llm.calls[1])
	}
}

func TestRunPromptNodeOutputSchemaValidationFailureFailsTurn(t *testing.T) {
	schema := `{"type":"object","required":["city"]}`
	llm := &fakeLLM{responses: []string{"sorry, I can't help with that"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := schemaChainGraph(schema, "")
	if _, err := engine.Run(context.Background(), graph, nil, "where should I go?", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want the turn to fail when the reply doesn't satisfy the output schema")
	}
}

func TestRunPromptTemplateUnknownPropertyErrors(t *testing.T) {
	schema := `{"type":"object","required":["city"],"properties":{"city":{"type":"string"}}}`
	llm := &fakeLLM{responses: []string{`{"city": "Paris"}`}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := schemaChainGraph(schema, "population of {{Classifier.population}}")
	if _, err := engine.Run(context.Background(), graph, nil, "where should I go?", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error for a template referencing an undeclared property")
	}
}

func TestRunPromptTemplateDotPathWithoutSchemaErrors(t *testing.T) {
	// Classifier has no OutputSchema, so .city can't be resolved even though
	// the plain {{Classifier}} raw-text reference would work fine.
	llm := &fakeLLM{responses: []string{"Paris is nice this time of year", "final reply"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := schemaChainGraph("", "population of {{Classifier.city}}")
	if _, err := engine.Run(context.Background(), graph, nil, "where should I go?", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error referencing a property on a node with no output schema")
	}
}

func TestRunPromptTemplateEmptyFallsBackToPreviousOutput(t *testing.T) {
	llm := &fakeLLM{responses: []string{"Paris is nice this time of year", "final reply"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := schemaChainGraph("", "") // no schema, no template: legacy pass-through behavior
	reply, err := engine.Run(context.Background(), graph, nil, "where should I go?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "final reply" {
		t.Errorf("Run() = %q, want %q", reply, "final reply")
	}
	if !strings.Contains(llm.calls[1], "USER: Paris is nice this time of year") {
		t.Errorf("llm.calls[1] = %q, want the previous node's raw output passed through unchanged", llm.calls[1])
	}
}

func TestRunConditionMatchTemplateChecksNamedNodeProperty(t *testing.T) {
	schema := `{"type":"object","required":["sentiment"],"properties":{"sentiment":{"type":"string"}}}`
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Analyzer", Model: "m", OutputSchema: schema}},
			{ID: "c1", Type: "condition", Data: registry.NodeData{ConditionType: "contains", ConditionValue: "positive", MatchTemplate: "{{Analyzer.sentiment}}"}},
			{ID: "yes", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "happy branch"}},
			{ID: "no", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "sad branch"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", Target: "c1"},
			{ID: "e3", Source: "c1", SourceHandle: "pass", Target: "yes"},
			{ID: "e4", Source: "c1", SourceHandle: "fail", Target: "no"},
		},
	}
	llm := &fakeLLM{responses: []string{`{"sentiment": "positive", "confidence": 0.9}`, "took happy branch"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "I love this!", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "took happy branch" {
		t.Errorf("Run() = %q, want %q", reply, "took happy branch")
	}
	if !strings.Contains(llm.calls[1], "happy branch") {
		t.Errorf("llm.calls[1] = %q, want the pass-branch (positive sentiment) system prompt", llm.calls[1])
	}
}

// knowledgeGraph is input(named "Input") -> knowledge, with the knowledge
// node configured by data and left as a dead end.
func knowledgeGraph(data registry.NodeData) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "k1", Type: "knowledge", Data: data},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "k1"},
		},
	}
}

func TestRunKnowledgeNode(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name: "faq",
		Records: []registry.KnowledgeRecord{
			{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."},
			{ID: "2", Title: "Shipping", Content: "We ship worldwide."},
		},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := knowledgeGraph(registry.NodeData{KnowledgeBaseName: "faq", KnowledgeQuery: "refunds"})
	reply, err := engine.Run(context.Background(), graph, nil, "how do refunds work?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "Refunds: Refunds take 3-5 business days."
	if reply != want {
		t.Errorf("Run() = %q, want %q", reply, want)
	}
}

func TestRunKnowledgeNodeQueryFallsBackToPreviousOutput(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name:    "faq",
		Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	// No KnowledgeQuery: the previous node's (Input's) raw output is used as
	// the query, matching the same empty-falls-back convention as
	// PromptTemplate/MatchTemplate.
	graph := knowledgeGraph(registry.NodeData{KnowledgeBaseName: "faq"})
	reply, err := engine.Run(context.Background(), graph, nil, "refunds", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(reply, "Refunds take 3-5 business days.") {
		t.Errorf("Run() = %q, want it to include the matched record", reply)
	}
}

func TestRunKnowledgeNodeTemplatedQuery(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name:    "faq",
		Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := knowledgeGraph(registry.NodeData{KnowledgeBaseName: "faq", KnowledgeQuery: "{{Input}}"})
	reply, err := engine.Run(context.Background(), graph, nil, "refunds", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(reply, "Refunds take 3-5 business days.") {
		t.Errorf("Run() = %q, want the templated query resolved from {{Input}} to match", reply)
	}
}

func TestRunKnowledgeNodeNoMatches(t *testing.T) {
	kb := registry.KnowledgeBase{Name: "faq", Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "..."}}}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := knowledgeGraph(registry.NodeData{KnowledgeBaseName: "faq", KnowledgeQuery: "nonexistent"})
	reply, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "No matching records found." {
		t.Errorf("Run() = %q, want the no-matches sentinel", reply)
	}
}

func TestRunKnowledgeNodeNoBaseSelected(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	graph := knowledgeGraph(registry.NodeData{})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when a knowledge node has no knowledge base selected")
	}
}

func TestRunKnowledgeNodeUnknownBase(t *testing.T) {
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	graph := knowledgeGraph(registry.NodeData{KnowledgeBaseName: "does-not-exist"})
	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when the named knowledge base doesn't exist")
	}
}

func TestRunKnowledgeNodeMaxResultsCaps(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name: "faq",
		Records: []registry.KnowledgeRecord{
			{ID: "1", Title: "Alpha", Content: "shared keyword here"},
			{ID: "2", Title: "Beta", Content: "shared keyword here"},
			{ID: "3", Title: "Gamma", Content: "shared keyword here"},
		},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := knowledgeGraph(registry.NodeData{KnowledgeBaseName: "faq", KnowledgeQuery: "shared keyword", KnowledgeMaxResults: 2})
	reply, err := engine.Run(context.Background(), graph, nil, "q", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(reply, "Gamma") {
		t.Errorf("Run() = %q, want only the first 2 matches (no Gamma)", reply)
	}
	if !strings.Contains(reply, "Alpha") || !strings.Contains(reply, "Beta") {
		t.Errorf("Run() = %q, want the first 2 matches (Alpha, Beta)", reply)
	}
}

// A prompt node whose OutputSchema rejects the reply routes to its "fail"
// handle when one is wired, instead of failing the turn — so a schema'd
// prompt inside a retry loop degrades gracefully.
func TestRunPromptNodeOutputSchemaFailureRoutesToFailHandle(t *testing.T) {
	schema := `{"type":"object","required":["city"]}`
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Classifier", Model: "m", OutputSchema: schema}},
			{ID: "p2", Type: "prompt", Data: registry.NodeData{Name: "Recover", Model: "m", PromptTemplate: "recover from: {{Classifier}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", SourceHandle: "fail", Target: "p2"},
		},
	}
	llm := &fakeLLM{responses: []string{"not json at all", "recovered reply"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want the fail handle to be followed", err)
	}
	if reply != "recovered reply" {
		t.Errorf("Run() = %q, want the fail branch's output", reply)
	}
	if len(llm.calls) != 2 || !strings.Contains(llm.calls[1], "recover from: not json at all") {
		t.Errorf("llm.calls[1] = %q, want the raw invalid reply passed to the fail branch", llm.calls[1])
	}
}

// With no "fail" edge wired, a schema mismatch still fails the turn (the
// pre-existing hard-fail behavior).
func TestRunPromptNodeOutputSchemaFailureWithoutFailHandleStillFails(t *testing.T) {
	schema := `{"type":"object","required":["city"]}`
	llm := &fakeLLM{responses: []string{"not json"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := schemaChainGraph(schema, "")
	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want a hard failure when the schema fails and no fail handle is wired")
	}
}

// An agent node's FINAL answer can be constrained by AgentOutputSchema; on
// success the parsed value is referenceable downstream as {{Name.property}}.
func TestRunAgentNodeOutputSchemaExposesPropertyDownstream(t *testing.T) {
	schema := `{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "a1", Type: "agent", Data: registry.NodeData{Name: "Solver", AgentModel: "m", AgentOutputSchema: schema}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Echo", Model: "m", PromptTemplate: "the answer is {{Solver.answer}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "a1"},
			{ID: "e2", Source: "a1", Target: "p1"},
		},
	}
	llm := &fakeLLM{responses: []string{`FINAL: {"answer": "42"}`, "done"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "solve it", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(llm.calls) != 2 || !strings.Contains(llm.calls[1], "the answer is 42") {
		t.Errorf("llm.calls[1] = %q, want {{Solver.answer}} resolved to 42", llm.calls[1])
	}
}

// An agent node whose FINAL answer doesn't satisfy AgentOutputSchema and has
// no "fail" edge fails the turn.
func TestRunAgentNodeOutputSchemaFailureFailsTurn(t *testing.T) {
	schema := `{"type":"object","required":["answer"]}`
	llm := &fakeLLM{responses: []string{"FINAL: just some prose, not json"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := agentGraph(registry.NodeData{Name: "Solver", AgentModel: "m", AgentOutputSchema: schema})
	if _, err := engine.Run(context.Background(), graph, nil, "solve it", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want the turn to fail on an unsatisfied agent output schema")
	}
}

func TestRunKnowledgeNodeDownstreamReference(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name:    "faq",
		Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "k1", Type: "knowledge", Data: registry.NodeData{Name: "KB", KnowledgeBaseName: "faq", KnowledgeQuery: "{{Input}}"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "Answer using this context: {{KB}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "k1"},
			{ID: "e2", Source: "k1", Target: "p1"},
		},
	}
	llm := &fakeLLM{responses: []string{"final reply"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	reply, err := engine.Run(context.Background(), graph, nil, "refunds", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "final reply" {
		t.Errorf("Run() = %q, want %q", reply, "final reply")
	}
	if !strings.Contains(llm.calls[0], "Refunds take 3-5 business days.") {
		t.Errorf("llm.calls[0] = %q, want it to include the knowledge node's matched record via {{KB}}", llm.calls[0])
	}
}

// --- loop / state / cyclic architecture --------------------------------------

// loopBodyGraph is: input -> L(loop_start, max) --body--> W(prompt) ->
// LE(loop_end paired to L); L --done--> F(prompt), F a dead end. The LE->L
// back-edge is implicit (synthesized in prepareGraph). W's PromptTemplate can
// reference {{L.iteration}}.
func loopBodyGraph(max int, workTemplate string) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: max}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "W", Model: "m", PromptTemplate: workTemplate}},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{Name: "LE", LoopStartName: "L"}},
			{ID: "f", Type: "prompt", Data: registry.NodeData{Name: "F", Model: "m", SystemPrompt: "after loop"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "w"},
			{ID: "e3", Source: "w", Target: "le"},
			{ID: "e4", Source: "l", SourceHandle: "done", Target: "f"},
		},
	}
}

func TestRunLoopIteratesToMaxThenTakesDone(t *testing.T) {
	// max 2: W runs twice, then the loop routes to F.
	llm := &fakeLLM{responses: []string{"work 1", "work 2", "after-loop reply"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), loopBodyGraph(2, ""), nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "after-loop reply" {
		t.Errorf("Run() = %q, want the F node's reply after the loop finishes", reply)
	}
	if len(llm.calls) != 3 {
		t.Fatalf("llm.calls = %d, want 3 (W twice + F once)", len(llm.calls))
	}
	if !strings.Contains(llm.calls[2], "after loop") {
		t.Errorf("llm.calls[2] = %q, want the F node's system prompt", llm.calls[2])
	}
}

func TestRunLoopExposesIterationInTemplate(t *testing.T) {
	llm := &fakeLLM{responses: []string{"w1", "w2", "w3", "done"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	_, err := engine.Run(context.Background(), loopBodyGraph(3, "iteration {{L.iteration}}"), nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for i, want := range []string{"iteration 1", "iteration 2", "iteration 3"} {
		if !strings.Contains(llm.calls[i], "USER: "+want) {
			t.Errorf("llm.calls[%d] = %q, want it to contain %q", i, llm.calls[i], want)
		}
	}
}

func TestRunStateAppendAccumulatesAcrossLoop(t *testing.T) {
	// input -> L(loop_start, max 2) --body--> S(state, append "x") ->
	// LE(loop_end); L --done--> P(prompt template "{{S}}").
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 2}},
			{ID: "s", Type: "state", Data: registry.NodeData{Name: "S", StateOp: "append", StateValue: "x"}},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "p", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "collected: {{S}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "s"},
			{ID: "e3", Source: "s", Target: "le"},
			{ID: "e4", Source: "l", SourceHandle: "done", Target: "p"},
		},
	}
	llm := &fakeLLM{responses: []string{"ok"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "collected: x\nx") {
		t.Errorf("llm.calls[0] = %q, want the state node to have appended twice (x\\nx)", llm.calls[0])
	}
}

// stateLoopGraph is input -> L(loop_start, max 2) --body--> S(state) ->
// LE(loop_end); L --done--> P(prompt template "{{S}}"). S is configured by
// stateData.
func stateLoopGraph(stateData registry.NodeData) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 2}},
			{ID: "s", Type: "state", Data: stateData},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "p", Type: "prompt", Data: registry.NodeData{Model: "m", PromptTemplate: "collected: {{S}}"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "s"},
			{ID: "e3", Source: "s", Target: "le"},
			{ID: "e4", Source: "l", SourceHandle: "done", Target: "p"},
		},
	}
}

func TestRunStateOpDefaultsToAppend(t *testing.T) {
	// No StateOp set at all — should behave like "append" (matches the
	// inspector default and the canvas subtitle).
	llm := &fakeLLM{responses: []string{"ok"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := stateLoopGraph(registry.NodeData{Name: "S", StateValue: "x"})
	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "collected: x\nx") {
		t.Errorf("llm.calls[0] = %q, want an unset StateOp to accumulate like append", llm.calls[0])
	}
}

func TestRunStateOpSetReplaces(t *testing.T) {
	// Explicit "set" — the value replaces rather than accumulating.
	llm := &fakeLLM{responses: []string{"ok"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := stateLoopGraph(registry.NodeData{Name: "S", StateOp: "set", StateValue: "x"})
	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(llm.calls[0], "x\nx") || !strings.Contains(llm.calls[0], "collected: x") {
		t.Errorf("llm.calls[0] = %q, want \"set\" to replace (a single x), not accumulate", llm.calls[0])
	}
}

func TestRunAgentNodeIncludesConversationHistory(t *testing.T) {
	llm := &fakeLLM{responses: []string{"FINAL: sure"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := agentGraph(registry.NodeData{AgentModel: "m", AgentInstructions: "help the user"})
	history := []ChatMessage{{Role: "user", Content: "my name is Ada"}, {Role: "assistant", Content: "hi Ada"}}
	if _, err := engine.Run(context.Background(), graph, history, "what's my name?", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "USER: my name is Ada") || !strings.Contains(llm.calls[0], "ASSISTANT: hi Ada") {
		t.Errorf("agent prompt = %q, want it to include the prior conversation turns", llm.calls[0])
	}
}

func TestRunPlanExecuteJudgeLoopTerminatesOnPass(t *testing.T) {
	// input -> Plan -> Execute -> Judge -> cond(contains GOOD):
	//   fail -> back to Execute; pass -> Done (dead end).
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "plan", Type: "prompt", Data: registry.NodeData{Name: "Plan", Model: "m", SystemPrompt: "plan it"}},
			{ID: "exec", Type: "prompt", Data: registry.NodeData{Name: "Execute", Model: "m", SystemPrompt: "do it"}},
			{ID: "judge", Type: "prompt", Data: registry.NodeData{Name: "Judge", Model: "m", SystemPrompt: "grade it"}},
			{ID: "cond", Type: "condition", Data: registry.NodeData{ConditionType: "contains", ConditionValue: "GOOD"}},
			{ID: "done", Type: "prompt", Data: registry.NodeData{Name: "Done", Model: "m", SystemPrompt: "wrap up"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "plan"},
			{ID: "e2", Source: "plan", Target: "exec"},
			{ID: "e3", Source: "exec", Target: "judge"},
			{ID: "e4", Source: "judge", Target: "cond"},
			{ID: "e5", Source: "cond", SourceHandle: "fail", Target: "exec"},
			{ID: "e6", Source: "cond", SourceHandle: "pass", Target: "done"},
		},
	}
	// Plan, Execute#1, Judge#1=REVISE, Execute#2, Judge#2=GOOD, Done.
	llm := &fakeLLM{responses: []string{"a plan", "attempt 1", "REVISE please", "attempt 2", "looks GOOD", "final answer"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "solve X", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "final answer" {
		t.Errorf("Run() = %q, want %q", reply, "final answer")
	}
	if len(llm.calls) != 6 {
		t.Errorf("llm.calls = %d, want 6 (plan, execute x2, judge x2, done)", len(llm.calls))
	}
}

// --- agent (LLM tool-calling loop) node -------------------------------------

func agentGraph(data registry.NodeData) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "a1", Type: "agent", Data: data},
		},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "a1"}},
	}
}

func TestRunAgentNodeCallsToolThenFinal(t *testing.T) {
	tools := &fakeTools{output: "Paris is the capital of France"}
	toolDefs := []registry.Tool{{
		Name:       "web_search",
		Command:    "search {{query}}",
		Parameters: []registry.ToolParameter{{Name: "query", Type: registry.ToolParamString, Required: true}},
	}}
	llm := &fakeLLM{responses: []string{
		"THOUGHT: I should search.\nACTION: web_search\nARGS: {\"query\": \"capital of France\"}",
		"FINAL: Paris",
	}}
	engine := NewEngine(llm, tools, &fakeKnowledgeReader{})

	var steps []StepEvent
	graph := agentGraph(registry.NodeData{Name: "Agent", AgentModel: "m", AgentInstructions: "Find the capital.", AgentTools: []string{"web_search"}})
	reply, err := engine.Run(context.Background(), graph, nil, "capital of France?", "container-1", toolDefs, &RunHooks{OnStep: func(s StepEvent) { steps = append(steps, s) }})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "Paris" {
		t.Errorf("Run() = %q, want %q", reply, "Paris")
	}
	if len(tools.calls) != 1 || tools.calls[0] != "search 'capital of France'" {
		t.Errorf("tools.calls = %v, want [search 'capital of France']", tools.calls)
	}
	sawAgentIteration := false
	for _, s := range steps {
		if s.NodeType == "agent" && strings.Contains(s.Output, "web_search") {
			sawAgentIteration = true
		}
	}
	if !sawAgentIteration {
		t.Errorf("steps = %+v, want an agent iteration step mentioning web_search", steps)
	}
}

func TestRunAgentNodeGracefulFallbackOnUnparseableReply(t *testing.T) {
	llm := &fakeLLM{responses: []string{"I think it is probably 42, but honestly I am not certain."}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	graph := agentGraph(registry.NodeData{AgentModel: "m"})
	reply, err := engine.Run(context.Background(), graph, nil, "the answer?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "I think it is probably 42, but honestly I am not certain." {
		t.Errorf("Run() = %q, want the raw reply passed through as the final answer", reply)
	}
}

func TestRunAgentNodeHitsMaxIterations(t *testing.T) {
	tools := &fakeTools{output: "some observation"}
	toolDefs := []registry.Tool{{
		Name:       "web_search",
		Command:    "search {{query}}",
		Parameters: []registry.ToolParameter{{Name: "query", Type: registry.ToolParamString, Required: true}},
	}}
	// Always asks for another tool call, never emits FINAL.
	llm := &fakeLLM{responses: []string{
		"ACTION: web_search\nARGS: {\"query\": \"a\"}",
		"ACTION: web_search\nARGS: {\"query\": \"b\"}",
	}}
	engine := NewEngine(llm, tools, &fakeKnowledgeReader{})

	graph := agentGraph(registry.NodeData{AgentModel: "m", AgentTools: []string{"web_search"}, AgentMaxIterations: 2})
	reply, err := engine.Run(context.Background(), graph, nil, "go", "container-1", toolDefs, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tools.calls) != 2 {
		t.Errorf("tools.calls = %d, want 2 (capped at AgentMaxIterations)", len(tools.calls))
	}
	if !strings.Contains(reply, "web_search") {
		t.Errorf("Run() = %q, want the model's last text returned best-effort after the cap", reply)
	}
}

func TestRunAgentNodeToolsSelectedButNoEnvironment(t *testing.T) {
	engine := NewEngine(&fakeLLM{responses: []string{"FINAL: hi"}}, &fakeTools{}, &fakeKnowledgeReader{})

	graph := agentGraph(registry.NodeData{AgentModel: "m", AgentTools: []string{"web_search"}})
	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when an agent node has tools selected but no Environment instance")
	}
}

func TestRunAgentNodeKnowledgeSearchNeedsNoEnvironment(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name:    "faq",
		Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	// The model searches the KB, then answers. No Environment / instance.
	llm := &fakeLLM{responses: []string{
		"ACTION: knowledge_search\nARGS: {\"query\": \"refunds\"}",
		"FINAL: 3-5 business days",
	}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := agentGraph(registry.NodeData{AgentModel: "m", AgentKnowledgeBases: []string{"faq"}})
	reply, err := engine.Run(context.Background(), graph, nil, "how long do refunds take?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "3-5 business days" {
		t.Errorf("Run() = %q, want the final answer", reply)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("llm.calls = %d, want 2 (search then answer)", len(llm.calls))
	}
	// The first prompt advertises knowledge_search; the second carries the
	// matched record back as an OBSERVATION.
	if !strings.Contains(llm.calls[0], "knowledge_search(query: string)") {
		t.Errorf("llm.calls[0] = %q, want it to advertise the knowledge_search tool", llm.calls[0])
	}
	if !strings.Contains(llm.calls[1], "Refunds take 3-5 business days.") {
		t.Errorf("llm.calls[1] = %q, want the knowledge_search result fed back", llm.calls[1])
	}
}

// Tiny models rarely emit "knowledge_search" with an exact {"query": ...}
// arg — they name the base directly and use a synonym key. Both are tolerated.
func TestRunAgentNodeKnowledgeSearchToleratesLooseCalls(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name:    "faq",
		Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	llm := &fakeLLM{responses: []string{
		"ACTION: faq\nARGS: {\"q\": \"refunds\"}", // base name as the action, synonym arg key
		"FINAL: 3-5 business days",
	}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := agentGraph(registry.NodeData{AgentModel: "m", AgentKnowledgeBases: []string{"faq"}})
	reply, err := engine.Run(context.Background(), graph, nil, "how long do refunds take?", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "3-5 business days" {
		t.Errorf("Run() = %q, want the final answer", reply)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("llm.calls = %d, want 2 (search then answer)", len(llm.calls))
	}
	if !strings.Contains(llm.calls[1], "Refunds take 3-5 business days.") {
		t.Errorf("llm.calls[1] = %q, want the knowledge_search result fed back", llm.calls[1])
	}
}

// When the model gives no usable query arg at all, the node's own input is
// used as the search text (same fallback the knowledge node uses).
func TestRunAgentNodeKnowledgeSearchFallsBackToInput(t *testing.T) {
	kb := registry.KnowledgeBase{
		Name:    "faq",
		Records: []registry.KnowledgeRecord{{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	llm := &fakeLLM{responses: []string{
		"ACTION: knowledge_search\nARGS: {}", // no query at all
		"FINAL: 3-5 business days",
	}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{bases: map[string]registry.KnowledgeBase{"faq": kb}})

	graph := agentGraph(registry.NodeData{AgentModel: "m", AgentKnowledgeBases: []string{"faq"}})
	if _, err := engine.Run(context.Background(), graph, nil, "refunds", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(llm.calls) < 2 || !strings.Contains(llm.calls[1], "Refunds take 3-5 business days.") {
		t.Errorf("llm.calls[1] = %q, want the input-derived search to have matched the record", llm.calls[len(llm.calls)-1])
	}
}

func TestParseActionForms(t *testing.T) {
	cases := []struct {
		in         string
		wantAction string
		wantArgs   string
	}{
		{"ACTION: web_search\nARGS: {\"q\": \"hi\"}", "web_search", `{"q": "hi"}`},
		{"THOUGHT: hmm\nACTION: read_file({\"path\": \"/a\"})", "read_file", `{"path": "/a"}`},
		{"ACTION: list_dir", "list_dir", ""},
		{"FINAL: done", "", ""},
		{"just some chatter", "", ""},
	}
	for _, c := range cases {
		gotAction, gotArgs := parseAction(c.in)
		if gotAction != c.wantAction || gotArgs != c.wantArgs {
			t.Errorf("parseAction(%q) = (%q, %q), want (%q, %q)", c.in, gotAction, gotArgs, c.wantAction, c.wantArgs)
		}
	}
}

func TestParseFinalForms(t *testing.T) {
	cases := []struct {
		in       string
		wantText string
		wantOK   bool
	}{
		{"FINAL: Paris", "Paris", true},
		{"Final Answer: 42", "42", true},
		{"no marker here", "", false},
	}
	for _, c := range cases {
		gotText, gotOK := parseFinal(c.in)
		if gotText != c.wantText || gotOK != c.wantOK {
			t.Errorf("parseFinal(%q) = (%q, %v), want (%q, %v)", c.in, gotText, gotOK, c.wantText, c.wantOK)
		}
	}
}

// --- loop_start / loop_end pairing -----------------------------------------

func TestRunLoopEndBreakBranchExitsLoop(t *testing.T) {
	// input -> L(loop_start, max 5) --body--> Work(prompt) -> Check(condition:
	// contains DONE):
	//   fail -> LE(loop_end -> L)   [continue]
	//   pass -> After(prompt)       [break out; dead end]
	// Work says "keep going" then "all DONE"; the loop should run exactly twice.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 5}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "Work", Model: "m", SystemPrompt: "work"}},
			{ID: "c", Type: "condition", Data: registry.NodeData{ConditionType: "contains", ConditionValue: "DONE"}},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "after", Type: "prompt", Data: registry.NodeData{Name: "After", Model: "m", SystemPrompt: "wrap up"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "w"},
			{ID: "e3", Source: "w", Target: "c"},
			{ID: "e4", Source: "c", SourceHandle: "fail", Target: "le"},
			{ID: "e5", Source: "c", SourceHandle: "pass", Target: "after"},
		},
	}
	llm := &fakeLLM{responses: []string{"keep going", "all DONE now", "wrapped up"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "wrapped up" {
		t.Errorf("Run() = %q, want the After node's reply", reply)
	}
	if len(llm.calls) != 3 {
		t.Errorf("llm.calls = %d, want 3 (Work twice + After once)", len(llm.calls))
	}
}

func TestRunLoopEndExhaustsToDoneHandle(t *testing.T) {
	// Same shape, but Work never says DONE — the loop_start's own max (2)
	// routes to "done" after two body passes.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 2}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "Work", Model: "m", SystemPrompt: "work"}},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "giveup", Type: "prompt", Data: registry.NodeData{Name: "GaveUp", Model: "m", SystemPrompt: "ran out of tries"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "w"},
			{ID: "e3", Source: "w", Target: "le"},
			{ID: "e4", Source: "l", SourceHandle: "done", Target: "giveup"},
		},
	}
	llm := &fakeLLM{responses: []string{"try 1", "try 2", "gave up"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "gave up" || len(llm.calls) != 3 {
		t.Errorf("reply=%q calls=%d, want the done-handle path after exactly 2 body passes", reply, len(llm.calls))
	}
}

func TestRunMultipleLoopEndsConvergeOnOneStart(t *testing.T) {
	// Two condition branches each continue the loop via their own loop_end;
	// both must jump back to the same loop_start. max 3 bounds it.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 3}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "W", Model: "m", SystemPrompt: "w"}},
			{ID: "c", Type: "condition", Data: registry.NodeData{ConditionType: "contains", ConditionValue: "left"}},
			{ID: "le1", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "le2", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L"}},
			{ID: "done", Type: "prompt", Data: registry.NodeData{Name: "D", Model: "m", SystemPrompt: "done"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "w"},
			{ID: "e3", Source: "w", Target: "c"},
			{ID: "e4", Source: "c", SourceHandle: "pass", Target: "le1"},
			{ID: "e5", Source: "c", SourceHandle: "fail", Target: "le2"},
			{ID: "e6", Source: "l", SourceHandle: "done", Target: "done"},
		},
	}
	llm := &fakeLLM{responses: []string{"go left", "go right", "go left", "finished"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "finished" || len(llm.calls) != 4 {
		t.Errorf("reply=%q calls=%d, want both loop_end branches to loop back (3 W passes + done)", reply, len(llm.calls))
	}
}

func TestRunLoopEndWithoutMatchingStartErrors(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{LoopStartName: "NoSuchLoop"}},
		},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "le"}},
	}
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})
	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error for a loop_end that names no existing loop_start")
	}
}

func TestRunLoopEndBlankNameResolvesWhenSingleLoop(t *testing.T) {
	// A lone loop pair: the loop_end can leave LoopStartName blank.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l", Type: "loop_start", Data: registry.NodeData{Name: "L", LoopMaxIterations: 2}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "W", Model: "m", SystemPrompt: "w"}},
			{ID: "le", Type: "loop_end", Data: registry.NodeData{}}, // blank LoopStartName
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l"},
			{ID: "e2", Source: "l", SourceHandle: "body", Target: "w"},
			{ID: "e3", Source: "w", Target: "le"},
		},
	}
	llm := &fakeLLM{responses: []string{"a", "b"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})
	// 2 body passes, then loop_start's "done" handle is unwired -> turn ends
	// on the loop_start's passthrough.
	if _, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(llm.calls) != 2 {
		t.Errorf("llm.calls = %d, want 2 body passes before the unwired done handle ends the turn", len(llm.calls))
	}
}

func TestRunNestedLoops(t *testing.T) {
	// Outer L1 (max 2) contains inner L2 (max 2). Inner body prompt runs
	// outer*inner = 4 times; then L1 "done" -> Fin.
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input"},
			{ID: "l1", Type: "loop_start", Data: registry.NodeData{Name: "L1", LoopMaxIterations: 2}},
			{ID: "l2", Type: "loop_start", Data: registry.NodeData{Name: "L2", LoopMaxIterations: 2}},
			{ID: "w", Type: "prompt", Data: registry.NodeData{Name: "W", Model: "m", SystemPrompt: "inner work"}},
			{ID: "le2", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L2"}},
			{ID: "le1", Type: "loop_end", Data: registry.NodeData{LoopStartName: "L1"}},
			{ID: "fin", Type: "prompt", Data: registry.NodeData{Name: "Fin", Model: "m", SystemPrompt: "finished"}},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "l1"},
			{ID: "e2", Source: "l1", SourceHandle: "body", Target: "l2"},
			{ID: "e3", Source: "l2", SourceHandle: "body", Target: "w"},
			{ID: "e4", Source: "w", Target: "le2"},
			{ID: "e5", Source: "l2", SourceHandle: "done", Target: "le1"},
			{ID: "e6", Source: "l1", SourceHandle: "done", Target: "fin"},
		},
	}
	llm := &fakeLLM{responses: []string{"1", "2", "3", "4", "done"}}
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), graph, nil, "go", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reply != "done" {
		t.Errorf("Run() = %q, want the Fin node's reply", reply)
	}
	if len(llm.calls) != 5 {
		t.Errorf("llm.calls = %d, want 5 (inner work 2x2 + Fin)", len(llm.calls))
	}
}
