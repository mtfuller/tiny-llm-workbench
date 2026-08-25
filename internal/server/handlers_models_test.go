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
