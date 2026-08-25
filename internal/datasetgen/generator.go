// Package datasetgen generates dataset variations from a single example
// using a local LLM, so users don't have to hand-write every training pair.
package datasetgen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// llmClient is the subset of ollama.Client that Generator needs.
type llmClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// Generator produces new dataset examples by asking a local LLM for
// variations on a seed example.
type Generator struct {
	llm llmClient
}

// New builds a Generator that prompts models via llm.
func New(llm llmClient) *Generator {
	return &Generator{llm: llm}
}

// Variations asks model (an Ollama model name) for n examples similar in
// style to seed, and returns them parsed as Examples.
func (g *Generator) Variations(ctx context.Context, model string, seed registry.Example, n int) ([]registry.Example, error) {
	if n <= 0 {
		return nil, nil
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
