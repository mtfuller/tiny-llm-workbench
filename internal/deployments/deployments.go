// Package deployments runs a saved Deployment (an agent bound to a REAL
// workspace) as a live chat session for actual work: starting one launches a
// sandbox with the real directory bind-mounted read-write, then every chat
// turn drives the agent's Tool/Agent nodes against that directory so its
// changes persist. Only the Deployment definition is durable (see
// registry.Deployment); a running Session and its transcript are in-memory
// only, like an agent chat run.
package deployments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// deploymentReader resolves a saved Deployment definition.
type deploymentReader interface {
	GetDeployment(name string) (registry.Deployment, error)
}

// workspaceReader resolves a Workspace so Start can reject a non-real one.
type workspaceReader interface {
	GetWorkspace(name string) (registry.Workspace, error)
}

// workspaceLauncher launches/stops the sandbox a session runs in.
type workspaceLauncher interface {
	Launch(ctx context.Context, workspaceName, instanceName string) (environments.Instance, error)
	Stop(ctx context.Context, instanceID string) error
}

// agentRunner drives the bound agent's turns inside the session's sandbox.
type agentRunner interface {
	StartRunInInstance(agentName, instanceID string) (*agents.Run, error)
	SendMessage(runID, message string) (agents.ChatMessage, error)
	StopRun(runID string) error
}

// Session is one running deployment: a launched sandbox plus an agent chat
// run against it. In-memory only. RunID is the underlying agents.Manager run
// id — the frontend filters the agent.step / agent.message SSE stream by it
// to show live progress.
type Session struct {
	ID             string               `json:"id"`
	DeploymentName string               `json:"deploymentName"`
	AgentName      string               `json:"agentName"`
	WorkspaceName  string               `json:"workspaceName"`
	InstanceID     string               `json:"instanceId"`
	WorkspacePath  string               `json:"workspacePath"`
	RunID          string               `json:"runId"`
	Messages       []agents.ChatMessage `json:"messages"`
	StartedAt      time.Time            `json:"startedAt"`
}

// Manager starts, tracks, and stops deployment sessions.
type Manager struct {
	ctx         context.Context
	deployments deploymentReader
	workspaces  workspaceReader
	envs        workspaceLauncher
	agents      agentRunner

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager builds a Manager. ctx is the server's lifetime context, used to
// bound a launched sandbox.
func NewManager(ctx context.Context, deployments deploymentReader, workspaces workspaceReader, envs workspaceLauncher, agents agentRunner) *Manager {
	return &Manager{
		ctx:         ctx,
		deployments: deployments,
		workspaces:  workspaces,
		envs:        envs,
		agents:      agents,
		sessions:    make(map[string]*Session),
	}
}

// Start launches a sandbox for the named deployment's real workspace and
// begins an agent chat run against it, returning the new session.
func (m *Manager) Start(deploymentName string) (*Session, error) {
	dep, err := m.deployments.GetDeployment(deploymentName)
	if err != nil {
		return nil, fmt.Errorf("look up deployment %q: %w", deploymentName, err)
	}

	ws, err := m.workspaces.GetWorkspace(dep.WorkspaceName)
	if err != nil {
		return nil, fmt.Errorf("look up workspace %q: %w", dep.WorkspaceName, err)
	}
	if ws.Type != registry.WorkspaceReal {
		return nil, fmt.Errorf("deployment %q targets workspace %q, which is not a real workspace", deploymentName, dep.WorkspaceName)
	}

	id := newSessionID()
	instance, err := m.envs.Launch(m.ctx, dep.WorkspaceName, fmt.Sprintf("deploy-%s", id))
	if err != nil {
		return nil, fmt.Errorf("launch workspace %q: %w", dep.WorkspaceName, err)
	}

	run, err := m.agents.StartRunInInstance(dep.AgentName, instance.ID)
	if err != nil {
		_ = m.envs.Stop(context.Background(), instance.ID)
		return nil, fmt.Errorf("start agent run: %w", err)
	}

	session := &Session{
		ID:             id,
		DeploymentName: deploymentName,
		AgentName:      dep.AgentName,
		WorkspaceName:  dep.WorkspaceName,
		InstanceID:     instance.ID,
		WorkspacePath:  instance.WorkspacePath,
		Messages:       []agents.ChatMessage{},
		StartedAt:      time.Now().UTC(),
		RunID:          run.ID,
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	return cloneSession(session), nil
}

// cloneSession copies session by value with a fresh Messages slice. A
// concurrent SendMessage appends to the live session's Messages under m.mu,
// so callers that read a session (Get/List/Start, e.g. a handler marshaling
// to JSON) must get a copy, not the live pointer.
func cloneSession(s *Session) *Session {
	cp := *s
	if s.Messages != nil {
		cp.Messages = append(make([]agents.ChatMessage, 0, len(s.Messages)), s.Messages...)
	}
	return &cp
}

// SendMessage runs one chat turn in the session and appends the exchange to
// its transcript.
func (m *Manager) SendMessage(sessionID, message string) (agents.ChatMessage, error) {
	if message == "" {
		return agents.ChatMessage{}, errors.New("message is required")
	}

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return agents.ChatMessage{}, fmt.Errorf("no such deployment session %q", sessionID)
	}

	reply, err := m.agents.SendMessage(session.RunID, message)

	userMsg := agents.ChatMessage{Role: "user", Content: message, Timestamp: time.Now().UTC()}
	if err != nil {
		m.mu.Lock()
		session.Messages = append(session.Messages, userMsg)
		m.mu.Unlock()
		return agents.ChatMessage{}, err
	}

	m.mu.Lock()
	session.Messages = append(session.Messages, userMsg, reply)
	m.mu.Unlock()

	return reply, nil
}

// Stop ends a session, stopping its agent run and its sandbox. Idempotent.
func (m *Manager) Stop(sessionID string) error {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if !ok {
		return nil
	}

	_ = m.agents.StopRun(session.RunID)
	return m.envs.Stop(context.Background(), session.InstanceID)
}

// Get returns a session by ID.
func (m *Manager) Get(sessionID string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, false
	}
	return cloneSession(session), true
}

// List returns a snapshot of every live session, most recently started
// first (the returned Sessions are copies — see cloneSession).
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, cloneSession(s))
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt.After(sessions[j].StartedAt) })
	return sessions
}

func newSessionID() string {
	return fmt.Sprintf("deploysess-%d", time.Now().UnixNano())
}
