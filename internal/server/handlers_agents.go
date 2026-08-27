package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeAgent ensures Graph.Nodes/Edges never serialize as JSON "null",
// same reasoning as normalizeEnvironment.
func normalizeAgent(a registry.Agent) registry.Agent {
	if a.Graph.Nodes == nil {
		a.Graph.Nodes = []registry.Node{}
	}
	if a.Graph.Edges == nil {
		a.Graph.Edges = []registry.Edge{}
	}
	return a
}

// listAgentsHandler responds with every saved agent definition.
func listAgentsHandler(store agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListAgents()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		normalized := make([]registry.Agent, len(list))
		for i, a := range list {
			normalized[i] = normalizeAgent(a)
		}
		writeJSON(w, http.StatusOK, normalized)
	}
}

// saveAgentRequest is the POST /api/agents request body.
type saveAgentRequest struct {
	Name        string         `json:"name"`
	Environment string         `json:"environment,omitempty"`
	Description string         `json:"description,omitempty"`
	Graph       registry.Graph `json:"graph"`
}

// saveAgentHandler creates or overwrites an agent's definition.
func saveAgentHandler(store agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req saveAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		agent := registry.Agent{Name: req.Name, Environment: req.Environment, Description: req.Description, Graph: req.Graph}
		if err := store.SaveAgent(agent); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeAgent(agent))
	}
}

// getAgentHandler responds with a single agent's definition.
func getAgentHandler(store agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent, err := store.GetAgent(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeAgent(agent))
	}
}

// deleteAgentHandler removes an agent's saved definition.
func deleteAgentHandler(store agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteAgent(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// startAgentRunHandler begins a new chat session against the named agent.
func startAgentRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, err := mgr.StartRun(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	}
}

// sendAgentMessageRequest is the POST /api/agents/runs/{id}/messages
// request body.
type sendAgentMessageRequest struct {
	Message string `json:"message"`
}

// sendAgentMessageHandler runs one chat turn synchronously and responds
// with the assistant's reply. Step-by-step progress is also published on
// /api/events (agent.step) while this blocks.
func sendAgentMessageHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var req sendAgentMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		reply, err := mgr.SendMessage(id, req.Message)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		writeJSON(w, http.StatusOK, reply)
	}
}

// getAgentRunHandler responds with a run's full message history.
func getAgentRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, ok := mgr.GetRun(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such run"))
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// stopAgentRunHandler ends a chat session, stopping its Environment
// instance (if any). Idempotent — stopping an already-stopped or unknown
// run still succeeds, since the frontend calls this as best-effort cleanup
// when the chat modal closes.
func stopAgentRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.StopRun(r.PathValue("id")); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
