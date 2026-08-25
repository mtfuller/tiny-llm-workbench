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
)

// Empty lists must serialize as JSON "[]", not "null": a bare null breaks
// frontend code that calls `.length` on the parsed response body.

func TestListDatasetsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps() // fakeDatasetStore.ListDatasets() returns a nil slice by default

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/datasets (empty) body = %q, want %q", got, "[]")
	}
}

func TestGetDatasetEmptyExamplesIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore() // examples["greetings"] left unset -> nil slice
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/greetings", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/datasets/greetings status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"examples":[]`) {
		t.Errorf("GET /api/datasets/greetings body = %q, want examples to serialize as []", rec.Body.String())
	}
}

func TestListDatasets(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.datasets = []registry.DatasetSummary{{Dataset: registry.Dataset{Name: "greetings"}, PairCount: 3}}
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/datasets status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.DatasetSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "greetings" || got[0].PairCount != 3 {
		t.Errorf("GET /api/datasets body = %+v, want a single greetings/3 entry", got)
	}
}

func TestCreateDataset(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createDatasetRequest{Name: "greetings"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/datasets status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.created) != 1 || store.created[0] != "greetings" {
		t.Errorf("store.created = %v, want [greetings]", store.created)
	}
}

func TestCreateDatasetRequiresName(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createDatasetRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/datasets (no name) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetDataset(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.examples["greetings"] = []registry.Example{{Input: "hi", Output: "hello!"}}
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/greetings", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/datasets/greetings status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got datasetDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Name != "greetings" || len(got.Examples) != 1 || got.Examples[0].Input != "hi" {
		t.Errorf("GET /api/datasets/greetings body = %+v, want name=greetings with 1 example", got)
	}
}

func TestGetDatasetNotFound(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.examplesErrs = map[string]error{"missing": errors.New("not found")}
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/datasets/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGenerateVariations(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store
	deps.Generator = &fakeGenerator{result: []registry.Example{{Input: "yo", Output: "hey!"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateVariationsRequest{
		Model: "qwen2.5:0.5b",
		Seed:  registry.Example{Input: "hi", Output: "hello!"},
		Count: 1,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/variations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/datasets/greetings/variations status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.appended["greetings"]) != 1 || store.appended["greetings"][0].Input != "yo" {
		t.Errorf("store.appended[greetings] = %+v, want the generated example", store.appended["greetings"])
	}
}

func TestGenerateVariationsRequiresModel(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateVariationsRequest{Count: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/variations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../variations (no model) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateVariationsGeneratorError(t *testing.T) {
	deps := testDeps()
	deps.Generator = &fakeGenerator{err: errors.New("ollama unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateVariationsRequest{Model: "qwen2.5:0.5b", Count: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/variations", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../variations (generator error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}
