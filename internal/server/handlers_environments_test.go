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

func TestDeleteEnvironment(t *testing.T) {
	deps := testDeps()
	store := &fakeEnvironmentStore{}
	deps.Environments = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/environments/my-env", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/environments/my-env status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "my-env" {
		t.Errorf("store.deleted = %v, want [my-env]", store.deleted)
	}
}

func TestDeleteEnvironmentNotFound(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{deleteErr: errors.New("environment not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/environments/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/environments/missing status = %d, want %d", rec.Code, http.StatusNotFound)
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

func TestGetEnvironment(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{get: registry.Environment{Name: "my-env", Image: "alpine:3.20"}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments/my-env", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/environments/my-env status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got registry.Environment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Name != "my-env" {
		t.Errorf("GET /api/environments/my-env body = %+v, want Name = my-env", got)
	}
}

func TestGetEnvironmentNotFound(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{getErr: errors.New("environment not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/environments/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/environments/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateEnvironmentConfig(t *testing.T) {
	deps := testDeps()
	store := &fakeEnvironmentStore{get: registry.Environment{Name: "my-env", Image: "alpine:3.21"}}
	deps.Environments = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(updateEnvironmentConfigRequest{
		Image:  "alpine:3.21",
		Mounts: []registry.Mount{{HostPath: "/host", ContainerPath: "/container", ReadOnly: true}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/environments/my-env/config", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT .../config status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.updatedConfigs) != 1 || store.updatedConfigs[0].Image != "alpine:3.21" {
		t.Errorf("store.updatedConfigs = %+v, want a single alpine:3.21 entry", store.updatedConfigs)
	}
}

func TestUpdateEnvironmentConfigRequiresImage(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(updateEnvironmentConfigRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/environments/my-env/config", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../config (no image) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateEnvironmentConfigError(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{updateConfigErr: errors.New("environment not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(updateEnvironmentConfigRequest{Image: "alpine:3.21"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/environments/missing/config", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../config (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAddTool(t *testing.T) {
	deps := testDeps()
	store := &fakeEnvironmentStore{get: registry.Environment{Name: "my-env", Tools: []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}}}
	deps.Environments = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Tool{Name: "read_file", Command: "cat {{path}}"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/my-env/tools", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../tools status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.addedTools) != 1 || store.addedTools[0].Name != "read_file" {
		t.Errorf("store.addedTools = %+v, want a single read_file entry", store.addedTools)
	}
}

func TestAddToolRequiresNameAndCommand(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Tool{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/my-env/tools", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../tools (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateTool(t *testing.T) {
	deps := testDeps()
	store := &fakeEnvironmentStore{get: registry.Environment{Name: "my-env", Tools: []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}}}
	deps.Environments = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Tool{Name: "read_file_v2", Command: "cat {{path}}"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/environments/my-env/tools/0", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT .../tools/0 status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.updatedTools) != 1 || store.updatedToolIndex[0] != 0 {
		t.Errorf("store.updatedTools = %+v, index = %v, want a single index-0 entry", store.updatedTools, store.updatedToolIndex)
	}
}

func TestUpdateToolError(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{updateToolErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.Tool{Name: "x", Command: "y"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/environments/my-env/tools/9", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../tools/9 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTool(t *testing.T) {
	deps := testDeps()
	store := &fakeEnvironmentStore{}
	deps.Environments = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/environments/my-env/tools/0", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE .../tools/0 status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deletedToolIndex) != 1 || store.deletedToolIndex[0] != 0 {
		t.Errorf("store.deletedToolIndex = %v, want [0]", store.deletedToolIndex)
	}
}

func TestDeleteToolError(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{deleteToolErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/environments/my-env/tools/9", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE .../tools/9 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTryTool(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{get: registry.Environment{
		Name:  "my-env",
		Tools: []registry.Tool{{Name: "read_file", Command: "cat {{path}}", Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}}}},
	}}
	mgr := &fakeEnvironmentManager{tryToolResult: &environments.Exec{ID: "exec-1", Status: environments.ExecRunning}}
	deps.Instances = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(tryToolRequest{InstanceID: "abc123", Args: map[string]string{"path": "/etc/hosts"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/my-env/tools/0/try", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST .../tools/0/try status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(mgr.tryToolCalls) != 1 || mgr.tryToolCalls[0] != "abc123" {
		t.Errorf("mgr.tryToolCalls = %v, want [abc123]", mgr.tryToolCalls)
	}
}

func TestTryToolRequiresInstanceID(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{get: registry.Environment{Name: "my-env", Tools: []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(tryToolRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/my-env/tools/0/try", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../tools/0/try (no instanceId) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTryToolIndexOutOfRange(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{get: registry.Environment{Name: "my-env", Tools: []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(tryToolRequest{InstanceID: "abc123"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/my-env/tools/9/try", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../tools/9/try (bad index) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTryToolValidationError(t *testing.T) {
	deps := testDeps()
	deps.Environments = &fakeEnvironmentStore{get: registry.Environment{
		Name:  "my-env",
		Tools: []registry.Tool{{Name: "read_file", Command: "cat {{path}}", Parameters: []registry.ToolParameter{{Name: "path", Type: registry.ToolParamString, Required: true}}}},
	}}
	mgr := &fakeEnvironmentManager{tryToolErr: errors.New(`missing required parameter "path"`)}
	deps.Instances = mgr

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(tryToolRequest{InstanceID: "abc123", Args: map[string]string{}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/my-env/tools/0/try", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../tools/0/try (validation error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
