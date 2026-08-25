package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeEnvironment ensures Tools/Mounts never serialize as JSON "null"
// (a nil slice's default), which breaks frontend code that calls array
// methods on a parsed response.
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
	Tools  []string         `json:"tools"`
	Mounts []registry.Mount `json:"mounts"`
}

// createEnvironmentHandler saves a new custom Environment definition.
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

		env := registry.Environment{
			Name:   req.Name,
			Image:  req.Image,
			Tools:  req.Tools,
			Mounts: req.Mounts,
		}
		if err := envs.SaveEnvironment(env); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeEnvironment(env))
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
