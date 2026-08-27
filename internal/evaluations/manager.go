// Package evaluations runs an Evaluation's test cases against a set of
// agents: for each agent, each test case's prompt is sent as a fresh chat
// turn and its reply checked against the test case's assertions.
package evaluations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// ProgressEvent and StatusEvent are the eventbus event types the
// Evaluations page's SSE stream listens for.
const (
	ProgressEvent = "evaluation.progress"
	StatusEvent   = "evaluation.status"
)

// Status is a Run's lifecycle state.
type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// TestCaseResult is one test case's outcome for one agent.
type TestCaseResult struct {
	TestCaseID string              `json:"testCaseId"`
	Prompt     string              `json:"prompt"`
	Reply      string              `json:"reply"`
	Assertions []assertions.Result `json:"assertions"`
	Passed     bool                `json:"passed"`
	Error      string              `json:"error,omitempty"`
}

// AgentResult aggregates one agent's results across every test case.
type AgentResult struct {
	AgentName string           `json:"agentName"`
	Results   []TestCaseResult `json:"results"`
	Passed    int              `json:"passed"`
	Total     int              `json:"total"`
}

// Run is a single evaluation run against one or more agents.
type Run struct {
	ID              string        `json:"id"`
	EvaluationName  string        `json:"evaluationName"`
	AgentNames      []string      `json:"agentNames"`
	EnvironmentName string        `json:"environmentName,omitempty"`
	InstanceID      string        `json:"instanceId,omitempty"`
	Status          Status        `json:"status"`
	AgentResults    []AgentResult `json:"agentResults"`
	StartedAt       time.Time     `json:"startedAt"`
	FinishedAt      *time.Time    `json:"finishedAt,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// evaluationReader is the subset of registry.Registry Manager needs.
type evaluationReader interface {
	GetEvaluation(name string) (registry.Evaluation, error)
}

// agentRunner is the subset of agents.Manager Manager needs to run a test
// case as a fresh chat turn.
type agentRunner interface {
	StartRun(agentName string) (*agents.Run, error)
	SendMessage(runID, message string) (agents.ChatMessage, error)
}

// environmentLauncher is the subset of environments.Manager Manager needs
// to launch/stop an Evaluation's optional Environment.
type environmentLauncher interface {
	Launch(ctx context.Context, environmentName, instanceName string) (environments.Instance, error)
	Stop(ctx context.Context, instanceID string) error
}

// Manager starts and tracks evaluation runs.
type Manager struct {
	ctx         context.Context
	evaluations evaluationReader
	agentRunner agentRunner
	envs        environmentLauncher
	bus         *eventbus.Bus

	mu   sync.Mutex
	runs map[string]*Run
}

// NewManager builds a Manager. ctx bounds the lifetime of a run (the
// server's shutdown context), since a run continues in the background
// after StartRun's caller gets its response.
func NewManager(ctx context.Context, evaluationsReader evaluationReader, agentRunner agentRunner, envs environmentLauncher, bus *eventbus.Bus) *Manager {
	return &Manager{
		ctx:         ctx,
		evaluations: evaluationsReader,
		agentRunner: agentRunner,
		envs:        envs,
		bus:         bus,
		runs:        make(map[string]*Run),
	}
}

// StartRun begins evaluating the named evaluation against agentNames in the
// background, returning immediately with the run in its "running" state.
func (m *Manager) StartRun(evaluationName string, agentNames []string) (*Run, error) {
	if len(agentNames) == 0 {
		return nil, errors.New("at least one agent is required")
	}

	eval, err := m.evaluations.GetEvaluation(evaluationName)
	if err != nil {
		return nil, fmt.Errorf("look up evaluation %q: %w", evaluationName, err)
	}
	if len(eval.TestCases) == 0 {
		return nil, fmt.Errorf("evaluation %q has no test cases", evaluationName)
	}

	run := &Run{
		ID:              newRunID(),
		EvaluationName:  evaluationName,
		AgentNames:      agentNames,
		EnvironmentName: eval.Environment,
		Status:          StatusRunning,
		AgentResults:    []AgentResult{},
		StartedAt:       time.Now().UTC(),
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	m.publishStatus(run)

	go m.run(run, eval)

	return run, nil
}

// ListRuns returns every known run, most recently started first.
func (m *Manager) ListRuns() []*Run {
	m.mu.Lock()
	defer m.mu.Unlock()

	runs := make([]*Run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })

	return runs
}

// GetRun returns the run with the given ID, if any.
func (m *Manager) GetRun(id string) (*Run, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	return run, ok
}

func (m *Manager) run(run *Run, eval registry.Evaluation) {
	if eval.Environment != "" {
		instance, err := m.envs.Launch(m.ctx, eval.Environment, fmt.Sprintf("eval-%s", run.ID))
		if err != nil {
			m.fail(run, fmt.Errorf("launch environment %q: %w", eval.Environment, err))
			return
		}

		m.mu.Lock()
		run.InstanceID = instance.ID
		m.mu.Unlock()

		defer func() {
			// Use a fresh context: m.ctx may already be cancelled if the
			// server is shutting down, but the container still needs
			// cleaning up.
			if err := m.envs.Stop(context.Background(), instance.ID); err != nil {
				// Best-effort: the container leaking is a lesser problem
				// than losing the run's actual results over a cleanup
				// failure.
				_ = err
			}
		}()
	}

	for _, agentName := range run.AgentNames {
		agentResult := AgentResult{AgentName: agentName, Results: []TestCaseResult{}}

		for _, tc := range eval.TestCases {
			result := m.runTestCase(agentName, tc)
			agentResult.Results = append(agentResult.Results, result)
			agentResult.Total++
			if result.Passed {
				agentResult.Passed++
			}
			m.publishProgress(run.ID, agentName, result)
		}

		m.mu.Lock()
		run.AgentResults = append(run.AgentResults, agentResult)
		m.mu.Unlock()
	}

	m.mu.Lock()
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Status = StatusSucceeded
	m.mu.Unlock()

	m.publishStatus(run)
}

func (m *Manager) runTestCase(agentName string, tc registry.TestCase) TestCaseResult {
	result := TestCaseResult{TestCaseID: tc.ID, Prompt: tc.Prompt, Assertions: []assertions.Result{}}

	agentRun, err := m.agentRunner.StartRun(agentName)
	if err != nil {
		result.Error = fmt.Sprintf("start agent run: %v", err)
		return result
	}

	reply, err := m.agentRunner.SendMessage(agentRun.ID, tc.Prompt)
	if err != nil {
		result.Error = fmt.Sprintf("send message: %v", err)
		return result
	}

	result.Reply = reply.Content
	result.Assertions, result.Passed = assertions.CheckAll(tc.Assertions, reply.Content)

	return result
}

func (m *Manager) fail(run *Run, err error) {
	m.mu.Lock()
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Status = StatusFailed
	run.Error = err.Error()
	m.mu.Unlock()

	m.publishStatus(run)
}

func (m *Manager) publishStatus(run *Run) {
	m.mu.Lock()
	data, err := json.Marshal(run)
	m.mu.Unlock()
	if err != nil {
		return
	}
	m.bus.Publish(eventbus.Event{Type: StatusEvent, Data: string(data)})
}

func (m *Manager) publishProgress(runID, agentName string, result TestCaseResult) {
	data, err := json.Marshal(struct {
		RunID     string `json:"runId"`
		AgentName string `json:"agentName"`
		TestCaseResult
	}{RunID: runID, AgentName: agentName, TestCaseResult: result})
	if err != nil {
		return
	}
	m.bus.Publish(eventbus.Event{Type: ProgressEvent, Data: string(data)})
}

func newRunID() string {
	return fmt.Sprintf("evalrun-%d", time.Now().UnixNano())
}
