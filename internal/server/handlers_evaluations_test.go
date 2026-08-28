package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListEvaluations(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{list: []registry.Evaluation{{Name: "greeting-eval"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/evaluations status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.Evaluation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "greeting-eval" {
		t.Errorf("GET /api/evaluations body = %+v, want a single greeting-eval entry", got)
	}
}

func TestListEvaluationsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/evaluations (empty) body = %q, want %q", got, "[]")
	}
}

func TestListEvaluationsNilAssertionsAreJSONArraysNotNull(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{list: []registry.Evaluation{
		{Name: "greeting-eval", TestCases: []registry.TestCase{{ID: "tc1", Prompt: "hi"}}},
	}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations", nil)
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"assertions":[]`) {
		t.Errorf("GET /api/evaluations body = %q, want assertions to serialize as []", rec.Body.String())
	}
}

func TestListEvaluationsNilVerifyStepAssertionsAreJSONArraysNotNull(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{list: []registry.Evaluation{
		{Name: "coding-eval", TestCases: []registry.TestCase{
			{ID: "tc1", Prompt: "write a file", VerifyCommands: []registry.VerifyStep{{Command: "cat /repo/out.txt"}}},
		}},
	}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations", nil)
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"command":"cat /repo/out.txt","assertions":[]`) {
		t.Errorf("GET /api/evaluations body = %q, want the verify step's assertions to serialize as []", body)
	}
}

func TestSaveEvaluation(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveEvaluationRequest{Name: "greeting-eval"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/evaluations status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "greeting-eval" {
		t.Errorf("store.saved = %+v, want [greeting-eval]", store.saved)
	}
}

func TestSaveEvaluationRequiresName(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveEvaluationRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/evaluations (no name) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetEvaluation(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{get: registry.Evaluation{Name: "greeting-eval"}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/evaluations/greeting-eval status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetEvaluationNotFound(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{getErr: errors.New("not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/evaluations/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteEvaluation(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/evaluations/tiny-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/evaluations/tiny-eval status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "tiny-eval" {
		t.Errorf("store.deleted = %v, want [tiny-eval]", store.deleted)
	}
}

func TestDeleteEvaluationNotFound(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{deleteErr: errors.New("evaluation not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/evaluations/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/evaluations/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartEvaluationRun(t *testing.T) {
	deps := testDeps()
	mgr := &fakeEvaluationManager{startResult: &evaluations.Run{ID: "evalrun-1", EvaluationName: "greeting-eval"}}
	deps.EvalRuns = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startEvaluationRunRequest{Version: 1, AgentNames: []string{"greeter"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/evaluations/greeting-eval/runs status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(mgr.started) != 1 || mgr.started[0] != "greeting-eval" {
		t.Errorf("mgr.started = %v, want [greeting-eval]", mgr.started)
	}
}

func TestStartEvaluationRunRequiresVersion(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startEvaluationRunRequest{AgentNames: []string{"greeter"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../runs (no version) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestStartEvaluationRunError(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{startErr: errors.New("no test cases")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startEvaluationRunRequest{Version: 1, AgentNames: []string{"greeter"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../runs (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListEvaluationRuns(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{runs: []*evaluations.Run{{ID: "evalrun-1"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations/runs", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/evaluations/runs status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []evaluations.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "evalrun-1" {
		t.Errorf("GET /api/evaluations/runs body = %+v, want a single evalrun-1 entry", got)
	}
}

func TestListEvaluationRunsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations/runs", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/evaluations/runs (empty) body = %q, want %q", got, "[]")
	}
}

func TestGetEvaluationRun(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{getResult: &evaluations.Run{ID: "evalrun-1"}, getOK: true}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations/runs/evalrun-1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/evaluations/runs/evalrun-1 status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetEvaluationRunNotFound(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{getOK: false}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluations/runs/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/evaluations/runs/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListEvaluationResults(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{results: []evaluations.RunResult{{EvaluationVersion: 1, AgentName: "greeter", Passed: 2, Total: 2}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluation-results/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../results status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []evaluations.RunResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].AgentName != "greeter" {
		t.Errorf("GET .../results body = %+v, want a single greeter entry", got)
	}
}

func TestListEvaluationResultsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluation-results/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET .../results (empty) body = %q, want %q", got, "[]")
	}
}

func TestListEvaluationResultsError(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{resultsErr: errors.New("read error")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluation-results/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET .../results (error) status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestAddEvaluationTestCases(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{get: registry.Evaluation{
		Name: "greeting-eval",
		TestCases: []registry.TestCase{
			{ID: "tc-1", Prompt: "say hi"},
			{ID: "tc-2", Prompt: "say bye"},
		},
	}}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addEvaluationTestCasesRequest{TestCases: []registry.TestCase{{Prompt: "say bye"}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/test-cases", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../test-cases status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.addedTestCases) != 1 || len(store.addedTestCases[0]) != 1 || store.addedTestCases[0][0].Prompt != "say bye" {
		t.Errorf("store.addedTestCases = %+v, want a single [say bye] batch", store.addedTestCases)
	}

	var got []registry.TestCase
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "tc-2" {
		t.Errorf("POST .../test-cases body = %+v, want the newly-added tc-2 entry", got)
	}
}

func TestAddEvaluationTestCasesRequiresAtLeastOne(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addEvaluationTestCasesRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/test-cases", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../test-cases (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateEvaluationTestCase(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{get: registry.Evaluation{
		Name:      "greeting-eval",
		TestCases: []registry.TestCase{{ID: "tc-1", Prompt: "say hello there"}},
	}}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.TestCase{Prompt: "say hello there"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/evaluations/greeting-eval/test-cases/0", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT .../test-cases/0 status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.updatedIndex != 0 || store.updatedTestCase.Prompt != "say hello there" {
		t.Errorf("store.updatedIndex/updatedTestCase = %d/%+v, want 0/say hello there", store.updatedIndex, store.updatedTestCase)
	}
}

func TestUpdateEvaluationTestCaseInvalidIndex(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.TestCase{Prompt: "say hello"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/evaluations/greeting-eval/test-cases/not-a-number", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../test-cases/not-a-number status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateEvaluationTestCaseError(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{updateTestCaseErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.TestCase{Prompt: "say hello"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/evaluations/greeting-eval/test-cases/5", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../test-cases/5 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteEvaluationTestCase(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/evaluations/greeting-eval/test-cases/1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE .../test-cases/1 status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if store.deletedIndex != 1 {
		t.Errorf("store.deletedIndex = %d, want 1", store.deletedIndex)
	}
}

func TestDeleteEvaluationTestCaseError(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{deleteTestCaseErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/evaluations/greeting-eval/test-cases/5", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE .../test-cases/5 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateEvaluationTestCases(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{get: registry.Evaluation{
		Name: "greeting-eval",
		TestCases: []registry.TestCase{
			{ID: "tc-1", Prompt: "say hi"},
			{ID: "tc-2", Prompt: "say good morning"},
			{ID: "tc-3", Prompt: "greet someone politely"},
		},
	}}
	deps.Evaluations = store
	gen := &fakeTestCaseGenerator{prompts: []string{"say good morning", "greet someone politely"}}
	deps.TestCaseGen = gen

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateEvaluationTestCasesRequest{
		Model:      "tiny-model",
		SeedPrompt: "say hi",
		Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}},
		Count:      2,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../test-cases/generate status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if gen.gotModel != "tiny-model" || gen.gotSeed != "say hi" || gen.gotN != 2 {
		t.Errorf("generator called with (%q, %q, %d), want (tiny-model, say hi, 2)", gen.gotModel, gen.gotSeed, gen.gotN)
	}
	if len(store.addedTestCases) != 1 || len(store.addedTestCases[0]) != 2 {
		t.Fatalf("store.addedTestCases = %+v, want a single batch of 2", store.addedTestCases)
	}

	var got []registry.TestCase
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("POST .../test-cases/generate body = %+v, want 2 entries", got)
	}
}

func TestGenerateEvaluationTestCasesRequiresModel(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateEvaluationTestCasesRequest{SeedPrompt: "say hi", Count: 2})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../generate (no model) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateEvaluationTestCasesGeneratorError(t *testing.T) {
	deps := testDeps()
	deps.TestCaseGen = &fakeTestCaseGenerator{err: errors.New("mlx_lm.server unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateEvaluationTestCasesRequest{Model: "tiny-model", SeedPrompt: "say hi", Count: 2})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../generate (generator error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestPublishEvaluationVersion(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{publishResult: registry.EvaluationVersion{Version: 1}}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/versions", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../versions status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.published) != 1 || store.published[0] != "greeting-eval" {
		t.Errorf("store.published = %v, want [greeting-eval]", store.published)
	}

	var got registry.EvaluationVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("POST .../versions body = %+v, want Version 1", got)
	}
}

func TestPublishEvaluationVersionError(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{publishErr: errors.New("no test cases to publish")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations/greeting-eval/versions", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../versions (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListEvaluationVersions(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{versions: []registry.EvaluationVersion{{Version: 1}, {Version: 2}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluation-versions/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/evaluation-versions/greeting-eval status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []registry.EvaluationVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GET /api/evaluation-versions/greeting-eval body = %+v, want 2 entries", got)
	}
}

func TestListEvaluationVersionsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluation-versions/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/evaluation-versions/greeting-eval (empty) body = %q, want %q", got, "[]")
	}
}

func TestListEvaluationVersionsError(t *testing.T) {
	deps := testDeps()
	deps.Evaluations = &fakeEvaluationStore{versionsErr: errors.New("read error")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/evaluation-versions/greeting-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET /api/evaluation-versions/greeting-eval (error) status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
