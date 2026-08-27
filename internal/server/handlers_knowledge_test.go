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

func TestListKnowledgeBases(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{list: []registry.KnowledgeBase{{Name: "faq"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/knowledge status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []registry.KnowledgeBase
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].Name != "faq" {
		t.Errorf("GET /api/knowledge body = %+v, want a single faq entry", got)
	}
}

func TestListKnowledgeBasesEmptyIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	handler.ServeHTTP(rec, req)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("GET /api/knowledge (empty) body = %q, want %q", got, "[]")
	}
}

func TestListKnowledgeBasesNilRecordsIsJSONArrayNotNull(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{list: []registry.KnowledgeBase{{Name: "faq"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"records":[]`) {
		t.Errorf("GET /api/knowledge body = %q, want records to serialize as []", rec.Body.String())
	}
}

func TestCreateKnowledgeBase(t *testing.T) {
	deps := testDeps()
	store := &fakeKnowledgeStore{}
	deps.Knowledge = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createKnowledgeBaseRequest{Name: "faq", Description: "Frequently asked questions"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/knowledge", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/knowledge status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].Name != "faq" {
		t.Errorf("store.saved = %+v, want [faq]", store.saved)
	}
}

func TestCreateKnowledgeBaseRequiresName(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(createKnowledgeBaseRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/knowledge", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/knowledge (no name) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetKnowledgeBase(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{get: registry.KnowledgeBase{Name: "faq"}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/faq", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/knowledge/faq status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetKnowledgeBaseNotFound(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{getErr: errors.New("not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/knowledge/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteKnowledgeBase(t *testing.T) {
	deps := testDeps()
	store := &fakeKnowledgeStore{}
	deps.Knowledge = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/knowledge/faq", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/knowledge/faq status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "faq" {
		t.Errorf("store.deleted = %v, want [faq]", store.deleted)
	}
}

func TestDeleteKnowledgeBaseNotFound(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{deleteErr: errors.New("not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/knowledge/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/knowledge/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAddRecords(t *testing.T) {
	deps := testDeps()
	store := &fakeKnowledgeStore{}
	deps.Knowledge = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addRecordsRequest{Records: []registry.KnowledgeRecord{{Title: "Refunds", Content: "..."}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/knowledge/faq/records", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../records status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(store.addedRecords) != 1 || len(store.addedRecords[0]) != 1 {
		t.Errorf("store.addedRecords = %+v, want a single batch of one record", store.addedRecords)
	}
}

func TestAddRecordsRequiresAtLeastOne(t *testing.T) {
	deps := testDeps()

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(addRecordsRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/knowledge/faq/records", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../records (empty) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateRecord(t *testing.T) {
	deps := testDeps()
	store := &fakeKnowledgeStore{}
	deps.Knowledge = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.KnowledgeRecord{Title: "Updated"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/knowledge/faq/records/0", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT .../records/0 status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.updatedRecords) != 1 || store.updatedRecordIdx[0] != 0 {
		t.Errorf("store.updatedRecords = %+v, index = %v, want a single index-0 entry", store.updatedRecords, store.updatedRecordIdx)
	}
}

func TestUpdateRecordError(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{updateRecordErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(registry.KnowledgeRecord{Title: "x"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/knowledge/faq/records/9", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("PUT .../records/9 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteRecord(t *testing.T) {
	deps := testDeps()
	store := &fakeKnowledgeStore{}
	deps.Knowledge = store

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/knowledge/faq/records/0", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE .../records/0 status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(store.deletedRecordIdx) != 1 || store.deletedRecordIdx[0] != 0 {
		t.Errorf("store.deletedRecordIdx = %v, want [0]", store.deletedRecordIdx)
	}
}

func TestDeleteRecordError(t *testing.T) {
	deps := testDeps()
	deps.Knowledge = &fakeKnowledgeStore{deleteRecordErr: errors.New("index out of range")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/knowledge/faq/records/9", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE .../records/9 (error) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
