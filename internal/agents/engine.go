// Package agents executes agent workflow graphs (registry.Graph) as chat
// turns: starting at the graph's input node, calling a local LLM for each
// prompt node, branching at decision nodes on a keyword match against the
// prior output, running a literal shell command for each tool node inside
// the agent's bound Environment, and returning whatever text reaches the
// output node.
package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

// llmClient is the subset of ollama.Client the engine needs.
type llmClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// toolRunner is the subset of environments.Manager the engine needs to
// execute a tool node's command.
type toolRunner interface {
	RunToolSync(ctx context.Context, instanceID, command string) (string, error)
}

// Engine walks an agent's graph to produce one chat reply.
type Engine struct {
	llm   llmClient
	tools toolRunner
}

// NewEngine builds an Engine that calls models via llm and runs tool node
// commands via tools.
func NewEngine(llm llmClient, tools toolRunner) *Engine {
	return &Engine{llm: llm, tools: tools}
}

// Run executes graph for one turn: starting at its input node with
// userMessage, following prompt/decision/tool nodes until an output node is
// reached, and returning the text that arrives there. history is the
// conversation so far (not including userMessage), used to give prompt
// nodes context. instanceID is the agent's launched Environment instance
// (empty if the agent has none configured) — required by any tool node in
// the graph. onStep, if non-nil, is called for every node visited.
func (e *Engine) Run(ctx context.Context, graph registry.Graph, history []ChatMessage, userMessage, instanceID string, onStep func(StepEvent)) (string, error) {
	nodesByID := make(map[string]registry.Node, len(graph.Nodes))
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
	}
	if inputNode == nil {
		return "", errors.New("graph has no input node")
	}

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
			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("input node %q has no outgoing edge", current.ID)
			}
			nextID = edge.Target

		case "prompt":
			reply, err := e.llm.Generate(ctx, current.Data.Model, buildPrompt(current.Data.SystemPrompt, history, output))
			if err != nil {
				return "", fmt.Errorf("prompt node %q: %w", current.ID, err)
			}
			output = reply

			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("prompt node %q has no outgoing edge", current.ID)
			}
			nextID = edge.Target

		case "decision":
			handle := "no"
			if current.Data.Keyword != "" && strings.Contains(strings.ToLower(output), strings.ToLower(current.Data.Keyword)) {
				handle = "yes"
			}
			edge := findEdge(graph.Edges, current.ID, handle)
			if edge == nil {
				return "", fmt.Errorf("decision node %q has no %q edge", current.ID, handle)
			}
			nextID = edge.Target

		case "tool":
			if instanceID == "" {
				return "", fmt.Errorf("tool node %q requires an Environment to be configured for this agent", current.ID)
			}
			command := strings.ReplaceAll(current.Data.Command, "{{input}}", output)
			result, err := e.tools.RunToolSync(ctx, instanceID, command)
			if err != nil {
				return "", fmt.Errorf("tool node %q: %w", current.ID, err)
			}
			output = result

			edge := findEdge(graph.Edges, current.ID, "")
			if edge == nil {
				return "", fmt.Errorf("tool node %q has no outgoing edge", current.ID)
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
