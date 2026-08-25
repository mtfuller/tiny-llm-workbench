package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("request path = %q, want /api/tags", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tagsResponse{
			Models: []ModelInfo{
				{Name: "qwen2.5:0.5b", Size: 397_000_000, ModifiedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		})
	}))
	defer srv.Close()

	client := New(srv.URL)
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 || models[0].Name != "qwen2.5:0.5b" {
		t.Errorf("ListModels() = %+v, want a single qwen2.5:0.5b entry", models)
	}
}

func TestListModelsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(srv.URL)
	if _, err := client.ListModels(context.Background()); err == nil {
		t.Error("ListModels() error = nil, want an error for a 500 response")
	}
}

func TestGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("request path = %q, want /api/generate", r.URL.Path)
		}

		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if req.Stream {
			t.Error("request Stream = true, want false")
		}
		if !strings.Contains(req.Prompt, "hello") {
			t.Errorf("request Prompt = %q, want it to contain %q", req.Prompt, "hello")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generateResponse{Response: "hi there!"})
	}))
	defer srv.Close()

	client := New(srv.URL)
	got, err := client.Generate(context.Background(), "qwen2.5:0.5b", "hello")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "hi there!" {
		t.Errorf("Generate() = %q, want %q", got, "hi there!")
	}
}

func TestGenerateServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("model not found"))
	}))
	defer srv.Close()

	client := New(srv.URL)
	if _, err := client.Generate(context.Background(), "missing-model", "hello"); err == nil {
		t.Error("Generate() error = nil, want an error for a 400 response")
	}
}
