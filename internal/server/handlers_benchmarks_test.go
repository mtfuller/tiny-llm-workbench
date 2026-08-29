package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/benchmarks"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListBenchmarks(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{list: []registry.Benchmark{{Name: "greeting-benchmark"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/benchmarks status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.Benchmark
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "greeting-benchmark" {
		t.Errorf("GET /api/benchmarks body = %+v, want a single greeting-benchmark entry", got)
	}
}

func TestListBenchmarksEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/benchmarks (empty) body = %q, want %q", got, "[]")
	}
}

func TestListBenchmarksNilAssertionsAreJSONArraysNotNull(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{list: []registry.Benchmark{
		{Name: "greeting-benchmark", TestCases: []registry.TestCase{{ID: "tc1", Prompt: "hi"}}},
	}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks", nil)
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"assertions":[]`) {
		t.Errorf("GET /api/benchmarks body = %q, want assertions to serialize as []", rec.Body.String())
	}
}

func TestSaveBenchmark(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveBenchmarkRequest{
		Name:      "greeting-benchmark",
		TestCases: []registry.TestCase{{ID: "tc1", Prompt: "hi", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/benchmarks status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "greeting-benchmark" {
		t.Errorf("store.saved = %+v, want [greeting-benchmark]", store.saved)
	}
}

func TestSaveBenchmarkRequiresName(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveBenchmarkRequest{TestCases: []registry.TestCase{{ID: "tc1", Prompt: "hi"}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/benchmarks (no name) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSaveBenchmarkAllowsNoTestCases(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveBenchmarkRequest{Name: "greeting-benchmark"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/benchmarks (no test cases) status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "greeting-benchmark" {
		t.Errorf("store.saved = %+v, want [greeting-benchmark]", store.saved)
	}
}

func TestGetBenchmark(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{get: registry.Benchmark{Name: "greeting-benchmark"}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/benchmarks/greeting-benchmark status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetBenchmarkNotFound(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{getErr: errors.New("not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/benchmarks/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteBenchmark(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/benchmarks/tiny-eval", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/benchmarks/tiny-eval status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "tiny-eval" {
		t.Errorf("store.deleted = %v, want [tiny-eval]", store.deleted)
	}
}

func TestDeleteBenchmarkNotFound(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{deleteErr: errors.New("benchmark not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/benchmarks/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/benchmarks/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartBenchmarkRun(t *testing.T) {
	deps := testDeps()
	mgr := &fakeBenchmarkManager{startResult: &benchmarks.Run{ID: "benchrun-1", BenchmarkName: "greeting-benchmark"}}
	deps.BenchRuns = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startBenchmarkRunRequest{Version: 1, ModelNames: []string{"tiny"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/benchmarks/greeting-benchmark/runs status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(mgr.started) != 1 || mgr.started[0] != "greeting-benchmark" {
		t.Errorf("mgr.started = %v, want [greeting-benchmark]", mgr.started)
	}
}

func TestStartBenchmarkRunError(t *testing.T) {
	deps := testDeps()
	deps.BenchRuns = &fakeBenchmarkManager{startErr: errors.New("no test cases")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startBenchmarkRunRequest{Version: 1, ModelNames: []string{"tiny"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../runs (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestStartBenchmarkRunRequiresVersion(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startBenchmarkRunRequest{ModelNames: []string{"tiny"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/runs", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../runs (no version) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListBenchmarkRuns(t *testing.T) {
	deps := testDeps()
	deps.BenchRuns = &fakeBenchmarkManager{runs: []*benchmarks.Run{{ID: "benchrun-1"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/runs", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/benchmarks/runs status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []benchmarks.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "benchrun-1" {
		t.Errorf("GET /api/benchmarks/runs body = %+v, want a single benchrun-1 entry", got)
	}
}

func TestListBenchmarkRunsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/runs", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/benchmarks/runs (empty) body = %q, want %q", got, "[]")
	}
}

func TestGetBenchmarkRun(t *testing.T) {
	deps := testDeps()
	deps.BenchRuns = &fakeBenchmarkManager{getResult: &benchmarks.Run{ID: "benchrun-1"}, getOK: true}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/runs/benchrun-1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/benchmarks/runs/benchrun-1 status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetBenchmarkRunNotFound(t *testing.T) {
	deps := testDeps()
	deps.BenchRuns = &fakeBenchmarkManager{getOK: false}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/runs/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/benchmarks/runs/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListBenchmarkResults(t *testing.T) {
	deps := testDeps()
	deps.BenchRuns = &fakeBenchmarkManager{results: []benchmarks.RunResult{{BenchmarkVersion: 1, ModelName: "tiny", Passed: 2, Total: 2}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmark-results/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../results status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []benchmarks.RunResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ModelName != "tiny" {
		t.Errorf("GET .../results body = %+v, want a single tiny entry", got)
	}
}

func TestListBenchmarkResultsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmark-results/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET .../results (empty) body = %q, want %q", got, "[]")
	}
}

func TestListBenchmarkResultsError(t *testing.T) {
	deps := testDeps()
	deps.BenchRuns = &fakeBenchmarkManager{resultsErr: errors.New("read error")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmark-results/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET .../results (error) status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestAddTestCases(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{get: registry.Benchmark{
		Name: "greeting-benchmark",
		TestCases: []registry.TestCase{
			{ID: "tc-1", Prompt: "say hi"},
			{ID: "tc-2", Prompt: "say bye"},
		},
	}}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addTestCasesRequest{TestCases: []registry.TestCase{{Prompt: "say bye"}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases", bytes.NewReader(body))
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

func TestAddTestCasesRequiresAtLeastOne(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addTestCasesRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../test-cases (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateTestCase(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{get: registry.Benchmark{
		Name:      "greeting-benchmark",
		TestCases: []registry.TestCase{{ID: "tc-1", Prompt: "say hello there"}},
	}}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.TestCase{Prompt: "say hello there"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/benchmarks/greeting-benchmark/test-cases/0", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT .../test-cases/0 status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.updatedIndex != 0 || store.updatedTestCase.Prompt != "say hello there" {
		t.Errorf("store.updatedIndex/updatedTestCase = %d/%+v, want 0/say hello there", store.updatedIndex, store.updatedTestCase)
	}
}

func TestUpdateTestCaseInvalidIndex(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.TestCase{Prompt: "say hello"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/benchmarks/greeting-benchmark/test-cases/not-a-number", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../test-cases/not-a-number status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateTestCaseError(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{updateTestCaseErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.TestCase{Prompt: "say hello"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/benchmarks/greeting-benchmark/test-cases/5", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../test-cases/5 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTestCase(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/benchmarks/greeting-benchmark/test-cases/1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE .../test-cases/1 status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if store.deletedIndex != 1 {
		t.Errorf("store.deletedIndex = %d, want 1", store.deletedIndex)
	}
}

func TestApproveTestCase(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/2/approve", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST .../test-cases/2/approve status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.approvedIndexes) != 1 || store.approvedIndexes[0] != 2 {
		t.Errorf("store.approvedIndexes = %v, want [2]", store.approvedIndexes)
	}
}

func TestFlagTestCase(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/1/flag", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST .../test-cases/1/flag status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.flaggedIndexes) != 1 || store.flaggedIndexes[0] != 1 {
		t.Errorf("store.flaggedIndexes = %v, want [1]", store.flaggedIndexes)
	}
}

func TestApproveTestCaseError(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{approveTestCaseErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/9/approve", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../test-cases/9/approve (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTestCaseError(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{deleteTestCaseErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/benchmarks/greeting-benchmark/test-cases/5", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE .../test-cases/5 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateTestCases(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{get: registry.Benchmark{
		Name: "greeting-benchmark",
		TestCases: []registry.TestCase{
			{ID: "tc-1", Prompt: "say hi"},
			{ID: "tc-2", Prompt: "say good morning"},
			{ID: "tc-3", Prompt: "greet someone politely"},
		},
	}}
	deps.Benchmarks = store
	gen := &fakeTestCaseGenerator{prompts: []string{"say good morning", "greet someone politely"}}
	deps.TestCaseGen = gen

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateTestCasesRequest{
		Model:      "tiny-model",
		SeedPrompt: "say hi",
		Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}},
		Count:      2,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/generate", bytes.NewReader(body))
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
	if store.addedTestCases[0][0].Prompt != "say good morning" || len(store.addedTestCases[0][0].Assertions) != 1 {
		t.Errorf("addedTestCases[0][0] = %+v, want prompt 'say good morning' with the seed's assertion", store.addedTestCases[0][0])
	}
	for i, tc := range store.addedTestCases[0] {
		if tc.Source != "ai" {
			t.Errorf("addedTestCases[0][%d].Source = %q, want \"ai\"", i, tc.Source)
		}
	}

	var got []registry.TestCase
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("POST .../test-cases/generate body = %+v, want 2 entries", got)
	}
}

func TestGenerateTestCasesRequiresModel(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateTestCasesRequest{SeedPrompt: "say hi", Count: 2})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../generate (no model) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateTestCasesRequiresSeedPrompt(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateTestCasesRequest{Model: "tiny-model", Count: 2})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../generate (no seed prompt) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateTestCasesRequiresPositiveCount(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateTestCasesRequest{Model: "tiny-model", SeedPrompt: "say hi", Count: 0})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../generate (count=0) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateTestCasesGeneratorError(t *testing.T) {
	deps := testDeps()
	deps.TestCaseGen = &fakeTestCaseGenerator{err: errors.New("mlx_lm.server unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(generateTestCasesRequest{Model: "tiny-model", SeedPrompt: "say hi", Count: 2})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/test-cases/generate", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../generate (generator error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestPublishBenchmarkVersion(t *testing.T) {
	deps := testDeps()
	store := &fakeBenchmarkStore{publishResult: registry.BenchmarkVersion{Version: 1}}
	deps.Benchmarks = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/versions", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../versions status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.published) != 1 || store.published[0] != "greeting-benchmark" {
		t.Errorf("store.published = %v, want [greeting-benchmark]", store.published)
	}

	var got registry.BenchmarkVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("POST .../versions body = %+v, want Version 1", got)
	}
}

func TestPublishBenchmarkVersionError(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{publishErr: errors.New("no test cases to publish")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/benchmarks/greeting-benchmark/versions", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../versions (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListBenchmarkVersions(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{versions: []registry.BenchmarkVersion{{Version: 1}, {Version: 2}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmark-versions/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/benchmark-versions/greeting-benchmark status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []registry.BenchmarkVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GET /api/benchmark-versions/greeting-benchmark body = %+v, want 2 entries", got)
	}
}

func TestListBenchmarkVersionsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmark-versions/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/benchmark-versions/greeting-benchmark (empty) body = %q, want %q", got, "[]")
	}
}

func TestListBenchmarkVersionsError(t *testing.T) {
	deps := testDeps()
	deps.Benchmarks = &fakeBenchmarkStore{versionsErr: errors.New("read error")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/benchmark-versions/greeting-benchmark", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET /api/benchmark-versions/greeting-benchmark (error) status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
