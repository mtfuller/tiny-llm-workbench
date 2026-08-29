// Package benchmarks runs a Benchmark's test cases directly against a set
// of registry models: for each model, each test case's prompt is sent as a
// single, independent generation (no chat history, no agent/environment
// lifecycle) and the reply checked against the test case's assertions —
// the deliberately simpler counterpart to internal/evaluations, which runs
// the same shape of test case against agents instead.
package benchmarks

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

	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// ProgressEvent and StatusEvent are the eventbus event types the
// Benchmarks page's SSE stream listens for.
const (
	ProgressEvent = "benchmark.progress"
	StatusEvent   = "benchmark.status"
)

// Status is a Run's lifecycle state.
type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// TestCaseResult is one test case's outcome for one model.
type TestCaseResult struct {
	TestCaseID string              `json:"testCaseId"`
	Prompt     string              `json:"prompt"`
	Reply      string              `json:"reply"`
	Assertions []assertions.Result `json:"assertions"`
	Passed     bool                `json:"passed"`
	Error      string              `json:"error,omitempty"`
}

// RunResult is one model's durable outcome for a specific version of a
// benchmark's test cases. It's persisted (see Manager.persistResult) keyed
// by (BenchmarkVersion, ModelName) — running the same benchmark version
// against the same model again overwrites its previous RunResult, rather
// than accumulating a growing history, so the benchmark detail page always
// shows one row per model actually tried.
type RunResult struct {
	BenchmarkVersion int              `json:"benchmarkVersion"`
	ModelName        string           `json:"modelName"`
	Results          []TestCaseResult `json:"results"`
	Passed           int              `json:"passed"`
	Total            int              `json:"total"`
	StartedAt        time.Time        `json:"startedAt"`
	FinishedAt       time.Time        `json:"finishedAt"`
	Error            string           `json:"error,omitempty"`
}

// Run tracks one in-progress (or just-finished) execution against one or
// more models — it's ephemeral (in-memory only, lost on restart), used to
// report live progress and let the UI know a run is active. The durable,
// queryable outcome of each model it covers is a separately persisted
// RunResult (see ListResults).
type Run struct {
	ID               string      `json:"id"`
	BenchmarkName    string      `json:"benchmarkName"`
	BenchmarkVersion int         `json:"benchmarkVersion"`
	ModelNames       []string    `json:"modelNames"`
	Status           Status      `json:"status"`
	Results          []RunResult `json:"results"`
	StartedAt        time.Time   `json:"startedAt"`
	FinishedAt       *time.Time  `json:"finishedAt,omitempty"`
	Error            string      `json:"error,omitempty"`
}

// benchmarkReader is the subset of registry.Registry Manager needs — a run
// always targets one immutable, published BenchmarkVersion, never the
// benchmark's live draft test cases.
type benchmarkReader interface {
	GetVersion(benchmarkName string, version int) (registry.BenchmarkVersion, error)
}

// modelResolver is the subset of registry.Registry Manager needs to resolve
// a model name into the path mlxrunner.Runner.Generate expects.
type modelResolver interface {
	GetModel(name string) (registry.Model, error)
}

// modelRunner is the subset of mlxrunner.Runner Manager needs to run a test
// case as a single, independent generation.
type modelRunner interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// Manager starts and tracks benchmark runs, and persists their durable
// per-model results to resultsDir.
type Manager struct {
	ctx        context.Context
	benchmarks benchmarkReader
	models     modelResolver
	runner     modelRunner
	bus        *eventbus.Bus
	resultsDir string

	mu   sync.Mutex
	runs map[string]*Run

	// resultsMu guards read-modify-write access to a benchmark's results
	// file, separate from mu (which guards in-memory run state) since
	// persisting is file I/O that shouldn't block run-state reads.
	resultsMu sync.Mutex
}

// NewManager builds a Manager. ctx bounds the lifetime of a run (the
// server's shutdown context), since a run continues in the background after
// StartRun's caller gets its response. resultsDir is where each benchmark's
// durable results are persisted, one JSON file per benchmark name.
func NewManager(ctx context.Context, benchmarksReader benchmarkReader, models modelResolver, runner modelRunner, bus *eventbus.Bus, resultsDir string) *Manager {
	return &Manager{
		ctx:        ctx,
		benchmarks: benchmarksReader,
		models:     models,
		runner:     runner,
		bus:        bus,
		resultsDir: resultsDir,
		runs:       make(map[string]*Run),
	}
}

// StartRun begins running one published version of the named benchmark
// against modelNames in the background, returning immediately with the run
// in its "running" state. version must be a version number that's already
// been published (see registry.Registry.PublishVersion) — a run can never
// target the benchmark's live, editable draft test cases, so its results
// always trace back to an exact, unchanging set of test cases.
func (m *Manager) StartRun(benchmarkName string, version int, modelNames []string) (*Run, error) {
	if len(modelNames) == 0 {
		return nil, errors.New("at least one model is required")
	}

	ver, err := m.benchmarks.GetVersion(benchmarkName, version)
	if err != nil {
		return nil, fmt.Errorf("look up benchmark %q version %d: %w", benchmarkName, version, err)
	}
	if len(ver.TestCases) == 0 {
		return nil, fmt.Errorf("benchmark %q version %d has no test cases", benchmarkName, version)
	}

	run := &Run{
		ID:               newRunID(),
		BenchmarkName:    benchmarkName,
		BenchmarkVersion: ver.Version,
		ModelNames:       modelNames,
		Status:           StatusRunning,
		Results:          []RunResult{},
		StartedAt:        time.Now().UTC(),
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
// to (ModelNames is immutable, Results grows) are reallocated. Each RunResult
// is frozen once appended under m.mu, so a shallow Results copy is enough.
func cloneRun(run *Run) *Run {
	cp := *run
	cp.ModelNames = append(make([]string, 0, len(run.ModelNames)), run.ModelNames...)
	if run.Results != nil {
		cp.Results = append(make([]RunResult, 0, len(run.Results)), run.Results...)
	}
	return &cp
}

// ListResults returns every persisted RunResult for benchmarkName, in no
// particular order — the caller (the benchmark detail page's "run results"
// view) sorts them as needed (e.g. by pass rate).
func (m *Manager) ListResults(benchmarkName string) ([]RunResult, error) {
	m.resultsMu.Lock()
	defer m.resultsMu.Unlock()

	return m.loadResults(benchmarkName)
}

func (m *Manager) run(run *Run, ver registry.BenchmarkVersion) {
	for _, modelName := range run.ModelNames {
		result := RunResult{
			BenchmarkVersion: ver.Version,
			ModelName:        modelName,
			Results:          []TestCaseResult{},
			StartedAt:        time.Now().UTC(),
		}

		model, modelErr := m.models.GetModel(modelName)

		for _, tc := range ver.TestCases {
			var tcResult TestCaseResult
			if modelErr != nil {
				tcResult = TestCaseResult{
					TestCaseID: tc.ID,
					Prompt:     tc.Prompt,
					Assertions: []assertions.Result{},
					Error:      fmt.Sprintf("look up model: %v", modelErr),
				}
			} else {
				tcResult = m.runTestCase(model, tc)
			}

			result.Results = append(result.Results, tcResult)
			result.Total++
			if tcResult.Passed {
				result.Passed++
			}
			m.publishProgress(run.ID, modelName, tcResult)
		}

		result.FinishedAt = time.Now().UTC()

		m.mu.Lock()
		run.Results = append(run.Results, result)
		m.mu.Unlock()

		if err := m.persistResult(run.BenchmarkName, result); err != nil {
			// Best-effort: a persistence hiccup shouldn't lose the rest of
			// the run's results, or fail the run outright.
			logger.Error("Failed to persist benchmark result for %q/%q: %v", run.BenchmarkName, modelName, err)
		}
	}

	m.mu.Lock()
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Status = StatusSucceeded
	m.mu.Unlock()

	m.publishStatus(run)
}

func (m *Manager) runTestCase(model registry.Model, tc registry.TestCase) TestCaseResult {
	result := TestCaseResult{TestCaseID: tc.ID, Prompt: tc.Prompt, Assertions: []assertions.Result{}}

	if model.Path == "" {
		result.Error = fmt.Sprintf("model %q has no local path to run", model.Name)
		return result
	}

	reply, err := m.runner.Generate(m.ctx, model.Path, tc.Prompt)
	if err != nil {
		result.Error = fmt.Sprintf("generate: %v", err)
		return result
	}

	result.Reply = reply
	result.Assertions, result.Passed = assertions.CheckAll(tc.Assertions, reply)

	return result
}

// persistResult upserts result into benchmarkName's results file, keyed by
// (BenchmarkVersion, ModelName) — a matching existing entry is replaced,
// not appended alongside.
func (m *Manager) persistResult(benchmarkName string, result RunResult) error {
	m.resultsMu.Lock()
	defer m.resultsMu.Unlock()

	existing, err := m.loadResults(benchmarkName)
	if err != nil {
		return err
	}

	replaced := false
	for i, r := range existing {
		if r.BenchmarkVersion == result.BenchmarkVersion && r.ModelName == result.ModelName {
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
		return fmt.Errorf("marshal benchmark results: %w", err)
	}

	if err := os.WriteFile(m.resultsPath(benchmarkName), data, 0o644); err != nil {
		return fmt.Errorf("write benchmark results: %w", err)
	}

	return nil
}

// loadResults reads benchmarkName's results file. A missing file (no runs
// persisted yet) is not an error — it just means no results exist.
func (m *Manager) loadResults(benchmarkName string) ([]RunResult, error) {
	data, err := os.ReadFile(m.resultsPath(benchmarkName))
	if os.IsNotExist(err) {
		return []RunResult{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read benchmark results: %w", err)
	}

	var results []RunResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse benchmark results: %w", err)
	}

	return results, nil
}

func (m *Manager) resultsPath(benchmarkName string) string {
	return filepath.Join(m.resultsDir, benchmarkName+".json")
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

func (m *Manager) publishProgress(runID, modelName string, result TestCaseResult) {
	data, err := json.Marshal(struct {
		RunID     string `json:"runId"`
		ModelName string `json:"modelName"`
		TestCaseResult
	}{RunID: runID, ModelName: modelName, TestCaseResult: result})
	if err != nil {
		return
	}
	m.bus.Publish(eventbus.Event{Type: ProgressEvent, Data: string(data)})
}

func newRunID() string {
	return fmt.Sprintf("benchrun-%d", time.Now().UnixNano())
}
