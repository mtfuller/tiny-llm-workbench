package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListAgents(t *testing.T) {
	deps := testDeps()
	deps.Agents = &fakeAgentStore{list: []registry.Agent{{Name: "greeter"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agents status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "greeter" {
		t.Errorf("GET /api/agents body = %+v, want a single greeter entry", got)
	}
}

func TestListAgentsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/agents (empty) body = %q, want %q", got, "[]")
	}
}

func TestListAgentsNilGraphFieldsAreJSONArraysNotNull(t *testing.T) {
	deps := testDeps()
	deps.Agents = &fakeAgentStore{list: []registry.Agent{{Name: "greeter"}}} // Graph.Nodes/Edges left nil

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"nodes":[]`) || !strings.Contains(rec.Body.String(), `"edges":[]`) {
		t.Errorf("GET /api/agents body = %q, want nodes/edges to serialize as []", rec.Body.String())
	}
}

func TestSaveAgent(t *testing.T) {
	deps := testDeps()
	store := &fakeAgentStore{}
	deps.Agents = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveAgentRequest{Name: "greeter", Graph: registry.Graph{Nodes: []registry.Node{{ID: "1", Type: "input"}}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/agents status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "greeter" {
		t.Errorf("store.saved = %+v, want [greeter]", store.saved)
	}
}

func TestSaveAgentIncludesEnvironment(t *testing.T) {
	deps := testDeps()
	store := &fakeAgentStore{}
	deps.Agents = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveAgentRequest{
		Name:        "researcher",
		Environment: "WebSearch",
		Graph:       registry.Graph{Nodes: []registry.Node{{ID: "1", Type: "input"}}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/agents status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Environment != "WebSearch" {
		t.Errorf("store.saved = %+v, want Environment=WebSearch", store.saved)
	}
}

func TestSaveAgentIncludesDescription(t *testing.T) {
	deps := testDeps()
	store := &fakeAgentStore{}
	deps.Agents = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveAgentRequest{
		Name:        "researcher",
		Description: "Looks things up on the web.",
		Graph:       registry.Graph{Nodes: []registry.Node{{ID: "1", Type: "input"}}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/agents status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Description != "Looks things up on the web." {
		t.Errorf("store.saved = %+v, want Description=%q", store.saved, "Looks things up on the web.")
	}
}

func TestSaveAgentRequiresName(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveAgentRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/agents (no name) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetAgent(t *testing.T) {
	deps := testDeps()
	deps.Agents = &fakeAgentStore{get: registry.Agent{Name: "greeter"}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/greeter", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agents/greeter status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	deps := testDeps()
	deps.Agents = &fakeAgentStore{getErr: errors.New("not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/agents/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteAgent(t *testing.T) {
	deps := testDeps()
	store := &fakeAgentStore{}
	deps.Agents = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/greeter", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/agents/greeter status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "greeter" {
		t.Errorf("store.deleted = %v, want [greeter]", store.deleted)
	}
}

func TestDeleteAgentNotFound(t *testing.T) {
	deps := testDeps()
	deps.Agents = &fakeAgentStore{deleteErr: errors.New("agent not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/agents/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartAgentRun(t *testing.T) {
	deps := testDeps()
	mgr := &fakeAgentManager{startResult: &agents.Run{ID: "run-1", AgentName: "greeter", Messages: []agents.ChatMessage{}}}
	deps.AgentRuns = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/greeter/runs", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/agents/greeter/runs status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(mgr.started) != 1 || mgr.started[0] != "greeter" {
		t.Errorf("mgr.started = %v, want [greeter]", mgr.started)
	}
}

func TestStartAgentRunError(t *testing.T) {
	deps := testDeps()
	deps.AgentRuns = &fakeAgentManager{startErr: errors.New("no such agent")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/missing/runs", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/agents/missing/runs status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSendAgentMessage(t *testing.T) {
	deps := testDeps()
	mgr := &fakeAgentManager{messageResult: agents.ChatMessage{Role: "assistant", Content: "hello!"}}
	deps.AgentRuns = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(sendAgentMessageRequest{Message: "hi"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/runs/run-1/messages", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../messages status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(mgr.messages) != 1 || mgr.messages[0] != "hi" {
		t.Errorf("mgr.messages = %v, want [hi]", mgr.messages)
	}
}

func TestSendAgentMessageError(t *testing.T) {
	deps := testDeps()
	deps.AgentRuns = &fakeAgentManager{messageErr: errors.New("model runner unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(sendAgentMessageRequest{Message: "hi"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/runs/run-1/messages", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../messages (error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestGetAgentRun(t *testing.T) {
	deps := testDeps()
	deps.AgentRuns = &fakeAgentManager{getResult: &agents.Run{ID: "run-1"}, getOK: true}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/runs/run-1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agents/runs/run-1 status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetAgentRunNotFound(t *testing.T) {
	deps := testDeps()
	deps.AgentRuns = &fakeAgentManager{getOK: false}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/runs/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/agents/runs/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStopAgentRun(t *testing.T) {
	deps := testDeps()
	mgr := &fakeAgentManager{}
	deps.AgentRuns = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/runs/run-1/stop", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST .../stop status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(mgr.stoppedRuns) != 1 || mgr.stoppedRuns[0] != "run-1" {
		t.Errorf("mgr.stoppedRuns = %v, want [run-1]", mgr.stoppedRuns)
	}
}

func TestStopAgentRunError(t *testing.T) {
	deps := testDeps()
	deps.AgentRuns = &fakeAgentManager{stopErr: errors.New("docker daemon unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/runs/run-1/stop", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../stop (error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}
