package server

import (
	"errors"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/models"
)

// listModelsHandler responds with every known model: TLW registry entries
// merged with Ollama's locally-pulled models.
func listModelsHandler(catalog modelCatalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := catalog.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if list == nil {
			list = []models.Model{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// deleteModelHandler removes a model, dispatching to Ollama or the registry
// depending on the required "source" query param (a model's Source field, as
// returned by GET /api/models) — the name alone doesn't say which backing
// store owns it.
func deleteModelHandler(catalog modelCatalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		if source == "" {
			writeError(w, http.StatusBadRequest, errors.New("source query param is required"))
			return
		}

		if err := catalog.Delete(r.Context(), r.PathValue("name"), source); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
