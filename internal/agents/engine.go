// Package agents executes agent workflow graphs (registry.Graph) as chat
// turns: starting at the graph's input node, calling a local LLM for each
// prompt node, branching at decision nodes on a keyword match, running a
// named Tool from the agent's bound Environment for each tool node,
// deterministically keyword-searching a named KnowledgeBase for each
// knowledge node (independent of any Environment — see internal/knowledge),
// and returning whatever text reaches the output node. Every node's output
// is kept (by its user-chosen Name) for the rest of the turn, so a
// downstream node's template can reference any earlier node's output — not
// just its immediate predecessor — and, for a prompt node with a declared
// output JSON Schema, a specific property of it. See runcontext.go.
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

// StepEvent reports one node the engine visited while executing a turn, for
// the Run view's live event log.
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

// Run executes graph for one turn: starting at its input node with
// userMessage, following prompt/decision/tool nodes until an output node is
// reached, and returning the text that arrives there. history is the
// conversation so far (not including userMessage), used to give prompt
// nodes context. instanceID is the agent's launched Environment instance
// (empty if the agent has none configured) — required by any tool node in
// the graph. tools is the bound Environment's declared Tool definitions (a
// tool node's ToolName resolves against this list); empty if the agent has
// no Environment configured. onStep, if non-nil, is called for every node
// visited.
//
// Every node that declares a Name (registry.NodeData.Name) becomes
// referenceable in a later node's template fields (PromptTemplate,
// MatchTemplate, ToolArgs values) as {{Name}} (its raw text output) or, if
// it's a prompt node with OutputSchema configured, {{Name.property}} for a
// specific property of its validated JSON reply — see runcontext.go.
func (e *Engine) Run(ctx context.Context, graph registry.Graph, history []ChatMessage, userMessage, instanceID string, tools []registry.Tool, onStep func(StepEvent)) (string, error) {
	nodesByID := make(map[string]registry.Node, len(graph.Nodes))
	seenNames := make(map[string]bool, len(graph.Nodes))
	var inputNode *registry.Node
	for i := range graph.Nodes {
		node := graph.Nodes[i]
		nodesByID[node.ID] = node
		if node.Type == "input" {
			if inputNode != nil {
				return "", errors.New("graph has more than one input node")
			}
			inputNode = &node
		}
		if node.Data.Name != "" {
			if seenNames[node.Data.Name] {
				return "", fmt.Errorf("more than one node is named %q — node names must be unique to be referenced in templates", node.Data.Name)
			}
			seenNames[node.Data.Name] = true
		}
	}
	if inputNode == nil {
		return "", errors.New("graph has no input node")
	}

	rc := newRunContext()
	current := inputNode
	output := userMessage

	for step := 0; ; step++ {
		if step >= maxSteps {
			return "", errors.New("agent exceeded maximum steps (likely a cycle in the graph)")
		}

		if onStep != nil {
			onStep(StepEvent{NodeID: current.ID, NodeType: current.Type, Output: output})
		}

		if current.Type == "output" {
			return output, nil
		}

		var nextID string
		switch current.Type {
		case "input":
			rc.set(current.Data.Name, output, nil)

			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("input node %q has no outgoing edge", current.ID)
			}
			nextID = edge.Target

		case "prompt":
			userTurn := output
			if current.Data.PromptTemplate != "" {
				rendered, err := rc.render(current.Data.PromptTemplate)
				if err != nil {
					return "", fmt.Errorf("prompt node %q: %w", current.ID, err)
				}
				userTurn = rendered
			}

			reply, err := e.llm.Generate(ctx, current.Data.Model, buildPrompt(current.Data.SystemPrompt, history, userTurn))
			if err != nil {
				return "", fmt.Errorf("prompt node %q: %w", current.ID, err)
			}

			var parsed any
			if current.Data.OutputSchema != "" {
				parsed, err = assertions.ValidateJSONSchema(current.Data.OutputSchema, reply)
				if err != nil {
					return "", fmt.Errorf("prompt node %q: reply did not satisfy its output schema: %w", current.ID, err)
				}
			}

			output = reply
			rc.set(current.Data.Name, output, parsed)

			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("prompt node %q has no outgoing edge", current.ID)
			}
			nextID = edge.Target

		case "decision":
			matchText := output
			if current.Data.MatchTemplate != "" {
				rendered, err := rc.render(current.Data.MatchTemplate)
				if err != nil {
					return "", fmt.Errorf("decision node %q: %w", current.ID, err)
				}
				matchText = rendered
			}

			handle := "no"
			if current.Data.Keyword != "" && strings.Contains(strings.ToLower(matchText), strings.ToLower(current.Data.Keyword)) {
				handle = "yes"
			}
			rc.set(current.Data.Name, output, nil)

			edge := findEdge(graph.Edges, current.ID, handle)
			if edge == nil {
				return "", fmt.Errorf("decision node %q has no %q edge", current.ID, handle)
			}
			nextID = edge.Target

		case "tool":
			if instanceID == "" {
				return "", fmt.Errorf("tool node %q requires an Environment to be configured for this agent", current.ID)
			}
			if current.Data.ToolName == "" {
				return "", fmt.Errorf("tool node %q has no tool selected", current.ID)
			}
			tool, ok := findTool(tools, current.Data.ToolName)
			if !ok {
				return "", fmt.Errorf("tool node %q: tool %q not found on this agent's environment", current.ID, current.Data.ToolName)
			}

			args := make(map[string]string, len(current.Data.ToolArgs))
			for k, v := range current.Data.ToolArgs {
				rendered, err := rc.render(v)
				if err != nil {
					return "", fmt.Errorf("tool node %q: parameter %q: %w", current.ID, k, err)
				}
				args[k] = rendered
			}

			command, err := environments.RenderToolCommand(tool, args)
			if err != nil {
				return "", fmt.Errorf("tool node %q: %w", current.ID, err)
			}
			result, err := e.tools.RunToolSync(ctx, instanceID, command)
			if err != nil {
				return "", fmt.Errorf("tool node %q: %w", current.ID, err)
			}
			output = result
			rc.set(current.Data.Name, output, nil)

			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("tool node %q has no outgoing edge", current.ID)
			}
			nextID = edge.Target

		case "knowledge":
			if current.Data.KnowledgeBaseName == "" {
				return "", fmt.Errorf("knowledge node %q has no knowledge base selected", current.ID)
			}
			kb, err := e.knowledge.GetKnowledgeBase(current.Data.KnowledgeBaseName)
			if err != nil {
				return "", fmt.Errorf("knowledge node %q: %w", current.ID, err)
			}

			query := output
			if current.Data.KnowledgeQuery != "" {
				rendered, err := rc.render(current.Data.KnowledgeQuery)
				if err != nil {
					return "", fmt.Errorf("knowledge node %q: %w", current.ID, err)
				}
				query = rendered
			}

			output = knowledge.FormatResults(knowledge.Query(kb, query))
			rc.set(current.Data.Name, output, nil)

			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("knowledge node %q has no outgoing edge", current.ID)
			}
			nextID = edge.Target

		default:
			return "", fmt.Errorf("node %q has unknown type %q", current.ID, current.Type)
		}

		next, ok := nodesByID[nextID]
		if !ok {
			return "", fmt.Errorf("edge from node %q targets unknown node %q", current.ID, nextID)
		}
		current = &next
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
