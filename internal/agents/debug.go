// debug.go implements a paused, step-by-step counterpart to Manager's
// normal StartRun/SendMessage flow: instead of walking the whole graph in
// one call, a debug session executes exactly one node per Step call, lets
// the caller inspect what it produced, and can Retry that same node (with
// the exact input it saw) to get a fresh result before deciding to move
// on. The goal, per the user's own framing, is to make it as easy as
// possible to iterate and experiment with a graph — see engine.go's
// runNode, which both Run and this file's Step/Retry call identically so a
// debugged node behaves exactly like it would in a real run.
package agents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// DebugState is the caller-visible snapshot of a debug session, returned by
// every method below. PendingNodeID/PendingNodeType name the node the next
// Step call will execute (empty once Finished, or before the first message
// is sent); LastStep is the most recently executed node's own result (nil
// until at least one Step has run), the same one Retry would redo.
type DebugState struct {
	ID              string        `json:"id"`
	AgentName       string        `json:"agentName"`
	InstanceID      string        `json:"instanceId,omitempty"`
	Messages        []ChatMessage `json:"messages"`
	PendingNodeID   string        `json:"pendingNodeId,omitempty"`
	PendingNodeType string        `json:"pendingNodeType,omitempty"`
	LastStep        *StepEvent    `json:"lastStep,omitempty"`
	Finished        bool          `json:"finished"`
	CreatedAt       time.Time     `json:"createdAt"`
}

// debugRun is the internal state backing one DebugState. It caches the
// graph/tools it started with — editing the agent's saved definition (or
// the Tool catalog) after StartDebugRun won't be reflected mid-session;
// start a new one to pick up changes. This deliberately mirrors Run's
// ownsInstance convention (see manager.go) for the same reason.
type debugRun struct {
	id           string
	agentName    string
	instanceID   string
	ownsInstance bool
	createdAt    time.Time

	graph     registry.Graph
	nodesByID map[string]registry.Node
	inputNode *registry.Node
	tools     []registry.Tool

	messages       []ChatMessage // completed turns only, like Run.Messages
	pendingUserMsg *ChatMessage  // this turn's user message, appended to messages once it finishes

	rc           *runContext // accumulated context, mutated as nodes execute
	rcBeforeLast *runContext // rc's state right before lastNode last ran — what Retry restores

	pending      *registry.Node // node the next Step call will execute; nil before a message or once finished
	pendingInput string

	lastNode  *registry.Node // node Step/Retry most recently executed; nil until the first Step
	lastInput string
	lastStep  *StepEvent

	finished bool
}

func (dr *debugRun) toState() *DebugState {
	state := &DebugState{
		ID:         dr.id,
		AgentName:  dr.agentName,
		InstanceID: dr.instanceID,
		Messages:   dr.messages,
		LastStep:   dr.lastStep,
		Finished:   dr.finished,
		CreatedAt:  dr.createdAt,
	}
	if dr.pending != nil {
		state.PendingNodeID = dr.pending.ID
		state.PendingNodeType = dr.pending.Type
	}
	return state
}

// applyStepResult records what node just produced (via Step or Retry) and
// determines what happens next: advance dr.pending to the node across the
// produced handle's edge, or — if there's none — finish the turn, moving
// the pending user message (plus a new assistant message for output) into
// dr.messages. It never touches dr.rcBeforeLast; only StepDebugRun sets
// that, right before running a new node, so repeated Retry calls always
// restore to the same pre-node snapshot.
func (dr *debugRun) applyStepResult(node *registry.Node, input, output, handle string) {
	dr.lastNode = node
	dr.lastInput = input
	dr.lastStep = &StepEvent{NodeID: node.ID, NodeType: node.Type, Output: output}

	var next *registry.Node
	if edge := findEdge(dr.graph.Edges, node.ID, handle); edge != nil {
		if n, found := dr.nodesByID[edge.Target]; found {
			next = &n
		}
	}

	if next == nil {
		// No outgoing edge (or, for a malformed graph, a dangling edge
		// target) — this is where the turn ends.
		dr.pending = nil
		dr.pendingInput = ""
		switch {
		case dr.pendingUserMsg != nil:
			// First time this turn has reached a finish.
			dr.messages = append(dr.messages, *dr.pendingUserMsg, ChatMessage{Role: "assistant", Content: output, Timestamp: time.Now().UTC()})
			dr.pendingUserMsg = nil
		case dr.finished && len(dr.messages) > 0:
			// Retrying the already-finished turn's terminal node — replace
			// the trailing assistant message with the fresh output rather
			// than leaving the discarded attempt's text in history.
			dr.messages[len(dr.messages)-1] = ChatMessage{Role: "assistant", Content: output, Timestamp: time.Now().UTC()}
		}
		dr.finished = true
		return
	}

	dr.pending = next
	dr.pendingInput = output
	dr.finished = false
}

// StartDebugRun begins a paused debug session for agentName. Unlike
// StartRun, the graph and environment binding it debugs come straight from
// the caller (graph, environment) rather than the agent's saved
// definition — so a session can debug the canvas's current, possibly
// unsaved edits without round-tripping through Save first. agentName is
// used only for display and to look up the agent when nothing else is
// needed; it plays no role in choosing which graph runs.
func (m *Manager) StartDebugRun(agentName string, graph registry.Graph, environment string) (*DebugState, error) {
	inputNode, nodesByID, err := findInputNode(graph)
	if err != nil {
		return nil, err
	}

	tools, err := m.resolveTools(environment)
	if err != nil {
		return nil, err
	}

	dr := &debugRun{
		id:        newDebugRunID(),
		agentName: agentName,
		createdAt: time.Now().UTC(),
		graph:     graph,
		nodesByID: nodesByID,
		inputNode: inputNode,
		tools:     tools,
		messages:  []ChatMessage{},
	}

	if environment != "" {
		instance, err := m.envs.Launch(m.ctx, environment, fmt.Sprintf("agent-debug-%s", dr.id))
		if err != nil {
			return nil, fmt.Errorf("launch environment %q: %w", environment, err)
		}
		dr.instanceID = instance.ID
		dr.ownsInstance = true
	}

	m.mu.Lock()
	m.debugRuns[dr.id] = dr
	m.mu.Unlock()

	return dr.toState(), nil
}

// SendDebugMessage starts a new turn: the input node becomes pending, ready
// for the first StepDebugRun call. It's an error to call this while a
// previous turn is still in progress (some nodes stepped but not yet
// finished) — step through (or retry within) that turn first.
func (m *Manager) SendDebugMessage(id, message string) (*DebugState, error) {
	if message == "" {
		return nil, errors.New("message is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dr, ok := m.debugRuns[id]
	if !ok {
		return nil, fmt.Errorf("no such debug run %q", id)
	}
	if dr.pending != nil {
		return nil, errors.New("a turn is already in progress — step through it (or retry the current node) before sending another message")
	}

	userMsg := ChatMessage{Role: "user", Content: message, Timestamp: time.Now().UTC()}
	dr.pendingUserMsg = &userMsg
	dr.rc = newRunContext()
	dr.pending = dr.inputNode
	dr.pendingInput = message
	dr.lastNode = nil
	dr.lastInput = ""
	dr.lastStep = nil
	dr.finished = false

	return dr.toState(), nil
}

// StepDebugRun executes exactly the pending node — the same runNode logic
// Run's own loop uses — and returns the resulting state, with LastStep
// showing what that node itself produced. A failure leaves the session
// exactly as it was (still pending the same node), so the caller can Retry
// or just call Step again after addressing whatever went wrong.
func (m *Manager) StepDebugRun(id string) (*DebugState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dr, ok := m.debugRuns[id]
	if !ok {
		return nil, fmt.Errorf("no such debug run %q", id)
	}
	if dr.pending == nil {
		return nil, errors.New("nothing pending — send a message to start a turn")
	}

	node := *dr.pending
	input := dr.pendingInput
	snapshot := dr.rc.clone() // state right before node runs — what a later Retry restores

	output, handle, err := m.engine.runNode(m.ctx, node, input, dr.instanceID, dr.tools, dr.messages, dr.rc)
	if err != nil {
		return nil, fmt.Errorf("run node %q: %w", node.ID, err)
	}

	dr.rcBeforeLast = snapshot
	dr.applyStepResult(&node, input, output, handle)
	m.publishStep(id, *dr.lastStep)

	return dr.toState(), nil
}

// RetryDebugRun re-executes the most recently stepped node with the exact
// input it saw, discarding whatever that node (and, transitively, its
// prior attempt's contribution to the run context) produced before —
// useful for re-sampling a prompt node's non-deterministic reply, or for
// re-running a tool node after fixing something in the environment by hand.
// It never re-runs anything upstream of that node, and — since a tool
// node's command may have real side effects — retrying it runs the command
// again for real; that's an inherent limit of debugging live effects, not
// something this can paper over.
func (m *Manager) RetryDebugRun(id string) (*DebugState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dr, ok := m.debugRuns[id]
	if !ok {
		return nil, fmt.Errorf("no such debug run %q", id)
	}
	if dr.lastNode == nil {
		return nil, errors.New("nothing to retry yet — step through at least one node first")
	}

	node := *dr.lastNode
	input := dr.lastInput
	dr.rc = dr.rcBeforeLast.clone()

	output, handle, err := m.engine.runNode(m.ctx, node, input, dr.instanceID, dr.tools, dr.messages, dr.rc)
	if err != nil {
		return nil, fmt.Errorf("run node %q: %w", node.ID, err)
	}

	dr.applyStepResult(&node, input, output, handle)
	m.publishStep(id, *dr.lastStep)

	return dr.toState(), nil
}

// StopDebugRun ends a debug session, stopping its Environment instance if
// it launched one. Idempotent, like StopRun.
func (m *Manager) StopDebugRun(id string) error {
	m.mu.Lock()
	dr, ok := m.debugRuns[id]
	delete(m.debugRuns, id)
	m.mu.Unlock()

	if !ok || dr.instanceID == "" || !dr.ownsInstance {
		return nil
	}

	return m.envs.Stop(context.Background(), dr.instanceID)
}

// GetDebugRun returns the debug session's current state, if any.
func (m *Manager) GetDebugRun(id string) (*DebugState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dr, ok := m.debugRuns[id]
	if !ok {
		return nil, false
	}
	return dr.toState(), true
}

func newDebugRunID() string {
	return fmt.Sprintf("agentdebug-%d", time.Now().UnixNano())
}
