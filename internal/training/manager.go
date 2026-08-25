package training

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

	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// ProgressEvent and StatusEvent are the eventbus event types the Training
// page's SSE stream listens for.
const (
	ProgressEvent = "training.progress"
	StatusEvent   = "training.status"
)

// datasetReader is the subset of registry.Registry Manager needs to load a
// run's training examples.
type datasetReader interface {
	ListExamples(name string) ([]registry.Example, error)
}

// modelSaver is the subset of registry.Registry Manager needs to register a
// successfully trained adapter as a usable model.
type modelSaver interface {
	SaveModel(m registry.Model) error
}

// Manager owns the lifecycle of training runs: starting them, tracking
// their progress, persisting history to disk, and registering the result
// as a model once a run succeeds.
type Manager struct {
	ctx      context.Context
	runsDir  string
	bus      *eventbus.Bus
	datasets datasetReader
	models   modelSaver
	trainer  Trainer

	mu   sync.Mutex
	runs map[string]*Run
}

// NewManager builds a Manager. ctx bounds the lifetime of every run it
// starts (e.g. the server's shutdown context) — it must outlive any single
// HTTP request, since training continues in the background after
// StartRun's caller gets its response.
func NewManager(ctx context.Context, runsDir string, bus *eventbus.Bus, datasets datasetReader, models modelSaver, trainer Trainer) *Manager {
	return &Manager{
		ctx:      ctx,
		runsDir:  runsDir,
		bus:      bus,
		datasets: datasets,
		models:   models,
		trainer:  trainer,
		runs:     make(map[string]*Run),
	}
}

// LoadRuns reads any run history previously persisted to runsDir, so it
// survives a restart of `tlw serve`. Any run still marked "running" from a
// previous process is a run that was killed mid-flight (the process that
// was tracking it is gone) and is marked "failed" accordingly.
func (m *Manager) LoadRuns() error {
	entries, err := os.ReadDir(m.runsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read runs directory: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.runsDir, entry.Name()))
		if err != nil {
			continue
		}

		var run Run
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		if run.Progress == nil {
			run.Progress = []ProgressPoint{}
		}

		if run.Status == StatusRunning {
			run.Status = StatusFailed
			run.Error = "server restarted while this run was in progress"
			now := time.Now().UTC()
			run.FinishedAt = &now
		}

		m.runs[run.ID] = &run
	}

	return nil
}

// StartRun validates cfg, creates a new Run in the "running" state, and
// begins training in the background. It returns as soon as the run is
// recorded — callers should poll GetRun or listen for ProgressEvent /
// StatusEvent to observe its progress.
func (m *Manager) StartRun(cfg Config) (*Run, error) {
	if cfg.BaseModel == "" {
		return nil, errors.New("baseModel is required")
	}
	if cfg.Dataset == "" {
		return nil, errors.New("dataset is required")
	}
	if cfg.OutputName == "" {
		return nil, errors.New("outputName is required")
	}
	if cfg.Iterations <= 0 {
		return nil, errors.New("iterations must be positive")
	}

	examples, err := m.datasets.ListExamples(cfg.Dataset)
	if err != nil {
		return nil, fmt.Errorf("load dataset %q: %w", cfg.Dataset, err)
	}
	if len(examples) == 0 {
		return nil, fmt.Errorf("dataset %q has no examples", cfg.Dataset)
	}

	run := &Run{
		ID:        newRunID(),
		Config:    cfg,
		Status:    StatusRunning,
		StartedAt: time.Now().UTC(),
		// Never nil: a nil slice marshals to JSON "null", which breaks
		// frontend code that calls .length on a run's progress array.
		Progress: []ProgressPoint{},
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	m.persist(run)
	m.publishStatus(run)

	go m.run(run, examples)

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

// run drives a single training job to completion. It always runs in its
// own goroutine, using m.ctx (not any per-request context) so it survives
// past the HTTP request that started it.
func (m *Manager) run(run *Run, examples []registry.Example) {
	onProgress := func(point ProgressPoint) {
		m.mu.Lock()
		run.Progress = append(run.Progress, point)
		m.mu.Unlock()

		m.persist(run)
		m.publishProgress(run.ID, point)
	}

	result, err := m.trainer.Train(m.ctx, run.Config, examples, onProgress)

	m.mu.Lock()
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err != nil {
		run.Status = StatusFailed
		run.Error = err.Error()
	} else {
		run.Status = StatusSucceeded
	}
	m.mu.Unlock()

	m.persist(run)
	m.publishStatus(run)

	if err != nil {
		return
	}

	if saveErr := m.models.SaveModel(registry.Model{
		Name:      run.Config.OutputName,
		Source:    "mlx",
		Path:      result.OutputDir,
		CreatedAt: now,
	}); saveErr != nil {
		logger.Error("Training run %s succeeded but failed to register its model: %v", run.ID, saveErr)
	}
}

func (m *Manager) persist(run *Run) {
	if err := os.MkdirAll(m.runsDir, 0o755); err != nil {
		logger.Error("Failed to create runs directory: %v", err)
		return
	}

	m.mu.Lock()
	data, err := json.MarshalIndent(run, "", "  ")
	m.mu.Unlock()
	if err != nil {
		logger.Error("Failed to marshal run %s: %v", run.ID, err)
		return
	}

	path := filepath.Join(m.runsDir, run.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logger.Error("Failed to persist run %s: %v", run.ID, err)
	}
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

func (m *Manager) publishProgress(runID string, point ProgressPoint) {
	data, err := json.Marshal(struct {
		RunID string `json:"runId"`
		ProgressPoint
	}{RunID: runID, ProgressPoint: point})
	if err != nil {
		return
	}
	m.bus.Publish(eventbus.Event{Type: ProgressEvent, Data: string(data)})
}

func newRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}
