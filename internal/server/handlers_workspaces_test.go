package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListWorkspaces(t *testing.T) {
	deps := testDeps()
	deps.Workspaces = &fakeWorkspaceStore{list: []registry.Workspace{{Name: "scratch", Type: registry.WorkspaceTest, HostPath: "/x/files"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []registry.Workspace
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "scratch" || got[0].Type != registry.WorkspaceTest {
		t.Errorf("GET /api/workspaces = %+v, want a single test workspace", got)
	}
}

func TestListWorkspacesEmptyIsJSONArrayNotNull(t *testing.T) {
	handler, err := New(testDeps())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Errorf("GET /api/workspaces body = %q, want an empty JSON array", body)
	}
}

func TestCreateTestWorkspace(t *testing.T) {
	deps := testDeps()
	store := &fakeWorkspaceStore{}
	deps.Workspaces = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createWorkspaceRequest{Name: "scratch", Type: "test"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspaces status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "scratch" || store.saved[0].Type != registry.WorkspaceTest {
		t.Errorf("store.saved = %+v, want a single test workspace", store.saved)
	}
}

func TestCreateRealWorkspaceValidatesDirectory(t *testing.T) {
	deps := testDeps()
	store := &fakeWorkspaceStore{}
	deps.Workspaces = store
	handler, _ := New(deps)

	// A path that doesn't exist -> 400.
	body, _ := json.Marshal(createWorkspaceRequest{Name: "proj", Type: "real", HostPath: "/definitely/not/here"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST real workspace with a bad path status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// An existing directory -> created.
	dir := t.TempDir()
	body, _ = json.Marshal(createWorkspaceRequest{Name: "proj", Type: "real", HostPath: dir})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST real workspace with a real dir status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Type != registry.WorkspaceReal || store.saved[0].HostPath != dir {
		t.Errorf("store.saved = %+v, want a real workspace pointing at %q", store.saved, dir)
	}
}

func TestCreateWorkspaceRejectsUnknownType(t *testing.T) {
	handler, _ := New(testDeps())
	body, _ := json.Marshal(createWorkspaceRequest{Name: "x", Type: "hybrid"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/workspaces (bad type) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	deps := testDeps()
	deps.Workspaces = &fakeWorkspaceStore{getErr: errors.New("workspace not found")}
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/workspaces/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteWorkspace(t *testing.T) {
	deps := testDeps()
	store := &fakeWorkspaceStore{}
	deps.Workspaces = store
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/workspaces/scratch", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/workspaces/scratch status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "scratch" {
		t.Errorf("store.deleted = %v, want [scratch]", store.deleted)
	}
}

func TestLaunchWorkspace(t *testing.T) {
	deps := testDeps()
	mgr := &fakeWorkspaceManager{launchResult: environments.Instance{ID: "abc123", WorkspaceName: "scratch", WorkspacePath: "/runs/abc123"}}
	deps.Instances = mgr
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/scratch/launch", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspaces/scratch/launch status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got environments.Instance
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != "abc123" || got.WorkspacePath != "/runs/abc123" {
		t.Errorf("launch response = %+v, want the instance with its staged path", got)
	}
	if len(mgr.launched) != 1 || mgr.launched[0] != "scratch" {
		t.Errorf("mgr.launched = %v, want [scratch]", mgr.launched)
	}
}

func TestLaunchWorkspaceError(t *testing.T) {
	deps := testDeps()
	deps.Instances = &fakeWorkspaceManager{launchErr: errors.New("docker daemon unreachable")}
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/scratch/launch", nil))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST .../launch (docker down) status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestListInstances(t *testing.T) {
	deps := testDeps()
	deps.Instances = &fakeWorkspaceManager{listResult: []environments.Instance{{ID: "abc123", WorkspaceName: "scratch"}}}
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces/instances", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces/instances status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []environments.Instance
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].WorkspaceName != "scratch" {
		t.Errorf("instances = %+v, want a single scratch instance", got)
	}
}

func TestStopInstance(t *testing.T) {
	deps := testDeps()
	mgr := &fakeWorkspaceManager{}
	deps.Instances = mgr
	handler, _ := New(deps)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/instances/abc123/stop", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST .../stop status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(mgr.stoppedIDs) != 1 || mgr.stoppedIDs[0] != "abc123" {
		t.Errorf("mgr.stoppedIDs = %v, want [abc123]", mgr.stoppedIDs)
	}
}

func TestStartExecAndGetExec(t *testing.T) {
	deps := testDeps()
	mgr := &fakeWorkspaceManager{
		execResult:    &environments.Exec{ID: "exec-1", Status: environments.ExecRunning},
		getExecResult: &environments.Exec{ID: "exec-1", Status: environments.ExecDone},
		getExecOK:     true,
	}
	deps.Instances = mgr
	handler, _ := New(deps)

	body, _ := json.Marshal(startExecRequest{Command: "ls -la"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/instances/abc123/exec", bytes.NewReader(body)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST .../exec status = %d, want %d, body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(mgr.execCalls) != 1 || mgr.execCalls[0] != "ls -la" {
		t.Errorf("mgr.execCalls = %v, want [ls -la]", mgr.execCalls)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces/instances/abc123/execs/exec-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../execs/exec-1 status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestListDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/alpha", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root+"/beta", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/file.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler, _ := New(testDeps())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fs/list?path="+root, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/fs/list status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got listDirectoryResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Path != root || got.Parent == "" {
		t.Errorf("listing = %+v, want path=%q and a non-empty parent", got, root)
	}
	if len(got.Entries) != 2 || got.Entries[0].Name != "alpha" || got.Entries[1].Name != "beta" {
		t.Errorf("entries = %+v, want [alpha, beta] (directories only, sorted)", got.Entries)
	}
}

func TestListDirectoryRejectsRelativePath(t *testing.T) {
	handler, _ := New(testDeps())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fs/list?path=relative/dir", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET /api/fs/list?path=relative status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
