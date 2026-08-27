// Package agents executes agent workflow graphs (registry.Graph) as chat
// turns: starting at the graph's input node, calling a local LLM for each
// prompt node, branching at decision nodes on a keyword match, running a
// named Tool from the agent's bound Environment for each tool node,
// deterministically keyword-searching a named KnowledgeBase for each
// knowledge node (independent of any Environment — see internal/knowledge),
// and returning whatever text the walk ends on. There's no dedicated
// "output" node type: a node with no outgoing edge for the handle it
// produces is simply where the turn ends, and its own output becomes the
// reply — this lets any node type serve as a graph's terminal, and means a
// user never has to remember to wire up a trailing node just to mark "this
// is the end." Every node's output is kept (by its user-chosen Name) for
// the rest of the turn, so a downstream node's template can reference any
// earlier node's output — not just its immediate predecessor — and, for a
// prompt node with a declared output JSON Schema, a specific property of
// it. See runcontext.go.
package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/knowledge"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// maxSteps bounds how many nodes a single turn can visit, so a cycle in the
// graph (e.g. decision -> prompt -> decision) fails fast instead of hanging.
const maxSteps = 50

// ChatMessage is one turn in a run's conversation history.
type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// StepEvent reports one node the engine visited while executing a turn —
// Output is what that node itself produced (not the value that flowed into
// it), for the Run view's live event log and the step-by-step debugger.
type StepEvent struct {
	NodeID   string `json:"nodeId"`
	NodeType string `json:"nodeType"`
	Output   string `json:"output"`
}

// llmClient is the subset of mlxrunner.Runner the engine needs to call a
// model for a prompt node.
type llmClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// toolRunner is the subset of environments.Manager the engine needs to
// execute a tool node's command.
type toolRunner interface {
	RunToolSync(ctx context.Context, instanceID, command string) (string, error)
}

// knowledgeReader is the subset of registry.Registry the engine needs to
// resolve a knowledge node's named KnowledgeBase. Independent of any
// Environment/toolRunner — querying records is plain in-process text
// matching, nothing to launch.
type knowledgeReader interface {
	GetKnowledgeBase(name string) (registry.KnowledgeBase, error)
}

// Engine walks an agent's graph to produce one chat reply.
type Engine struct {
	llm       llmClient
	tools     toolRunner
	knowledge knowledgeReader
}

// NewEngine builds an Engine that calls models via llm, runs tool node
// commands via tools, and resolves knowledge node lookups via kb.
func NewEngine(llm llmClient, tools toolRunner, kb knowledgeReader) *Engine {
	return &Engine{llm: llm, tools: tools, knowledge: kb}
}

// findInputNode validates graph (exactly one input node, no two nodes
// sharing a Name — a template ambiguity better caught up front than mid-turn)
// and returns it alongside a by-ID index, both reused by Run and by the
// step-by-step debugger (see debug.go).
func findInputNode(graph registry.Graph) (*registry.Node, map[string]registry.Node, error) {
	nodesByID := make(map[string]registry.Node, len(graph.Nodes))
	seenNames := make(map[string]bool, len(graph.Nodes))
	var inputNode *registry.Node
	for i := range graph.Nodes {
		node := graph.Nodes[i]
		nodesByID[node.ID] = node
		if node.Type == "input" {
			if inputNode != nil {
				return nil, nil, errors.New("graph has more than one input node")
			}
			inputNode = &node
		}
		if node.Data.Name != "" {
			if seenNames[node.Data.Name] {
				return nil, nil, fmt.Errorf("more than one node is named %q — node names must be unique to be referenced in templates", node.Data.Name)
			}
			seenNames[node.Data.Name] = true
		}
	}
	if inputNode == nil {
		return nil, nil, errors.New("graph has no input node")
	}
	return inputNode, nodesByID, nil
}

// Run executes graph for one turn: starting at its input node with
// userMessage, following prompt/decision/tool/knowledge nodes until one has
// no outgoing edge for the handle it produced, and returning the text that
// node itself produced. history is the conversation so far (not including
// userMessage), used to give prompt nodes context. instanceID is the
// agent's launched Environment instance (empty if the agent has none
// configured) — required by any tool node in the graph. tools is the bound
// Environment's declared Tool definitions (a tool node's ToolName resolves
// against this list); empty if the agent has no Environment configured.
// onStep, if non-nil, is called for every node visited, with what that node
// itself produced.
//
// Every node that declares a Name (registry.NodeData.Name) becomes
// referenceable in a later node's template fields (PromptTemplate,
// MatchTemplate, ToolArgs values) as {{Name}} (its raw text output) or, if
// it's a prompt node with OutputSchema configured, {{Name.property}} for a
// specific property of its validated JSON reply — see runcontext.go.
func (e *Engine) Run(ctx context.Context, graph registry.Graph, history []ChatMessage, userMessage, instanceID string, tools []registry.Tool, onStep func(StepEvent)) (string, error) {
	inputNode, nodesByID, err := findInputNode(graph)
	if err != nil {
		return "", err
	}

	rc := newRunContext()
	current := inputNode
	output := userMessage

	for step := 0; ; step++ {
		if step >= maxSteps {
			return "", errors.New("agent exceeded maximum steps (likely a cycle in the graph)")
		}

		newOutput, handle, err := e.runNode(ctx, *current, output, instanceID, tools, history, rc)
		if err != nil {
			return "", err
		}
		output = newOutput

		if onStep != nil {
			onStep(StepEvent{NodeID: current.ID, NodeType: current.Type, Output: output})
		}

		edge := findEdge(graph.Edges, current.ID, handle)
		if edge == nil {
			// No outgoing edge for this node's handle — this is where the
			// graph ends; its own output is the turn's reply.
			return output, nil
		}

		next, ok := nodesByID[edge.Target]
		if !ok {
			return "", fmt.Errorf("edge from node %q targets unknown node %q", current.ID, edge.Target)
		}
		current = &next
	}
}

// runNode executes exactly one node given the value flowing into it
// (input), returning what the node itself produces and which outgoing
// handle to follow next ("" for every node type except decision, which
// produces "yes"/"no"). Shared by Run's loop above and the step-by-step
// debugger (see debug.go) so both execute a node identically.
func (e *Engine) runNode(ctx context.Context, node registry.Node, input, instanceID string, tools []registry.Tool, history []ChatMessage, rc *runContext) (output, handle string, err error) {
	switch node.Type {
	case "input":
		rc.set(node.Data.Name, input, nil)
		return input, "", nil

	case "prompt":
		userTurn := input
		if node.Data.PromptTemplate != "" {
			rendered, err := rc.render(node.Data.PromptTemplate)
			if err != nil {
				return "", "", fmt.Errorf("prompt node %q: %w", node.ID, err)
			}
			userTurn = rendered
		}

		reply, err := e.llm.Generate(ctx, node.Data.Model, buildPrompt(node.Data.SystemPrompt, history, userTurn))
		if err != nil {
			return "", "", fmt.Errorf("prompt node %q: %w", node.ID, err)
		}

		var parsed any
		if node.Data.OutputSchema != "" {
			parsed, err = assertions.ValidateJSONSchema(node.Data.OutputSchema, reply)
			if err != nil {
				return "", "", fmt.Errorf("prompt node %q: reply did not satisfy its output schema: %w", node.ID, err)
			}
		}

		rc.set(node.Data.Name, reply, parsed)
		return reply, "", nil

	case "decision":
		matchText := input
		if node.Data.MatchTemplate != "" {
			rendered, err := rc.render(node.Data.MatchTemplate)
			if err != nil {
				return "", "", fmt.Errorf("decision node %q: %w", node.ID, err)
			}
			matchText = rendered
		}

		branch := "no"
		if node.Data.Keyword != "" && strings.Contains(strings.ToLower(matchText), strings.ToLower(node.Data.Keyword)) {
			branch = "yes"
		}
		rc.set(node.Data.Name, input, nil)
		return input, branch, nil

	case "tool":
		if instanceID == "" {
			return "", "", fmt.Errorf("tool node %q requires an Environment to be configured for this agent", node.ID)
		}
		if node.Data.ToolName == "" {
			return "", "", fmt.Errorf("tool node %q has no tool selected", node.ID)
		}
		tool, ok := findTool(tools, node.Data.ToolName)
		if !ok {
			return "", "", fmt.Errorf("tool node %q: tool %q not found on this agent's environment", node.ID, node.Data.ToolName)
		}

		args := make(map[string]string, len(node.Data.ToolArgs))
		for k, v := range node.Data.ToolArgs {
			rendered, err := rc.render(v)
			if err != nil {
				return "", "", fmt.Errorf("tool node %q: parameter %q: %w", node.ID, k, err)
			}
			args[k] = rendered
		}

		command, err := environments.RenderToolCommand(tool, args)
		if err != nil {
			return "", "", fmt.Errorf("tool node %q: %w", node.ID, err)
		}
		result, err := e.tools.RunToolSync(ctx, instanceID, command)
		if err != nil {
			return "", "", fmt.Errorf("tool node %q: %w", node.ID, err)
		}
		rc.set(node.Data.Name, result, nil)
		return result, "", nil

	case "knowledge":
		if node.Data.KnowledgeBaseName == "" {
			return "", "", fmt.Errorf("knowledge node %q has no knowledge base selected", node.ID)
		}
		kb, err := e.knowledge.GetKnowledgeBase(node.Data.KnowledgeBaseName)
		if err != nil {
			return "", "", fmt.Errorf("knowledge node %q: %w", node.ID, err)
		}

		query := input
		if node.Data.KnowledgeQuery != "" {
			rendered, err := rc.render(node.Data.KnowledgeQuery)
			if err != nil {
				return "", "", fmt.Errorf("knowledge node %q: %w", node.ID, err)
			}
			query = rendered
		}

		result := knowledge.FormatResults(knowledge.Query(kb, query))
		rc.set(node.Data.Name, result, nil)
		return result, "", nil

	default:
		return "", "", fmt.Errorf("node %q has unknown type %q", node.ID, node.Type)
	}
}

// findEdge returns the first edge from nodeID with the given handle (empty
// handle for every node type except decision, which has "yes"/"no").
func findEdge(edges []registry.Edge, nodeID, handle string) *registry.Edge {
	for i := range edges {
		if edges[i].Source == nodeID && edges[i].SourceHandle == handle {
			return &edges[i]
		}
	}
	return nil
}

// findTool returns the tool with the given name, if any.
func findTool(tools []registry.Tool, name string) (registry.Tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return registry.Tool{}, false
}

// buildPrompt renders a plain-text completion prompt from a prompt node's
// system prompt, the conversation history, and the latest input.
func buildPrompt(systemPrompt string, history []ChatMessage, latest string) string {
	var b strings.Builder

	if systemPrompt != "" {
		b.WriteString(systemPrompt)
		b.WriteString("\n\n")
	}
	for _, m := range history {
		b.WriteString(strings.ToUpper(m.Role))
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	b.WriteString("USER: ")
	b.WriteString(latest)
	b.WriteString("\nASSISTANT:")

	return b.String()
}
