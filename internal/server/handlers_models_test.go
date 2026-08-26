package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListModels(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-finetune", Source: "mlx"}}}

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
	if len(got) != 1 || got[0].Name != "my-finetune" || got[0].Source != "mlx" {
		t.Errorf("GET /api/models body = %+v, want a single my-finetune entry", got)
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
