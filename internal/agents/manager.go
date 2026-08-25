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

// Run is a chat session against one agent. History is in-memory only —
// unlike Phase 1's training runs, losing it on a `tlw serve` restart isn't
// costly enough to warrant persisting to disk. InstanceID is set for the
// run's lifetime if the agent has an Environment configured, so its Tool
// nodes all share one running container across turns.
type Run struct {
	ID         string        `json:"id"`
	AgentName  string        `json:"agentName"`
	InstanceID string        `json:"instanceId,omitempty"`
	Messages   []ChatMessage `json:"messages"`
	CreatedAt  time.Time     `json:"createdAt"`
}

// Manager starts chat runs against saved agents and drives each turn
// through the Engine.
type Manager struct {
	ctx    context.Context
	agents agentReader
	envs   environmentRunner
	engine *Engine
	bus    *eventbus.Bus

	mu   sync.Mutex
	runs map[string]*Run
}

// NewManager builds a Manager. ctx bounds the lifetime of the LLM/tool calls
// a turn makes; SendMessage itself is synchronous, so in practice this just
// needs to outlive individual HTTP requests, but using the server's
// lifetime context (not a request context) keeps this consistent with
// Phase 1/2's managers.
func NewManager(ctx context.Context, agentsReader agentReader, llm llmClient, envs environmentRunner, bus *eventbus.Bus) *Manager {
	return &Manager{
		ctx:    ctx,
		agents: agentsReader,
		envs:   envs,
		engine: NewEngine(llm, envs),
		bus:    bus,
		runs:   make(map[string]*Run),
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
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	return run, nil
}

// StopRun ends a chat session, stopping its Environment instance (if any).
// It's idempotent: stopping an unknown or already-stopped run isn't an
// error, since callers use this for best-effort cleanup (e.g. the Run
// modal closing).
func (m *Manager) StopRun(runID string) error {
	m.mu.Lock()
	run, ok := m.runs[runID]
	delete(m.runs, runID)
	m.mu.Unlock()

	if !ok || run.InstanceID == "" {
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

	reply, err := m.engine.Run(m.ctx, agent.Graph, history, message, run.InstanceID, func(step StepEvent) {
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
