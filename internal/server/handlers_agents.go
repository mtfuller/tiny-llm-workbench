package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeAgent ensures Graph.Nodes/Edges and the Tools/KnowledgeBases sets
// never serialize as JSON "null", which breaks frontend code that calls
// array methods on a parsed response.
func normalizeAgent(a registry.Agent) registry.Agent {
	if a.Graph.Nodes == nil {
		a.Graph.Nodes = []registry.Node{}
	}
	if a.Graph.Edges == nil {
		a.Graph.Edges = []registry.Edge{}
	}
	if a.Tools == nil {
		a.Tools = []string{}
	}
	if a.KnowledgeBases == nil {
		a.KnowledgeBases = []string{}
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
	Name           string         `json:"name"`
	Workspace      string         `json:"workspace,omitempty"`
	Tools          []string       `json:"tools"`
	KnowledgeBases []string       `json:"knowledgeBases"`
	Description    string         `json:"description,omitempty"`
	Graph          registry.Graph `json:"graph"`
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

		agent := registry.Agent{
			Name:           req.Name,
			Workspace:      req.Workspace,
			Tools:          req.Tools,
			KnowledgeBases: req.KnowledgeBases,
			Description:    req.Description,
			Graph:          req.Graph,
		}
		if err := store.SaveAgent(agent); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Re-read rather than echo the local agent value: SaveAgent stamps
		// CreatedAt on its own copy (Go passes it by value), so the request's
		// struct never reflects it — same reasoning as createEnvironmentHandler.
		saved, err := store.GetAgent(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeAgent(saved))
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

// startAgentRunRequest is the optional POST /api/agents/{name}/runs body. A
// non-empty Workspace overrides the agent's own bound (test) workspace for
// this run — the chat/debug UI's "run against a different test workspace"
// picker.
type startAgentRunRequest struct {
	Workspace string `json:"workspace,omitempty"`
}

// startAgentRunHandler begins a new chat session against the named agent.
func startAgentRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startAgentRunRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // body is optional

		run, err := mgr.StartRun(r.PathValue("name"), req.Workspace)
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

// agentPromptDefaultHandler returns the built-in default agent prompt
// template, so the editor can load it into the field as an editable start.
func agentPromptDefaultHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"template": agents.DefaultAgentPromptTemplate})
	}
}

// previewNodeRequest is the POST /api/agents/preview-node request body: one
// prompt- or agent-node's data (from the live canvas) plus a sample input.
type previewNodeRequest struct {
	NodeType string            `json:"nodeType"`
	Data     registry.NodeData `json:"data"`
	Input    string            `json:"input"`
}

// previewNodeHandler runs a standalone one-shot preview of a single prompt
// or agent node against a sample input — no graph, workspace, tools, or
// history. See agents.Engine.PreviewNode.
func previewNodeHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req previewNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.NodeType != "prompt" && req.NodeType != "agent" {
			writeError(w, http.StatusBadRequest, errors.New("preview is only available for prompt and agent nodes"))
			return
		}

		res, err := mgr.PreviewNode(r.Context(), registry.Node{Type: req.NodeType, Data: req.Data}, req.Input)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// startDebugRunRequest is the POST /api/agents/{name}/debug request body.
// The graph, workspace, and tool set come straight from the caller (the
// live canvas, which may have unsaved edits) rather than the agent's saved
// definition, so a session can debug work in progress without a Save
// round-trip.
type startDebugRunRequest struct {
	Graph          registry.Graph `json:"graph"`
	Workspace      string         `json:"workspace,omitempty"`
	Tools          []string       `json:"tools,omitempty"`
	KnowledgeBases []string       `json:"knowledgeBases,omitempty"` // accepted for parity; the engine resolves knowledge bases per node
}

// startDebugRunHandler begins a new paused debug session.
func startDebugRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startDebugRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		state, err := mgr.StartDebugRun(r.PathValue("name"), req.Graph, req.Workspace, req.Tools)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, state)
	}
}

// sendDebugMessageHandler starts a new turn in a debug session: the input
// node becomes pending, ready for the first step.
func sendDebugMessageHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var req sendAgentMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		state, err := mgr.SendDebugMessage(id, req.Message)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	}
}

// stepDebugRunHandler executes the session's pending node and responds with
// the resulting state.
func stepDebugRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := mgr.StepDebugRun(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	}
}

// retryDebugRunHandler re-executes the session's most recently stepped node.
func retryDebugRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := mgr.RetryDebugRun(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	}
}

// getDebugRunHandler responds with a debug session's current state.
func getDebugRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, ok := mgr.GetDebugRun(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("no such debug run"))
			return
		}
		writeJSON(w, http.StatusOK, state)
	}
}

// stopDebugRunHandler ends a debug session, stopping its Environment
// instance (if any). Idempotent, like stopAgentRunHandler.
func stopDebugRunHandler(mgr agentManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.StopDebugRun(r.PathValue("id")); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
