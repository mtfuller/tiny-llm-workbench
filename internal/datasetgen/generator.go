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

	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
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

	// Flag every generated pair as unreviewed AI output so the dataset UI
	// can warn against training on it before a human has approved it.
	for i := range examples {
		examples[i].Source = "ai"
		examples[i].Approved = false
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

// parseExamples extracts a list of examples from an LLM response, tolerating
// the ways small local models mangle "respond with only a JSON array":
// surrounding prose or markdown fences, a trailing comma before the closing
// bracket, a dropped array wrapper (a bare object, or objects separated by
// newlines), and individual malformed objects mixed in with good ones.
func parseExamples(response string) ([]registry.Example, error) {
	raw, ok := firstJSONArray(response)
	if !ok {
		// No array at all — stitch together whatever balanced {...} objects
		// are present (bare object, or newline-separated objects).
		objs := allJSONObjects(response)
		if len(objs) == 0 {
			return nil, fmt.Errorf("no JSON array found in response: %q", response)
		}
		raw = "[" + strings.Join(objs, ",") + "]"
	}

	raw = stripTrailingCommas(raw)

	var examples []registry.Example
	err := json.Unmarshal([]byte(raw), &examples)
	if err == nil {
		return examples, nil
	}

	// The array wrapper was fine but something inside it didn't parse — pull
	// the objects out one at a time and keep the usable ones.
	examples = examples[:0]
	for _, obj := range allJSONObjects(raw) {
		var ex registry.Example
		if json.Unmarshal([]byte(stripTrailingCommas(obj)), &ex) == nil && (ex.Input != "" || ex.Output != "") {
			examples = append(examples, ex)
		}
	}
	if len(examples) == 0 {
		return nil, fmt.Errorf("invalid JSON array: %w", err)
	}
	return examples, nil
}

// firstJSONArray returns the first balanced [...] substring in s (ignoring
// brackets inside string literals), or ok=false if there isn't one.
func firstJSONArray(s string) (string, bool) {
	for {
		i := strings.IndexByte(s, '[')
		if i == -1 {
			return "", false
		}
		if v, ok := assertions.ExtractJSONValue(s[i:]); ok {
			return v, true
		}
		s = s[i+1:]
	}
}

// allJSONObjects returns every balanced {...} substring in s, in order.
func allJSONObjects(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '{')
		if i == -1 {
			return out
		}
		v, ok := assertions.ExtractJSONValue(s[i:])
		if !ok {
			return out
		}
		out = append(out, v)
		s = s[i+len(v):]
	}
}

// stripTrailingCommas removes a comma that's followed only by whitespace and
// a closing } or ] — the trailing-comma mistake small models routinely make
// in JSON arrays. Commas inside string literals are left untouched.
func stripTrailingCommas(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inString {
			b.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		if c == '"' {
			inString = true
			b.WriteByte(c)
			continue
		}

		if c == ',' {
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				continue // drop the trailing comma
			}
		}

		b.WriteByte(c)
	}

	return b.String()
}
