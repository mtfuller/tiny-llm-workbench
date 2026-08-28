package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// listDeploymentsHandler responds with every saved deployment.
func listDeploymentsHandler(store deploymentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListDeployments()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if list == nil {
			list = []registry.Deployment{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// createDeploymentRequest is the POST /api/deployments request body.
type createDeploymentRequest struct {
	Name          string `json:"name"`
	AgentName     string `json:"agentName"`
	WorkspaceName string `json:"workspaceName"`
}

// createDeploymentHandler saves a new deployment (an agent bound to a real
// workspace). Starting it is a separate call.
func createDeploymentHandler(store deploymentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDeploymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" || req.AgentName == "" || req.WorkspaceName == "" {
			writeError(w, http.StatusBadRequest, errors.New("name, agentName and workspaceName are required"))
			return
		}

		dep := registry.Deployment{Name: req.Name, AgentName: req.AgentName, WorkspaceName: req.WorkspaceName}
		if err := store.SaveDeployment(dep); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		saved, err := store.GetDeployment(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

// getDeploymentHandler responds with a single deployment's definition.
func getDeploymentHandler(store deploymentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dep, err := store.GetDeployment(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, dep)
	}
}

// deleteDeploymentHandler removes a deployment's definition.
func deleteDeploymentHandler(store deploymentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteDeployment(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// startDeploymentHandler launches a sandbox for the deployment's real
// workspace and begins an agent chat session against it.
func startDeploymentHandler(mgr deploymentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := mgr.Start(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusCreated, session)
	}
}

// sendDeploymentMessageHandler runs one chat turn in a deployment session.
func sendDeploymentMessageHandler(mgr deploymentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sendAgentMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		reply, err := mgr.SendMessage(r.PathValue("id"), req.Message)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, reply)
	}
}

// getDeploymentSessionHandler responds with a session's current transcript.
func getDeploymentSessionHandler(mgr deploymentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := mgr.Get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such deployment session"))
			return
		}
		writeJSON(w, http.StatusOK, session)
	}
}

// stopDeploymentSessionHandler ends a deployment session, stopping its agent
// run and its sandbox. Idempotent.
func stopDeploymentSessionHandler(mgr deploymentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Stop(r.PathValue("id")); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
