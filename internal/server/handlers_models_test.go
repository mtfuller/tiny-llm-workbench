package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/models"
)

func TestListModels(t *testing.T) {
	deps := testDeps()
	deps.Catalog = &fakeCatalog{list: []models.Model{{Name: "qwen2.5:0.5b", Source: "ollama", Size: 397_000_000}}}

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

	var got []models.Model
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "qwen2.5:0.5b" {
		t.Errorf("GET /api/models body = %+v, want a single qwen2.5:0.5b entry", got)
	}
}

func TestListModelsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	deps.Catalog = &fakeCatalog{list: nil}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	handler.ServeHTTP(rec, req)

	// A bare "null" body breaks frontend code expecting an array
	// (e.g. `.length` on the parsed result), so an empty catalog must
	// serialize as "[]".
	got := strings.TrimSpace(rec.Body.String())
	if got != "[]" {
		t.Errorf("GET /api/models (empty) body = %q, want %q", got, "[]")
	}
}

func TestDeleteModel(t *testing.T) {
	deps := testDeps()
	catalog := &fakeCatalog{}
	deps.Catalog = catalog

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/qwen2.5:0.5b?source=ollama", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/models/qwen2.5:0.5b status = %d, want %d, body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(catalog.deleted) != 1 || catalog.deleted[0] != "qwen2.5:0.5b/ollama" {
		t.Errorf("catalog.deleted = %v, want [qwen2.5:0.5b/ollama]", catalog.deleted)
	}
}

func TestDeleteModelRequiresSource(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/qwen2.5:0.5b", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE /api/models/qwen2.5:0.5b (no source) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteModelCatalogError(t *testing.T) {
	deps := testDeps()
	deps.Catalog = &fakeCatalog{deleteErr: errors.New("ollama unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/qwen2.5:0.5b?source=ollama", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("DELETE /api/models/qwen2.5:0.5b (catalog error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestListModelsCatalogError(t *testing.T) {
	deps := testDeps()
	deps.Catalog = &fakeCatalog{err: errors.New("boom")}

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
