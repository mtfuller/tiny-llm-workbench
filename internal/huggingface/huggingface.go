// Package huggingface is a tiny read-only client for the Hugging Face Hub's
// public model-search API, used by the Models page to discover mlx-community
// models without leaving the app. It only ever performs GET requests to the
// Hub's JSON API, triggered by an explicit user search — there is no
// background polling, no auth, and nothing is uploaded.
package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultBaseURL is the Hugging Face Hub API root. Overridable on Client for
// tests.
const defaultBaseURL = "https://huggingface.co"

// mlxCommunityAuthor is the only org searched — it publishes MLX-ready
// conversions of small models, so every result is loadable by mlx_lm.
const mlxCommunityAuthor = "mlx-community"

// Model is one search result: just the fields the Models page shows.
type Model struct {
	ID           string    `json:"id"` // e.g. "mlx-community/Qwen2.5-0.5B-Instruct-4bit"
	Downloads    int       `json:"downloads"`
	Likes        int       `json:"likes"`
	Tags         []string  `json:"tags"`
	LastModified time.Time `json:"lastModified"`
}

// Client talks to the Hugging Face Hub API. The zero value is not usable;
// build one with New.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client pointed at the public Hub with a sensible timeout.
func New() *Client {
	return &Client{
		BaseURL: defaultBaseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// SearchModels returns up to 30 mlx-community models matching query, ordered
// by download count (most downloaded first). An empty query lists the most
// downloaded mlx-community models overall.
func (c *Client) SearchModels(ctx context.Context, query string) ([]Model, error) {
	q := url.Values{}
	q.Set("author", mlxCommunityAuthor)
	if s := strings.TrimSpace(query); s != "" {
		q.Set("search", s)
	}
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", "30")
	q.Set("full", "true") // needed for the tags list

	endpoint := c.BaseURL + "/api/models?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build Hugging Face request: %w", err)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach Hugging Face: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hugging Face search returned %s", resp.Status)
	}

	var models []Model
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("parse Hugging Face response: %w", err)
	}
	return models, nil
}

// IsMLXCommunityRepo reports whether id is an "mlx-community/<name>" repo id —
// the only kind the Models page will add.
func IsMLXCommunityRepo(id string) bool {
	rest, ok := strings.CutPrefix(id, mlxCommunityAuthor+"/")
	return ok && rest != "" && !strings.Contains(rest, "/")
}

// RepoShortName is the part of an mlx-community repo id after the "/", used
// as the registry model name (e.g. "Qwen2.5-0.5B-Instruct-4bit").
func RepoShortName(id string) string {
	if i := strings.IndexByte(id, '/'); i >= 0 {
		return id[i+1:]
	}
	return id
}
