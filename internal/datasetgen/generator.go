// Package datasetgen generates dataset variations from a single example
// using a local MLX model, so users don't have to hand-write every
// training pair.
package datasetgen

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// checkResolvedModel fails fast if model is a bare name mlx-lm can't load —
// neither an "org/repo" Hugging Face id nor a local path. Without this such a
// value reaches mlx_lm.server, which retries a 401'd Hub download until the
// client times out (the "generate just hangs" report).
func checkResolvedModel(model string) error {
	if strings.Contains(model, "/") || filepath.IsAbs(model) {
		return nil
	}
	return fmt.Errorf("model %q is not a known registry model or a full Hugging Face repo id (\"org/name\") — pick one from the list", model)
}

// llmClient is the subset of mlxrunner.Runner that Generator needs.
type llmClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// modelResolver turns a model reference the UI picker supplies (which is a
// registry model *name*, e.g. "Llama-3.2-1B-Instruct-4bit") into the path /
// repo id mlx-lm actually needs — see registry.ResolveModelRef. Without this
// an org-less name reaches mlx_lm.server, which retries a 401'd Hub download
// until the client times out (the "generate just hangs" symptom).
type modelResolver interface {
	ResolveModelRef(ref string) string
}

// Generator produces new dataset examples by asking a local LLM for
// variations on a seed example.
type Generator struct {
	llm      llmClient
	resolver modelResolver
}

// New builds a Generator that prompts models via llm, resolving a picked
// registry model name through resolver first (nil is allowed — the model
// string is then used as-is).
func New(llm llmClient, resolver modelResolver) *Generator {
	return &Generator{llm: llm, resolver: resolver}
}

// Variations asks model for n examples similar in style to seed, and returns
// them parsed as Examples. model may be a registry model name (resolved to
// its path / repo id first) or a raw repo id / path.
func (g *Generator) Variations(ctx context.Context, model string, seed registry.Example, n int) ([]registry.Example, error) {
	if n <= 0 {
		return nil, nil
	}

	if g.resolver != nil {
		model = g.resolver.ResolveModelRef(model)
	}
	if err := checkResolvedModel(model); err != nil {
		return nil, err
	}

	response, err := g.llm.Generate(ctx, model, prompt(seed, n))
	if err != nil {
		return nil, fmt.Errorf("generate variations: %w", err)
	}

	examples, err := parseExamples(response)
	if err != nil {
		return nil, fmt.Errorf("parse generated variations: %w", err)
	}

	return examples, nil
}

func prompt(seed registry.Example, n int) string {
	return fmt.Sprintf(`You generate training data for fine-tuning a small language model.

Given this example input/output pair:
Input: %s
Output: %s

Generate %d new, different example pairs that follow the same style and task, but vary the
wording and content. Respond with ONLY a JSON array of objects, each with an "input" and
"output" string field. Do not include any other text.`, seed.Input, seed.Output, n)
}

// parseExamples extracts a JSON array of examples from an LLM response,
// tolerating surrounding prose or markdown code fences.
func parseExamples(response string) ([]registry.Example, error) {
	start := strings.IndexByte(response, '[')
	end := strings.LastIndexByte(response, ']')
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("no JSON array found in response: %q", response)
	}

	var examples []registry.Example
	if err := json.Unmarshal([]byte(response[start:end+1]), &examples); err != nil {
		return nil, fmt.Errorf("invalid JSON array: %w", err)
	}

	return examples, nil
}
