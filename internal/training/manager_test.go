package training

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeDatasets struct {
	examples map[string][]registry.Example
	err      error
}

func (f *fakeDatasets) ListExamples(name string) ([]registry.Example, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.examples[name], nil
}

type fakeModelSaver struct {
	saved []registry.Model
	err   error
}

func (f *fakeModelSaver) SaveModel(m registry.Model) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, m)
	return nil
}

// fakeTrainer lets tests control exactly what a training run does without
// touching a real subprocess.
type fakeTrainer struct {
	progress []ProgressPoint
	result   Result
	err      error
	started  chan struct{} // closed once Train is invoked, for synchronization
}

func (f *fakeTrainer) Train(ctx context.Context, cfg Config, examples []registry.Example, onProgress func(ProgressPoint)) (Result, error) {
	if f.started != nil {
		close(f.started)
	}
	for _, p := range f.progress {
		onProgress(p)
	}
	return f.result, f.err
}

func validConfig() Config {
	return Config{BaseModel: "mlx-community/test-model", Dataset: "greetings", OutputName: "my-finetune", Iterations: 100}
}

func waitForStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) *Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if run, ok := m.GetRun(id); ok && run.Status == want {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach status %q within %s", id, want, timeout)
	return nil
}

func TestStartRunSucceeds(t *testing.T) {
	loss := 0.5
	trainer := &fakeTrainer{
		progress: []ProgressPoint{{Iteration: 10, TrainLoss: &loss}},
		result:   Result{OutputDir: "/tmp/adapter"},
	}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	models := &fakeModelSaver{}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, models, trainer)

	run, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.Status != StatusRunning {
		t.Errorf("StartRun() status = %q, want %q", run.Status, StatusRunning)
	}

	finished := waitForStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if len(finished.Progress) != 1 || *finished.Progress[0].TrainLoss != loss {
		t.Errorf("finished.Progress = %+v, want the reported progress point", finished.Progress)
	}
	if finished.FinishedAt == nil {
		t.Error("finished.FinishedAt = nil, want it set")
	}

	if len(models.saved) != 1 || models.saved[0].Name != "my-finetune" || models.saved[0].Source != "mlx" {
		t.Errorf("models.saved = %+v, want the trained model registered", models.saved)
	}
}

func TestStartRunFails(t *testing.T) {
	trainer := &fakeTrainer{err: errors.New("mlx_lm not installed")}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	models := &fakeModelSaver{}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, models, trainer)

	run, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForStatus(t, m, run.ID, StatusFailed, time.Second)
	if finished.Error != "mlx_lm not installed" {
		t.Errorf("finished.Error = %q, want %q", finished.Error, "mlx_lm not installed")
	}
	if len(models.saved) != 0 {
		t.Errorf("models.saved = %+v, want no model registered for a failed run", models.saved)
	}
}

func TestStartRunProgressIsJSONArrayNotNull(t *testing.T) {
	// A nil Progress slice marshals to JSON "null", which breaks frontend
	// code that calls .length on a run's progress array — this happened
	// for real when testing the Training page in the browser.
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, &fakeTrainer{})

	run, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"progress":[]`) {
		t.Errorf("marshaled run = %s, want progress to serialize as []", data)
	}
}

func TestStartRunValidatesConfig(t *testing.T) {
	datasets := &fakeDatasets{}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, &fakeTrainer{})

	tests := []struct {
		name string
		cfg  Config
	}{
		{"missing base model", Config{Dataset: "d", OutputName: "o", Iterations: 1}},
		{"missing dataset", Config{BaseModel: "m", OutputName: "o", Iterations: 1}},
		{"missing output name", Config{BaseModel: "m", Dataset: "d", Iterations: 1}},
		{"zero iterations", Config{BaseModel: "m", Dataset: "d", OutputName: "o"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := m.StartRun(tt.cfg); err == nil {
				t.Error("StartRun() error = nil, want a validation error")
			}
		})
	}
}

func TestStartRunUnknownDataset(t *testing.T) {
	datasets := &fakeDatasets{err: errors.New("open dataset examples: no such file or directory")}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, &fakeTrainer{})

	if _, err := m.StartRun(validConfig()); err == nil {
		t.Error("StartRun() error = nil, want an error for a missing dataset")
	}
}

func TestStartRunEmptyDataset(t *testing.T) {
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {}}}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, &fakeTrainer{})

	if _, err := m.StartRun(validConfig()); err == nil {
		t.Error("StartRun() error = nil, want an error for an empty dataset")
	}
}

func TestListRunsMostRecentFirst(t *testing.T) {
	trainer := &fakeTrainer{result: Result{OutputDir: "/tmp/adapter"}}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, trainer)

	first, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForStatus(t, m, first.ID, StatusSucceeded, time.Second)

	time.Sleep(2 * time.Millisecond) // ensure a distinct StartedAt
	cfg2 := validConfig()
	cfg2.OutputName = "second-finetune"
	second, err := m.StartRun(cfg2)
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForStatus(t, m, second.ID, StatusSucceeded, time.Second)

	runs := m.ListRuns()
	if len(runs) != 2 || runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Errorf("ListRuns() = %+v, want [second, first]", runs)
	}
}

func TestLoadRunsMarksInterruptedRunsFailed(t *testing.T) {
	dir := t.TempDir()
	trainer := &fakeTrainer{}
	datasets := &fakeDatasets{}
	m := NewManager(context.Background(), dir, eventbus.New(), datasets, &fakeModelSaver{}, trainer)

	interrupted := &Run{ID: "run-old", Status: StatusRunning, StartedAt: time.Now().Add(-time.Hour)}
	m.persist(interrupted)

	// Fresh manager simulating a server restart.
	m2 := NewManager(context.Background(), dir, eventbus.New(), datasets, &fakeModelSaver{}, trainer)
	if err := m2.LoadRuns(); err != nil {
		t.Fatalf("LoadRuns() error = %v", err)
	}

	run, ok := m2.GetRun("run-old")
	if !ok {
		t.Fatal("GetRun(run-old) not found after LoadRuns()")
	}
	if run.Status != StatusFailed {
		t.Errorf("run.Status = %q, want %q for an interrupted run", run.Status, StatusFailed)
	}
}
