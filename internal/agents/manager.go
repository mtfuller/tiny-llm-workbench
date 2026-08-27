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

// agentReader is the subset of registry.Registry Manager needs.
type agentReader interface {
	GetAgent(name string) (registry.Agent, error)
}

// environmentRunner is the subset of environments.Manager Manager needs:
// launching/stopping the instance a run's Tool nodes execute in.
type environmentRunner interface {
	Launch(ctx context.Context, environmentName, instanceName string) (environments.Instance, error)
	Stop(ctx context.Context, instanceID string) error
	RunToolSync(ctx context.Context, instanceID, command string) (string, error)
}

// environmentReader is the subset of registry.Registry Manager needs to
// resolve a Tool node's named tool against its agent's bound Environment.
type environmentReader interface {
	GetEnvironment(name string) (registry.Environment, error)
}

// toolReader is the subset of registry.Registry Manager needs to resolve
// the Tool catalog entries an Environment names by reference (Environment
// itself only stores tool names, not full definitions — see registry.Tool).
type toolReader interface {
	GetTool(name string) (registry.Tool, error)
}

// Run is a chat session against one agent. History is in-memory only —
// unlike Phase 1's training runs, losing it on a `tlw serve` restart isn't
// costly enough to warrant persisting to disk. InstanceID is set for the
// run's lifetime if the agent has an Environment configured, so its Tool
// nodes all share one running container across turns. ownsInstance is
// unexported (never serialized) — it's true only when StartRun itself
// launched InstanceID; a run started via StartRunInInstance reuses an
// instance some other caller (Evaluations) owns the lifecycle of, so
// StopRun must not stop it out from under them.
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
	agents    agentReader
	envs      environmentRunner
	envReader environmentReader
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
// Phase 1/2's managers. envReader resolves an agent's bound Environment
// (its list of attached tool names); toolStore resolves each of those names
// against the global Tool catalog; kb resolves a knowledge node's named
// KnowledgeBase (independent of any Environment).
func NewManager(ctx context.Context, agentsReader agentReader, llm llmClient, envs environmentRunner, envReader environmentReader, toolStore toolReader, kb knowledgeReader, bus *eventbus.Bus) *Manager {
	return &Manager{
		ctx:       ctx,
		agents:    agentsReader,
		envs:      envs,
		envReader: envReader,
		toolStore: toolStore,
		engine:    NewEngine(llm, envs, kb),
		bus:       bus,
		runs:      make(map[string]*Run),
		debugRuns: make(map[string]*debugRun),
	}
}

// StartRun begins a new chat session against the named agent. If the agent
// has an Environment configured, a real instance of it is launched for the
// run's duration — its Tool nodes execute in that same instance across
// every turn of this run.
func (m *Manager) StartRun(agentName string) (*Run, error) {
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

	if agent.Environment != "" {
		instance, err := m.envs.Launch(m.ctx, agent.Environment, fmt.Sprintf("agent-%s", run.ID))
		if err != nil {
			return nil, fmt.Errorf("launch environment %q: %w", agent.Environment, err)
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

	tools, err := m.resolveTools(agent.Environment)
	if err != nil {
		return ChatMessage{}, err
	}

	reply, err := m.engine.Run(m.ctx, agent.Graph, history, message, run.InstanceID, tools, func(step StepEvent) {
		m.publishStep(runID, step)
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

// resolveTools returns the real Tool catalog entries the named Environment
// makes available (empty if environment is ""), — shared by SendMessage and
// the step-by-step debugger (see debug.go) so both resolve tools identically.
func (m *Manager) resolveTools(environment string) ([]registry.Tool, error) {
	if environment == "" {
		return nil, nil
	}

	env, err := m.envReader.GetEnvironment(environment)
	if err != nil {
		return nil, fmt.Errorf("look up environment %q: %w", environment, err)
	}

	var tools []registry.Tool
	// A tool name the environment references but that's since been deleted
	// from the catalog is skipped here, not an error — the engine's own
	// "tool not found" reporting for a node that actually tries to use it
	// is the same graceful-degradation path a tool removed from the
	// environment already goes through.
	for _, toolName := range env.Tools {
		if tool, err := m.toolStore.GetTool(toolName); err == nil {
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

func newRunID() string {
	return fmt.Sprintf("agentrun-%d", time.Now().UnixNano())
}
