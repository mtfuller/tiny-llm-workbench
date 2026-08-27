package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeTool ensures Parameters never serializes as JSON "null".
func normalizeTool(t registry.Tool) registry.Tool {
	if t.Parameters == nil {
		t.Parameters = []registry.ToolParameter{}
	}
	return t
}

// listToolsHandler responds with every catalog tool (prebuilt and custom).
func listToolsHandler(store toolStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListTools()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		normalized := make([]registry.Tool, len(list))
		for i, t := range list {
			normalized[i] = normalizeTool(t)
		}
		writeJSON(w, http.StatusOK, normalized)
	}
}

// saveToolRequest is the request body for both creating (POST) and
// overwriting (PUT) a catalog tool.
type saveToolRequest struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Command     string                   `json:"command"`
	Parameters  []registry.ToolParameter `json:"parameters"`
}

func (req saveToolRequest) validate() error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.Command == "" {
		return errors.New("command is required")
	}
	return nil
}

// createToolHandler adds a new tool to the catalog.
func createToolHandler(store toolStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req saveToolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		tool := registry.Tool{Name: req.Name, Description: req.Description, Command: req.Command, Parameters: req.Parameters}
		if err := store.SaveTool(tool); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		saved, err := store.GetTool(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, normalizeTool(saved))
	}
}

// getToolHandler responds with a single catalog tool's definition.
func getToolHandler(store toolStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tool, err := store.GetTool(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeTool(tool))
	}
}

// updateToolHandler overwrites a catalog tool's definition — a live
// reference, so this changes what every Environment that has it attached
// sees the next time it's used.
func updateToolHandler(store toolStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req saveToolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		req.Name = name
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		tool := registry.Tool{Name: name, Description: req.Description, Command: req.Command, Parameters: req.Parameters}
		if err := store.SaveTool(tool); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		saved, err := store.GetTool(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeTool(saved))
	}
}

// deleteToolHandler removes a tool from the catalog. It doesn't cascade to
// any Environment that references it — see registry.DeleteTool.
func deleteToolHandler(store toolStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteTool(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
