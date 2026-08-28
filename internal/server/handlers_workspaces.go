package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeEnvironment ensures Tools/Mounts never serialize as JSON "null",
// which breaks frontend code that calls array methods on a parsed response.
func normalizeEnvironment(e registry.Environment) registry.Environment {
	if e.Tools == nil {
		e.Tools = []string{}
	}
	if e.Mounts == nil {
		e.Mounts = []registry.Mount{}
	}
	return e
}

// listEnvironmentsHandler responds with every registry-tracked Environment
// definition (prebuilt and custom).
func listEnvironmentsHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := envs.ListEnvironments()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		normalized := make([]registry.Environment, len(list))
		for i, e := range list {
			normalized[i] = normalizeEnvironment(e)
		}
		writeJSON(w, http.StatusOK, normalized)
	}
}

// createEnvironmentRequest is the POST /api/environments request body.
type createEnvironmentRequest struct {
	Name   string           `json:"name"`
	Image  string           `json:"image"`
	Mounts []registry.Mount `json:"mounts"`
}

// createEnvironmentHandler saves a new custom Environment definition. It
// starts with no tools — those are added afterward from the environment's
// own workspace page, the same way a Benchmark starts with no test cases.
func createEnvironmentHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createEnvironmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		if req.Image == "" {
			writeError(w, http.StatusBadRequest, errors.New("image is required"))
			return
		}

		env := registry.Environment{Name: req.Name, Image: req.Image, Mounts: req.Mounts}
		if err := envs.SaveEnvironment(env); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeEnvironment(env))
	}
}

// getEnvironmentHandler responds with a single Environment's definition.
func getEnvironmentHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env, err := envs.GetEnvironment(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeEnvironment(env))
	}
}

// deleteEnvironmentHandler removes an Environment definition. It doesn't
// touch any already-running instances of it.
func deleteEnvironmentHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := envs.DeleteEnvironment(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// updateEnvironmentConfigRequest is the PUT /api/environments/{name}/config
// request body.
type updateEnvironmentConfigRequest struct {
	Image  string           `json:"image"`
	Mounts []registry.Mount `json:"mounts"`
}

// updateEnvironmentConfigHandler overwrites an environment's image and
// mounts, leaving its tools untouched — the "Configuration" side of the
// environment workspace page.
func updateEnvironmentConfigHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req updateEnvironmentConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Image == "" {
			writeError(w, http.StatusBadRequest, errors.New("image is required"))
			return
		}

		if err := envs.UpdateConfig(name, req.Image, req.Mounts); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		env, err := envs.GetEnvironment(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeEnvironment(env))
	}
}

// attachToolRequest is the POST /api/environments/{name}/tools request
// body.
type attachToolRequest struct {
	ToolName string `json:"toolName"`
}

// attachToolHandler references an existing catalog tool from an
// environment — a live reference, not a copy (see registry.Tool).
func attachToolHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req attachToolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ToolName == "" {
			writeError(w, http.StatusBadRequest, errors.New("toolName is required"))
			return
		}

		if err := envs.AttachTool(name, req.ToolName); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		env, err := envs.GetEnvironment(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, normalizeEnvironment(env))
	}
}

// detachToolHandler removes a tool reference from an environment. The
// catalog tool itself is untouched — see registry.DetachTool.
func detachToolHandler(envs environmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		toolName := r.PathValue("toolName")

		if err := envs.DetachTool(name, toolName); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// tryToolRequest is the POST /api/environments/{name}/tools/{toolName}/try
// request body.
type tryToolRequest struct {
	InstanceID string            `json:"instanceId"`
	Args       map[string]string `json:"args"`
}

// tryToolHandler renders a tool's command with the given arguments and runs
// it inside a running instance, the same way plain ad hoc exec does — the
// environment workspace's "Playground" tab uses this so trying a tool
// streams live output over the same /api/events mechanism. toolName is
// resolved against the global catalog directly (not the environment's own
// list) — the environment only needs to have it attached, checked here so a
// stale/removed attachment can't be used to run an arbitrary catalog tool.
func tryToolHandler(envs environmentStore, tools toolStore, mgr environmentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		toolName := r.PathValue("toolName")

		var req tryToolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.InstanceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("instanceId is required"))
			return
		}

		env, err := envs.GetEnvironment(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		attached := false
		for _, t := range env.Tools {
			if t == toolName {
				attached = true
				break
			}
		}
		if !attached {
			writeError(w, http.StatusBadRequest, fmt.Errorf("tool %q is not attached to environment %q", toolName, name))
			return
		}

		tool, err := tools.GetTool(toolName)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}

		exec, err := mgr.TryTool(req.InstanceID, tool, req.Args)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, exec)
	}
}

// launchEnvironmentRequest is the POST /api/environments/{name}/launch
// request body.
type launchEnvironmentRequest struct {
	InstanceName string `json:"instanceName"`
}

// launchEnvironmentHandler starts a new container from the named
// Environment definition.
func launchEnvironmentHandler(mgr environmentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req launchEnvironmentRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // instanceName is optional; ignore an empty/absent body

		instance, err := mgr.Launch(r.Context(), name, req.InstanceName)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		writeJSON(w, http.StatusCreated, instance)
	}
}

// listInstancesHandler responds with every running (or recently stopped)
// Environment instance, reflecting Docker's live state.
func listInstancesHandler(mgr environmentManager) http.HandlerFunc {
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

// stopInstanceHandler stops and removes an Environment instance.
func stopInstanceHandler(mgr environmentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if err := mgr.Stop(r.Context(), id); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// startExecRequest is the POST /api/environments/instances/{id}/exec
// request body.
type startExecRequest struct {
	Command string `json:"command"`
}

// startExecHandler runs a command inside an Environment instance in the
// background, streaming output over /api/events.
func startExecHandler(mgr environmentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var req startExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		exec, err := mgr.StartExec(id, req.Command)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusAccepted, exec)
	}
}

// getExecHandler responds with a single exec's current state, for polling
// as a fallback alongside the SSE stream.
func getExecHandler(mgr environmentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exec, ok := mgr.GetExec(r.PathValue("execId"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such exec"))
			return
		}
		writeJSON(w, http.StatusOK, exec)
	}
}
