package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// normalizeKnowledgeBase ensures Records never serializes as JSON "null".
func normalizeKnowledgeBase(kb registry.KnowledgeBase) registry.KnowledgeBase {
	if kb.Records == nil {
		kb.Records = []registry.KnowledgeRecord{}
	}
	return kb
}

// listKnowledgeBasesHandler responds with every registry-tracked knowledge
// base.
func listKnowledgeBasesHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListKnowledgeBases()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		normalized := make([]registry.KnowledgeBase, len(list))
		for i, kb := range list {
			normalized[i] = normalizeKnowledgeBase(kb)
		}
		writeJSON(w, http.StatusOK, normalized)
	}
}

// createKnowledgeBaseRequest is the POST /api/knowledge request body.
type createKnowledgeBaseRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// createKnowledgeBaseHandler saves a new knowledge base. It starts with no
// records — those are added afterward from its own detail page, the same
// way a Dataset/Benchmark starts empty.
func createKnowledgeBaseHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createKnowledgeBaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		kb := registry.KnowledgeBase{Name: req.Name, Description: req.Description}
		if err := store.SaveKnowledgeBase(kb); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, normalizeKnowledgeBase(kb))
	}
}

// getKnowledgeBaseHandler responds with a single knowledge base.
func getKnowledgeBaseHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := store.GetKnowledgeBase(r.PathValue("name"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeKnowledgeBase(kb))
	}
}

// deleteKnowledgeBaseHandler removes a knowledge base.
func deleteKnowledgeBaseHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteKnowledgeBase(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// addRecordsRequest is the POST /api/knowledge/{name}/records request body.
type addRecordsRequest struct {
	Records []registry.KnowledgeRecord `json:"records"`
}

// addRecordsHandler appends new records to a knowledge base.
func addRecordsHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req addRecordsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if len(req.Records) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("at least one record is required"))
			return
		}

		if err := store.AddRecords(name, req.Records); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		kb, err := store.GetKnowledgeBase(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, normalizeKnowledgeBase(kb))
	}
}

// updateRecordHandler overwrites a single record, addressed by its position
// in the knowledge base (as returned by GET /api/knowledge/{name}).
func updateRecordHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid record index: %w", err))
			return
		}

		var record registry.KnowledgeRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		if err := store.UpdateRecord(name, index, record); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		kb, err := store.GetKnowledgeBase(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, normalizeKnowledgeBase(kb))
	}
}

// deleteRecordHandler removes a single record, addressed by its position in
// the knowledge base (as returned by GET /api/knowledge/{name}).
func deleteRecordHandler(store knowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid record index: %w", err))
			return
		}

		if err := store.DeleteRecord(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
