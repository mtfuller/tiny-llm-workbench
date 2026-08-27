package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const agentMetadataFile = "definition.json"

// Position is a node's location on the canvas.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// NodeData holds every node type's config in one place — which fields
// matter depends on the node's Type. Keeping this flat (rather than one
// struct per node type) matches React Flow's own node data shape and keeps
// (de)serialization simple.
//
// Name is a stable, user-editable, unique-within-the-graph display name —
// used both on the canvas and as the token a downstream node's template
// references (see internal/agents.Engine): {{Name}} for a node's raw text
// output, or {{Name.property}} for a specific property of a node whose
// OutputSchema made that output parseable JSON. A node with no Name isn't
// referenceable at all, just unused metadata.
//
// Tool nodes name a real Tool (see environments.go) declared on the agent's
// bound Environment, rather than embedding a raw shell command — the same
// structured-parameter-list schema the Environments workspace's Playground
// tab already uses to run a tool. Every value in ToolArgs (and
// PromptTemplate, MatchTemplate) may itself contain {{...}} template
// references, resolved against every node that already ran earlier in the
// same turn — not just the immediately preceding node.
type NodeData struct {
	Name string `json:"name,omitempty"`

	Model          string `json:"model,omitempty"`          // prompt nodes: which MLX model to call
	SystemPrompt   string `json:"systemPrompt,omitempty"`   // prompt nodes
	PromptTemplate string `json:"promptTemplate,omitempty"` // prompt nodes: templated user turn text; falls back to the previous node's raw output when empty
	OutputSchema   string `json:"outputSchema,omitempty"`   // prompt nodes: optional JSON Schema the reply must validate against; failing validation fails the turn

	Keyword       string `json:"keyword,omitempty"`       // decision nodes: substring to match
	MatchTemplate string `json:"matchTemplate,omitempty"` // decision nodes: templated text to search Keyword within; falls back to the previous node's raw output when empty

	ToolName string            `json:"toolName,omitempty"` // tool nodes: name of a Tool on the agent's Environment
	ToolArgs map[string]string `json:"toolArgs,omitempty"` // tool nodes: templated value per parameter name
}

// Node is one node in an agent's graph. Type is one of "input", "prompt",
// "decision", "tool", "output".
type Node struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Position Position `json:"position"`
	Data     NodeData `json:"data"`
}

// Edge connects two nodes. SourceHandle distinguishes a decision node's two
// outgoing edges ("yes"/"no"); it's empty for every other node type, which
// only ever has one outgoing edge.
type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceHandle string `json:"sourceHandle,omitempty"`
	Target       string `json:"target"`
}

// Graph is an agent's full node/edge workflow.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Agent is a registry-tracked agent workflow definition. Environment is
// optional — it's the Environment (see environments.go) a run launches an
// instance of for the run's duration, giving the graph's Tool nodes
// something to execute commands in. An agent with no Tool nodes doesn't
// need one. Description is free-text, shown on the list/detail pages —
// purely informational, no behavior depends on it.
type Agent struct {
	Name        string    `json:"name"`
	Environment string    `json:"environment,omitempty"`
	Description string    `json:"description,omitempty"`
	Graph       Graph     `json:"graph"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (r *Registry) agentDir(name string) string {
	return filepath.Join(r.agentsDir(), name)
}

func (r *Registry) agentsDir() string {
	return filepath.Join(r.root, "agents")
}

// SaveAgent writes agent's definition, creating or overwriting it — used
// for both creating a new agent and saving edits to its graph. CreatedAt is
// set on first save and preserved on every later overwrite, regardless of
// what the caller passed in (mirrors the same fix in SaveBenchmark/
// SaveEvaluation).
func (r *Registry) SaveAgent(agent Agent) error {
	if existing, err := r.GetAgent(agent.Name); err == nil {
		agent.CreatedAt = existing.CreatedAt
	} else if agent.CreatedAt.IsZero() {
		agent.CreatedAt = time.Now().UTC()
	}

	dir := r.agentDir(agent.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create agent directory: %w", err)
	}

	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent definition: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, agentMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write agent definition: %w", err)
	}

	return nil
}

// GetAgent returns the named agent's definition.
func (r *Registry) GetAgent(name string) (Agent, error) {
	data, err := os.ReadFile(filepath.Join(r.agentDir(name), agentMetadataFile))
	if err != nil {
		return Agent{}, fmt.Errorf("read agent %q: %w", name, err)
	}

	var agent Agent
	if err := json.Unmarshal(data, &agent); err != nil {
		return Agent{}, fmt.Errorf("parse definition for agent %q: %w", name, err)
	}

	return agent, nil
}

// DeleteAgent removes an agent's directory (its graph definition). It's an
// error to delete an agent that doesn't exist.
func (r *Registry) DeleteAgent(name string) error {
	dir := r.agentDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("agent %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete agent %q: %w", name, err)
	}
	return nil
}

// ListAgents returns every registry-tracked agent, sorted by name.
func (r *Registry) ListAgents() ([]Agent, error) {
	entries, err := os.ReadDir(r.agentsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agents directory: %w", err)
	}

	var agents []Agent
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		agent, err := r.GetAgent(entry.Name())
		if err != nil {
			continue // skip directories without a valid definition
		}
		agents = append(agents, agent)
	}

	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })

	return agents, nil
}
