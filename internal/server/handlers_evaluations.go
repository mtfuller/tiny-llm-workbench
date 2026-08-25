package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeEvaluation ensures TestCases (and each test case's Assertions)
// never serialize as JSON "null", same reasoning as normalizeEnvironment.
func normalizeEvaluation(e registry.Evaluation) registry.Evaluation {
	if e.TestCases == nil {
		e.TestCases = []registry.TestCase{}
		return e
	}
	for i, tc := range e.TestCases {
		if tc.Assertions == nil {
			e.TestCases[i].Assertions = []registry.Assertion{}
		}
	}
	return e
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
	Name        string              `json:"name"`
	Environment string              `json:"environment,omitempty"`
	TestCases   []registry.TestCase `json:"testCases"`
}

// saveEvaluationHandler creates or overwrites an evaluation's definition.
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
		if len(req.TestCases) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("at least one test case is required"))
			return
		}

		eval := registry.Evaluation{Name: req.Name, Environment: req.Environment, TestCases: req.TestCases}
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

// startEvaluationRunRequest is the POST /api/evaluations/{name}/runs
// request body.
type startEvaluationRunRequest struct {
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

		run, err := mgr.StartRun(name, req.AgentNames)
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
