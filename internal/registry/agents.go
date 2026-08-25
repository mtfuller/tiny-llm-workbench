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
type NodeData struct {
	Label        string `json:"label,omitempty"`
	Model        string `json:"model,omitempty"`        // prompt nodes: which Ollama model to call
	SystemPrompt string `json:"systemPrompt,omitempty"` // prompt nodes
	Keyword      string `json:"keyword,omitempty"`      // decision nodes: substring to match
	Command      string `json:"command,omitempty"`      // tool nodes: shell command; "{{input}}" is replaced with the prior node's output
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
// need one.
type Agent struct {
	Name        string    `json:"name"`
	Environment string    `json:"environment,omitempty"`
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
// for both creating a new agent and saving edits to its graph.
func (r *Registry) SaveAgent(agent Agent) error {
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
