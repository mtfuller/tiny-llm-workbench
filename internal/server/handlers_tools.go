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
// any agent that references it in its Tools set.
func deleteToolHandler(store toolStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteTool(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// tryCatalogToolRequest is the POST /api/tools/{name}/try request body. If
// InstanceID is empty a fresh sandbox is launched from WorkspaceName (which
// must be a TEST workspace) and its id is returned for reuse on subsequent
// runs; the sandbox is left running so the user can re-run and inspect the
// effects, and stopped via the usual instance-stop route.
type tryCatalogToolRequest struct {
	WorkspaceName string            `json:"workspaceName"`
	InstanceID    string            `json:"instanceId"`
	Args          map[string]string `json:"args"`
}

// tryCatalogToolResponse pairs the started exec with the sandbox it runs in
// and that sandbox's host-side path (so the user can open it in an editor).
type tryCatalogToolResponse struct {
	Exec          any    `json:"exec"`
	InstanceID    string `json:"instanceId"`
	WorkspacePath string `json:"workspacePath,omitempty"`
}

// tryCatalogToolHandler renders a catalog tool's command and runs it inside
// a test workspace's sandbox — the Tools page's Playground. Output streams
// live over /api/events like a plain ad hoc exec.
func tryCatalogToolHandler(tools toolStore, workspaces workspaceStore, mgr workspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		toolName := r.PathValue("name")

		var req tryCatalogToolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		tool, err := tools.GetTool(toolName)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}

		instanceID := req.InstanceID
		workspacePath := ""
		if instanceID == "" {
			if req.WorkspaceName == "" {
				writeError(w, http.StatusBadRequest, errors.New("workspaceName is required to launch a sandbox"))
				return
			}
			ws, err := workspaces.GetWorkspace(req.WorkspaceName)
			if err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if ws.Type != registry.WorkspaceTest {
				writeError(w, http.StatusBadRequest, fmt.Errorf("workspace %q is not a test workspace", req.WorkspaceName))
				return
			}
			instance, err := mgr.Launch(r.Context(), req.WorkspaceName, "")
			if err != nil {
				writeError(w, http.StatusBadGateway, err)
				return
			}
			instanceID = instance.ID
			workspacePath = instance.WorkspacePath
		}

		exec, err := mgr.TryTool(instanceID, tool, req.Args)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, tryCatalogToolResponse{Exec: exec, InstanceID: instanceID, WorkspacePath: workspacePath})
	}
}
