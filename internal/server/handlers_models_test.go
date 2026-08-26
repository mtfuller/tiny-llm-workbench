package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

func TestListModels(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-finetune", BaseModel: "mlx-community/base", Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/models status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []modelJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "my-finetune" || got[0].Source != "mlx" || got[0].BaseModel != "mlx-community/base" {
		t.Errorf("GET /api/models body = %+v, want a single my-finetune entry with its base model", got)
	}
}

func TestGetModelIncludesTrainingRun(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-finetune", BaseModel: "mlx-community/base", Source: "mlx"}}}
	deps.Training = &fakeTrainingManager{runs: []*training.Run{
		{ID: "run-1", Config: training.Config{OutputName: "my-finetune", BaseModel: "mlx-community/base"}, Status: training.StatusSucceeded},
		{ID: "run-0", Config: training.Config{OutputName: "other-model"}, Status: training.StatusSucceeded},
	}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/my-finetune", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/models/my-finetune status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got modelDetailJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Name != "my-finetune" || got.BaseModel != "mlx-community/base" {
		t.Errorf("GET /api/models/my-finetune body = %+v, want name/baseModel set", got)
	}
	if got.TrainingRun == nil || got.TrainingRun.ID != "run-1" {
		t.Errorf("GET /api/models/my-finetune TrainingRun = %+v, want the matching run", got.TrainingRun)
	}
}

func TestGetModelNotFound(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{getErr: errors.New("model not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/models/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestChatWithModel(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-finetune", Path: "/tlw/models/my-finetune", Source: "mlx"}}}
	runner := &fakeModelRunner{completion: "hello there"}
	deps.ModelRunner = runner

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(chatWithModelRequest{Messages: []chatMessageJSON{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello!"},
		{Role: "user", Content: "say hi"},
	}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/my-finetune/chat", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/models/my-finetune/chat status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got chatWithModelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Completion != "hello there" {
		t.Errorf("completion = %q, want %q", got.Completion, "hello there")
	}
	if len(runner.calls) != 1 || runner.calls[0].model != "/tlw/models/my-finetune" || len(runner.calls[0].messages) != 3 {
		t.Errorf("runner.calls = %+v, want the model's Path and the full message history passed through", runner.calls)
	}
}

func TestChatWithModelRequiresMessages(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-finetune", Path: "/tlw/models/my-finetune", Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(chatWithModelRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/my-finetune/chat", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/models/my-finetune/chat (no messages) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListModelsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: nil}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	handler.ServeHTTP(rec, req)

	// A bare "null" body breaks frontend code expecting an array
	// (e.g. `.length` on the parsed result), so an empty list must
	// serialize as "[]".
	got := strings.TrimSpace(rec.Body.String())
	if got != "[]" {
		t.Errorf("GET /api/models (empty) body = %q, want %q", got, "[]")
	}
}

func TestDeleteModel(t *testing.T) {
	deps := testDeps()
	models := &fakeModelStore{}
	deps.Models = models

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/my-finetune", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/models/my-finetune status = %d, want %d, body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(models.deleted) != 1 || models.deleted[0] != "my-finetune" {
		t.Errorf("models.deleted = %v, want [my-finetune]", models.deleted)
	}
}

func TestDeleteModelError(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{deleteErr: errors.New("model not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/my-finetune", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("DELETE /api/models/my-finetune (store error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestListModelsStoreError(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{err: errors.New("boom")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET /api/models status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
