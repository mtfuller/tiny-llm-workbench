package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestListTools(t *testing.T) {
	deps := testDeps()
	deps.Tools = &fakeToolStore{list: []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tools status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.Tool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "read_file" {
		t.Errorf("GET /api/tools body = %+v, want a single read_file entry", got)
	}
}

func TestListToolsEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/tools (empty) body = %q, want %q", got, "[]")
	}
}

func TestListToolsNilParametersIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	deps.Tools = &fakeToolStore{list: []registry.Tool{{Name: "read_file", Command: "cat {{path}}"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"parameters":[]`) {
		t.Errorf("GET /api/tools body = %q, want parameters to serialize as []", rec.Body.String())
	}
}

func TestCreateTool(t *testing.T) {
	deps := testDeps()
	store := &fakeToolStore{}
	deps.Tools = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveToolRequest{Name: "read_file", Command: "cat {{path}}"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tools", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/tools status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "read_file" {
		t.Errorf("store.saved = %+v, want [read_file]", store.saved)
	}
}

func TestCreateToolRequiresNameAndCommand(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveToolRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tools", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/tools (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetTool(t *testing.T) {
	deps := testDeps()
	deps.Tools = &fakeToolStore{get: registry.Tool{Name: "read_file", Command: "cat {{path}}"}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools/read_file", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tools/read_file status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetToolNotFound(t *testing.T) {
	deps := testDeps()
	deps.Tools = &fakeToolStore{getErr: errors.New("tool not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/tools/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateTool(t *testing.T) {
	deps := testDeps()
	store := &fakeToolStore{get: registry.Tool{Name: "read_file", Command: "cat -A {{path}}"}}
	deps.Tools = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveToolRequest{Command: "cat -A {{path}}"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/tools/read_file", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/tools/read_file status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Command != "cat -A {{path}}" {
		t.Errorf("store.saved = %+v, want the updated command", store.saved)
	}
}

func TestUpdateToolRequiresCommand(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(saveToolRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/tools/read_file", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT /api/tools/read_file (no command) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTool(t *testing.T) {
	deps := testDeps()
	store := &fakeToolStore{}
	deps.Tools = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tools/read_file", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/tools/read_file status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "read_file" {
		t.Errorf("store.deleted = %v, want [read_file]", store.deleted)
	}
}

func TestDeleteToolNotFound(t *testing.T) {
	deps := testDeps()
	deps.Tools = &fakeToolStore{deleteErr: errors.New("tool not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tools/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/tools/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
