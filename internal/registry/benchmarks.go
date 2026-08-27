package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"
)

const benchmarkMetadataFile = "definition.json"

// Benchmark is a registry-tracked test suite run directly against a set of
// models (see internal/benchmarks) — unlike Evaluation, which runs against
// agents, a benchmark's test cases are sent straight to each model's own
// inference endpoint, so results are comparable model-to-model.
//
// Version increments each time TestCases actually changes (SaveBenchmark
// diffs against the previously stored definition) — a no-op save (e.g.
// opening the editor and saving without changing anything) leaves it
// unchanged. internal/benchmarks stamps a run's results with the Version
// that was current when it ran, so a stored result can be told apart from
// one produced by a since-edited test suite.
type Benchmark struct {
	Name      string     `json:"name"`
	Version   int        `json:"version"`
	TestCases []TestCase `json:"testCases"`
	CreatedAt time.Time  `json:"createdAt"`
}

func (r *Registry) benchmarkDir(name string) string {
	return filepath.Join(r.benchmarksDir(), name)
}

func (r *Registry) benchmarksDir() string {
	return filepath.Join(r.root, "benchmarks")
}

// SaveBenchmark writes b's definition, creating or overwriting it. Version
// and CreatedAt are computed here, not taken from b: a first save gets
// Version 1 and CreatedAt set to now; a later save keeps the existing
// CreatedAt and only bumps Version if TestCases actually changed.
func (r *Registry) SaveBenchmark(b Benchmark) error {
	if existing, err := r.GetBenchmark(b.Name); err == nil {
		b.CreatedAt = existing.CreatedAt
		b.Version = existing.Version
		if !reflect.DeepEqual(existing.TestCases, b.TestCases) {
			b.Version++
		}
	} else {
		b.CreatedAt = time.Now().UTC()
		b.Version = 1
	}

	dir := r.benchmarkDir(b.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create benchmark directory: %w", err)
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal benchmark definition: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, benchmarkMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write benchmark definition: %w", err)
	}

	return nil
}

// GetBenchmark returns the named benchmark's definition.
func (r *Registry) GetBenchmark(name string) (Benchmark, error) {
	data, err := os.ReadFile(filepath.Join(r.benchmarkDir(name), benchmarkMetadataFile))
	if err != nil {
		return Benchmark{}, fmt.Errorf("read benchmark %q: %w", name, err)
	}

	var b Benchmark
	if err := json.Unmarshal(data, &b); err != nil {
		return Benchmark{}, fmt.Errorf("parse definition for benchmark %q: %w", name, err)
	}

	return b, nil
}

// DeleteBenchmark removes a benchmark's directory (its definition). It's an
// error to delete a benchmark that doesn't exist.
func (r *Registry) DeleteBenchmark(name string) error {
	dir := r.benchmarkDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("benchmark %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete benchmark %q: %w", name, err)
	}
	return nil
}

// AddTestCases appends one or more manually-entered (or generated) test
// cases to the named benchmark, ignoring any ID the caller set on tcs (a
// fresh one is always assigned) — mirrors AppendExamples for datasets.
func (r *Registry) AddTestCases(benchmarkName string, tcs []TestCase) error {
	b, err := r.GetBenchmark(benchmarkName)
	if err != nil {
		return err
	}

	now := time.Now().UnixNano()
	for i, tc := range tcs {
		tc.ID = fmt.Sprintf("tc-%d-%d", now, i)
		b.TestCases = append(b.TestCases, tc)
	}

	return r.SaveBenchmark(b)
}

// UpdateTestCase overwrites the test case at index (0-based, in the order
// GetBenchmark returns them), keeping its existing ID. It's an error if
// index is out of range.
func (r *Registry) UpdateTestCase(benchmarkName string, index int, tc TestCase) error {
	b, err := r.GetBenchmark(benchmarkName)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(b.TestCases) {
		return fmt.Errorf("test case index %d out of range (benchmark has %d test cases)", index, len(b.TestCases))
	}

	tc.ID = b.TestCases[index].ID
	b.TestCases[index] = tc
	return r.SaveBenchmark(b)
}

// DeleteTestCase removes the test case at index (0-based, in the order
// GetBenchmark returns them). It's an error if index is out of range.
func (r *Registry) DeleteTestCase(benchmarkName string, index int) error {
	b, err := r.GetBenchmark(benchmarkName)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(b.TestCases) {
		return fmt.Errorf("test case index %d out of range (benchmark has %d test cases)", index, len(b.TestCases))
	}

	b.TestCases = append(b.TestCases[:index], b.TestCases[index+1:]...)
	return r.SaveBenchmark(b)
}

// ListBenchmarks returns every registry-tracked benchmark, sorted by name.
func (r *Registry) ListBenchmarks() ([]Benchmark, error) {
	entries, err := os.ReadDir(r.benchmarksDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read benchmarks directory: %w", err)
	}

	var benchmarks []Benchmark
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		b, err := r.GetBenchmark(entry.Name())
		if err != nil {
			continue // skip directories without a valid definition
		}
		benchmarks = append(benchmarks, b)
	}

	sort.Slice(benchmarks, func(i, j int) bool { return benchmarks[i].Name < benchmarks[j].Name })

	return benchmarks, nil
}
