// Package testcasegen generates benchmark test case prompt variations from a
// single seed prompt using a local MLX model, mirroring internal/datasetgen's
// approach for dataset examples.
//
// Only the prompt varies — a generated variation reuses the seed's own
// assertions unchanged, rather than asking the model to also invent new
// pass/fail criteria. Tiny local models are unreliable at emitting
// structured/parseable output (the same reasoning behind Phase 3's
// keyword-match decision nodes and Evaluations'/Benchmarks' deterministic
// assertions), so having the model generate JSON Schema documents or regex
// patterns it can't validate itself isn't a fight worth having; paraphrasing
// a prompt while the assertions stay fixed is what "test the same thing,
// asked differently" actually means anyway.
package testcasegen

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/assertions"
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

// modelResolver turns a registry model name (what the UI picker supplies)
// into the path / repo id mlx-lm needs — see registry.ResolveModelRef.
// Without it an org-less name reaches mlx_lm.server, which retries a 401'd
// Hub download until the client times out.
type modelResolver interface {
	ResolveModelRef(ref string) string
}

// Generator produces new test case prompts by asking a local LLM for
// rephrasings of a seed prompt.
type Generator struct {
	llm      llmClient
	resolver modelResolver
}

// New builds a Generator that prompts models via llm, resolving a picked
// registry model name through resolver first (nil is allowed).
func New(llm llmClient, resolver modelResolver) *Generator {
	return &Generator{llm: llm, resolver: resolver}
}

// Variations asks model for n prompts that test the same thing as
// seedPrompt, phrased differently, and returns them as plain strings — the
// caller pairs each with the seed's own assertions/tags. model may be a
// registry model name (resolved first) or a raw repo id / path.
func (g *Generator) Variations(ctx context.Context, model, seedPrompt string, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}

	if g.resolver != nil {
		model = g.resolver.ResolveModelRef(model)
	}
	if err := checkResolvedModel(model); err != nil {
		return nil, err
	}

	response, err := g.llm.Generate(ctx, model, prompt(seedPrompt, n))
	if err != nil {
		return nil, fmt.Errorf("generate variations: %w", err)
	}

	prompts, err := parsePrompts(response)
	if err != nil {
		return nil, fmt.Errorf("parse generated variations: %w", err)
	}

	return prompts, nil
}

func prompt(seedPrompt string, n int) string {
	return fmt.Sprintf(`You generate test prompts for evaluating a small language model.

Given this example prompt:
%s

Generate %d new, different prompts that test the same thing in a different way — vary the
wording and phrasing, but keep the same intent, so the same pass/fail criteria still apply.
Respond with ONLY a JSON array of strings. Do not include any other text.`, seedPrompt, n)
}

// parsePrompts extracts a JSON array of strings from an LLM response,
// tolerating the ways small local models mangle "respond with only a JSON
// array": surrounding prose or markdown fences, a trailing comma before the
// closing bracket, a stray "]" in trailing prose, and individual malformed
// entries mixed in with good ones. Mirrors internal/datasetgen.parseExamples.
func parsePrompts(response string) ([]string, error) {
	raw, ok := firstJSONArray(response)
	if !ok {
		return nil, fmt.Errorf("no JSON array found in response: %q", response)
	}

	raw = stripTrailingCommas(raw)

	var prompts []string
	if err := json.Unmarshal([]byte(raw), &prompts); err == nil {
		return nonEmpty(prompts), nil
	} else {
		// The array wrapper was fine but something inside it didn't parse —
		// pull the quoted strings out one at a time and keep the usable ones.
		salvaged := nonEmpty(quotedStrings(raw))
		if len(salvaged) == 0 {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
		return salvaged, nil
	}
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

// quotedStrings returns every JSON string literal in s, decoded — the
// last-resort salvage when an array as a whole won't parse.
func quotedStrings(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '"' {
			continue
		}
		j := i + 1
		for j < len(s) {
			if s[j] == '\\' {
				j += 2
				continue
			}
			if s[j] == '"' {
				break
			}
			j++
		}
		if j >= len(s) {
			break
		}
		var decoded string
		if json.Unmarshal([]byte(s[i:j+1]), &decoded) == nil {
			out = append(out, decoded)
		}
		i = j
	}
	return out
}

// nonEmpty drops blank entries (a salvaged list can pick up empty strings
// from a "" the model emitted between real ones).
func nonEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// stripTrailingCommas removes a comma that's followed only by whitespace and
// a closing } or ] — the trailing-comma mistake small models routinely make.
// Commas inside string literals are left untouched.
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
