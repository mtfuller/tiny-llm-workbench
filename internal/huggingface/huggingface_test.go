package huggingface

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchModelsParsesResults(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"mlx-community/Qwen2.5-0.5B-Instruct-4bit","downloads":12345,"likes":20,"tags":["mlx","4-bit","text-generation"],"lastModified":"2025-03-05T03:15:11.000Z","config":{"model_type":"qwen2"}},
			{"id":"mlx-community/Llama-3.2-1B-Instruct-4bit","downloads":999,"likes":5,"tags":["mlx"],"lastModified":"2025-01-01T00:00:00.000Z"}
		]`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	models, err := c.SearchModels(context.Background(), "qwen 0.5b")
	if err != nil {
		t.Fatalf("SearchModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].ID != "mlx-community/Qwen2.5-0.5B-Instruct-4bit" || models[0].Downloads != 12345 || models[0].Likes != 20 {
		t.Errorf("models[0] = %+v, want the Qwen entry parsed", models[0])
	}
	if len(models[0].Tags) != 3 || models[0].LastModified.Year() != 2025 {
		t.Errorf("models[0] tags/date = %v / %v", models[0].Tags, models[0].LastModified)
	}
	// The request is scoped to mlx-community and passes the search term.
	for _, want := range []string{"author=mlx-community", "search=qwen", "sort=downloads"} {
		if !contains(gotQuery, want) {
			t.Errorf("request query %q missing %q", gotQuery, want)
		}
	}
}

func TestSearchModelsEmptyQueryOmitsSearchParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.SearchModels(context.Background(), "   "); err != nil {
		t.Fatalf("SearchModels() error = %v", err)
	}
	if contains(gotQuery, "search=") {
		t.Errorf("query %q should not include a search param for a blank query", gotQuery)
	}
}

func TestSearchModelsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.SearchModels(context.Background(), "x"); err == nil {
		t.Error("SearchModels() error = nil, want an error on a non-200 response")
	}
}

func TestIsMLXCommunityRepo(t *testing.T) {
	cases := map[string]bool{
		"mlx-community/Qwen2.5-0.5B-Instruct-4bit": true,
		"mlx-community/":             false,
		"meta-llama/Llama-3.2-1B":    false,
		"Qwen2.5-0.5B-Instruct-4bit": false,
		"mlx-community/nested/repo":  false,
	}
	for id, want := range cases {
		if got := IsMLXCommunityRepo(id); got != want {
			t.Errorf("IsMLXCommunityRepo(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestRepoShortName(t *testing.T) {
	if got := RepoShortName("mlx-community/Qwen2.5-0.5B-Instruct-4bit"); got != "Qwen2.5-0.5B-Instruct-4bit" {
		t.Errorf("RepoShortName() = %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
