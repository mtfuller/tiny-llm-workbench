package server

import (
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
