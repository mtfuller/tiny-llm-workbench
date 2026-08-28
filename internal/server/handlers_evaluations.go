package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeEvaluation ensures TestCases (and each test case's Assertions/
// VerifyCommands, and each verify step's own Assertions) never serialize as
// JSON "null".
func normalizeEvaluation(e registry.Evaluation) registry.Evaluation {
	if e.TestCases == nil {
		e.TestCases = []registry.TestCase{}
		return e
	}
	for i := range e.TestCases {
		e.TestCases[i] = normalizeTestCase(e.TestCases[i])
	}
	return e
}

func normalizeTestCase(tc registry.TestCase) registry.TestCase {
	if tc.Assertions == nil {
		tc.Assertions = []registry.Assertion{}
	}
	if tc.VerifyCommands == nil {
		tc.VerifyCommands = []registry.VerifyStep{}
	}
	for i, vs := range tc.VerifyCommands {
		if vs.Assertions == nil {
			tc.VerifyCommands[i].Assertions = []registry.Assertion{}
		}
	}
	return tc
}

// listEvaluationsHandler responds with every saved evaluation definition.
func listEvaluationsHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListEvaluations()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		normalized := make([]registry.Evaluation, len(list))
		for i, e := range list {
			normalized[i] = normalizeEvaluation(e)
		}
		writeJSON(w, http.StatusOK, normalized)
	}
}

// saveEvaluationRequest is the POST /api/evaluations request body.
type saveEvaluationRequest struct {
	Name string `json:"name"`
}

// saveEvaluationHandler creates a new evaluation with no test cases at
// all — they're added afterward from its detail page (one at a time,
// edited in place, or generated), the same way a Benchmark starts empty.
func saveEvaluationHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req saveEvaluationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		eval := registry.Evaluation{Name: req.Name}
		if err := store.SaveEvaluation(eval); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeEvaluation(eval))
	}
}

// getEvaluationHandler responds with a single evaluation's definition.
func getEvaluationHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eval, err := store.GetEvaluation(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeEvaluation(eval))
	}
}

// deleteEvaluationHandler removes an evaluation's saved definition.
func deleteEvaluationHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteEvaluation(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// startEvaluationRunRequest is the POST /api/evaluations/{name}/runs
// request body. Version must name an already-published version (see
// publishEvaluationVersionHandler) — a run can never target the
// evaluation's live draft test cases.
type startEvaluationRunRequest struct {
	Version    int      `json:"version"`
	AgentNames []string `json:"agentNames"`
}

// startEvaluationRunHandler starts a new evaluation run in the background
// and responds immediately with its initial ("running") state.
func startEvaluationRunHandler(mgr evaluationManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req startEvaluationRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Version <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("version is required"))
			return
		}

		run, err := mgr.StartRun(name, req.Version, req.AgentNames)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, run)
	}
}

// listEvaluationRunsHandler responds with every known evaluation run, most
// recently started first.
func listEvaluationRunsHandler(mgr evaluationManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs := mgr.ListRuns()
		if runs == nil {
			runs = []*evaluations.Run{}
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

// getEvaluationRunHandler responds with a single run's current state.
func getEvaluationRunHandler(mgr evaluationManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, ok := mgr.GetRun(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such evaluation run"))
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// publishEvaluationVersionHandler snapshots the evaluation's current draft
// test cases into a new, immutable EvaluationVersion, advancing the
// evaluation's Version to it. This is the only way Version ever changes —
// adding/editing/deleting draft test cases never does.
func publishEvaluationVersionHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := store.PublishEvaluationVersion(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	}
}

// listEvaluationVersionsHandler responds with every published version of an
// evaluation, oldest first — used to populate the run modal's version
// picker.
func listEvaluationVersionsHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := store.ListEvaluationVersions(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if versions == nil {
			versions = []registry.EvaluationVersion{}
		}
		writeJSON(w, http.StatusOK, versions)
	}
}

// listEvaluationResultsHandler responds with every persisted RunResult for
// one evaluation — the durable, agent-to-agent comparison the evaluation
// detail page's "run results" view renders, as opposed to
// listEvaluationRunsHandler's ephemeral in-progress/recent Run tracking.
func listEvaluationResultsHandler(mgr evaluationManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, err := mgr.ListResults(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if results == nil {
			results = []evaluations.RunResult{}
		}
		writeJSON(w, http.StatusOK, results)
	}
}

// addEvaluationTestCasesRequest is the POST
// /api/evaluations/{name}/test-cases request body.
type addEvaluationTestCasesRequest struct {
	TestCases []registry.TestCase `json:"testCases"`
}

// addEvaluationTestCasesHandler appends one or more manually-entered (or
// generated) test cases to an evaluation, mirroring addTestCasesHandler for
// Benchmarks.
func addEvaluationTestCasesHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req addEvaluationTestCasesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if len(req.TestCases) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("at least one test case is required"))
			return
		}

		if err := store.AddEvaluationTestCases(name, req.TestCases); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		eval, err := store.GetEvaluation(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		added := eval.TestCases[len(eval.TestCases)-len(req.TestCases):]
		writeJSON(w, http.StatusCreated, added)
	}
}

// updateEvaluationTestCaseHandler overwrites a single test case, addressed
// by its position in the evaluation (as returned by
// GET /api/evaluations/{name}).
func updateEvaluationTestCaseHandler(store evaluationStore) http.HandlerFunc {
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

		if err := store.UpdateEvaluationTestCase(name, index, tc); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		eval, err := store.GetEvaluation(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, eval.TestCases[index])
	}
}

// deleteEvaluationTestCaseHandler removes a single test case, addressed by
// its position in the evaluation (as returned by
// GET /api/evaluations/{name}).
func deleteEvaluationTestCaseHandler(store evaluationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid test case index: %w", err))
			return
		}

		if err := store.DeleteEvaluationTestCase(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// generateEvaluationTestCasesRequest is the POST
// /api/evaluations/{name}/test-cases/generate request body.
type generateEvaluationTestCasesRequest struct {
	Model      string               `json:"model"`
	SeedPrompt string               `json:"seedPrompt"`
	Assertions []registry.Assertion `json:"assertions"`
	Tags       []string             `json:"tags,omitempty"`
	Count      int                  `json:"count"`
}

// generateEvaluationTestCasesHandler asks a local LLM for prompt variations
// on a seed test case and appends the results (each paired with the seed's
// own assertions/tags, unchanged) to the named evaluation. Setup/
// VerifyCommands are deliberately not generated — same reasoning as
// Benchmarks not generating assertions: tiny local models are unreliable
// at emitting structured output a shell command or assertion list would
// need to be, so only the prompt text varies.
func generateEvaluationTestCasesHandler(store evaluationStore, generator testCaseGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req generateEvaluationTestCasesRequest
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
			newTestCases[i] = registry.TestCase{Prompt: p, Assertions: req.Assertions, Tags: req.Tags}
		}

		if err := store.AddEvaluationTestCases(name, newTestCases); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		eval, err := store.GetEvaluation(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		added := eval.TestCases[len(eval.TestCases)-len(newTestCases):]
		writeJSON(w, http.StatusCreated, added)
	}
}
