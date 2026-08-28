package testcasegen

import (
	"context"
	"testing"
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
	llm := &fakeLLM{response: `["say good morning", "greet someone politely"]`}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", "say hello", 2)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Variations() = %+v, want 2 prompts", got)
	}
	if got[0] != "say good morning" || got[1] != "greet someone politely" {
		t.Errorf("Variations() = %+v, want [say good morning, greet someone politely]", got)
	}
	if llm.gotModel != "mlx-community/Qwen2.5-0.5B-Instruct-4bit" {
		t.Errorf("Generate() called with model = %q, want %q", llm.gotModel, "mlx-community/Qwen2.5-0.5B-Instruct-4bit")
	}
}

type fakeResolver struct{ resolved map[string]string }

func (f *fakeResolver) ResolveModelRef(ref string) string {
	if r, ok := f.resolved[ref]; ok {
		return r
	}
	return ref
}

func TestVariationsResolvesModelName(t *testing.T) {
	llm := &fakeLLM{response: `["a", "b"]`}
	gen := New(llm, &fakeResolver{resolved: map[string]string{
		"Llama-3.2-1B-Instruct-4bit": "mlx-community/Llama-3.2-1B-Instruct-4bit",
	}})

	if _, err := gen.Variations(context.Background(), "Llama-3.2-1B-Instruct-4bit", "say hi", 2); err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if llm.gotModel != "mlx-community/Llama-3.2-1B-Instruct-4bit" {
		t.Errorf("Generate() model = %q, want the resolved repo id", llm.gotModel)
	}
}

func TestVariationsTolerateSurroundingProseAndFences(t *testing.T) {
	llm := &fakeLLM{response: "Sure! Here you go:\n```json\n[\"say good morning\"]\n```\nHope that helps."}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", "say hello", 1)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 1 || got[0] != "say good morning" {
		t.Errorf("Variations() = %+v, want a single [say good morning]", got)
	}
}

func TestVariationsInvalidJSON(t *testing.T) {
	llm := &fakeLLM{response: "I can't help with that."}
	gen := New(llm, nil)

	if _, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", "say hello", 1); err == nil {
		t.Error("Variations() error = nil, want an error when the response has no JSON array")
	}
}

func TestVariationsRejectsUnresolvableBareName(t *testing.T) {
	llm := &fakeLLM{response: `[]`}
	gen := New(llm, &fakeResolver{})

	if _, err := gen.Variations(context.Background(), "Some-Random-Model", "say hi", 1); err == nil {
		t.Fatal("Variations() error = nil, want a fast failure for an unresolvable bare name")
	}
	if llm.gotModel != "" {
		t.Errorf("Generate() was called with %q — the guard should fail first", llm.gotModel)
	}
}

func TestVariationsZeroRequested(t *testing.T) {
	llm := &fakeLLM{}
	gen := New(llm, nil)

	got, err := gen.Variations(context.Background(), "mlx-community/Qwen2.5-0.5B-Instruct-4bit", "say hello", 0)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if got != nil {
		t.Errorf("Variations() = %+v, want nil for n=0", got)
	}
}
