package datasetgen

import (
	"context"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeLLM struct {
	response string
	err      error
	gotModel string
}

func (f *fakeLLM) Generate(ctx context.Context, model, prompt string) (string, error) {
	f.gotModel = model
	return f.response, f.err
}

func TestVariationsParsesJSONArray(t *testing.T) {
	llm := &fakeLLM{response: `[{"input":"hi","output":"hello!"},{"input":"yo","output":"hey!"}]`}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "hey", Output: "hi there!"}, 2)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Variations() = %+v, want 2 examples", got)
	}
	if got[0].Input != "hi" || got[0].Output != "hello!" {
		t.Errorf("Variations()[0] = %+v, want {hi hello!}", got[0])
	}
	for i, ex := range got {
		if ex.Source != "ai" || ex.Approved {
			t.Errorf("Variations()[%d] = %+v, want Source=\"ai\" and Approved=false", i, ex)
		}
	}
	if llm.gotModel != "mlx-community/Qwen2.5-0.5B-Instruct-4bit" {
		t.Errorf("Generate() called with model = %q, want %q", llm.gotModel, "mlx-community/Qwen2.5-0.5B-Instruct-4bit")
	}
}

func TestVariationsTolerateSurroundingProseAndFences(t *testing.T) {
	llm := &fakeLLM{response: "Sure! Here you go:\n```json\n[{\"input\":\"hi\",\"output\":\"hello!\"}]\n```\nHope that helps."}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "hey", Output: "hi there!"}, 1)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 1 || got[0].Input != "hi" {
		t.Errorf("Variations() = %+v, want a single {hi hello!} example", got)
	}
}

func TestVariationsInvalidJSON(t *testing.T) {
	llm := &fakeLLM{response: "I can't help with that."}
	gen := New(llm, nil)

	if _, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "hey", Output: "hi there!"}, 1); err == nil {
		t.Error("Variations() error = nil, want an error when the response has no JSON array")
	}
}

// Llama 3.2 1B routinely leaves a trailing comma before the closing bracket,
// which encoding/json rejects with "invalid character ']' looking for
// beginning of value" — the reported bug. parseExamples now strips it.
func TestVariationsToleratesTrailingComma(t *testing.T) {
	llm := &fakeLLM{response: "[\n  {\"input\": \"a\", \"output\": \"b\"},\n  {\"input\": \"c\", \"output\": \"d\"},\n]"}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "x", Output: "y"}, 2)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 || got[1].Input != "c" || got[1].Output != "d" {
		t.Errorf("Variations() = %+v, want the two examples with the trailing comma ignored", got)
	}
}

// A stray ']' in trailing prose used to be swallowed by LastIndexByte(']'),
// producing a garbled span. The balanced scan stops at the real array end.
func TestVariationsIgnoresBracketsInSurroundingProse(t *testing.T) {
	llm := &fakeLLM{response: "Here are the pairs:\n[{\"input\": \"a\", \"output\": \"b\"}]\nNote: keep values in [square brackets] if needed."}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "x", Output: "y"}, 1)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 1 || got[0].Input != "a" {
		t.Errorf("Variations() = %+v, want a single {a b} example", got)
	}
}

// A comma-then-bracket sequence *inside* a string value must not be treated
// as a trailing comma.
func TestVariationsKeepsCommasInsideStrings(t *testing.T) {
	llm := &fakeLLM{response: `[{"input": "list: a, b,", "output": "ok, ]done"}]`}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "x", Output: "y"}, 1)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 1 || got[0].Input != "list: a, b," || got[0].Output != "ok, ]done" {
		t.Errorf("Variations() = %+v, want the string contents preserved verbatim", got)
	}
}

// Some models drop the array wrapper and emit newline-separated objects.
func TestVariationsStitchesBareObjects(t *testing.T) {
	llm := &fakeLLM{response: "{\"input\": \"a\", \"output\": \"b\"}\n{\"input\": \"c\", \"output\": \"d\"}"}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "x", Output: "y"}, 2)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 || got[0].Input != "a" || got[1].Input != "c" {
		t.Errorf("Variations() = %+v, want both bare objects collected", got)
	}
}

// A malformed object mixed in with good ones is skipped, not fatal.
func TestVariationsSkipsMalformedObjectsInArray(t *testing.T) {
	llm := &fakeLLM{response: `[{"input": "a", "output": "b"}, {"input": "c", "output": }, {"input": "e", "output": "f"}]`}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "x", Output: "y"}, 3)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 || got[0].Input != "a" || got[1].Input != "e" {
		t.Errorf("Variations() = %+v, want the two well-formed examples", got)
	}
}

type fakeResolver struct{ resolved map[string]string }

func (f *fakeResolver) ResolveModelRef(ref string) string {
	if r, ok := f.resolved[ref]; ok {
		return r
	}
	return ref
}

// A registry model NAME picked in the UI is resolved to its path / repo id
// before mlx-lm sees it — otherwise an org-less name 401s and the request
// hangs until the client times out.
func TestVariationsResolvesModelName(t *testing.T) {
	llm := &fakeLLM{response: `[{"input":"hi","output":"hello!"}]`}
	gen := New(llm, &fakeResolver{resolved: map[string]string{
		"Llama-3.2-1B-Instruct-4bit": "mlx-community/Llama-3.2-1B-Instruct-4bit",
	}})

	if _, err := gen.Variations(context.Background(), "Llama-3.2-1B-Instruct-4bit", registry.Example{Input: "a", Output: "b"}, 1); err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if llm.gotModel != "mlx-community/Llama-3.2-1B-Instruct-4bit" {
		t.Errorf("Generate() model = %q, want the resolved repo id", llm.gotModel)
	}
}

// A bare model name that resolves to nothing (a typo, or a model never
// added) fails fast with a clear message instead of hanging until the
// mlx_lm.server client times out.
func TestVariationsRejectsUnresolvableBareName(t *testing.T) {
	llm := &fakeLLM{response: `[]`}
	gen := New(llm, &fakeResolver{}) // resolves nothing

	_, err := gen.Variations(context.Background(), "Some-Random-Model", registry.Example{Input: "a", Output: "b"}, 1)
	if err == nil {
		t.Fatal("Variations() error = nil, want a fast failure for an unresolvable bare name")
	}
	if llm.gotModel != "" {
		t.Errorf("Generate() was called with %q — the guard should fail before any model call", llm.gotModel)
	}
}

func TestVariationsZeroRequested(t *testing.T) {
	llm := &fakeLLM{}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", registry.Example{Input: "hey", Output: "hi there!"}, 0)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if got != nil {
		t.Errorf("Variations() = %+v, want nil for n=0", got)
	}
}
