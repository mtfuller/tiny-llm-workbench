package registry

import "testing"

func testEvaluation(name string) Evaluation {
	return Evaluation{
		Name: name,
		TestCases: []TestCase{
			{ID: "tc1", Prompt: "say hello", Assertions: []Assertion{{Type: "contains", Value: "hello"}}},
		},
	}
}

func TestSaveAndGetEvaluation(t *testing.T) {
	reg := New(t.TempDir())

	want := testEvaluation("greeting-eval")
	if err := reg.SaveEvaluation(want); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	got, err := reg.GetEvaluation("greeting-eval")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if got.Name != want.Name || len(got.TestCases) != 1 {
		t.Errorf("GetEvaluation() = %+v, want %+v", got, want)
	}
	if len(got.TestCases[0].Assertions) != 1 || got.TestCases[0].Assertions[0].Value != "hello" {
		t.Errorf("GetEvaluation().TestCases[0].Assertions = %+v, want a single 'hello' assertion", got.TestCases[0].Assertions)
	}
	if got.Version != 0 {
		t.Errorf("GetEvaluation().Version = %d, want 0 (nothing published yet) for a first save", got.Version)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetEvaluation().CreatedAt is zero, want it set on first save")
	}
}

func TestSaveEvaluationOverwritesWithoutBumpingVersion(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting-eval")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}
	first, err := reg.GetEvaluation("greeting-eval")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}

	updated := Evaluation{Name: "greeting-eval", TestCases: nil}
	if err := reg.SaveEvaluation(updated); err != nil {
		t.Fatalf("SaveEvaluation() (update) error = %v", err)
	}

	got, err := reg.GetEvaluation("greeting-eval")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if len(got.TestCases) != 0 {
		t.Errorf("GetEvaluation().TestCases = %+v, want the overwrite to have cleared them", got.TestCases)
	}
	if got.Version != first.Version {
		t.Errorf("GetEvaluation().Version = %d, want unchanged at %d — draft edits never bump Version", got.Version, first.Version)
	}
	if !got.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("GetEvaluation().CreatedAt = %v, want it preserved from the first save (%v)", got.CreatedAt, first.CreatedAt)
	}
}

func TestGetEvaluationUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetEvaluation("does-not-exist"); err == nil {
		t.Error("GetEvaluation() error = nil, want an error for an unknown evaluation")
	}
}

func TestListEvaluationsEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	evals, err := reg.ListEvaluations()
	if err != nil {
		t.Fatalf("ListEvaluations() error = %v", err)
	}
	if len(evals) != 0 {
		t.Errorf("ListEvaluations() = %v, want empty", evals)
	}
}

func TestListEvaluationsSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha"} {
		if err := reg.SaveEvaluation(testEvaluation(name)); err != nil {
			t.Fatalf("SaveEvaluation(%q) error = %v", name, err)
		}
	}

	evals, err := reg.ListEvaluations()
	if err != nil {
		t.Fatalf("ListEvaluations() error = %v", err)
	}
	if len(evals) != 2 || evals[0].Name != "alpha" || evals[1].Name != "zeta" {
		t.Errorf("ListEvaluations() = %+v, want [alpha, zeta]", evals)
	}
}

func TestDeleteEvaluation(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(Evaluation{Name: "throwaway"}); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	if err := reg.DeleteEvaluation("throwaway"); err != nil {
		t.Fatalf("DeleteEvaluation() error = %v", err)
	}

	if _, err := reg.GetEvaluation("throwaway"); err == nil {
		t.Error("GetEvaluation() error = nil, want an error after delete")
	}
}

func TestDeleteEvaluationNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteEvaluation("does-not-exist"); err == nil {
		t.Error("DeleteEvaluation() error = nil, want an error for an unknown evaluation")
	}
}

func TestAddEvaluationTestCasesDoesNotBumpVersion(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(Evaluation{Name: "greeting"}); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	if err := reg.AddEvaluationTestCases("greeting", []TestCase{
		{Prompt: "say hi", Assertions: []Assertion{{Type: "contains", Value: "hi"}}},
		{
			Prompt:         "write a file",
			Workspace:      "repo-scenario",
			Assertions:     []Assertion{{Type: "contains", Value: "done"}},
			VerifyCommands: []VerifyStep{{Command: "cat /workspace/out.txt", Assertions: []Assertion{{Type: "contains", Value: "hello"}}}},
			Tags:           []string{"software-dev"},
		},
	}); err != nil {
		t.Fatalf("AddEvaluationTestCases() error = %v", err)
	}

	got, err := reg.GetEvaluation("greeting")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if len(got.TestCases) != 2 {
		t.Fatalf("GetEvaluation().TestCases = %+v, want 2", got.TestCases)
	}
	if got.TestCases[0].ID == "" || got.TestCases[1].ID == "" || got.TestCases[0].ID == got.TestCases[1].ID {
		t.Errorf("GetEvaluation().TestCases IDs = %q, %q, want distinct, non-empty IDs assigned", got.TestCases[0].ID, got.TestCases[1].ID)
	}
	if got.TestCases[1].Workspace != "repo-scenario" {
		t.Errorf("GetEvaluation().TestCases[1].Workspace = %q, want repo-scenario", got.TestCases[1].Workspace)
	}
	if len(got.TestCases[1].VerifyCommands) != 1 || got.TestCases[1].VerifyCommands[0].Command != "cat /workspace/out.txt" {
		t.Errorf("GetEvaluation().TestCases[1].VerifyCommands = %+v, want a single cat command", got.TestCases[1].VerifyCommands)
	}
	if got.Version != 0 {
		t.Errorf("GetEvaluation().Version = %d, want 0 — adding draft test cases never publishes a version", got.Version)
	}
}

func TestUpdateEvaluationTestCaseDoesNotBumpVersion(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}
	before, err := reg.GetEvaluation("greeting")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	originalID := before.TestCases[0].ID

	if err := reg.UpdateEvaluationTestCase("greeting", 0, TestCase{Prompt: "say hello there", Assertions: []Assertion{{Type: "contains", Value: "hello"}}}); err != nil {
		t.Fatalf("UpdateEvaluationTestCase() error = %v", err)
	}

	got, err := reg.GetEvaluation("greeting")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if got.TestCases[0].Prompt != "say hello there" {
		t.Errorf("GetEvaluation().TestCases[0].Prompt = %q, want %q", got.TestCases[0].Prompt, "say hello there")
	}
	if got.TestCases[0].ID != originalID {
		t.Errorf("GetEvaluation().TestCases[0].ID = %q, want unchanged %q", got.TestCases[0].ID, originalID)
	}
	if got.Version != before.Version {
		t.Errorf("GetEvaluation().Version = %d, want unchanged at %d — editing a draft test case never publishes a version", got.Version, before.Version)
	}
}

func TestUpdateEvaluationTestCaseOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(Evaluation{Name: "greeting"}); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	if err := reg.UpdateEvaluationTestCase("greeting", 0, TestCase{Prompt: "say hi"}); err == nil {
		t.Error("UpdateEvaluationTestCase() error = nil, want an error for an out-of-range index")
	}
}

func TestDeleteEvaluationTestCase(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(Evaluation{Name: "greeting"}); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}
	if err := reg.AddEvaluationTestCases("greeting", []TestCase{
		{Prompt: "say hi", Assertions: []Assertion{{Type: "contains", Value: "hi"}}},
		{Prompt: "say bye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}},
	}); err != nil {
		t.Fatalf("AddEvaluationTestCases() error = %v", err)
	}

	if err := reg.DeleteEvaluationTestCase("greeting", 0); err != nil {
		t.Fatalf("DeleteEvaluationTestCase() error = %v", err)
	}

	got, err := reg.GetEvaluation("greeting")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if len(got.TestCases) != 1 || got.TestCases[0].Prompt != "say bye" {
		t.Errorf("GetEvaluation().TestCases = %+v, want only 'say bye' to remain", got.TestCases)
	}
}

func TestDeleteEvaluationTestCaseOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(Evaluation{Name: "greeting"}); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	if err := reg.DeleteEvaluationTestCase("greeting", 0); err == nil {
		t.Error("DeleteEvaluationTestCase() error = nil, want an error for an out-of-range index")
	}
}

func TestPublishEvaluationVersion(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	v, err := reg.PublishEvaluationVersion("greeting")
	if err != nil {
		t.Fatalf("PublishEvaluationVersion() error = %v", err)
	}
	if v.Version != 1 {
		t.Errorf("PublishEvaluationVersion().Version = %d, want 1", v.Version)
	}
	if len(v.TestCases) != 1 || v.TestCases[0].Prompt != "say hello" {
		t.Errorf("PublishEvaluationVersion().TestCases = %+v, want the draft's single test case", v.TestCases)
	}
	if v.PublishedAt.IsZero() {
		t.Error("PublishEvaluationVersion().PublishedAt is zero, want it set")
	}

	got, err := reg.GetEvaluation("greeting")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if got.Version != 1 {
		t.Errorf("GetEvaluation().Version = %d, want 1 after publishing", got.Version)
	}
}

func TestPublishEvaluationVersionRequiresTestCases(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(Evaluation{Name: "empty"}); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	if _, err := reg.PublishEvaluationVersion("empty"); err == nil {
		t.Error("PublishEvaluationVersion() error = nil, want an error for an evaluation with no test cases")
	}
}

func TestPublishEvaluationVersionIsImmutableToLaterDraftEdits(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}
	v1, err := reg.PublishEvaluationVersion("greeting")
	if err != nil {
		t.Fatalf("PublishEvaluationVersion() error = %v", err)
	}

	// Editing the draft afterward must not retroactively change the
	// already-published version's snapshot.
	if err := reg.UpdateEvaluationTestCase("greeting", 0, TestCase{Prompt: "say goodbye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}}); err != nil {
		t.Fatalf("UpdateEvaluationTestCase() error = %v", err)
	}

	got, err := reg.GetEvaluationVersion("greeting", v1.Version)
	if err != nil {
		t.Fatalf("GetEvaluationVersion() error = %v", err)
	}
	if got.TestCases[0].Prompt != "say hello" {
		t.Errorf("GetEvaluationVersion().TestCases[0].Prompt = %q, want unchanged %q despite the later draft edit", got.TestCases[0].Prompt, "say hello")
	}
}

func TestPublishEvaluationVersionIncrementsAcrossMultiplePublishes(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}
	if _, err := reg.PublishEvaluationVersion("greeting"); err != nil {
		t.Fatalf("PublishEvaluationVersion() (1st) error = %v", err)
	}

	if err := reg.AddEvaluationTestCases("greeting", []TestCase{{Prompt: "say bye", Assertions: []Assertion{{Type: "contains", Value: "bye"}}}}); err != nil {
		t.Fatalf("AddEvaluationTestCases() error = %v", err)
	}
	v2, err := reg.PublishEvaluationVersion("greeting")
	if err != nil {
		t.Fatalf("PublishEvaluationVersion() (2nd) error = %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("PublishEvaluationVersion() (2nd).Version = %d, want 2", v2.Version)
	}
	if len(v2.TestCases) != 2 {
		t.Errorf("PublishEvaluationVersion() (2nd).TestCases = %+v, want 2 (both draft test cases)", v2.TestCases)
	}

	versions, err := reg.ListEvaluationVersions("greeting")
	if err != nil {
		t.Fatalf("ListEvaluationVersions() error = %v", err)
	}
	if len(versions) != 2 || versions[0].Version != 1 || versions[1].Version != 2 {
		t.Errorf("ListEvaluationVersions() = %+v, want [v1, v2]", versions)
	}
}

func TestListEvaluationVersionsEmptyForUnpublishedEvaluation(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	versions, err := reg.ListEvaluationVersions("greeting")
	if err != nil {
		t.Fatalf("ListEvaluationVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("ListEvaluationVersions() = %+v, want empty for an evaluation with nothing published", versions)
	}
}

func TestGetEvaluationVersionUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}
	if _, err := reg.PublishEvaluationVersion("greeting"); err != nil {
		t.Fatalf("PublishEvaluationVersion() error = %v", err)
	}

	if _, err := reg.GetEvaluationVersion("greeting", 99); err == nil {
		t.Error("GetEvaluationVersion() error = nil, want an error for an unpublished version number")
	}
}
