package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mtfuller/tiny-llm-workbench/internal/benchmarks"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeBenchmark ensures TestCases (and each test case's Assertions)
// never serialize as JSON "null", same reasoning as normalizeEvaluation.
func normalizeBenchmark(b registry.Benchmark) registry.Benchmark {
	if b.TestCases == nil {
		b.TestCases = []registry.TestCase{}
		return b
	}
	for i, tc := range b.TestCases {
		if tc.Assertions == nil {
			b.TestCases[i].Assertions = []registry.Assertion{}
		}
	}
	return b
}

// listBenchmarksHandler responds with every saved benchmark definition.
func listBenchmarksHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListBenchmarks()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		normalized := make([]registry.Benchmark, len(list))
		for i, b := range list {
			normalized[i] = normalizeBenchmark(b)
		}
		writeJSON(w, http.StatusOK, normalized)
	}
}

// saveBenchmarkRequest is the POST /api/benchmarks request body.
type saveBenchmarkRequest struct {
	Name      string              `json:"name"`
	TestCases []registry.TestCase `json:"testCases"`
}

// saveBenchmarkHandler creates or overwrites a benchmark's definition. A
// benchmark can be created with no test cases at all — they're added
// afterward from its detail page (one at a time, edited in place, or
// generated), the same way a Dataset starts empty and gets examples added
// to it.
func saveBenchmarkHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req saveBenchmarkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		bm := registry.Benchmark{Name: req.Name, TestCases: req.TestCases}
		if err := store.SaveBenchmark(bm); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeBenchmark(bm))
	}
}

// getBenchmarkHandler responds with a single benchmark's definition.
func getBenchmarkHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bm, err := store.GetBenchmark(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeBenchmark(bm))
	}
}

// deleteBenchmarkHandler removes a benchmark's saved definition.
func deleteBenchmarkHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteBenchmark(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// startBenchmarkRunRequest is the POST /api/benchmarks/{name}/runs request
// body. Version must name an already-published version (see
// publishBenchmarkVersionHandler) — a run can never target the benchmark's
// live draft test cases.
type startBenchmarkRunRequest struct {
	Version    int      `json:"version"`
	ModelNames []string `json:"modelNames"`
}

// startBenchmarkRunHandler starts a new benchmark run in the background and
// responds immediately with its initial ("running") state.
func startBenchmarkRunHandler(mgr benchmarkManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req startBenchmarkRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Version <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("version is required"))
			return
		}

		run, err := mgr.StartRun(name, req.Version, req.ModelNames)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, run)
	}
}

// listBenchmarkRunsHandler responds with every known benchmark run, most
// recently started first.
func listBenchmarkRunsHandler(mgr benchmarkManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs := mgr.ListRuns()
		if runs == nil {
			runs = []*benchmarks.Run{}
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

// getBenchmarkRunHandler responds with a single run's current state.
func getBenchmarkRunHandler(mgr benchmarkManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, ok := mgr.GetRun(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such benchmark run"))
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// publishBenchmarkVersionHandler snapshots the benchmark's current draft
// test cases into a new, immutable BenchmarkVersion, advancing the
// benchmark's Version to it. This is the only way Version ever changes —
// adding/editing/deleting draft test cases never does.
func publishBenchmarkVersionHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := store.PublishVersion(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	}
}

// listBenchmarkVersionsHandler responds with every published version of a
// benchmark, oldest first — used to populate the run modal's version picker.
func listBenchmarkVersionsHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := store.ListVersions(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if versions == nil {
			versions = []registry.BenchmarkVersion{}
		}
		writeJSON(w, http.StatusOK, versions)
	}
}

// listBenchmarkResultsHandler responds with every persisted RunResult for
// one benchmark — the durable, model-to-model comparison the benchmark
// detail page's "run results" view renders, as opposed to
// listBenchmarkRunsHandler's ephemeral in-progress/recent Run tracking.
func listBenchmarkResultsHandler(mgr benchmarkManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, err := mgr.ListResults(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if results == nil {
			results = []benchmarks.RunResult{}
		}
		writeJSON(w, http.StatusOK, results)
	}
}

// addTestCasesRequest is the POST /api/benchmarks/{name}/test-cases request
// body.
type addTestCasesRequest struct {
	TestCases []registry.TestCase `json:"testCases"`
}

// addTestCasesHandler appends one or more manually-entered (or generated)
// test cases to a benchmark, mirroring addExamplesHandler for datasets.
func addTestCasesHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req addTestCasesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if len(req.TestCases) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("at least one test case is required"))
			return
		}

		if err := store.AddTestCases(name, req.TestCases); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		bm, err := store.GetBenchmark(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		added := bm.TestCases[len(bm.TestCases)-len(req.TestCases):]
		writeJSON(w, http.StatusCreated, added)
	}
}

// updateTestCaseHandler overwrites a single test case, addressed by its
// position in the benchmark (as returned by GET /api/benchmarks/{name}).
func updateTestCaseHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid test case index: %w", err))
			return
		}

		var tc registry.TestCase
		if err := json.NewDecoder(r.Body).Decode(&tc); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		if err := store.UpdateTestCase(name, index, tc); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		bm, err := store.GetBenchmark(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, bm.TestCases[index])
	}
}

// approveTestCaseHandler marks a single draft test case as human-reviewed,
// addressed by its position in the benchmark.
func approveTestCaseHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid test case index: %w", err))
			return
		}

		if err := store.ApproveTestCase(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// flagTestCaseHandler marks a single draft test case as needing another
// human review, addressed by its position in the benchmark.
func flagTestCaseHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid test case index: %w", err))
			return
		}

		if err := store.FlagTestCaseForReview(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteTestCaseHandler removes a single test case, addressed by its
// position in the benchmark (as returned by GET /api/benchmarks/{name}).
func deleteTestCaseHandler(store benchmarkStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid test case index: %w", err))
			return
		}

		if err := store.DeleteTestCase(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// generateTestCasesRequest is the POST
// /api/benchmarks/{name}/test-cases/generate request body.
type generateTestCasesRequest struct {
	Model      string               `json:"model"`
	SeedPrompt string               `json:"seedPrompt"`
	Assertions []registry.Assertion `json:"assertions"`
	Tags       []string             `json:"tags,omitempty"`
	Count      int                  `json:"count"`
}

// generateTestCasesHandler asks a local LLM for prompt variations on a seed
// test case and appends the results (each paired with the seed's own
// assertions/tags, unchanged) to the named benchmark.
func generateTestCasesHandler(store benchmarkStore, generator testCaseGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req generateTestCasesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Model == "" {
			writeError(w, http.StatusBadRequest, errors.New("model is required"))
			return
		}
		if req.SeedPrompt == "" {
			writeError(w, http.StatusBadRequest, errors.New("seed prompt is required"))
			return
		}
		if req.Count <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("count must be positive"))
			return
		}

		prompts, err := generator.Variations(r.Context(), req.Model, req.SeedPrompt, req.Count)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		newTestCases := make([]registry.TestCase, len(prompts))
		for i, p := range prompts {
			// Flag every generated case as unreviewed AI so the UI warns
			// against publishing a version before a human has checked it.
			newTestCases[i] = registry.TestCase{Prompt: p, Assertions: req.Assertions, Tags: req.Tags, Source: "ai"}
		}

		if err := store.AddTestCases(name, newTestCases); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		bm, err := store.GetBenchmark(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		added := bm.TestCases[len(bm.TestCases)-len(newTestCases):]
		writeJSON(w, http.StatusCreated, added)
	}
}
