package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

// startTrainingRunHandler starts a new training run in the background and
// responds immediately with its initial ("running") state. Progress and
// completion are reported over /api/events (training.progress /
// training.status), and can also be polled via GET /api/training/runs/{id}.
func startTrainingRunHandler(mgr trainingManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg training.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		run, err := mgr.StartRun(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, run)
	}
}

// listTrainingRunsHandler responds with every known training run, most
// recently started first.
func listTrainingRunsHandler(mgr trainingManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs := mgr.ListRuns()
		if runs == nil {
			runs = []*training.Run{}
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

// cancelTrainingRunHandler stops a running training job's subprocess. It's a
// no-op if the run has already finished.
func cancelTrainingRunHandler(mgr trainingManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.CancelRun(r.PathValue("id")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// getTrainingRunHandler responds with a single run's current state.
func getTrainingRunHandler(mgr trainingManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, ok := mgr.GetRun(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Errorf("no such training run"))
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}
