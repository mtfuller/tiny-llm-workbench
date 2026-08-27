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
	gen := New(llm)

	got, err := gen.Variations(context.Background(), "qwen2.5:0.5b", "say hello", 2)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Variations() = %+v, want 2 prompts", got)
	}
	if got[0] != "say good morning" || got[1] != "greet someone politely" {
		t.Errorf("Variations() = %+v, want [say good morning, greet someone politely]", got)
	}
	if llm.gotModel != "qwen2.5:0.5b" {
		t.Errorf("Generate() called with model = %q, want %q", llm.gotModel, "qwen2.5:0.5b")
	}
}

func TestVariationsTolerateSurroundingProseAndFences(t *testing.T) {
	llm := &fakeLLM{response: "Sure! Here you go:\n```json\n[\"say good morning\"]\n```\nHope that helps."}
	gen := New(llm)

	got, err := gen.Variations(context.Background(), "qwen2.5:0.5b", "say hello", 1)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 1 || got[0] != "say good morning" {
		t.Errorf("Variations() = %+v, want a single [say good morning]", got)
	}
}

func TestVariationsInvalidJSON(t *testing.T) {
	llm := &fakeLLM{response: "I can't help with that."}
	gen := New(llm)

	if _, err := gen.Variations(context.Background(), "qwen2.5:0.5b", "say hello", 1); err == nil {
		t.Error("Variations() error = nil, want an error when the response has no JSON array")
	}
}

func TestVariationsZeroRequested(t *testing.T) {
	llm := &fakeLLM{}
	gen := New(llm)

	got, err := gen.Variations(context.Background(), "qwen2.5:0.5b", "say hello", 0)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if got != nil {
		t.Errorf("Variations() = %+v, want nil for n=0", got)
	}
}
