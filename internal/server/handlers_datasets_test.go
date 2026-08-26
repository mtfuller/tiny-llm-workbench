package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestCreateDatasetWithTitleAndDescription(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createDatasetRequest{Name: "greetings", Title: "Greetings", Description: "Casual hello/goodbye pairs"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/datasets status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got registry.Dataset
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Title != "Greetings" || got.Description != "Casual hello/goodbye pairs" {
		t.Errorf("POST /api/datasets body = %+v, want Title=Greetings Description set", got)
	}

	// GET should also surface the metadata alongside the examples.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/datasets/greetings", nil)
	handler.ServeHTTP(rec, req)

	var detail datasetDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if detail.Title != "Greetings" || detail.Description != "Casual hello/goodbye pairs" {
		t.Errorf("GET /api/datasets/greetings body = %+v, want Title=Greetings Description set", detail)
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

func TestDeleteDataset(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/datasets/greetings", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/datasets/greetings status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "greetings" {
		t.Errorf("store.deleted = %v, want [greetings]", store.deleted)
	}
}

func TestDeleteDatasetNotFound(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.deleteErr = errors.New("dataset not found")
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/datasets/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/datasets/missing status = %d, want %d", rec.Code, http.StatusNotFound)
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
	deps.Generator = &fakeGenerator{err: errors.New("model runner unreachable")}

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

func TestAddExamples(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addExamplesRequest{Examples: []registry.Example{{Input: "hi", Output: "hello!"}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/examples", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../examples status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.appended["greetings"]) != 1 || store.appended["greetings"][0].Input != "hi" {
		t.Errorf("store.appended[greetings] = %+v, want the posted example", store.appended["greetings"])
	}
}

func TestAddExamplesRequiresAtLeastOne(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addExamplesRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/examples", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../examples (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateExample(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Example{Input: "hi", Output: "hey!"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/datasets/greetings/examples/1", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT .../examples/1 status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, ok := store.updatedExamples[1]; !ok || got.Output != "hey!" {
		t.Errorf("store.updatedExamples[1] = %+v, want the posted example", store.updatedExamples[1])
	}
}

func TestUpdateExampleInvalidIndex(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Example{Input: "hi", Output: "hey!"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/datasets/greetings/examples/not-a-number", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../examples/not-a-number status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateExampleOutOfRange(t *testing.T) {
	deps := testDeps()
	deps.Datasets = &fakeDatasetStore{updateExampleErr: errors.New("example index 5 out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Example{Input: "hi", Output: "hey!"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/datasets/greetings/examples/5", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../examples/5 (out of range) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteExample(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/datasets/greetings/examples/0", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE .../examples/0 status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deletedExamples) != 1 || store.deletedExamples[0] != 0 {
		t.Errorf("store.deletedExamples = %v, want [0]", store.deletedExamples)
	}
}

func TestDeleteExampleOutOfRange(t *testing.T) {
	deps := testDeps()
	deps.Datasets = &fakeDatasetStore{deleteExampleErr: errors.New("example index 5 out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/datasets/greetings/examples/5", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE .../examples/5 (out of range) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestExportDatasetJSONL(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.examples["greetings"] = []registry.Example{{Input: "hi", Output: "hello!"}}
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/greetings/export", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../export status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"input":"hi","output":"hello!"}` {
		t.Errorf("GET .../export body = %q, want a single JSONL line", got)
	}
	if disp := rec.Header().Get("Content-Disposition"); !strings.Contains(disp, "greetings.jsonl") {
		t.Errorf("Content-Disposition = %q, want it to name greetings.jsonl", disp)
	}
}

func TestExportDatasetCSV(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.examples["greetings"] = []registry.Example{{Input: "hi", Output: "hello!"}}
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/greetings/export?format=csv", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../export?format=csv status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := "input,output,description,tags\nhi,hello!,,\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("GET .../export?format=csv body = %q, want %q", got, want)
	}
}

func TestExportDatasetCSVIncludesDescriptionAndTags(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	store.examples["greetings"] = []registry.Example{
		{Input: "hi", Output: "hello!", Description: "a greeting", Tags: []string{"casual", "greeting"}},
	}
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/greetings/export?format=csv", nil)
	handler.ServeHTTP(rec, req)

	want := "input,output,description,tags\nhi,hello!,a greeting,casual;greeting\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("GET .../export?format=csv body = %q, want %q", got, want)
	}
}

func TestExportDatasetUnsupportedFormat(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/datasets/greetings/export?format=xml", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET .../export?format=xml status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestImportDatasetJSONL(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(importDatasetRequest{
		Format:  "jsonl",
		Content: `{"input":"hi","output":"hello!"}` + "\n" + `{"input":"hey","output":"hey there!"}`,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/import", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../import (jsonl) status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.appended["greetings"]) != 2 {
		t.Errorf("store.appended[greetings] = %+v, want 2 examples", store.appended["greetings"])
	}
}

func TestImportDatasetCSV(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(importDatasetRequest{
		Format:  "csv",
		Content: "input,output\nhi,hello!\n\"hey, there\",\"hey, yourself!\"\n",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/import", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../import (csv) status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.appended["greetings"]) != 2 || store.appended["greetings"][1].Input != "hey, there" {
		t.Errorf("store.appended[greetings] = %+v, want 2 examples with quoted commas preserved", store.appended["greetings"])
	}
}

func TestImportDatasetCSVWithDescriptionAndTags(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(importDatasetRequest{
		Format:  "csv",
		Content: "input,output,description,tags\nhi,hello!,a greeting,casual;greeting\n",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/import", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../import (csv) status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	got := store.appended["greetings"]
	if len(got) != 1 || got[0].Description != "a greeting" || !reflect.DeepEqual(got[0].Tags, []string{"casual", "greeting"}) {
		t.Errorf("store.appended[greetings] = %+v, want description %q and tags [casual greeting]", got, "a greeting")
	}
}

func TestImportDatasetCSVColumnOrderIndependent(t *testing.T) {
	deps := testDeps()
	store := newFakeDatasetStore()
	deps.Datasets = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(importDatasetRequest{
		Format:  "csv",
		Content: "tags,output,input\ncasual,hello!,hi\n",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/import", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../import (csv) status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	got := store.appended["greetings"]
	if len(got) != 1 || got[0].Input != "hi" || got[0].Output != "hello!" || !reflect.DeepEqual(got[0].Tags, []string{"casual"}) {
		t.Errorf("store.appended[greetings] = %+v, want input=hi output=hello! tags=[casual] regardless of column order", got)
	}
}

func TestImportDatasetCSVBadHeader(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(importDatasetRequest{Format: "csv", Content: "foo,bar\n1,2\n"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/import", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../import (bad header) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestImportDatasetEmpty(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(importDatasetRequest{Format: "jsonl", Content: ""})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/greetings/import", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../import (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
