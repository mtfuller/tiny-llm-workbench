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
	saved  []registry.Model
	err    error
	models map[string]registry.Model // registry models GetModel can resolve
}

func (f *fakeModelSaver) SaveModel(m registry.Model) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, m)
	return nil
}

func (f *fakeModelSaver) ResolveModelRef(ref string) string {
	if m, ok := f.models[ref]; ok && m.Path != "" {
		return m.Path
	}
	return ref
}

func (f *fakeModelSaver) ModelDir(name string) string {
	return "/tmp/fake-registry/models/" + name
}

// fakeTrainer lets tests control exactly what a training run does without
// touching a real subprocess.
type fakeTrainer struct {
	progress []ProgressPoint
	result   Result
	err      error
	started  chan struct{} // closed once Train is invoked, for synchronization

	// blockUntilCancel, when set, makes Train hang until ctx is cancelled
	// and return ctx.Err() — simulating a subprocess killed by CancelRun.
	blockUntilCancel bool

	// fuseErr, when set, makes Fuse fail — simulating a run whose training
	// succeeded but couldn't be turned into a servable model.
	fuseErr error
	fused   []string // baseModel|adapterDir|savePath for each Fuse call
}

func (f *fakeTrainer) Train(ctx context.Context, cfg Config, examples []registry.Example, onProgress func(ProgressPoint)) (Result, error) {
	if f.started != nil {
		close(f.started)
	}
	for _, p := range f.progress {
		onProgress(p)
	}
	if f.blockUntilCancel {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	return f.result, f.err
}

func (f *fakeTrainer) Fuse(ctx context.Context, baseModel, adapterDir, savePath string) error {
	f.fused = append(f.fused, baseModel+"|"+adapterDir+"|"+savePath)
	return f.fuseErr
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

	if len(models.saved) != 1 || models.saved[0].Name != "my-finetune" || models.saved[0].Source != "mlx" ||
		models.saved[0].BaseModel != "mlx-community/test-model" {
		t.Errorf("models.saved = %+v, want the trained model registered with its base model", models.saved)
	}
	wantPath := models.ModelDir("my-finetune")
	if models.saved[0].Path != wantPath {
		t.Errorf("models.saved[0].Path = %q, want the fused model directory %q, not the raw adapter dir", models.saved[0].Path, wantPath)
	}
	if len(trainer.fused) != 1 || trainer.fused[0] != "mlx-community/test-model|/tmp/adapter|"+wantPath {
		t.Errorf("trainer.fused = %v, want one Fuse call from the adapter dir into the registry model dir", trainer.fused)
	}
}

// A base model given as a registry model's name (what the Training page's
// picker sends) is resolved to that model's Path before mlx-lm sees it — so
// an added Hugging Face model ("Llama-3.2-1B-Instruct-4bit", Path
// "mlx-community/Llama-3.2-1B-Instruct-4bit") trains instead of failing with
// a 401 on the org-less name.
func TestStartRunResolvesRegistryModelBaseModel(t *testing.T) {
	trainer := &fakeTrainer{result: Result{OutputDir: "/tmp/adapter"}}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	models := &fakeModelSaver{models: map[string]registry.Model{
		"Llama-3.2-1B-Instruct-4bit": {
			Name: "Llama-3.2-1B-Instruct-4bit",
			Path: "mlx-community/Llama-3.2-1B-Instruct-4bit",
		},
	}}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, models, trainer)

	cfg := validConfig()
	cfg.BaseModel = "Llama-3.2-1B-Instruct-4bit"
	run, err := m.StartRun(cfg)
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	// The stored run config reflects the resolved value.
	if run.Config.BaseModel != "mlx-community/Llama-3.2-1B-Instruct-4bit" {
		t.Errorf("run.Config.BaseModel = %q, want the resolved repo id", run.Config.BaseModel)
	}

	finished := waitForStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if len(trainer.fused) != 1 || !strings.HasPrefix(trainer.fused[0], "mlx-community/Llama-3.2-1B-Instruct-4bit|") {
		t.Errorf("trainer.fused = %v, want Fuse called with the resolved repo id", trainer.fused)
	}
	if len(models.saved) != 1 || models.saved[0].BaseModel != "mlx-community/Llama-3.2-1B-Instruct-4bit" {
		t.Errorf("models.saved[0].BaseModel = %q, want the resolved repo id recorded", models.saved[0].BaseModel)
	}
	_ = finished
}

// A base model that isn't a known registry model — a raw HF repo id or a
// local path typed directly — passes through untouched.
func TestStartRunLeavesUnknownBaseModelUnchanged(t *testing.T) {
	trainer := &fakeTrainer{result: Result{OutputDir: "/tmp/adapter"}}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	models := &fakeModelSaver{} // no registry models to resolve against
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, models, trainer)

	cfg := validConfig()
	cfg.BaseModel = "mlx-community/Qwen2.5-0.5B-Instruct-4bit"
	run, err := m.StartRun(cfg)
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.Config.BaseModel != "mlx-community/Qwen2.5-0.5B-Instruct-4bit" {
		t.Errorf("run.Config.BaseModel = %q, want it unchanged", run.Config.BaseModel)
	}
}

func TestStartRunFuseFailureMarksRunFailed(t *testing.T) {
	trainer := &fakeTrainer{
		result:  Result{OutputDir: "/tmp/adapter"},
		fuseErr: errors.New("mlx_lm.fuse: unknown data type: U32"),
	}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	models := &fakeModelSaver{}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, models, trainer)

	run, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForStatus(t, m, run.ID, StatusFailed, time.Second)
	if !strings.Contains(finished.Error, "unknown data type: U32") {
		t.Errorf("finished.Error = %q, want it to include the underlying fuse error", finished.Error)
	}
	if len(models.saved) != 0 {
		t.Errorf("models.saved = %+v, want no model registered when fusing fails", models.saved)
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

func TestCancelRunStopsAndMarksCancelled(t *testing.T) {
	started := make(chan struct{})
	trainer := &fakeTrainer{started: started, blockUntilCancel: true}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, trainer)

	run, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	<-started

	if err := m.CancelRun(run.ID); err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}

	finished := waitForStatus(t, m, run.ID, StatusCancelled, time.Second)
	if finished.Error == "" {
		t.Error("finished.Error = \"\", want a message explaining the cancellation")
	}
}

func TestCancelRunUnknownRun(t *testing.T) {
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), &fakeDatasets{}, &fakeModelSaver{}, &fakeTrainer{})

	if err := m.CancelRun("does-not-exist"); err == nil {
		t.Error("CancelRun() error = nil, want an error for an unknown run")
	}
}

func TestCancelRunAlreadyFinishedIsNoop(t *testing.T) {
	trainer := &fakeTrainer{result: Result{OutputDir: "/tmp/adapter"}}
	datasets := &fakeDatasets{examples: map[string][]registry.Example{"greetings": {{Input: "hi", Output: "hello!"}}}}
	m := NewManager(context.Background(), t.TempDir(), eventbus.New(), datasets, &fakeModelSaver{}, trainer)

	run, err := m.StartRun(validConfig())
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForStatus(t, m, run.ID, StatusSucceeded, time.Second)

	if err := m.CancelRun(run.ID); err != nil {
		t.Fatalf("CancelRun() error = %v, want nil for an already-finished run", err)
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
