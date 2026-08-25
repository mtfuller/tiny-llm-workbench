package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListEnvironments(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{list: []registry.Environment{{Name: "WebSearch", Image: "curlimages/curl:8.10.1"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/environments status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.Environment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "WebSearch" {
		t.Errorf("GET /api/environments body = %+v, want a single WebSearch entry", got)
	}
}

func TestListEnvironmentsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/environments (empty) body = %q, want %q", got, "[]")
	}
}

func TestListEnvironmentsNilMountsIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{list: []registry.Environment{{Name: "WebSearch", Image: "curlimages/curl:8.10.1"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments", nil)
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"mounts":[]`) || !strings.Contains(rec.Body.String(), `"tools":[]`) {
		t.Errorf("GET /api/environments body = %q, want tools/mounts to serialize as []", rec.Body.String())
	}
}

func TestCreateEnvironment(t *testing.T) {
	deps := testDeps()
	store := &fakeEnvironmentStore{}
	deps.Environments = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createEnvironmentRequest{Name: "my-env", Image: "alpine:3.20"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/environments status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "my-env" {
		t.Errorf("store.saved = %+v, want [my-env]", store.saved)
	}
}

func TestCreateEnvironmentRequiresImage(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createEnvironmentRequest{Name: "my-env"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/environments (no image) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLaunchEnvironment(t *testing.T) {
	deps := testDeps()
	mgr := &fakeEnvironmentManager{launchResult: environments.Instance{ID: "abc123", EnvironmentName: "WebSearch"}}
	deps.Instances = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/WebSearch/launch", bytes.NewReader([]byte(`{}`)))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/environments/WebSearch/launch status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(mgr.launched) != 1 || mgr.launched[0] != "WebSearch" {
		t.Errorf("mgr.launched = %v, want [WebSearch]", mgr.launched)
	}
}

func TestLaunchEnvironmentError(t *testing.T) {
	deps := testDeps()
	deps.Instances = &fakeEnvironmentManager{launchErr: errors.New("docker daemon unreachable")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/WebSearch/launch", bytes.NewReader([]byte(`{}`)))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST /api/environments/WebSearch/launch (error) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestListInstances(t *testing.T) {
	deps := testDeps()
	deps.Instances = &fakeEnvironmentManager{listResult: []environments.Instance{{ID: "abc123", EnvironmentName: "WebSearch"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments/instances", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/environments/instances status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []environments.Instance
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "abc123" {
		t.Errorf("GET /api/environments/instances body = %+v, want a single abc123 entry", got)
	}
}

func TestStopInstance(t *testing.T) {
	deps := testDeps()
	mgr := &fakeEnvironmentManager{}
	deps.Instances = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/instances/abc123/stop", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST .../stop status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(mgr.stoppedIDs) != 1 || mgr.stoppedIDs[0] != "abc123" {
		t.Errorf("mgr.stoppedIDs = %v, want [abc123]", mgr.stoppedIDs)
	}
}

func TestStartExec(t *testing.T) {
	deps := testDeps()
	mgr := &fakeEnvironmentManager{execResult: &environments.Exec{ID: "exec-1", Status: environments.ExecRunning}}
	deps.Instances = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(startExecRequest{Command: "echo hi"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/instances/abc123/exec", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST .../exec status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(mgr.execCalls) != 1 || mgr.execCalls[0] != "echo hi" {
		t.Errorf("mgr.execCalls = %v, want [echo hi]", mgr.execCalls)
	}
}

func TestGetExec(t *testing.T) {
	deps := testDeps()
	deps.Instances = &fakeEnvironmentManager{getExecResult: &environments.Exec{ID: "exec-1", Status: environments.ExecDone}, getExecOK: true}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments/instances/abc123/execs/exec-1", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../execs/exec-1 status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetExecNotFound(t *testing.T) {
	deps := testDeps()
	deps.Instances = &fakeEnvironmentManager{getExecOK: false}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments/instances/abc123/execs/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET .../execs/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
