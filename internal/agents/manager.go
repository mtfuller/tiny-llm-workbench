package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// StepEventType is the eventbus event type the Run view's SSE stream
// listens for to show live execution steps.
const StepEventType = "agent.step"

// MessageEventType is the eventbus event type carrying a user-facing message
// a "say" node emitted mid-turn — progress narration or the turn's final
// answer — for the chat UI to render live as the agent works. Each payload
// is a TurnMessage with the run id attached.
const MessageEventType = "agent.message"

// agentStore is the subset of registry.Registry Manager needs: loading an
// agent's saved definition, and resolving a model reference (a prompt or
// agent node's model field may be a registry model name, which mlx-lm can't
// resolve on its own — see registry.ResolveModelRef).
type agentStore interface {
	GetAgent(name string) (registry.Agent, error)
	ResolveModelRef(ref string) string
}

// workspaceRunner is the subset of environments.Manager Manager needs:
// launching/stopping the sandbox instance a run's Tool/Agent nodes execute
// in (a fresh copy of the agent's test workspace).
type workspaceRunner interface {
	Launch(ctx context.Context, workspaceName, instanceName string) (environments.Instance, error)
	Stop(ctx context.Context, instanceID string) error
	RunToolSync(ctx context.Context, instanceID, command string) (string, error)
}

// toolReader is the subset of registry.Registry Manager needs to resolve
// the Tool catalog entries an agent's Tools set names by reference (the
// agent stores tool names, not full definitions — see registry.Tool).
type toolReader interface {
	GetTool(name string) (registry.Tool, error)
}

// Run is a chat session against one agent. History is in-memory only —
// unlike Phase 1's training runs, losing it on a `tlw serve` restart isn't
// costly enough to warrant persisting to disk. InstanceID is set for the
// run's lifetime if the agent (or the run's caller) named a workspace, so
// its Tool/Agent nodes all share one running sandbox across turns.
// ownsInstance is unexported (never serialized) — it's true only when
// StartRun itself launched InstanceID; a run started via StartRunInInstance
// reuses an instance some other caller (Evaluations, Deployments) owns the
// lifecycle of, so StopRun must not stop it out from under them.
type Run struct {
	ID           string        `json:"id"`
	AgentName    string        `json:"agentName"`
	InstanceID   string        `json:"instanceId,omitempty"`
	Messages     []ChatMessage `json:"messages"`
	CreatedAt    time.Time     `json:"createdAt"`
	ownsInstance bool
}

// Manager starts chat runs against saved agents and drives each turn
// through the Engine.
type Manager struct {
	ctx       context.Context
	agents    agentStore
	envs      workspaceRunner
	toolStore toolReader
	engine    *Engine
	bus       *eventbus.Bus

	mu        sync.Mutex
	runs      map[string]*Run
	debugRuns map[string]*debugRun
}

// NewManager builds a Manager. ctx bounds the lifetime of the LLM/tool calls
// a turn makes; SendMessage itself is synchronous, so in practice this just
// needs to outlive individual HTTP requests, but using the server's
// lifetime context (not a request context) keeps this consistent with
// Phase 1/2's managers. toolStore resolves each name in an agent's Tools
// set against the global Tool catalog; kb resolves a knowledge node's named
// KnowledgeBase; envs launches the sandbox an agent's workspace runs in.
func NewManager(ctx context.Context, agentsReader agentStore, llm llmClient, envs workspaceRunner, toolStore toolReader, kb knowledgeReader, bus *eventbus.Bus) *Manager {
	return &Manager{
		ctx:       ctx,
		agents:    agentsReader,
		envs:      envs,
		toolStore: toolStore,
		engine:    NewEngine(llm, envs, kb),
		bus:       bus,
		runs:      make(map[string]*Run),
		debugRuns: make(map[string]*debugRun),
	}
}

// StartRun begins a new chat session against the named agent. The workspace
// used is workspaceOverride when non-empty (the per-run test workspace the
// chat/debug UI lets you pick), otherwise the agent's own bound Workspace.
// If either names one, a fresh sandbox is launched for the run's duration —
// the graph's Tool/Agent nodes execute in that same sandbox across every
// turn of this run.
func (m *Manager) StartRun(agentName, workspaceOverride string) (*Run, error) {
	agent, err := m.agents.GetAgent(agentName)
	if err != nil {
		return nil, fmt.Errorf("look up agent %q: %w", agentName, err)
	}

	run := &Run{
		ID:        newRunID(),
		AgentName: agentName,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now().UTC(),
	}

	workspace := workspaceOverride
	if workspace == "" {
		workspace = agent.Workspace
	}
	if workspace != "" {
		instance, err := m.envs.Launch(m.ctx, workspace, fmt.Sprintf("agent-%s", run.ID))
		if err != nil {
			return nil, fmt.Errorf("launch workspace %q: %w", workspace, err)
		}
		run.InstanceID = instance.ID
		run.ownsInstance = true
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	return run, nil
}

// StartRunInInstance begins a new chat session against agentName, reusing
// an already-launched Environment instance (instanceID) instead of
// launching a fresh one — used by Evaluations, which owns instanceID's
// whole lifecycle itself (it runs Setup commands before the agent's turn
// and Verify commands after, all against the same container the agent's
// Tool nodes act in during SendMessage). StopRun will not stop an instance
// started this way; the caller remains responsible for it.
func (m *Manager) StartRunInInstance(agentName, instanceID string) (*Run, error) {
	if _, err := m.agents.GetAgent(agentName); err != nil {
		return nil, fmt.Errorf("look up agent %q: %w", agentName, err)
	}

	run := &Run{
		ID:         newRunID(),
		AgentName:  agentName,
		InstanceID: instanceID,
		Messages:   []ChatMessage{},
		CreatedAt:  time.Now().UTC(),
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	return run, nil
}

// StopRun ends a chat session, stopping its Environment instance if this
// run itself launched it (StartRun; not StartRunInInstance, whose instance
// belongs to the caller). It's idempotent: stopping an unknown or
// already-stopped run isn't an error, since callers use this for
// best-effort cleanup (e.g. the Run modal closing).
func (m *Manager) StopRun(runID string) error {
	m.mu.Lock()
	run, ok := m.runs[runID]
	delete(m.runs, runID)
	m.mu.Unlock()

	if !ok || run.InstanceID == "" || !run.ownsInstance {
		return nil
	}

	// Use a fresh context, not m.ctx: cleanup should still happen even if
	// the server is mid-shutdown.
	return m.envs.Stop(context.Background(), run.InstanceID)
}

// SendMessage runs one chat turn synchronously: it looks up the run and its
// agent, executes the graph (publishing StepEventType events as it goes),
// appends the user message and assistant reply to the run's history, and
// returns the assistant's reply.
func (m *Manager) SendMessage(runID, message string) (ChatMessage, error) {
	if message == "" {
		return ChatMessage{}, errors.New("message is required")
	}

	m.mu.Lock()
	run, ok := m.runs[runID]
	var history []ChatMessage
	if ok {
		history = append([]ChatMessage{}, run.Messages...)
	}
	m.mu.Unlock()
	if !ok {
		return ChatMessage{}, fmt.Errorf("no such run %q", runID)
	}

	agent, err := m.agents.GetAgent(run.AgentName)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("look up agent %q: %w", run.AgentName, err)
	}

	tools, err := m.resolveTools(agent.Tools)
	if err != nil {
		return ChatMessage{}, err
	}

	graph := m.resolveGraphModels(agent.Graph)
	reply, err := m.engine.Run(m.ctx, graph, history, message, run.InstanceID, tools, &RunHooks{
		OnStep:    func(step StepEvent) { m.publishStep(runID, step) },
		OnMessage: func(msg TurnMessage) { m.publishTurnMessage(runID, msg) },
	})

	userMsg := ChatMessage{Role: "user", Content: message, Timestamp: time.Now().UTC()}
	if err != nil {
		m.mu.Lock()
		run.Messages = append(run.Messages, userMsg)
		m.mu.Unlock()
		return ChatMessage{}, fmt.Errorf("run agent %q: %w", run.AgentName, err)
	}

	assistantMsg := ChatMessage{Role: "assistant", Content: reply, Timestamp: time.Now().UTC()}
	m.mu.Lock()
	run.Messages = append(run.Messages, userMsg, assistantMsg)
	m.mu.Unlock()

	return assistantMsg, nil
}

// PreviewNode resolves the node's model reference (a registry model name →
// its path / repo id, like resolveGraphModels does for a whole graph) and
// runs a standalone one-shot preview of it — see Engine.PreviewNode.
func (m *Manager) PreviewNode(ctx context.Context, node registry.Node, input string) (PreviewResult, error) {
	node.Data.Model = m.agents.ResolveModelRef(node.Data.Model)
	node.Data.AgentModel = m.agents.ResolveModelRef(node.Data.AgentModel)
	return m.engine.PreviewNode(ctx, node, input)
}

// resolveGraphModels returns a copy of g with every prompt node's Model and
// every agent node's AgentModel run through the registry — so a model
// picked by its registry name (e.g. "Llama-3.2-1B-Instruct-4bit") becomes
// the path / repo id mlx-lm's --model expects before the engine ever calls
// the runner. The saved graph is untouched; the canvas still shows the
// friendly name. Shared by SendMessage and the step-by-step debugger.
func (m *Manager) resolveGraphModels(g registry.Graph) registry.Graph {
	nodes := make([]registry.Node, len(g.Nodes))
	for i, n := range g.Nodes {
		switch n.Type {
		case "prompt":
			n.Data.Model = m.agents.ResolveModelRef(n.Data.Model)
		case "agent":
			n.Data.AgentModel = m.agents.ResolveModelRef(n.Data.AgentModel)
		}
		nodes[i] = n
	}
	return registry.Graph{Nodes: nodes, Edges: g.Edges}
}

// resolveTools returns the real Tool catalog entries for the given names
// (the agent's Tools set) — shared by SendMessage and the step-by-step
// debugger (see debug.go) so both resolve tools identically. A name that's
// since been deleted from the catalog is skipped here, not an error: the
// engine's own "tool not found" reporting for a node that actually tries to
// use it is the same graceful-degradation path.
func (m *Manager) resolveTools(toolNames []string) ([]registry.Tool, error) {
	var tools []registry.Tool
	for _, name := range toolNames {
		if tool, err := m.toolStore.GetTool(name); err == nil {
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

// GetRun returns the run with the given ID, if any.
func (m *Manager) GetRun(id string) (*Run, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	return run, ok
}

func (m *Manager) publishStep(runID string, step StepEvent) {
	data, err := json.Marshal(struct {
		RunID string `json:"runId"`
		StepEvent
	}{RunID: runID, StepEvent: step})
	if err != nil {
		return
	}
	m.bus.Publish(eventbus.Event{Type: StepEventType, Data: string(data)})
}

func (m *Manager) publishTurnMessage(runID string, msg TurnMessage) {
	data, err := json.Marshal(struct {
		RunID string `json:"runId"`
		TurnMessage
	}{RunID: runID, TurnMessage: msg})
	if err != nil {
		return
	}
	m.bus.Publish(eventbus.Event{Type: MessageEventType, Data: string(data)})
}

func newRunID() string {
	return fmt.Sprintf("agentrun-%d", time.Now().UnixNano())
}
