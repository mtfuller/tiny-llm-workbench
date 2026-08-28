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
// OutputSchema made that output parseable JSON. A loop node additionally
// exposes {{Name.iteration}} (its 1-based visit count this turn). A node
// with no Name isn't referenceable at all, just unused metadata.
//
// Tool nodes name a real Tool (see environments.go) declared on the agent's
// bound Environment, rather than embedding a raw shell command — the same
// structured-parameter-list schema the Environments workspace's Playground
// tab already uses to run a tool. Every value in ToolArgs (and
// PromptTemplate, MatchTemplate, StateValue, AgentInstructions) may itself
// contain {{...}} template references, resolved against every node that
// already ran earlier in the same turn — not just the immediately
// preceding node.
type NodeData struct {
	Name string `json:"name,omitempty"`

	Model          string `json:"model,omitempty"`          // prompt nodes: which MLX model to call
	SystemPrompt   string `json:"systemPrompt,omitempty"`   // prompt nodes
	PromptTemplate string `json:"promptTemplate,omitempty"` // prompt nodes: templated user turn text; falls back to the previous node's raw output when empty
	OutputSchema   string `json:"outputSchema,omitempty"`   // prompt nodes: optional JSON Schema the reply must validate against; on a mismatch the node routes to its "fail" handle if wired, else the turn fails

	// condition nodes: a single deterministic check (the fields of
	// registry.Assertion, inlined) run via internal/assertions.Check against
	// MatchTemplate's rendered text — routing to the "pass" or "fail"
	// outgoing handle. MatchTemplate falls back to the inbound value when
	// empty, same convention as PromptTemplate. This generalizes the former
	// "decision" node (keyword-only) to contains/not_contains/regex/
	// json_schema/similarity.
	ConditionType      string  `json:"conditionType,omitempty"`
	ConditionValue     string  `json:"conditionValue,omitempty"`
	ConditionThreshold float64 `json:"conditionThreshold,omitempty"`
	MatchTemplate      string  `json:"matchTemplate,omitempty"`

	// switch nodes: an ordered list of cases checked (case-insensitive
	// substring, first match wins) against MatchTemplate's rendered text
	// (falling back to the inbound value when empty, same as a condition
	// node). Each case's Value is also the name of its outgoing handle; a
	// node with no matching case takes the "default" handle. This is the
	// N-way sibling of the two-way condition node.
	SwitchCases []SwitchCase `json:"switchCases,omitempty"`

	// loop_start / loop_end nodes: a loop is a matched pair. A loop_start has
	// two outgoing handles, "body" (into the loop) and "done" (taken once the
	// walk has entered it LoopMaxIterations times — engine applies a default
	// when <= 0), and exposes {{Name.iteration}}. A loop_end names its
	// loop_start via LoopStartName; reaching a loop_end jumps the walk back to
	// that loop_start for the next iteration (the back-edge is implicit — the
	// user never draws it). Any number of body branches can converge on a
	// loop_end to "continue"; a branch wired anywhere else "breaks" out.
	LoopMaxIterations int    `json:"loopMaxIterations,omitempty"`
	LoopStartName     string `json:"loopStartName,omitempty"`

	// state nodes: StateOp is "set" or "append"; StateValue is the templated
	// value (falls back to the inbound value when empty). The accumulator
	// lives under the node's own Name, so {{StateNodeName}} reads its current
	// value — there's no separate variable namespace.
	StateOp    string `json:"stateOp,omitempty"`
	StateValue string `json:"stateValue,omitempty"`

	ToolName string            `json:"toolName,omitempty"` // tool nodes: name of a Tool on the agent's Environment
	ToolArgs map[string]string `json:"toolArgs,omitempty"` // tool nodes: templated value per parameter name

	// say nodes: emit a user-facing message mid-turn. SayTemplate is the
	// templated text (falls back to the inbound value when empty, same
	// convention as PromptTemplate). SayFinal marks it as the turn's
	// definitive reply rather than a progress update: the last "final" say
	// message emitted during a turn is what the run returns; if no say node
	// is marked final, the terminal node's own output is the reply (the
	// pre-say-node behavior). Progress messages stream to the chat UI live
	// but are not added to the run's conversation history.
	SayTemplate string `json:"sayTemplate,omitempty"`
	SayFinal    bool   `json:"sayFinal,omitempty"`

	// agent nodes: a bounded LLM tool-calling loop — the model is asked to
	// emit ACTION/ARGS (to call one of AgentTools, a subset of the bound
	// Environment's tools; or the built-in "knowledge_search" when
	// AgentKnowledgeBases is non-empty) or FINAL (to answer).
	// AgentInstructions is the templated goal/system text; AgentMaxIterations
	// caps the internal loop (engine default when <= 0). AgentOutputSchema,
	// if set, is a JSON Schema the final answer must validate against — on a
	// mismatch the node routes to its "fail" handle if one is wired
	// (degrade-and-retry), else the turn fails; on success the parsed value
	// is referenceable downstream as {{Name.property}}, same as a prompt
	// node's OutputSchema. This is a deliberate, scoped exception to the
	// project's otherwise-deterministic control flow.
	AgentInstructions   string   `json:"agentInstructions,omitempty"`
	AgentModel          string   `json:"agentModel,omitempty"`
	AgentMaxIterations  int      `json:"agentMaxIterations,omitempty"`
	AgentTools          []string `json:"agentTools,omitempty"`
	AgentKnowledgeBases []string `json:"agentKnowledgeBases,omitempty"`
	AgentOutputSchema   string   `json:"agentOutputSchema,omitempty"`

	// KnowledgeBaseName and KnowledgeQuery are knowledge nodes: which
	// registry.KnowledgeBase to search (independent of any Environment —
	// querying records doesn't need a container) and what templated query
	// text to search with; an empty KnowledgeQuery falls back to the
	// previous node's raw output, same convention as PromptTemplate and
	// MatchTemplate. KnowledgeMaxResults caps how many matching records are
	// passed downstream (0 = all, keeping the previous behavior).
	KnowledgeBaseName   string `json:"knowledgeBaseName,omitempty"`
	KnowledgeQuery      string `json:"knowledgeQuery,omitempty"`
	KnowledgeMaxResults int    `json:"knowledgeMaxResults,omitempty"`
}

// SwitchCase is one branch of a switch node. Value is matched
// case-insensitively as a substring of the switch's rendered match text, and
// is also the name of the outgoing edge handle taken when it matches.
type SwitchCase struct {
	Value string `json:"value"`
}

// Node is one node in an agent's graph. Type is one of "input", "prompt",
// "condition", "switch", "loop_start", "loop_end", "state", "say", "tool",
// "knowledge", "agent". There's no dedicated "output" type — a node with no
// outgoing edge for the handle it produces is simply where a turn ends (see
// internal/agents.Engine), so any node type can be a graph's terminal. Edges
// may form cycles: a loop_start/loop_end pair (or a condition routing back to
// an earlier node) is how plan-execute-judge, Ralph loops, and similar
// architectures are expressed.
type Node struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Position Position `json:"position"`
	Data     NodeData `json:"data"`
}

// Edge connects two nodes. SourceHandle distinguishes a node's multiple
// outgoing handles: "pass"/"fail" for a condition node (also "fail" on a
// prompt/agent node with an OutputSchema, taken on a validation mismatch),
// "body"/"done" for a loop_start node, and each SwitchCase.Value plus
// "default" for a switch node. It's empty for every other node type, which
// only ever has one outgoing edge. A loop_end's jump back to its loop_start
// is not stored as an Edge — it's derived from the pairing (see
// internal/agents).
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
