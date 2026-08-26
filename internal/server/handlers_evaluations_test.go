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

func TestSaveEvaluation(t *testing.T) {
	deps := testDeps()
	store := &fakeEvaluationStore{}
	deps.Evaluations = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveEvaluationRequest{
		Name:      "greeting-eval",
		TestCases: []registry.TestCase{{ID: "tc1", Prompt: "hi", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}}},
	})
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

	body, _ := json.Marshal(saveEvaluationRequest{TestCases: []registry.TestCase{{ID: "tc1", Prompt: "hi"}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/evaluations (no name) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSaveEvaluationRequiresTestCases(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveEvaluationRequest{Name: "greeting-eval"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/evaluations (no test cases) status = %d, want %d", rec.Code, http.StatusBadRequest)
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
	req := httptest.NewRequest(http.MethodDelete, "/api/evaluations/greeter-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/evaluations/greeter-eval status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "greeter-eval" {
		t.Errorf("store.deleted = %v, want [greeter-eval]", store.deleted)
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

	body, _ := json.Marshal(startEvaluationRunRequest{AgentNames: []string{"greeter"}})
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

func TestStartEvaluationRunError(t *testing.T) {
	deps := testDeps()
	deps.EvalRuns = &fakeEvaluationManager{startErr: errors.New("no test cases")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startEvaluationRunRequest{AgentNames: []string{"greeter"}})
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
