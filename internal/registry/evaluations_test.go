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
	want.Environment = "WebSearch"
	if err := reg.SaveEvaluation(want); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
	}

	got, err := reg.GetEvaluation("greeting-eval")
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if got.Name != want.Name || got.Environment != "WebSearch" || len(got.TestCases) != 1 {
		t.Errorf("GetEvaluation() = %+v, want %+v", got, want)
	}
	if len(got.TestCases[0].Assertions) != 1 || got.TestCases[0].Assertions[0].Value != "hello" {
		t.Errorf("GetEvaluation().TestCases[0].Assertions = %+v, want a single 'hello' assertion", got.TestCases[0].Assertions)
	}
}

func TestSaveEvaluationOverwrites(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEvaluation(testEvaluation("greeting-eval")); err != nil {
		t.Fatalf("SaveEvaluation() error = %v", err)
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
