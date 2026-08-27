package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	benchmarkMetadataFile = "definition.json"
	benchmarkVersionsFile = "versions.json"
)

// Benchmark is a registry-tracked test suite run directly against a set of
// models (see internal/benchmarks) — unlike Evaluation, which runs against
// agents, a benchmark's test cases are sent straight to each model's own
// inference endpoint, so results are comparable model-to-model.
//
// TestCases here is the *draft*: freely editable (Add/Update/DeleteTestCase)
// and never run directly. Version is the number of the most recently
// published BenchmarkVersion (0 if none has ever been published) — it does
// NOT track draft edits, unlike an earlier design where it auto-incremented
// on every test case change. Publishing (PublishVersion) is the only way
// Version changes: it snapshots the current draft TestCases into a new,
// immutable BenchmarkVersion, which is what a run actually targets. This
// exists so a run's results can always be traced back to the exact,
// unchanging set of test cases that produced them — a run can't silently
// start meaning something different because someone edited a test case
// afterward.
type Benchmark struct {
	Name      string     `json:"name"`
	Version   int        `json:"version"`
	TestCases []TestCase `json:"testCases"`
	CreatedAt time.Time  `json:"createdAt"`
}

// BenchmarkVersion is an immutable snapshot of a benchmark's test cases,
// created by PublishVersion. Once published, a version's TestCases never
// change — editing the benchmark's draft afterward has no effect on
// versions already published.
type BenchmarkVersion struct {
	Version     int        `json:"version"`
	TestCases   []TestCase `json:"testCases"`
	PublishedAt time.Time  `json:"publishedAt"`
}

func (r *Registry) benchmarkDir(name string) string {
	return filepath.Join(r.benchmarksDir(), name)
}

func (r *Registry) benchmarksDir() string {
	return filepath.Join(r.root, "benchmarks")
}

// SaveBenchmark writes b's definition, creating or overwriting it. Version
// and CreatedAt are always computed here, not taken from b: a first save
// starts at Version 0 (nothing published yet) with CreatedAt set to now; a
// later save preserves both regardless of what b's caller set — draft edits
// (Add/Update/DeleteTestCase) never change Version. Only PublishVersion
// changes Version.
func (r *Registry) SaveBenchmark(b Benchmark) error {
	if existing, err := r.GetBenchmark(b.Name); err == nil {
		b.CreatedAt = existing.CreatedAt
		b.Version = existing.Version
	} else {
		b.CreatedAt = time.Now().UTC()
		b.Version = 0
	}

	return r.writeBenchmarkDefinition(b)
}

func (r *Registry) writeBenchmarkDefinition(b Benchmark) error {
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

// DeleteBenchmark removes a benchmark's directory (its definition, and any
// published versions). It's an error to delete a benchmark that doesn't
// exist.
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

// PublishVersion snapshots the benchmark's current draft TestCases into a
// new, immutable BenchmarkVersion (number = the previous latest + 1) and
// advances the benchmark's Version to it. It's an error to publish a
// benchmark with no draft test cases — there'd be nothing for a run to
// exercise.
func (r *Registry) PublishVersion(benchmarkName string) (BenchmarkVersion, error) {
	b, err := r.GetBenchmark(benchmarkName)
	if err != nil {
		return BenchmarkVersion{}, err
	}
	if len(b.TestCases) == 0 {
		return BenchmarkVersion{}, fmt.Errorf("benchmark %q has no test cases to publish", benchmarkName)
	}

	versions, err := r.ListVersions(benchmarkName)
	if err != nil {
		return BenchmarkVersion{}, err
	}

	newVersion := BenchmarkVersion{
		Version:     b.Version + 1,
		TestCases:   append([]TestCase(nil), b.TestCases...),
		PublishedAt: time.Now().UTC(),
	}

	if err := r.writeVersions(benchmarkName, append(versions, newVersion)); err != nil {
		return BenchmarkVersion{}, err
	}

	b.Version = newVersion.Version
	if err := r.writeBenchmarkDefinition(b); err != nil {
		return BenchmarkVersion{}, err
	}

	return newVersion, nil
}

// ListVersions returns every published version of the named benchmark,
// oldest first. A benchmark with nothing published yet returns an empty
// slice, not an error.
func (r *Registry) ListVersions(benchmarkName string) ([]BenchmarkVersion, error) {
	data, err := os.ReadFile(filepath.Join(r.benchmarkDir(benchmarkName), benchmarkVersionsFile))
	if os.IsNotExist(err) {
		return []BenchmarkVersion{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read benchmark versions: %w", err)
	}

	var versions []BenchmarkVersion
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("parse benchmark versions: %w", err)
	}

	return versions, nil
}

// GetVersion returns one published version of the named benchmark.
func (r *Registry) GetVersion(benchmarkName string, version int) (BenchmarkVersion, error) {
	versions, err := r.ListVersions(benchmarkName)
	if err != nil {
		return BenchmarkVersion{}, err
	}

	for _, v := range versions {
		if v.Version == version {
			return v, nil
		}
	}

	return BenchmarkVersion{}, fmt.Errorf("benchmark %q has no version %d", benchmarkName, version)
}

func (r *Registry) writeVersions(benchmarkName string, versions []BenchmarkVersion) error {
	dir := r.benchmarkDir(benchmarkName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create benchmark directory: %w", err)
	}

	data, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal benchmark versions: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, benchmarkVersionsFile), data, 0o644); err != nil {
		return fmt.Errorf("write benchmark versions: %w", err)
	}

	return nil
}

// AddTestCases appends one or more manually-entered (or generated) test
// cases to the named benchmark's draft, ignoring any ID the caller set on
// tcs (a fresh one is always assigned) — mirrors AppendExamples for
// datasets. This never touches any published version.
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

// UpdateTestCase overwrites the draft test case at index (0-based, in the
// order GetBenchmark returns them), keeping its existing ID. It's an error
// if index is out of range. This never touches any published version.
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

// DeleteTestCase removes the draft test case at index (0-based, in the
// order GetBenchmark returns them). It's an error if index is out of range.
// This never touches any published version.
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
