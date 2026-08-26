package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

func TestStartTrainingRun(t *testing.T) {
	deps := testDeps()
	mgr := &fakeTrainingManager{}
	deps.Training = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cfg := training.Config{BaseModel: "mlx-community/test", Dataset: "greetings", OutputName: "my-finetune", Iterations: 100}
	body, _ := json.Marshal(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/training/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/training/runs status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(mgr.started) != 1 || mgr.started[0].OutputName != "my-finetune" {
		t.Errorf("mgr.started = %+v, want the posted config", mgr.started)
	}

	var run training.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if run.Status != training.StatusRunning {
		t.Errorf("run.Status = %q, want %q", run.Status, training.StatusRunning)
	}
}

func TestStartTrainingRunValidationError(t *testing.T) {
	deps := testDeps()
	deps.Training = &fakeTrainingManager{startErr: errors.New("dataset is required")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(training.Config{BaseModel: "m"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/training/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/training/runs status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListTrainingRuns(t *testing.T) {
	deps := testDeps()
	deps.Training = &fakeTrainingManager{runs: []*training.Run{{ID: "run-1", Status: training.StatusSucceeded}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/training/runs", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/training/runs status = %d, want %d", rec.Code, http.StatusOK)
	}

	var runs []training.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "run-1" {
		t.Errorf("runs = %+v, want a single run-1 entry", runs)
	}
}

func TestListTrainingRunsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	deps.Training = &fakeTrainingManager{}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/training/runs", nil)
	handler.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("GET /api/training/runs (empty) body = %q, want %q", got, "[]\n")
	}
}

func TestCancelTrainingRun(t *testing.T) {
	deps := testDeps()
	mgr := &fakeTrainingManager{}
	deps.Training = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/training/runs/run-1/cancel", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/training/runs/run-1/cancel status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(mgr.cancelledRuns) != 1 || mgr.cancelledRuns[0] != "run-1" {
		t.Errorf("mgr.cancelledRuns = %v, want [run-1]", mgr.cancelledRuns)
	}
}

func TestCancelTrainingRunNotFound(t *testing.T) {
	deps := testDeps()
	deps.Training = &fakeTrainingManager{cancelErr: errors.New("no such run")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/training/runs/missing/cancel", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /api/training/runs/missing/cancel status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetTrainingRun(t *testing.T) {
	deps := testDeps()
	deps.Training = &fakeTrainingManager{run: &training.Run{ID: "run-1", Status: training.StatusRunning}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/training/runs/run-1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/training/runs/run-1 status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetTrainingRunNotFound(t *testing.T) {
	deps := testDeps()
	deps.Training = &fakeTrainingManager{}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/training/runs/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/training/runs/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
