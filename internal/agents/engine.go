// Package agents executes agent workflow graphs (registry.Graph) as chat
// turns: starting at the graph's input node, calling a local LLM for each
// prompt node, branching at condition nodes (two-way) and switch nodes
// (N-way) on a deterministic match, running a
// named Tool from the agent's Tools set for each tool node,
// deterministically keyword-searching a named KnowledgeBase for each
// knowledge node (independent of any Environment — see internal/knowledge),
// emitting a user-facing progress or final message for each "say" node, and
// returning whatever text the walk ends on. There's no dedicated
// "output" node type: a node with no outgoing edge for the handle it
// produces is simply where the turn ends, and its own output becomes the
// reply (unless a "say" node marked final ran, in which case that message
// is the reply) — this lets any node type serve as a graph's terminal, and
// means a user never has to remember to wire up a trailing node just to mark
// "this is the end." Every node's output is kept (by its user-chosen Name) for
// the rest of the turn, so a downstream node's template can reference any
// earlier node's output — not just its immediate predecessor — and, for a
// prompt node with a declared output JSON Schema, a specific property of
// it. See runcontext.go.
package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/knowledge"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// maxSteps bounds how many nodes a single turn can visit. Cycles are a
// supported, first-class way to build agent architectures (loop and
// condition nodes routing back to an earlier node), so this is a backstop
// against an *unbounded* cycle — one with no loop node capping its
// iterations and no condition that ever routes out — not a ban on cycles.
const maxSteps = 250

// defaultLoopMaxIterations caps a loop node whose LoopMaxIterations is unset.
const defaultLoopMaxIterations = 10

// defaultAgentMaxIterations caps an agent node's internal tool-calling loop
// when AgentMaxIterations is unset.
const defaultAgentMaxIterations = 6

// schemaMismatchError is what runNode returns when a prompt or agent node's
// declared output schema rejects the model's reply. It's recoverable: if the
// node has a "fail" edge wired, the walk follows it (soft mode — e.g. a retry
// loop); with no "fail" edge it's surfaced as a hard turn failure. The
// offending text is still returned as the node's output so a fail branch can
// inspect or re-prompt on it.
type schemaMismatchError struct {
	nodeID string
	err    error
}

func (e *schemaMismatchError) Error() string { return e.err.Error() }
func (e *schemaMismatchError) Unwrap() error { return e.err }

// resolveStepErr post-processes a runNode error against the graph's edges so
// Run and the step-by-step debugger treat a schema failure identically: a
// schemaMismatchError with a wired "fail" edge becomes a soft route
// (returns handle "fail", nil fatal); anything else passes through as fatal.
func resolveStepErr(edges []registry.Edge, nodeID string, err error) (softHandle string, fatal error) {
	var sm *schemaMismatchError
	if errors.As(err, &sm) && findEdge(edges, nodeID, "fail") != nil {
		return "fail", nil
	}
	return "", err
}

// ChatMessage is one turn in a run's conversation history.
type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// StepEvent reports something a node did while executing a turn, for the Run
// view's live event log and the step-by-step debugger. Phase distinguishes:
//   - "start"  — a node is beginning long-running work (Output is a short
//     status like "calling model X"), so the debugger isn't a black box
//     while a prompt/agent node waits on the model;
//   - "tool"   — a tool node / an agent node's ReAct loop just invoked a
//     tool; Command holds the exact rendered shell command that ran (or the
//     built-in knowledge_search call), so the activity feed shows the raw
//     tool call, not just its summarised result;
//   - ""       — the normal case: Output is what the node itself produced.
//
// "start" and "tool" events are stream-only — they never become the
// debugger's LastStep.
type StepEvent struct {
	NodeID   string `json:"nodeId"`
	NodeType string `json:"nodeType"`
	Output   string `json:"output"`
	Command  string `json:"command,omitempty"` // set on "tool" phase events
	Phase    string `json:"phase,omitempty"`   // "" (result), "start", or "tool"
}

// TurnMessage is a user-facing message a "say" node emits mid-turn: a
// progress update (Kind "progress"), shown live in the chat as the agent
// works, or the turn's definitive reply (Kind "final"). The engine streams
// these via RunHooks.OnMessage; the last "final" one emitted is what Run
// returns as the turn's reply, and if none is emitted the terminal node's
// own output is used instead (the pre-say-node behavior).
type TurnMessage struct {
	Kind    string `json:"kind"` // "progress" | "final"
	Content string `json:"content"`
}

const (
	turnMessageProgress = "progress"
	turnMessageFinal    = "final"
)

// RunHooks carries the optional per-turn callbacks Run and the step-by-step
// debugger invoke while walking the graph. A nil *RunHooks, or a nil field,
// disables the corresponding callback — the emit* helpers are nil-safe so
// callers never have to check.
type RunHooks struct {
	// OnStep fires once per node visited, and once per internal iteration of
	// an agent node, for the Run view's live execution trace.
	OnStep func(StepEvent)
	// OnMessage fires when a "say" node emits a user-facing message, for
	// live chat streaming.
	OnMessage func(TurnMessage)
}

func (h *RunHooks) emitStep(s StepEvent) {
	if h != nil && h.OnStep != nil {
		h.OnStep(s)
	}
}

func (h *RunHooks) emitMessage(m TurnMessage) {
	if h != nil && h.OnMessage != nil {
		h.OnMessage(m)
	}
}

// llmClient is the subset of mlxrunner.Runner the engine needs to call a
// model for a prompt node.
type llmClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// toolRunner is the subset of environments.Manager the engine needs to
// execute a tool node's command inside the run's workspace sandbox.
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

// prepareGraph validates graph (exactly one input node; no two nodes sharing
// a Name — a template ambiguity better caught up front than mid-turn; every
// loop_end resolves to a loop_start) and returns the input node, a by-ID
// index, and the edge list augmented with a synthetic back-edge per loop_end
// (loop_end -> its paired loop_start). All three are reused by Run and by the
// step-by-step debugger (see debug.go) so both walk the graph identically.
func prepareGraph(graph registry.Graph) (*registry.Node, map[string]registry.Node, []registry.Edge, error) {
	nodesByID := make(map[string]registry.Node, len(graph.Nodes))
	seenNames := make(map[string]bool, len(graph.Nodes))
	loopStartIDByName := make(map[string]string)
	var inputNode *registry.Node
	var onlyLoopStartID string
	loopStartCount := 0

	for i := range graph.Nodes {
		node := graph.Nodes[i]
		nodesByID[node.ID] = node
		if node.Type == "input" {
			if inputNode != nil {
				return nil, nil, nil, errors.New("graph has more than one input node")
			}
			inputNode = &graph.Nodes[i]
		}
		if node.Type == "loop_start" {
			loopStartCount++
			onlyLoopStartID = node.ID
			if node.Data.Name != "" {
				loopStartIDByName[node.Data.Name] = node.ID
			}
		}
		if node.Data.Name != "" {
			if seenNames[node.Data.Name] {
				return nil, nil, nil, fmt.Errorf("more than one node is named %q — node names must be unique to be referenced in templates", node.Data.Name)
			}
			seenNames[node.Data.Name] = true
		}
	}
	if inputNode == nil {
		return nil, nil, nil, errors.New("graph has no input node")
	}

	edges := append(make([]registry.Edge, 0, len(graph.Edges)+2), graph.Edges...)
	for i := range graph.Nodes {
		node := graph.Nodes[i]
		if node.Type != "loop_end" {
			continue
		}
		targetID := loopStartIDByName[node.Data.LoopStartName]
		if targetID == "" && node.Data.LoopStartName == "" && loopStartCount == 1 {
			targetID = onlyLoopStartID // convenience: a lone loop pair needs no explicit reference
		}
		if targetID == "" {
			return nil, nil, nil, fmt.Errorf("loop end node %q does not point to a loop start (set its loop start, or its name %q matches none)", node.ID, node.Data.LoopStartName)
		}
		edges = append(edges, registry.Edge{ID: "loopback-" + node.ID, Source: node.ID, Target: targetID})
	}

	return inputNode, nodesByID, edges, nil
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
// hooks, if non-nil, carries per-turn callbacks: OnStep for every node
// visited (with what that node itself produced) and OnMessage for each
// user-facing message a "say" node emits.
//
// The turn's reply is the last "final" TurnMessage a say node emitted, or —
// if no say node was marked final — the terminal node's own output.
//
// Every node that declares a Name (registry.NodeData.Name) becomes
// referenceable in a later node's template fields (PromptTemplate,
// MatchTemplate, ToolArgs values) as {{Name}} (its raw text output) or, if
// it's a prompt node with OutputSchema configured, {{Name.property}} for a
// specific property of its validated JSON reply — see runcontext.go.
func (e *Engine) Run(ctx context.Context, graph registry.Graph, history []ChatMessage, userMessage, instanceID string, tools []registry.Tool, hooks *RunHooks) (string, error) {
	inputNode, nodesByID, edges, err := prepareGraph(graph)
	if err != nil {
		return "", err
	}

	rc := newRunContext()
	current := inputNode
	output := userMessage

	// Wrap the caller's hooks so we can capture the last "final" say message
	// as the turn's reply while still forwarding every message on.
	var finalOverride *string
	inner := &RunHooks{
		OnStep: func(s StepEvent) { hooks.emitStep(s) },
		OnMessage: func(m TurnMessage) {
			if m.Kind == turnMessageFinal {
				c := m.Content
				finalOverride = &c
			}
			hooks.emitMessage(m)
		},
	}

	for step := 0; ; step++ {
		if step >= maxSteps {
			return "", fmt.Errorf("agent exceeded %d steps — an unbounded loop in the graph; give a loop start a max, or add a condition that eventually routes out", maxSteps)
		}

		newOutput, handle, err := e.runNode(ctx, *current, output, instanceID, tools, history, rc, inner)
		if err != nil {
			soft, fatal := resolveStepErr(edges, current.ID, err)
			if fatal != nil {
				return "", fatal
			}
			handle = soft // schema mismatch with a wired "fail" edge — route down it
		}
		output = newOutput

		inner.emitStep(StepEvent{NodeID: current.ID, NodeType: current.Type, Output: output})

		edge := nextEdge(edges, current.ID, current.Type, handle)
		if edge == nil {
			// No outgoing edge for this node's handle — the turn ends here.
			if finalOverride != nil {
				return *finalOverride, nil
			}
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
// handle to follow next: "" for most node types, "pass"/"fail" for a
// condition node, "body"/"done" for a loop_start node. Shared by Run's loop
// above and the step-by-step debugger (see debug.go) so both execute a
// node identically. hooks (nil-safe) carries OnStep — one event per
// internal iteration of an agent node's tool-calling loop — and OnMessage,
// invoked when a "say" node emits a user-facing message.
func (e *Engine) runNode(ctx context.Context, node registry.Node, input, instanceID string, tools []registry.Tool, history []ChatMessage, rc *runContext, hooks *RunHooks) (output, handle string, err error) {
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

		emitNodeStart(hooks, node.ID, node.Type, node.Data.Model)
		reply, err := e.llm.Generate(ctx, node.Data.Model, buildPrompt(node.Data.SystemPrompt, history, userTurn))
		if err != nil {
			return "", "", fmt.Errorf("prompt node %q: %w", node.ID, err)
		}

		if node.Data.OutputSchema != "" {
			parsed, verr := assertions.ValidateJSONSchema(node.Data.OutputSchema, reply)
			if verr != nil {
				// Keep the raw reply referenceable as {{Name}} for a fail
				// branch to re-prompt on; {{Name.property}} stays unavailable
				// (no validated value). Run/debugger route to "fail" if wired.
				rc.set(node.Data.Name, reply, nil)
				return reply, "fail", &schemaMismatchError{node.ID, fmt.Errorf("prompt node %q: reply did not satisfy its output schema: %w", node.ID, verr)}
			}
			rc.set(node.Data.Name, reply, parsed)
			return reply, "", nil
		}

		rc.set(node.Data.Name, reply, nil)
		return reply, "", nil

	case "condition":
		matchText := input
		if node.Data.MatchTemplate != "" {
			rendered, err := rc.render(node.Data.MatchTemplate)
			if err != nil {
				return "", "", fmt.Errorf("condition node %q: %w", node.ID, err)
			}
			matchText = rendered
		}

		a := registry.Assertion{
			Type:      node.Data.ConditionType,
			Value:     node.Data.ConditionValue,
			Threshold: node.Data.ConditionThreshold,
		}
		ok, err := assertions.Check(a, matchText)
		if err != nil {
			return "", "", fmt.Errorf("condition node %q: %w", node.ID, err)
		}
		handle := "fail"
		if ok {
			handle = "pass"
		}
		// Pass the inbound value through unchanged so the real payload keeps
		// flowing on either branch — the condition only routes, it doesn't
		// transform.
		rc.set(node.Data.Name, input, nil)
		return input, handle, nil

	case "switch":
		matchText := input
		if node.Data.MatchTemplate != "" {
			rendered, err := rc.render(node.Data.MatchTemplate)
			if err != nil {
				return "", "", fmt.Errorf("switch node %q: %w", node.ID, err)
			}
			matchText = rendered
		}
		lower := strings.ToLower(matchText)
		handle := "default"
		for _, c := range node.Data.SwitchCases {
			if c.Value != "" && strings.Contains(lower, strings.ToLower(c.Value)) {
				handle = c.Value
				break
			}
		}
		// Route-only, like condition: the inbound payload flows on unchanged.
		rc.set(node.Data.Name, input, nil)
		return input, handle, nil

	case "loop_start":
		// Keyed by node ID (always unique and present) so an unnamed loop
		// still counts and terminates; the count is also exposed under the
		// node's Name for {{Name.iteration}} templates. The walk re-enters
		// here from a paired loop_end (via the synthetic back-edge built in
		// prepareGraph), so the max check happens on every iteration.
		n := rc.loopCounts[node.ID] + 1
		rc.loopCounts[node.ID] = n
		max := node.Data.LoopMaxIterations
		if max <= 0 {
			max = defaultLoopMaxIterations
		}
		rc.set(node.Data.Name, input, map[string]any{"iteration": n, "input": input})
		if n > max {
			// Reset so that if this loop_start is itself the body of an
			// enclosing loop, the next time the outer loop re-enters it the
			// count starts fresh (nested "loop 2×" inside "loop 2×" = 4 inner
			// passes, not 2 then immediate exit).
			rc.loopCounts[node.ID] = 0
			return input, "done", nil
		}
		return input, "body", nil

	case "loop_end":
		// A pure marker: it passes the inbound value through, and the
		// synthetic back-edge from prepareGraph carries the walk to the
		// paired loop_start. Whatever flowed into here is what that
		// loop_start (and the body) sees next iteration.
		rc.set(node.Data.Name, input, nil)
		return input, "", nil

	case "state":
		value := input
		if node.Data.StateValue != "" {
			rendered, err := rc.render(node.Data.StateValue)
			if err != nil {
				return "", "", fmt.Errorf("state node %q: %w", node.ID, err)
			}
			value = rendered
		}

		// "append" is the default; only an explicit "set" replaces (this
		// matches the inspector's default selection and the canvas subtitle).
		newVal := value
		if node.Data.StateOp != "set" {
			existing := ""
			if node.Data.Name != "" {
				existing = rc.results[node.Data.Name].raw
			}
			if existing != "" {
				newVal = existing + "\n" + value
			}
		}
		rc.set(node.Data.Name, newVal, nil)
		return newVal, "", nil

	case "say":
		text := input
		if node.Data.SayTemplate != "" {
			rendered, err := rc.render(node.Data.SayTemplate)
			if err != nil {
				return "", "", fmt.Errorf("say node %q: %w", node.ID, err)
			}
			text = rendered
		}
		kind := turnMessageProgress
		if node.Data.SayFinal {
			kind = turnMessageFinal
		}
		hooks.emitMessage(TurnMessage{Kind: kind, Content: text})
		// The rendered text also flows on to the next node so it's
		// referenceable downstream as {{Name}} / {{Name.property}}.
		rc.set(node.Data.Name, text, parseIfJSON(text))
		return text, "", nil

	case "agent":
		final, err := e.runAgentNode(ctx, node, input, instanceID, tools, history, rc, hooks)
		if err != nil {
			return "", "", err
		}
		if node.Data.AgentOutputSchema != "" {
			parsed, verr := assertions.ValidateJSONSchema(node.Data.AgentOutputSchema, final)
			if verr != nil {
				rc.set(node.Data.Name, final, nil)
				return final, "fail", &schemaMismatchError{node.ID, fmt.Errorf("agent node %q: final answer did not satisfy its output schema: %w", node.ID, verr)}
			}
			rc.set(node.Data.Name, final, parsed)
			return final, "", nil
		}
		rc.set(node.Data.Name, final, nil)
		return final, "", nil

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
		// Stream the exact rendered command so the debug activity feed shows
		// the raw tool call, not just its result.
		hooks.emitStep(StepEvent{NodeID: node.ID, NodeType: "tool", Phase: "tool", Command: command})
		result, err := e.tools.RunToolSync(ctx, instanceID, command)
		if err != nil {
			return "", "", fmt.Errorf("tool node %q: %w", node.ID, err)
		}
		rc.set(node.Data.Name, result, parseIfJSON(result))
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

		records := knowledge.Query(kb, query)
		if max := node.Data.KnowledgeMaxResults; max > 0 && len(records) > max {
			records = records[:max]
		}
		result := knowledge.FormatResults(records)
		rc.set(node.Data.Name, result, parseIfJSON(result))
		return result, "", nil

	default:
		return "", "", fmt.Errorf("node %q has unknown type %q", node.ID, node.Type)
	}
}

// findEdge returns the first edge from nodeID with the given handle: ""
// for most node types (including a loop_end's synthetic back-edge),
// "pass"/"fail" for a condition node, "body"/"done" for a loop_start node.
func findEdge(edges []registry.Edge, nodeID, handle string) *registry.Edge {
	for i := range edges {
		if edges[i].Source == nodeID && edges[i].SourceHandle == handle {
			return &edges[i]
		}
	}
	return nil
}

// branchingNodeTypes are the node types whose outgoing handle is a genuine
// choice — a handle with no matching edge there means "that branch simply
// isn't wired", a valid way for a turn to end.
var branchingNodeTypes = map[string]bool{"condition": true, "switch": true, "loop_start": true}

// nextEdge picks the edge the walk should follow out of a node that produced
// `handle`. It prefers an exact handle match. For a *non-branching* node
// (anything but condition/switch/loop_start), if the exact match fails but
// the node has exactly one outgoing edge, it follows that edge anyway — so a
// stray or legacy sourceHandle on an otherwise-linear connection (e.g. left
// over from a since-removed output schema) doesn't silently dead-end the turn
// and echo the node's own input back. Returns nil only when the node truly
// has no way forward. Shared by Run and the step-by-step debugger.
func nextEdge(edges []registry.Edge, nodeID, nodeType, handle string) *registry.Edge {
	if e := findEdge(edges, nodeID, handle); e != nil {
		return e
	}
	if branchingNodeTypes[nodeType] {
		return nil
	}
	var only *registry.Edge
	for i := range edges {
		if edges[i].Source != nodeID {
			continue
		}
		if only != nil {
			return nil // more than one outgoing edge — can't guess which
		}
		only = &edges[i]
	}
	return only
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

// --- agent node: bounded textual-ReAct tool-calling loop -------------------

var (
	actionLineRe = regexp.MustCompile(`(?i)action\s*:\s*(.+)`)
	finalRe      = regexp.MustCompile(`(?i)\bfinal(?:\s+answer)?\s*:\s*`)
	argsRe       = regexp.MustCompile(`(?i)args\s*:`)
	fillerAction = map[string]bool{"none": true, "n/a": true, "na": true, "final": true, "nothing": true, "": true}
)

// runAgentNode runs the internal loop for an "agent" node: each iteration
// prompts AgentModel with the instructions, the available tools, and the
// running ACTION/OBSERVATION transcript, then parses the reply. A parseable
// ACTION calls the named tool (rendered + shell-quoted via the same
// environments.RenderToolCommand path tool nodes use) and appends its
// output as an OBSERVATION; a FINAL reply (or, as a graceful fallback, any
// reply with neither a usable ACTION nor a FINAL) ends the loop and becomes
// the node's output; hitting AgentMaxIterations returns the model's last
// text best-effort. hooks.OnStep (nil-safe) fires once per iteration for the
// live log.
func (e *Engine) runAgentNode(ctx context.Context, node registry.Node, input, instanceID string, tools []registry.Tool, history []ChatMessage, rc *runContext, hooks *RunHooks) (string, error) {
	instructions := input
	if node.Data.AgentInstructions != "" {
		rendered, err := rc.render(node.Data.AgentInstructions)
		if err != nil {
			return "", fmt.Errorf("agent node %q: %w", node.ID, err)
		}
		instructions = rendered
	}

	// The subset of the environment's tools this node may call; an unknown
	// name is skipped (same graceful degradation as Manager.resolveTools).
	var usable []registry.Tool
	for _, name := range node.Data.AgentTools {
		if t, ok := findTool(tools, name); ok {
			usable = append(usable, t)
		}
	}
	if len(node.Data.AgentTools) > 0 && instanceID == "" {
		return "", fmt.Errorf("agent node %q has tools selected but this agent has no Environment configured", node.ID)
	}

	// KnowledgeBases the model can search via the built-in "knowledge_search"
	// pseudo-tool (no Environment needed — see the knowledge node). An
	// unresolvable name is skipped.
	var kbs []registry.KnowledgeBase
	for _, name := range node.Data.AgentKnowledgeBases {
		if kb, err := e.knowledge.GetKnowledgeBase(name); err == nil {
			kbs = append(kbs, kb)
		}
	}

	maxIter := node.Data.AgentMaxIterations
	if maxIter <= 0 {
		maxIter = defaultAgentMaxIterations
	}

	emitNodeStart(hooks, node.ID, node.Type, node.Data.AgentModel)

	var transcript strings.Builder
	lastReply := ""
	for i := 0; i < maxIter; i++ {
		reply, err := e.llm.Generate(ctx, node.Data.AgentModel, buildAgentPrompt(instructions, usable, kbs, history, transcript.String()))
		if err != nil {
			return "", fmt.Errorf("agent node %q: %w", node.ID, err)
		}
		lastReply = reply

		action, rawArgs := parseAction(reply)
		if fillerAction[strings.ToLower(action)] {
			if final, ok := parseFinal(reply); ok {
				return final, nil
			}
			// Neither a real action nor a FINAL — take the whole reply.
			return strings.TrimSpace(reply), nil
		}

		args := map[string]string{}
		if rawArgs != "" {
			_ = json.Unmarshal([]byte(rawArgs), &args) // tolerant: bad JSON leaves args empty; tool validation then reports it
		}
		for k, v := range args {
			if r, err := rc.render(v); err == nil {
				args[k] = r
			}
		}

		// The built-in knowledge_search pseudo-tool short-circuits the normal
		// tool dispatch. Tiny models rarely emit the exact name — accept
		// "knowledge_search", a bare "knowledge"/"kb", or the literal name of
		// one of the bound bases (in which case only that base is searched).
		if searchKBs, ok := matchKnowledgeSearch(action, kbs); ok {
			// Tiny models are unreliable about the exact arg key: accept the
			// documented "query", any common synonym, else the first arg value,
			// else fall back to the node's own input (same "empty query falls
			// back to the previous output" convention the knowledge node uses).
			query := firstNonEmpty(args["query"], args["q"], args["search"], args["question"], args["input"])
			if query == "" {
				for _, v := range args {
					if strings.TrimSpace(v) != "" {
						query = v
						break
					}
				}
			}
			if query == "" {
				query = input
			}
			var recs []registry.KnowledgeRecord
			for _, kb := range searchKBs {
				recs = append(recs, knowledge.Query(kb, query)...)
			}
			obs := knowledge.FormatResults(recs)
			emitToolCall(hooks, node.ID, fmt.Sprintf("knowledge_search %s", strings.TrimSpace(rawArgs)))
			transcript.WriteString(fmt.Sprintf("ACTION: %s\nARGS: %s\nOBSERVATION: %s\n\n", action, rawArgs, truncate(obs, 500)))
			emitAgentStep(hooks, node.ID, i+1, action, obs)
			continue
		}

		tool, found := findTool(usable, action)
		if !found {
			available := toolNames(usable)
			if len(kbs) > 0 {
				available = append(available, knowledgeSearchTool)
			}
			obs := fmt.Sprintf("error: no tool named %q (available: %s)", action, strings.Join(available, ", "))
			transcript.WriteString(fmt.Sprintf("ACTION: %s\nOBSERVATION: %s\n\n", action, obs))
			emitAgentStep(hooks, node.ID, i+1, action, obs)
			continue
		}

		obs := ""
		if command, err := environments.RenderToolCommand(tool, args); err != nil {
			obs = "error: " + err.Error()
		} else {
			// Stream the exact rendered command every time the model calls a
			// tool, so the debug activity feed shows the raw tool call.
			emitToolCall(hooks, node.ID, command)
			if out, err := e.tools.RunToolSync(ctx, instanceID, command); err != nil {
				obs = "error: " + err.Error()
			} else {
				obs = strings.TrimSpace(out)
			}
		}

		transcript.WriteString(fmt.Sprintf("ACTION: %s\nARGS: %s\nOBSERVATION: %s\n\n", action, rawArgs, truncate(obs, 500)))
		emitAgentStep(hooks, node.ID, i+1, action, obs)
	}

	return strings.TrimSpace(lastReply), nil
}

// knowledgeSearchTool is the built-in pseudo-tool name an agent node's model
// uses to query the node's AgentKnowledgeBases.
const knowledgeSearchTool = "knowledge_search"

func emitAgentStep(hooks *RunHooks, nodeID string, iter int, action, obs string) {
	hooks.emitStep(StepEvent{NodeID: nodeID, NodeType: "agent", Output: fmt.Sprintf("iteration %d: %s -> %s", iter, action, truncate(obs, 200))})
}

// emitToolCall streams a stream-only "tool" phase event carrying the exact
// tool invocation (a rendered shell command, or the built-in
// knowledge_search call) so the debug activity feed shows the raw tool call
// separately from its summarised result.
func emitToolCall(hooks *RunHooks, nodeID, command string) {
	hooks.emitStep(StepEvent{NodeID: nodeID, NodeType: "agent", Phase: "tool", Command: command})
}

// emitNodeStart streams a "start" phase event just before a node begins
// work that can block for a while (an LLM call — cold model-server start,
// download, generation), so the step debugger can show "calling model X…"
// instead of freezing silently.
func emitNodeStart(hooks *RunHooks, nodeID, nodeType, model string) {
	msg := "calling model"
	if model != "" {
		msg = "calling model " + model
	}
	hooks.emitStep(StepEvent{NodeID: nodeID, NodeType: nodeType, Output: msg, Phase: "start"})
}

// buildAgentPrompt assembles one iteration's completion prompt for an agent
// node — the instructions, the prior conversation turns (so a chat agent
// isn't amnesiac mid-conversation, matching a prompt node), the available
// tools (plus the built-in knowledge_search when kbs is non-empty), and the
// running ACTION/OBSERVATION transcript.
func buildAgentPrompt(instructions string, tools []registry.Tool, kbs []registry.KnowledgeBase, history []ChatMessage, transcript string) string {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString("\n\n")

	if len(history) > 0 {
		b.WriteString("Conversation so far:\n")
		for _, m := range history {
			b.WriteString(strings.ToUpper(m.Role))
			b.WriteString(": ")
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(tools) > 0 || len(kbs) > 0 {
		b.WriteString("You can use these tools:\n")
		for _, t := range tools {
			b.WriteString("- ")
			b.WriteString(t.Name)
			if len(t.Parameters) > 0 {
				params := make([]string, len(t.Parameters))
				for i, p := range t.Parameters {
					params[i] = fmt.Sprintf("%s: %s", p.Name, p.Type)
				}
				b.WriteString("(" + strings.Join(params, ", ") + ")")
			}
			if t.Description != "" {
				b.WriteString(" — " + t.Description)
			}
			b.WriteString("\n")
		}
		if len(kbs) > 0 {
			names := make([]string, len(kbs))
			for i, kb := range kbs {
				names[i] = kb.Name
			}
			b.WriteString(fmt.Sprintf("- %s(query: string) — search the knowledge base(s): %s\n", knowledgeSearchTool, strings.Join(names, ", ")))
		}
		b.WriteString("\nTo use a tool, reply exactly:\nACTION: <tool name>\nARGS: ")
		b.WriteString(agentArgsExample(tools, kbs))
		b.WriteString("\n\n")
	}
	b.WriteString("When you have the answer, reply:\nFINAL: <your answer>\n\n")

	if transcript != "" {
		b.WriteString(transcript)
	}
	b.WriteString("What is your next step?\nASSISTANT:")
	return b.String()
}

// parseAction pulls the tool name and raw JSON args out of a model reply.
// It tolerates "ACTION: web_search" + a later "ARGS: {...}" line, the
// inline "ACTION: web_search({...})" form, and a bare JSON object anywhere
// after the ACTION line. action is "" when no ACTION: marker is present.
func parseAction(reply string) (action, rawArgs string) {
	m := actionLineRe.FindStringSubmatchIndex(reply)
	if m == nil {
		return "", ""
	}
	rest := reply[m[2]:m[3]]
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, "`*\"' ")
	if i := strings.IndexAny(rest, "( \t"); i >= 0 {
		action = strings.TrimSpace(rest[:i])
	} else {
		action = rest
	}
	action = strings.Trim(action, "`*\"'.,: ")

	// Search for args from just after the "ACTION:" marker (m[2]) rather than
	// the end of the ACTION line, so the inline "tool({...})" form is caught
	// too. An explicit "ARGS:" marker, if present, wins.
	after := reply[m[2]:]
	if loc := argsRe.FindStringIndex(after); loc != nil {
		after = after[loc[1]:]
	}
	if v, ok := assertions.ExtractJSONValue(after); ok {
		rawArgs = v
	}
	return action, rawArgs
}

// parseFinal returns the text after a FINAL: / FINAL ANSWER: marker.
func parseFinal(reply string) (string, bool) {
	loc := finalRe.FindStringIndex(reply)
	if loc == nil {
		return "", false
	}
	return strings.TrimSpace(reply[loc[1]:]), true
}

func toolNames(tools []registry.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// agentArgsExample builds a concrete ARGS: line for the agent prompt using a
// real parameter name from the first available tool (or the knowledge_search
// pseudo-tool). Tiny models copy the example literally, so a placeholder like
// {"param": "value"} teaches them to send the key "param" — a real key name
// avoids that.
func agentArgsExample(tools []registry.Tool, kbs []registry.KnowledgeBase) string {
	for _, t := range tools {
		if len(t.Parameters) > 0 {
			return fmt.Sprintf("{%q: %q}", t.Parameters[0].Name, "value")
		}
	}
	if len(kbs) > 0 {
		return `{"query": "search terms"}`
	}
	return `{"param": "value"}`
}

// matchKnowledgeSearch decides whether a model's chosen action refers to the
// built-in knowledge_search pseudo-tool. It returns the bases to search: all
// of them for "knowledge_search"/"knowledge"/"kb"/"search", or just the one
// whose name the model named directly. ok is false when no bases are bound or
// the action is something else entirely.
func matchKnowledgeSearch(action string, kbs []registry.KnowledgeBase) ([]registry.KnowledgeBase, bool) {
	if len(kbs) == 0 || action == "" {
		return nil, false
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case knowledgeSearchTool, "knowledge", "knowledge_base", "knowledgebase", "kb", "search":
		return kbs, true
	}
	for _, kb := range kbs {
		if strings.EqualFold(action, kb.Name) {
			return []registry.KnowledgeBase{kb}, true
		}
	}
	return nil, false
}

// parseIfJSON returns the parsed JSON value embedded in s, or nil if there
// isn't one — so a tool or knowledge node whose output happens to be JSON
// (or contains a JSON blob) becomes referenceable downstream as
// {{Name.property}}, the same way a schema-validated prompt node's output is.
// A node whose output is plain text keeps parsed == nil and only {{Name}}
// resolves, unchanged.
func parseIfJSON(s string) any {
	if v, ok := assertions.ParseJSONValue(s); ok {
		return v
	}
	return nil
}

// firstNonEmpty returns the first argument that isn't blank after trimming.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
