package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// listDatasetsHandler responds with every registry-tracked dataset and its
// example count.
func listDatasetsHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := datasets.ListDatasets()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if list == nil {
			list = []registry.DatasetSummary{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// createDatasetRequest is the POST /api/datasets request body.
type createDatasetRequest struct {
	Name string `json:"name"`
}

// createDatasetHandler creates a new, empty dataset.
func createDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDatasetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		dataset, err := datasets.CreateDataset(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, dataset)
	}
}

// datasetDetail is the GET /api/datasets/{name} response body.
type datasetDetail struct {
	Name     string             `json:"name"`
	Examples []registry.Example `json:"examples"`
}

// getDatasetHandler responds with a single dataset's input/output pairs.
func getDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		examples, err := datasets.ListExamples(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if examples == nil {
			examples = []registry.Example{}
		}
		writeJSON(w, http.StatusOK, datasetDetail{Name: name, Examples: examples})
	}
}

// generateVariationsRequest is the POST /api/datasets/{name}/variations
// request body.
type generateVariationsRequest struct {
	Model string           `json:"model"`
	Seed  registry.Example `json:"seed"`
	Count int              `json:"count"`
}

// generateVariationsHandler asks a local LLM for variations on a seed
// example and appends the results to the named dataset.
func generateVariationsHandler(datasets datasetStore, generator variationGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req generateVariationsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Model == "" {
			writeError(w, http.StatusBadRequest, errors.New("model is required"))
			return
		}
		if req.Count <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("count must be positive"))
			return
		}

		examples, err := generator.Variations(r.Context(), req.Model, req.Seed, req.Count)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		if err := datasets.AppendExamples(name, examples); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, examples)
	}
}
