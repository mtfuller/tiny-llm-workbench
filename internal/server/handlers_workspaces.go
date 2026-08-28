package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// listWorkspacesHandler responds with every registry-tracked Workspace.
func listWorkspacesHandler(store workspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if list == nil {
			list = []registry.Workspace{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// createWorkspaceRequest is the POST /api/workspaces request body. For a
// "test" workspace HostPath is ignored (an editable folder is created under
// the registry root); for a "real" workspace HostPath must be an existing
// directory on the user's machine.
type createWorkspaceRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	HostPath string `json:"hostPath"`
}

// createWorkspaceHandler saves a new workspace.
func createWorkspaceHandler(store workspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createWorkspaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		wsType := registry.WorkspaceType(req.Type)
		switch wsType {
		case registry.WorkspaceTest:
			// HostPath is derived by the registry.
		case registry.WorkspaceReal:
			if req.HostPath == "" {
				writeError(w, http.StatusBadRequest, errors.New("a real workspace needs a directory"))
				return
			}
			info, err := os.Stat(req.HostPath)
			if err != nil || !info.IsDir() {
				writeError(w, http.StatusBadRequest, fmt.Errorf("%q is not an existing directory", req.HostPath))
				return
			}
		default:
			writeError(w, http.StatusBadRequest, errors.New(`type must be "test" or "real"`))
			return
		}

		ws := registry.Workspace{Name: req.Name, Type: wsType, HostPath: req.HostPath}
		if err := store.SaveWorkspace(ws); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		saved, err := store.GetWorkspace(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

// getWorkspaceHandler responds with a single Workspace.
func getWorkspaceHandler(store workspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := store.GetWorkspace(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, ws)
	}
}

// deleteWorkspaceHandler removes a workspace. A real workspace's target
// directory is never touched — see registry.DeleteWorkspace.
func deleteWorkspaceHandler(store workspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteWorkspace(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// launchWorkspaceRequest is the POST /api/workspaces/{name}/launch request
// body.
type launchWorkspaceRequest struct {
	InstanceName string `json:"instanceName"`
}

// launchWorkspaceHandler starts a new sandbox for the named workspace.
func launchWorkspaceHandler(mgr workspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req launchWorkspaceRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // instanceName is optional

		instance, err := mgr.Launch(r.Context(), name, req.InstanceName)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		writeJSON(w, http.StatusCreated, instance)
	}
}

// listInstancesHandler responds with every running (or recently stopped)
// workspace sandbox, reflecting Docker's live state.
func listInstancesHandler(mgr workspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instances, err := mgr.ListInstances(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if instances == nil {
			instances = []environments.Instance{}
		}
		writeJSON(w, http.StatusOK, instances)
	}
}

// stopInstanceHandler stops and removes a workspace sandbox.
func stopInstanceHandler(mgr workspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Stop(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// startExecRequest is the POST /api/workspaces/instances/{id}/exec request
// body.
type startExecRequest struct {
	Command string `json:"command"`
}

// startExecHandler runs a command inside a workspace sandbox in the
// background, streaming output over /api/events.
func startExecHandler(mgr workspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		exec, err := mgr.StartExec(r.PathValue("id"), req.Command)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, exec)
	}
}

// getExecHandler responds with a single exec's current state, for polling
// as a fallback alongside the SSE stream.
func getExecHandler(mgr workspaceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exec, ok := mgr.GetExec(r.PathValue("execId"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such exec"))
			return
		}
		writeJSON(w, http.StatusOK, exec)
	}
}
