// Package ollama is a small client for the local Ollama API
// (https://github.com/ollama/ollama), used to list locally-pulled models and
// to run tiny local models for tasks like dataset variation generation.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is where Ollama listens by default.
const DefaultBaseURL = "http://localhost:11434"

// Client talks to a local Ollama server.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a Client for the Ollama server at baseURL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// ModelInfo describes a single locally-pulled Ollama model.
type ModelInfo struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

// tagsResponse mirrors Ollama's GET /api/tags response.
type tagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ListModels returns every model Ollama has pulled locally.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode Ollama response: %w", err)
	}

	return tags.Models, nil
}

// deleteRequest mirrors Ollama's DELETE /api/delete request body.
type deleteRequest struct {
	Model string `json:"model"`
}

// DeleteModel removes a locally-pulled Ollama model.
func (c *Client) DeleteModel(ctx context.Context, name string) error {
	body, err := json.Marshal(deleteRequest{Model: name})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, respBody)
	}

	return nil
}

// generateRequest mirrors Ollama's POST /api/generate request body.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse mirrors Ollama's POST /api/generate response body (with
// stream: false, Ollama returns the whole completion as one JSON object).
type generateResponse struct {
	Response string `json:"response"`
}

// Generate runs model on prompt and returns its full completion.
func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	body, err := json.Marshal(generateRequest{Model: model, Prompt: prompt, Stream: false})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, respBody)
	}

	var generated generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&generated); err != nil {
		return "", fmt.Errorf("decode Ollama response: %w", err)
	}

	return generated.Response, nil
}
