package registry

import "testing"

func testBenchmark(name string) Benchmark {
	return Benchmark{
		Name: name,
		TestCases: []TestCase{
			{ID: "tc1", Prompt: "say hello", Assertions: []Assertion{{Type: "contains", Value: "hello"}}},
		},
	}
}

func TestSaveAndGetBenchmark(t *testing.T) {
	reg := New(t.TempDir())

	want := testBenchmark("greeting-benchmark")
	if err := reg.SaveBenchmark(want); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	got, err := reg.GetBenchmark("greeting-benchmark")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if got.Name != want.Name || len(got.TestCases) != 1 {
		t.Errorf("GetBenchmark() = %+v, want %+v", got, want)
	}
	if len(got.TestCases[0].Assertions) != 1 || got.TestCases[0].Assertions[0].Value != "hello" {
		t.Errorf("GetBenchmark().TestCases[0].Assertions = %+v, want a single 'hello' assertion", got.TestCases[0].Assertions)
	}
	if got.Version != 1 {
		t.Errorf("GetBenchmark().Version = %d, want 1 for a first save", got.Version)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetBenchmark().CreatedAt is zero, want it set on first save")
	}
}

func TestSaveBenchmarkOverwrites(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting-benchmark")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	first, err := reg.GetBenchmark("greeting-benchmark")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}

	updated := Benchmark{Name: "greeting-benchmark", TestCases: nil}
	if err := reg.SaveBenchmark(updated); err != nil {
		t.Fatalf("SaveBenchmark() (update) error = %v", err)
	}

	got, err := reg.GetBenchmark("greeting-benchmark")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if len(got.TestCases) != 0 {
		t.Errorf("GetBenchmark().TestCases = %+v, want the overwrite to have cleared them", got.TestCases)
	}
	if got.Version != first.Version+1 {
		t.Errorf("GetBenchmark().Version = %d, want %d after changing TestCases", got.Version, first.Version+1)
	}
	if !got.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("GetBenchmark().CreatedAt = %v, want it preserved from the first save (%v)", got.CreatedAt, first.CreatedAt)
	}
}

func TestSaveBenchmarkVersionUnchangedWhenTestCasesIdentical(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting-benchmark")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	first, err := reg.GetBenchmark("greeting-benchmark")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}

	// Re-save with the exact same test cases (e.g. opening the editor and
	// saving without changing anything) shouldn't bump the version.
	if err := reg.SaveBenchmark(testBenchmark("greeting-benchmark")); err != nil {
		t.Fatalf("SaveBenchmark() (no-op update) error = %v", err)
	}

	got, err := reg.GetBenchmark("greeting-benchmark")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if got.Version != first.Version {
		t.Errorf("GetBenchmark().Version = %d, want unchanged at %d for identical TestCases", got.Version, first.Version)
	}
}

func TestGetBenchmarkUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetBenchmark("does-not-exist"); err == nil {
		t.Error("GetBenchmark() error = nil, want an error for an unknown benchmark")
	}
}

func TestListBenchmarksEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	benchmarks, err := reg.ListBenchmarks()
	if err != nil {
		t.Fatalf("ListBenchmarks() error = %v", err)
	}
	if len(benchmarks) != 0 {
		t.Errorf("ListBenchmarks() = %v, want empty", benchmarks)
	}
}

func TestListBenchmarksSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha"} {
		if err := reg.SaveBenchmark(testBenchmark(name)); err != nil {
			t.Fatalf("SaveBenchmark(%q) error = %v", name, err)
		}
	}

	benchmarks, err := reg.ListBenchmarks()
	if err != nil {
		t.Fatalf("ListBenchmarks() error = %v", err)
	}
	if len(benchmarks) != 2 || benchmarks[0].Name != "alpha" || benchmarks[1].Name != "zeta" {
		t.Errorf("ListBenchmarks() = %+v, want [alpha, zeta]", benchmarks)
	}
}

func TestDeleteBenchmark(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "throwaway"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	if err := reg.DeleteBenchmark("throwaway"); err != nil {
		t.Fatalf("DeleteBenchmark() error = %v", err)
	}

	if _, err := reg.GetBenchmark("throwaway"); err == nil {
		t.Error("GetBenchmark() error = nil, want an error after delete")
	}
}

func TestDeleteBenchmarkNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteBenchmark("does-not-exist"); err == nil {
		t.Error("DeleteBenchmark() error = nil, want an error for an unknown benchmark")
	}
}

func TestAddTestCases(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "greeting"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	if err := reg.AddTestCases("greeting", []TestCase{
		{Prompt: "say hi", Assertions: []Assertion{{Type: "contains", Value: "hi"}}},
		{Prompt: "say bye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}, Tags: []string{"farewell"}},
	}); err != nil {
		t.Fatalf("AddTestCases() error = %v", err)
	}

	got, err := reg.GetBenchmark("greeting")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if len(got.TestCases) != 2 {
		t.Fatalf("GetBenchmark().TestCases = %+v, want 2", got.TestCases)
	}
	if got.TestCases[0].ID == "" || got.TestCases[1].ID == "" || got.TestCases[0].ID == got.TestCases[1].ID {
		t.Errorf("GetBenchmark().TestCases IDs = %q, %q, want distinct, non-empty IDs assigned", got.TestCases[0].ID, got.TestCases[1].ID)
	}
	if len(got.TestCases[1].Tags) != 1 || got.TestCases[1].Tags[0] != "farewell" {
		t.Errorf("GetBenchmark().TestCases[1].Tags = %v, want [farewell]", got.TestCases[1].Tags)
	}
	if got.Version != 2 {
		t.Errorf("GetBenchmark().Version = %d, want 2 (bumped by adding test cases)", got.Version)
	}
}

func TestUpdateTestCase(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	before, err := reg.GetBenchmark("greeting")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	originalID := before.TestCases[0].ID

	if err := reg.UpdateTestCase("greeting", 0, TestCase{Prompt: "say hello there", Assertions: []Assertion{{Type: "contains", Value: "hello"}}}); err != nil {
		t.Fatalf("UpdateTestCase() error = %v", err)
	}

	got, err := reg.GetBenchmark("greeting")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if got.TestCases[0].Prompt != "say hello there" {
		t.Errorf("GetBenchmark().TestCases[0].Prompt = %q, want %q", got.TestCases[0].Prompt, "say hello there")
	}
	if got.TestCases[0].ID != originalID {
		t.Errorf("GetBenchmark().TestCases[0].ID = %q, want unchanged %q", got.TestCases[0].ID, originalID)
	}
	if got.Version != before.Version+1 {
		t.Errorf("GetBenchmark().Version = %d, want %d after changing a test case", got.Version, before.Version+1)
	}
}

func TestUpdateTestCaseOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "greeting"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	if err := reg.UpdateTestCase("greeting", 0, TestCase{Prompt: "say hi"}); err == nil {
		t.Error("UpdateTestCase() error = nil, want an error for an out-of-range index")
	}
}

func TestDeleteTestCase(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "greeting"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	if err := reg.AddTestCases("greeting", []TestCase{
		{Prompt: "say hi", Assertions: []Assertion{{Type: "contains", Value: "hi"}}},
		{Prompt: "say bye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}},
	}); err != nil {
		t.Fatalf("AddTestCases() error = %v", err)
	}

	if err := reg.DeleteTestCase("greeting", 0); err != nil {
		t.Fatalf("DeleteTestCase() error = %v", err)
	}

	got, err := reg.GetBenchmark("greeting")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if len(got.TestCases) != 1 || got.TestCases[0].Prompt != "say bye" {
		t.Errorf("GetBenchmark().TestCases = %+v, want only 'say bye' to remain", got.TestCases)
	}
}

func TestDeleteTestCaseOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "greeting"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	if err := reg.DeleteTestCase("greeting", 0); err == nil {
		t.Error("DeleteTestCase() error = nil, want an error for an out-of-range index")
	}
}
