// Package evaluations runs an Evaluation's published test cases against a
// set of agents: for each agent, each test case runs as a fresh chat turn
// inside its own freshly-launched sandbox — a fresh copy of the test case's
// TEST workspace (TestCase.Workspace), so the starting scenario is real
// files. The agent's own Tool/Agent nodes act in that same sandbox during
// the turn, and VerifyCommands check its resulting state afterward,
// alongside the usual assertions against the agent's reply. This is the
// deliberately richer counterpart to internal/benchmarks, which sends a bare
// prompt straight to a model with no agent, workspace, or scenario
// lifecycle.
package evaluations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
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

// VerifyStepResult is one verification command's outcome. Passed reflects
// only its assertions (vacuously true if it declares none) — a non-zero
// exit or exec error is recorded in Error for visibility, but deliberately
// isn't itself pass/fail (see registry.VerifyStep's doc comment).
type VerifyStepResult struct {
	Command    string              `json:"command"`
	Output     string              `json:"output"`
	Assertions []assertions.Result `json:"assertions"`
	Passed     bool                `json:"passed"`
	Error      string              `json:"error,omitempty"`
}

// TestCaseResult is one test case's outcome for one agent. InstanceID is
// the (test-case-scoped, freshly launched and already-stopped-by-the-time
// this is returned) sandbox instance the agent's turn and Verify commands
// shared, present only if the test case names a workspace.
type TestCaseResult struct {
	TestCaseID    string              `json:"testCaseId"`
	Prompt        string              `json:"prompt"`
	InstanceID    string              `json:"instanceId,omitempty"`
	Reply         string              `json:"reply"`
	Assertions    []assertions.Result `json:"assertions"`
	VerifyResults []VerifyStepResult  `json:"verifyResults,omitempty"`
	Passed        bool                `json:"passed"`
	Error         string              `json:"error,omitempty"`
}

// AgentResult aggregates one agent's results across every test case. It's
// the per-agent element of a durable RunResult below (Evaluations' analog
// of internal/benchmarks.RunResult, just keyed by agent instead of model).
type AgentResult struct {
	AgentName string           `json:"agentName"`
	Results   []TestCaseResult `json:"results"`
	Passed    int              `json:"passed"`
	Total     int              `json:"total"`
}

// RunResult is one agent's durable outcome for a specific version of an
// evaluation's test cases. It's persisted (see Manager.persistResult) keyed
// by (EvaluationVersion, AgentName) — running the same evaluation version
// against the same agent again overwrites its previous RunResult, rather
// than accumulating a growing history, mirroring internal/benchmarks.
type RunResult struct {
	EvaluationVersion int              `json:"evaluationVersion"`
	AgentName         string           `json:"agentName"`
	Results           []TestCaseResult `json:"results"`
	Passed            int              `json:"passed"`
	Total             int              `json:"total"`
	StartedAt         time.Time        `json:"startedAt"`
	FinishedAt        time.Time        `json:"finishedAt"`
	Error             string           `json:"error,omitempty"`
}

// Run tracks one in-progress (or just-finished) execution against one or
// more agents — ephemeral (in-memory only, lost on restart), used to report
// live progress and let the UI know a run is active. The durable, queryable
// outcome of each agent it covers is a separately persisted RunResult (see
// ListResults).
type Run struct {
	ID                string      `json:"id"`
	EvaluationName    string      `json:"evaluationName"`
	EvaluationVersion int         `json:"evaluationVersion"`
	AgentNames        []string    `json:"agentNames"`
	Status            Status      `json:"status"`
	Results           []RunResult `json:"results"`
	StartedAt         time.Time   `json:"startedAt"`
	FinishedAt        *time.Time  `json:"finishedAt,omitempty"`
	Error             string      `json:"error,omitempty"`
}

// evaluationReader is the subset of registry.Registry Manager needs — a run
// always targets one immutable, published EvaluationVersion (never the
// evaluation's live draft test cases). GetEvaluation is kept only to resolve
// the run to a real evaluation up front.
type evaluationReader interface {
	GetEvaluation(name string) (registry.Evaluation, error)
	GetEvaluationVersion(name string, version int) (registry.EvaluationVersion, error)
}

// agentRunner is the subset of agents.Manager Manager needs to run a test
// case as a fresh chat turn, optionally inside an instance Manager itself
// already launched (see runTestCase).
type agentRunner interface {
	StartRun(agentName, workspaceOverride string) (*agents.Run, error)
	StartRunInInstance(agentName, instanceID string) (*agents.Run, error)
	SendMessage(runID, message string) (agents.ChatMessage, error)
	StopRun(runID string) error
}

// workspaceLauncher is the subset of environments.Manager Manager needs to
// launch/stop a test case's own sandbox (a fresh copy of its TEST
// workspace) and run its VerifyCommands in it.
type workspaceLauncher interface {
	Launch(ctx context.Context, workspaceName, instanceName string) (environments.Instance, error)
	Stop(ctx context.Context, instanceID string) error
	RunToolSync(ctx context.Context, instanceID, command string) (string, error)
}

// Manager starts and tracks evaluation runs, and persists their durable
// per-agent results to resultsDir.
type Manager struct {
	ctx         context.Context
	evaluations evaluationReader
	agentRunner agentRunner
	envs        workspaceLauncher
	bus         *eventbus.Bus
	resultsDir  string

	mu   sync.Mutex
	runs map[string]*Run

	// resultsMu guards read-modify-write access to an evaluation's results
	// file, separate from mu (which guards in-memory run state) since
	// persisting is file I/O that shouldn't block run-state reads.
	resultsMu sync.Mutex
}

// NewManager builds a Manager. ctx bounds the lifetime of a run (the
// server's shutdown context), since a run continues in the background
// after StartRun's caller gets its response. resultsDir is where each
// evaluation's durable results are persisted, one JSON file per evaluation
// name.
func NewManager(ctx context.Context, evaluationsReader evaluationReader, agentRunner agentRunner, envs workspaceLauncher, bus *eventbus.Bus, resultsDir string) *Manager {
	return &Manager{
		ctx:         ctx,
		evaluations: evaluationsReader,
		agentRunner: agentRunner,
		envs:        envs,
		bus:         bus,
		resultsDir:  resultsDir,
		runs:        make(map[string]*Run),
	}
}

// StartRun begins running one published version of the named evaluation
// against agentNames in the background, returning immediately with the run
// in its "running" state. version must already be published (see
// registry.Registry.PublishEvaluationVersion) — a run can never target the
// evaluation's live, editable draft test cases.
func (m *Manager) StartRun(evaluationName string, version int, agentNames []string) (*Run, error) {
	if len(agentNames) == 0 {
		return nil, errors.New("at least one agent is required")
	}

	if _, err := m.evaluations.GetEvaluation(evaluationName); err != nil {
		return nil, fmt.Errorf("look up evaluation %q: %w", evaluationName, err)
	}

	ver, err := m.evaluations.GetEvaluationVersion(evaluationName, version)
	if err != nil {
		return nil, fmt.Errorf("look up evaluation %q version %d: %w", evaluationName, version, err)
	}
	if len(ver.TestCases) == 0 {
		return nil, fmt.Errorf("evaluation %q version %d has no test cases", evaluationName, version)
	}

	run := &Run{
		ID:                newRunID(),
		EvaluationName:    evaluationName,
		EvaluationVersion: ver.Version,
		AgentNames:        agentNames,
		Status:            StatusRunning,
		Results:           []RunResult{},
		StartedAt:         time.Now().UTC(),
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	m.publishStatus(run)

	// Snapshot before the goroutine can touch run — the caller must not get
	// the live pointer (see ListRuns).
	snapshot := cloneRun(run)

	go m.run(run, ver)

	return snapshot, nil
}

// ListRuns returns a snapshot of every known run, most recently started
// first. The returned Runs are copies: the background goroutine keeps
// mutating the live ones under m.mu, so handing those out directly would
// race any caller that reads them (e.g. a handler marshaling to JSON).
func (m *Manager) ListRuns() []*Run {
	m.mu.Lock()
	defer m.mu.Unlock()

	runs := make([]*Run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, cloneRun(r))
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })

	return runs
}

// GetRun returns a snapshot copy of the run with the given ID, if any (see
// ListRuns for why it's a copy).
func (m *Manager) GetRun(id string) (*Run, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[id]
	if !ok {
		return nil, false
	}
	return cloneRun(run), true
}

// cloneRun returns a copy of run safe for a caller to read without holding
// m.mu: the struct is copied by value and the slices the goroutine appends
// to (AgentNames is immutable, Results grows) are reallocated. Each RunResult
// is frozen once appended under m.mu, so a shallow Results copy is enough.
func cloneRun(run *Run) *Run {
	cp := *run
	cp.AgentNames = append(make([]string, 0, len(run.AgentNames)), run.AgentNames...)
	if run.Results != nil {
		cp.Results = append(make([]RunResult, 0, len(run.Results)), run.Results...)
	}
	return &cp
}

// ListResults returns every persisted RunResult for evaluationName, in no
// particular order — the caller (the evaluation detail page's "run
// results" view) sorts them as needed.
func (m *Manager) ListResults(evaluationName string) ([]RunResult, error) {
	m.resultsMu.Lock()
	defer m.resultsMu.Unlock()

	return m.loadResults(evaluationName)
}

func (m *Manager) run(run *Run, ver registry.EvaluationVersion) {
	for _, agentName := range run.AgentNames {
		result := RunResult{
			EvaluationVersion: ver.Version,
			AgentName:         agentName,
			Results:           []TestCaseResult{},
			StartedAt:         time.Now().UTC(),
		}

		for _, tc := range ver.TestCases {
			tcResult := m.runTestCase(agentName, tc)
			result.Results = append(result.Results, tcResult)
			result.Total++
			if tcResult.Passed {
				result.Passed++
			}
			m.publishProgress(run.ID, agentName, tcResult)
		}

		result.FinishedAt = time.Now().UTC()

		m.mu.Lock()
		run.Results = append(run.Results, result)
		m.mu.Unlock()

		if err := m.persistResult(run.EvaluationName, result); err != nil {
			// Best-effort: a persistence hiccup shouldn't lose the rest of
			// the run's results, or fail the run outright.
			logger.Error("Failed to persist evaluation result for %q/%q: %v", run.EvaluationName, agentName, err)
		}
	}

	m.mu.Lock()
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Status = StatusSucceeded
	m.mu.Unlock()

	m.publishStatus(run)
}

// runTestCase runs one test case against one agent. If the test case names a
// TEST workspace, a fresh sandbox (a fresh copy of that workspace's files)
// is launched just for this (agent, test case) pair — isolating one
// scenario's file changes from every other test case and agent. The agent's
// own StartRunInInstance turn acts in that exact same sandbox (so its
// Tool/Agent nodes see the workspace's starting files), and VerifyCommands
// run in it afterward, before it's torn down. A test case that declares
// VerifyCommands but no workspace fails immediately with a clear error
// rather than silently skipping them.
func (m *Manager) runTestCase(agentName string, tc registry.TestCase) TestCaseResult {
	result := TestCaseResult{TestCaseID: tc.ID, Prompt: tc.Prompt, Assertions: []assertions.Result{}, VerifyResults: []VerifyStepResult{}}

	if len(tc.VerifyCommands) > 0 && tc.Workspace == "" {
		result.Error = "test case declares verification commands but selects no workspace"
		return result
	}

	var instanceID string
	if tc.Workspace != "" {
		instance, err := m.envs.Launch(m.ctx, tc.Workspace, fmt.Sprintf("eval-%s", newRunID()))
		if err != nil {
			result.Error = fmt.Sprintf("launch workspace %q: %v", tc.Workspace, err)
			return result
		}
		instanceID = instance.ID
		result.InstanceID = instanceID
		defer func() {
			// Use a fresh context: m.ctx may already be cancelled if the
			// server is shutting down, but the container still needs
			// cleaning up.
			_ = m.envs.Stop(context.Background(), instanceID)
		}()
	}

	var agentRun *agents.Run
	var err error
	if instanceID != "" {
		agentRun, err = m.agentRunner.StartRunInInstance(agentName, instanceID)
	} else {
		agentRun, err = m.agentRunner.StartRun(agentName, "")
	}
	if err != nil {
		result.Error = fmt.Sprintf("start agent run: %v", err)
		return result
	}
	defer func() { _ = m.agentRunner.StopRun(agentRun.ID) }()

	reply, err := m.agentRunner.SendMessage(agentRun.ID, tc.Prompt)
	if err != nil {
		result.Error = fmt.Sprintf("send message: %v", err)
		return result
	}

	result.Reply = reply.Content
	result.Assertions, result.Passed = assertions.CheckAll(tc.Assertions, reply.Content)

	for _, vs := range tc.VerifyCommands {
		output, cmdErr := m.envs.RunToolSync(m.ctx, instanceID, vs.Command)
		vsResult := VerifyStepResult{Command: vs.Command, Output: output}
		if cmdErr != nil {
			vsResult.Error = cmdErr.Error()
		}
		vsResult.Assertions, vsResult.Passed = assertions.CheckAll(vs.Assertions, output)
		result.VerifyResults = append(result.VerifyResults, vsResult)
		if !vsResult.Passed {
			result.Passed = false
		}
	}

	return result
}

// persistResult upserts result into evaluationName's results file, keyed
// by (EvaluationVersion, AgentName) — a matching existing entry is
// replaced, not appended alongside.
func (m *Manager) persistResult(evaluationName string, result RunResult) error {
	m.resultsMu.Lock()
	defer m.resultsMu.Unlock()

	existing, err := m.loadResults(evaluationName)
	if err != nil {
		return err
	}

	replaced := false
	for i, r := range existing {
		if r.EvaluationVersion == result.EvaluationVersion && r.AgentName == result.AgentName {
			existing[i] = result
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, result)
	}

	if err := os.MkdirAll(m.resultsDir, 0o755); err != nil {
		return fmt.Errorf("create results directory: %w", err)
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evaluation results: %w", err)
	}

	if err := os.WriteFile(m.resultsPath(evaluationName), data, 0o644); err != nil {
		return fmt.Errorf("write evaluation results: %w", err)
	}

	return nil
}

// loadResults reads evaluationName's results file. A missing file (no runs
// persisted yet) is not an error — it just means no results exist.
func (m *Manager) loadResults(evaluationName string) ([]RunResult, error) {
	data, err := os.ReadFile(m.resultsPath(evaluationName))
	if os.IsNotExist(err) {
		return []RunResult{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read evaluation results: %w", err)
	}

	var results []RunResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse evaluation results: %w", err)
	}

	return results, nil
}

func (m *Manager) resultsPath(evaluationName string) string {
	return filepath.Join(m.resultsDir, evaluationName+".json")
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
