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
	gen := New(llm)

	got, err := gen.Variations(context.Background(), "qwen2.5:0.5b", registry.Example{Input: "hey", Output: "hi there!"}, 2)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Variations() = %+v, want 2 examples", got)
	}
	if got[0].Input != "hi" || got[0].Output != "hello!" {
		t.Errorf("Variations()[0] = %+v, want {hi hello!}", got[0])
	}
	if llm.gotModel != "qwen2.5:0.5b" {
		t.Errorf("Generate() called with model = %q, want %q", llm.gotModel, "qwen2.5:0.5b")
	}
}

func TestVariationsTolerateSurroundingProseAndFences(t *testing.T) {
	llm := &fakeLLM{response: "Sure! Here you go:\n```json\n[{\"input\":\"hi\",\"output\":\"hello!\"}]\n```\nHope that helps."}
	gen := New(llm)

	got, err := gen.Variations(context.Background(), "qwen2.5:0.5b", registry.Example{Input: "hey", Output: "hi there!"}, 1)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if len(got) != 1 || got[0].Input != "hi" {
		t.Errorf("Variations() = %+v, want a single {hi hello!} example", got)
	}
}

func TestVariationsInvalidJSON(t *testing.T) {
	llm := &fakeLLM{response: "I can't help with that."}
	gen := New(llm)

	if _, err := gen.Variations(context.Background(), "qwen2.5:0.5b", registry.Example{Input: "hey", Output: "hi there!"}, 1); err == nil {
		t.Error("Variations() error = nil, want an error when the response has no JSON array")
	}
}

func TestVariationsZeroRequested(t *testing.T) {
	llm := &fakeLLM{}
	gen := New(llm)

	got, err := gen.Variations(context.Background(), "qwen2.5:0.5b", registry.Example{Input: "hey", Output: "hi there!"}, 0)
	if err != nil {
		t.Fatalf("Variations() error = %v", err)
	}
	if got != nil {
		t.Errorf("Variations() = %+v, want nil for n=0", got)
	}
}
