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
	if got.Version != 0 {
		t.Errorf("GetBenchmark().Version = %d, want 0 (nothing published yet) for a first save", got.Version)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetBenchmark().CreatedAt is zero, want it set on first save")
	}
}

func TestSaveBenchmarkOverwritesWithoutBumpingVersion(t *testing.T) {
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
	if got.Version != first.Version {
		t.Errorf("GetBenchmark().Version = %d, want unchanged at %d — draft edits never bump Version", got.Version, first.Version)
	}
	if !got.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("GetBenchmark().CreatedAt = %v, want it preserved from the first save (%v)", got.CreatedAt, first.CreatedAt)
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

func TestAddTestCasesDoesNotBumpVersion(t *testing.T) {
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
	if got.Version != 0 {
		t.Errorf("GetBenchmark().Version = %d, want 0 — adding draft test cases never publishes a version", got.Version)
	}
}

func TestUpdateTestCaseDoesNotBumpVersion(t *testing.T) {
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
	if got.Version != before.Version {
		t.Errorf("GetBenchmark().Version = %d, want unchanged at %d — editing a draft test case never publishes a version", got.Version, before.Version)
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

func TestApproveAndFlagTestCase(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "greeting"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	if err := reg.AddTestCases("greeting", []TestCase{
		{Prompt: "say hi", Source: "ai", Assertions: []Assertion{{Type: "contains", Value: "hi"}}},
	}); err != nil {
		t.Fatalf("AddTestCases() error = %v", err)
	}

	if err := reg.ApproveTestCase("greeting", 0); err != nil {
		t.Fatalf("ApproveTestCase() error = %v", err)
	}
	got, _ := reg.GetBenchmark("greeting")
	if !got.TestCases[0].Approved || got.TestCases[0].NeedsReview {
		t.Errorf("after ApproveTestCase: %+v, want Approved=true NeedsReview=false", got.TestCases[0])
	}
	if got.TestCases[0].Source != "ai" {
		t.Errorf("ApproveTestCase changed Source to %q, want it left as \"ai\"", got.TestCases[0].Source)
	}

	if err := reg.FlagTestCaseForReview("greeting", 0); err != nil {
		t.Fatalf("FlagTestCaseForReview() error = %v", err)
	}
	got, _ = reg.GetBenchmark("greeting")
	if !got.TestCases[0].NeedsReview || got.TestCases[0].Approved {
		t.Errorf("after FlagTestCaseForReview: %+v, want NeedsReview=true Approved=false", got.TestCases[0])
	}

	if err := reg.ApproveTestCase("greeting", 5); err == nil {
		t.Error("ApproveTestCase(out of range) error = nil, want an error")
	}
	if err := reg.FlagTestCaseForReview("greeting", 5); err == nil {
		t.Error("FlagTestCaseForReview(out of range) error = nil, want an error")
	}
}

func TestPublishVersion(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	v, err := reg.PublishVersion("greeting")
	if err != nil {
		t.Fatalf("PublishVersion() error = %v", err)
	}
	if v.Version != 1 {
		t.Errorf("PublishVersion().Version = %d, want 1", v.Version)
	}
	if len(v.TestCases) != 1 || v.TestCases[0].Prompt != "say hello" {
		t.Errorf("PublishVersion().TestCases = %+v, want the draft's single test case", v.TestCases)
	}
	if v.PublishedAt.IsZero() {
		t.Error("PublishVersion().PublishedAt is zero, want it set")
	}

	got, err := reg.GetBenchmark("greeting")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if got.Version != 1 {
		t.Errorf("GetBenchmark().Version = %d, want 1 after publishing", got.Version)
	}
}

func TestPublishVersionRequiresTestCases(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(Benchmark{Name: "empty"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	if _, err := reg.PublishVersion("empty"); err == nil {
		t.Error("PublishVersion() error = nil, want an error for a benchmark with no test cases")
	}
}

func TestPublishVersionIsImmutableToLaterDraftEdits(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	v1, err := reg.PublishVersion("greeting")
	if err != nil {
		t.Fatalf("PublishVersion() error = %v", err)
	}

	// Editing the draft afterward must not retroactively change the
	// already-published version's snapshot.
	if err := reg.UpdateTestCase("greeting", 0, TestCase{Prompt: "say goodbye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}}); err != nil {
		t.Fatalf("UpdateTestCase() error = %v", err)
	}

	got, err := reg.GetVersion("greeting", v1.Version)
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if got.TestCases[0].Prompt != "say hello" {
		t.Errorf("GetVersion().TestCases[0].Prompt = %q, want unchanged %q despite the later draft edit", got.TestCases[0].Prompt, "say hello")
	}
}

func TestPublishVersionIncrementsAcrossMultiplePublishes(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	if _, err := reg.PublishVersion("greeting"); err != nil {
		t.Fatalf("PublishVersion() (1st) error = %v", err)
	}

	if err := reg.AddTestCases("greeting", []TestCase{{Prompt: "say bye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}}}); err != nil {
		t.Fatalf("AddTestCases() error = %v", err)
	}
	v2, err := reg.PublishVersion("greeting")
	if err != nil {
		t.Fatalf("PublishVersion() (2nd) error = %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("PublishVersion() (2nd).Version = %d, want 2", v2.Version)
	}
	if len(v2.TestCases) != 2 {
		t.Errorf("PublishVersion() (2nd).TestCases = %+v, want 2 (both draft test cases)", v2.TestCases)
	}

	versions, err := reg.ListVersions("greeting")
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 2 || versions[0].Version != 1 || versions[1].Version != 2 {
		t.Errorf("ListVersions() = %+v, want [v1, v2]", versions)
	}
}

func TestListVersionsEmptyForUnpublishedBenchmark(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	versions, err := reg.ListVersions("greeting")
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("ListVersions() = %+v, want empty for a benchmark with nothing published", versions)
	}
}

func TestGetVersionUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveBenchmark(testBenchmark("greeting")); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}
	if _, err := reg.PublishVersion("greeting"); err != nil {
		t.Fatalf("PublishVersion() error = %v", err)
	}

	if _, err := reg.GetVersion("greeting", 99); err == nil {
		t.Error("GetVersion() error = nil, want an error for an unpublished version number")
	}
}
