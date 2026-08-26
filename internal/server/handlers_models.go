package server

import (
	"net/http"
)

// modelJSON is a model as returned by GET /api/models — deliberately
// slimmer than registry.Model (drops Path/CreatedAt, which are internal
// filesystem details, not part of the public API).
type modelJSON struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// listModelsHandler responds with every registry-tracked model.
func listModelsHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := models.ListModels()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		out := make([]modelJSON, len(list))
		for i, m := range list {
			out[i] = modelJSON{Name: m.Name, Source: m.Source}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// deleteModelHandler removes a registry-tracked model.
func deleteModelHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := models.DeleteModel(r.PathValue("name")); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
