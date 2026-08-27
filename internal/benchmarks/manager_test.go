package benchmarks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// fakeBenchmarkReader keys published versions by (benchmarkName, version),
// mirroring registry.Registry.GetVersion — a run always targets a specific,
// already-published version, never a benchmark's live draft test cases.
type fakeBenchmarkReader struct {
	versions map[string]map[int]registry.BenchmarkVersion
}

func (f *fakeBenchmarkReader) GetVersion(benchmarkName string, version int) (registry.BenchmarkVersion, error) {
	byVersion, ok := f.versions[benchmarkName]
	if !ok {
		return registry.BenchmarkVersion{}, errors.New("not found")
	}
	v, ok := byVersion[version]
	if !ok {
		return registry.BenchmarkVersion{}, errors.New("not found")
	}
	return v, nil
}

func newFakeBenchmarkReader(benchmarkName string, versions ...registry.BenchmarkVersion) *fakeBenchmarkReader {
	byVersion := make(map[int]registry.BenchmarkVersion)
	for _, v := range versions {
		byVersion[v.Version] = v
	}
	return &fakeBenchmarkReader{versions: map[string]map[int]registry.BenchmarkVersion{benchmarkName: byVersion}}
}

type fakeModelResolver struct {
	models map[string]registry.Model
}

func (f *fakeModelResolver) GetModel(name string) (registry.Model, error) {
	m, ok := f.models[name]
	if !ok {
		return registry.Model{}, errors.New("not found")
	}
	return m, nil
}

// fakeModelRunner replies to every Generate with the next canned reply keyed
// by prompt, so different test cases can get different replies.
type fakeModelRunner struct {
	repliesByPrompt map[string]string
	genErr          error
	sentPrompts     []string
	sentModels      []string
}

func (f *fakeModelRunner) Generate(ctx context.Context, model, prompt string) (string, error) {
	f.sentModels = append(f.sentModels, model)
	f.sentPrompts = append(f.sentPrompts, prompt)
	if f.genErr != nil {
		return "", f.genErr
	}
	return f.repliesByPrompt[prompt], nil
}

func simpleVersion(version int) registry.BenchmarkVersion {
	return registry.BenchmarkVersion{
		Version: version,
		TestCases: []registry.TestCase{
			{ID: "tc1", Prompt: "say hi", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}},
			{ID: "tc2", Prompt: "say bye", Assertions: []registry.Assertion{{Type: "contains", Value: "bye"}}},
		},
	}
}

func waitForRunStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) *Run {
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

func TestStartRunSucceedsWithMixedResults(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "see you later"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), t.TempDir())

	run, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.Status != StatusRunning {
		t.Errorf("StartRun().Status = %q, want %q", run.Status, StatusRunning)
	}
	if run.BenchmarkVersion != 1 {
		t.Errorf("StartRun().BenchmarkVersion = %d, want 1", run.BenchmarkVersion)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if len(finished.Results) != 1 {
		t.Fatalf("finished.Results = %+v, want 1 model", finished.Results)
	}

	mr := finished.Results[0]
	if mr.ModelName != "tiny" || mr.Total != 2 || mr.Passed != 1 {
		t.Errorf("RunResult = %+v, want tiny with 1/2 passed (tc2's reply doesn't contain 'bye')", mr)
	}
	if !mr.Results[0].Passed {
		t.Errorf("Results[0] = %+v, want passed (reply contains 'hello')", mr.Results[0])
	}
	if mr.Results[1].Passed {
		t.Errorf("Results[1] = %+v, want failed (reply doesn't contain 'bye')", mr.Results[1])
	}
	if len(runner.sentModels) != 2 || runner.sentModels[0] != "/models/tiny" {
		t.Errorf("runner.sentModels = %v, want Generate called with the model's Path", runner.sentModels)
	}
}

func TestStartRunRequiresModels(t *testing.T) {
	m := NewManager(context.Background(), newFakeBenchmarkReader("greeting"), &fakeModelResolver{}, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	if _, err := m.StartRun("greeting", 1, nil); err == nil {
		t.Error("StartRun() error = nil, want an error when no models are given")
	}
}

func TestStartRunUnknownBenchmark(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting")
	m := NewManager(context.Background(), benchReader, &fakeModelResolver{}, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	if _, err := m.StartRun("does-not-exist", 1, []string{"tiny"}); err == nil {
		t.Error("StartRun() error = nil, want an error for an unknown benchmark")
	}
}

func TestStartRunUnknownVersion(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	m := NewManager(context.Background(), benchReader, &fakeModelResolver{}, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	if _, err := m.StartRun("greeting", 99, []string{"tiny"}); err == nil {
		t.Error("StartRun() error = nil, want an error for an unpublished version")
	}
}

func TestStartRunEmptyVersion(t *testing.T) {
	benchReader := newFakeBenchmarkReader("empty", registry.BenchmarkVersion{Version: 1})
	m := NewManager(context.Background(), benchReader, &fakeModelResolver{}, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	if _, err := m.StartRun("empty", 1, []string{"tiny"}); err == nil {
		t.Error("StartRun() error = nil, want an error for a version with no test cases")
	}
}

func TestStartRunUnknownModelRecordsErrorPerTestCase(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	m := NewManager(context.Background(), benchReader, &fakeModelResolver{}, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	run, err := m.StartRun("greeting", 1, []string{"does-not-exist"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	mr := finished.Results[0]
	if mr.Passed != 0 || mr.Total != 2 {
		t.Errorf("RunResult = %+v, want 0/2 passed", mr)
	}
	for _, r := range mr.Results {
		if r.Error == "" {
			t.Errorf("TestCaseResult = %+v, want an error recorded", r)
		}
	}
}

func TestStartRunModelWithNoPathRecordsErrorPerTestCase(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny"}}}
	m := NewManager(context.Background(), benchReader, models, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	run, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	mr := finished.Results[0]
	for _, r := range mr.Results {
		if r.Error == "" {
			t.Errorf("TestCaseResult = %+v, want an error recorded for a model with no local path", r)
		}
	}
}

func TestStartRunGenerateFailureRecordsErrorPerTestCase(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{genErr: errors.New("mlx_lm.server unreachable")}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), t.TempDir())

	run, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	mr := finished.Results[0]
	if mr.Passed != 0 || mr.Total != 2 {
		t.Errorf("RunResult = %+v, want 0/2 passed", mr)
	}
	for _, r := range mr.Results {
		if r.Error == "" {
			t.Errorf("TestCaseResult = %+v, want an error recorded", r)
		}
	}
}

func TestStartRunMultipleModels(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{
		"a": {Name: "a", Path: "/models/a"},
		"b": {Name: "b", Path: "/models/b"},
	}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), t.TempDir())

	run, err := m.StartRun("greeting", 1, []string{"a", "b"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if len(finished.Results) != 2 {
		t.Fatalf("finished.Results = %+v, want 2 models", finished.Results)
	}
	for _, mr := range finished.Results {
		if mr.Passed != 2 || mr.Total != 2 {
			t.Errorf("RunResult = %+v, want 2/2 passed", mr)
		}
	}
}

func TestStartRunPersistsResultStampedWithBenchmarkVersion(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(3))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), t.TempDir())

	run, err := m.StartRun("greeting", 3, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)

	results, err := m.ListResults("greeting")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 1 || results[0].BenchmarkVersion != 3 || results[0].ModelName != "tiny" {
		t.Errorf("ListResults() = %+v, want a single result for tiny stamped with version 3", results)
	}
	if results[0].StartedAt.IsZero() || results[0].FinishedAt.IsZero() {
		t.Errorf("ListResults()[0] = %+v, want StartedAt/FinishedAt set", results[0])
	}
}

func TestStartRunResultsPersistAcrossManagerInstances(t *testing.T) {
	resultsDir := t.TempDir()
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), resultsDir)

	run, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)

	// A fresh Manager pointed at the same resultsDir (simulating a
	// `tlw serve` restart) should see the same persisted results.
	restarted := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), resultsDir)
	results, err := restarted.ListResults("greeting")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 1 || results[0].ModelName != "tiny" {
		t.Errorf("ListResults() (after restart) = %+v, want the persisted tiny result", results)
	}
}

func TestStartRunOverwritesResultForSameVersionAndModel(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "see you later"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), t.TempDir())

	run1, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, run1.ID, StatusSucceeded, time.Second)

	// Same benchmark version, same model, but the model now answers
	// differently — the previous result for (version 1, tiny) should be
	// replaced, not appended alongside.
	runner.repliesByPrompt["say bye"] = "bye!"
	run2, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() (second) error = %v", err)
	}
	waitForRunStatus(t, m, run2.ID, StatusSucceeded, time.Second)

	results, err := m.ListResults("greeting")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("ListResults() = %+v, want exactly 1 result (overwritten, not appended)", results)
	}
	if results[0].Passed != 2 {
		t.Errorf("ListResults()[0].Passed = %d, want 2 (reflecting the second run's replies)", results[0].Passed)
	}
}

func TestStartRunKeepsSeparateResultsForDifferentVersions(t *testing.T) {
	resultsDir := t.TempDir()
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1), simpleVersion(2))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), resultsDir)

	run1, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, run1.ID, StatusSucceeded, time.Second)

	run2, err := m.StartRun("greeting", 2, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() (v2) error = %v", err)
	}
	waitForRunStatus(t, m, run2.ID, StatusSucceeded, time.Second)

	results, err := m.ListResults("greeting")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("ListResults() = %+v, want 2 results (one per version)", results)
	}
}

func TestListResultsEmptyForUnknownBenchmark(t *testing.T) {
	m := NewManager(context.Background(), newFakeBenchmarkReader("never-run"), &fakeModelResolver{}, &fakeModelRunner{}, eventbus.New(), t.TempDir())

	results, err := m.ListResults("never-run")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ListResults() = %+v, want empty for a benchmark with no persisted results", results)
	}
}

func TestListRunsMostRecentFirst(t *testing.T) {
	benchReader := newFakeBenchmarkReader("greeting", simpleVersion(1))
	models := &fakeModelResolver{models: map[string]registry.Model{"tiny": {Name: "tiny", Path: "/models/tiny"}}}
	runner := &fakeModelRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := NewManager(context.Background(), benchReader, models, runner, eventbus.New(), t.TempDir())

	first, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, first.ID, StatusSucceeded, time.Second)

	time.Sleep(2 * time.Millisecond)
	second, err := m.StartRun("greeting", 1, []string{"tiny"})
	if err != nil {
		t.Fatalf("StartRun() (second) error = %v", err)
	}
	waitForRunStatus(t, m, second.ID, StatusSucceeded, time.Second)

	runs := m.ListRuns()
	if len(runs) != 2 || runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Errorf("ListRuns() = %+v, want [second, first]", runs)
	}
}
