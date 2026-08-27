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
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	var steps []StepEvent
	reply, err := engine.Run(context.Background(), linearGraph(), nil, "hi", "", nil, func(s StepEvent) { steps = append(steps, s) })
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
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), decisionGraph("weather"), nil, "what's the weather?", "", nil, nil)
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
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	reply, err := engine.Run(context.Background(), decisionGraph("weather"), nil, "tell me a joke", "", nil, nil)
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
	engine := NewEngine(llm, &fakeTools{}, &fakeKnowledgeReader{})

	_, err := engine.Run(context.Background(), decisionGraph("Weather"), nil, "WEATHER report please", "", nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(llm.calls[0], "matched branch") {
		t.Error("keyword match should be case-insensitive")
	}
}

func TestRunNoInputNode(t *testing.T) {
	graph := registry.Graph{Nodes: []registry.Node{{ID: "out", Type: "output"}}}
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

func TestRunDeadEndNode(t *testing.T) {
	graph := registry.Graph{
		Nodes: []registry.Node{{ID: "in", Type: "input"}, {ID: "p1", Type: "prompt"}},
		Edges: []registry.Edge{{ID: "e1", Source: "in", Target: "p1"}},
	}
	engine := NewEngine(&fakeLLM{responses: []string{"reply"}}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
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
	engine := NewEngine(&fakeLLM{}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when the graph cycles past the step limit")
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
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", Target: "out"},
		},
	}
	engine := NewEngine(&fakeLLM{responses: []string{"reply"}}, &fakeTools{}, &fakeKnowledgeReader{})

	if _, err := engine.Run(context.Background(), graph, nil, "hi", "", nil, nil); err == nil {
		t.Error("Run() error = nil, want an error when two nodes share a name")
	}
}

// toolGraph is input(named "Input") -> tool -> output, with the tool node
// configured by data.
func toolGraph(data registry.NodeData) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "t1", Type: "tool", Data: data},
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
// PromptTemplate referencing {{Classifier.city}}) -> output.
func schemaChainGraph(outputSchema, promptTemplate string) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Classifier", Model: "m", OutputSchema: outputSchema}},
			{ID: "p2", Type: "prompt", Data: registry.NodeData{Name: "Responder", Model: "m", PromptTemplate: promptTemplate}},
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", Target: "p2"},
			{ID: "e3", Source: "p2", Target: "out"},
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

func TestRunDecisionMatchTemplateChecksNamedNodeProperty(t *testing.T) {
	schema := `{"type":"object","required":["sentiment"],"properties":{"sentiment":{"type":"string"}}}`
	graph := registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "p1", Type: "prompt", Data: registry.NodeData{Name: "Analyzer", Model: "m", OutputSchema: schema}},
			{ID: "d1", Type: "decision", Data: registry.NodeData{Keyword: "positive", MatchTemplate: "{{Analyzer.sentiment}}"}},
			{ID: "yes", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "happy branch"}},
			{ID: "no", Type: "prompt", Data: registry.NodeData{Model: "m", SystemPrompt: "sad branch"}},
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "p1"},
			{ID: "e2", Source: "p1", Target: "d1"},
			{ID: "e3", Source: "d1", SourceHandle: "yes", Target: "yes"},
			{ID: "e4", Source: "d1", SourceHandle: "no", Target: "no"},
			{ID: "e5", Source: "yes", Target: "out"},
			{ID: "e6", Source: "no", Target: "out"},
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
		t.Errorf("llm.calls[1] = %q, want the yes-branch (positive sentiment) system prompt", llm.calls[1])
	}
}

// knowledgeGraph is input(named "Input") -> knowledge -> output, with the
// knowledge node configured by data.
func knowledgeGraph(data registry.NodeData) registry.Graph {
	return registry.Graph{
		Nodes: []registry.Node{
			{ID: "in", Type: "input", Data: registry.NodeData{Name: "Input"}},
			{ID: "k1", Type: "knowledge", Data: data},
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "k1"},
			{ID: "e2", Source: "k1", Target: "out"},
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
			{ID: "out", Type: "output"},
		},
		Edges: []registry.Edge{
			{ID: "e1", Source: "in", Target: "k1"},
			{ID: "e2", Source: "k1", Target: "p1"},
			{ID: "e3", Source: "p1", Target: "out"},
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
