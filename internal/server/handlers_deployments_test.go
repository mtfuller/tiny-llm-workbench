package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/deployments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestCreateDeployment(t *testing.T) {
	deps := testDeps()
	store := &fakeDeploymentStore{}
	deps.Deployments = store
	handler, _ := New(deps)

	body, _ := json.Marshal(createDeploymentRequest{Name: "prod", AgentName: "coder", WorkspaceName: "my-project"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/deployments status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].AgentName != "coder" || store.saved[0].WorkspaceName != "my-project" {
		t.Errorf("store.saved = %+v, want the deployment persisted", store.saved)
	}
}

func TestCreateDeploymentRequiresAllFields(t *testing.T) {
	handler, _ := New(testDeps())
	body, _ := json.Marshal(createDeploymentRequest{Name: "prod"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/deployments (missing fields) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListDeploymentsEmptyIsJSONArray(t *testing.T) {
	handler, _ := New(testDeps())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/deployments", nil))
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Errorf("GET /api/deployments body = %q, want an empty JSON array", body)
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	deps := testDeps()
	deps.Deployments = &fakeDeploymentStore{getErr: errors.New("deployment not found")}
	handler, _ := New(deps)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/deployments/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/deployments/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartDeploymentSession(t *testing.T) {
	deps := testDeps()
	mgr := &fakeDeploymentManager{startResult: &deployments.Session{ID: "sess-1", AgentName: "coder", InstanceID: "c1", WorkspacePath: "/home/me/project"}}
	deps.DeploymentSessions = mgr
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments/prod/start", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/deployments/prod/start status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(mgr.started) != 1 || mgr.started[0] != "prod" {
		t.Errorf("mgr.started = %v, want [prod]", mgr.started)
	}
	var got deployments.Session
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != "sess-1" || got.WorkspacePath != "/home/me/project" {
		t.Errorf("session = %+v, want it to carry the real workspace path", got)
	}
}

func TestStartDeploymentSessionError(t *testing.T) {
	deps := testDeps()
	deps.DeploymentSessions = &fakeDeploymentManager{startErr: errors.New("not a real workspace")}
	handler, _ := New(deps)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments/prod/start", nil))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../start (error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestSendDeploymentMessage(t *testing.T) {
	deps := testDeps()
	mgr := &fakeDeploymentManager{sendResult: agents.ChatMessage{Role: "assistant", Content: "made the file"}}
	deps.DeploymentSessions = mgr
	handler, _ := New(deps)

	body, _ := json.Marshal(sendAgentMessageRequest{Message: "create report.md"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments/sessions/sess-1/messages", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../messages status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(mgr.sent) != 1 || mgr.sent[0] != "create report.md" {
		t.Errorf("mgr.sent = %v, want [create report.md]", mgr.sent)
	}
}

func TestStopDeploymentSession(t *testing.T) {
	deps := testDeps()
	mgr := &fakeDeploymentManager{}
	deps.DeploymentSessions = mgr
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments/sessions/sess-1/stop", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST .../stop status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(mgr.stopped) != 1 || mgr.stopped[0] != "sess-1" {
		t.Errorf("mgr.stopped = %v, want [sess-1]", mgr.stopped)
	}
}

func TestDeploymentRoutesDoNotShadowSessions(t *testing.T) {
	// GET /api/deployments/{name} must not swallow /api/deployments/sessions/{id}.
	deps := testDeps()
	deps.Deployments = &fakeDeploymentStore{get: registry.Deployment{Name: "prod"}}
	mgr := &fakeDeploymentManager{getResult: &deployments.Session{ID: "sess-1"}, getOK: true}
	deps.DeploymentSessions = mgr
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/deployments/sessions/sess-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/deployments/sessions/sess-1 status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got deployments.Session
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != "sess-1" {
		t.Errorf("got = %+v, want the session, not a deployment", got)
	}
}
