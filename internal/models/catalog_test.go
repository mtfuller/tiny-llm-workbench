package models

import (
	"context"
	"errors"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/ollama"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeRegistry struct {
	models []registry.Model
	err    error
}

func (f *fakeRegistry) ListModels() ([]registry.Model, error) {
	return f.models, f.err
}

type fakeOllama struct {
	models []ollama.ModelInfo
	err    error
}

func (f *fakeOllama) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	return f.models, f.err
}

func TestCatalogListMergesRegistryAndOllama(t *testing.T) {
	reg := &fakeRegistry{models: []registry.Model{{Name: "my-finetune", Source: "mlx"}}}
	oll := &fakeOllama{models: []ollama.ModelInfo{{Name: "qwen2.5:0.5b", Size: 397_000_000}}}

	catalog := NewCatalog(reg, oll)
	got, err := catalog.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	want := []Model{
		{Name: "my-finetune", Source: "mlx"},
		{Name: "qwen2.5:0.5b", Source: "ollama", Size: 397_000_000},
	}
	if len(got) != len(want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCatalogListToleratesOllamaBeingUnavailable(t *testing.T) {
	reg := &fakeRegistry{models: []registry.Model{{Name: "my-finetune", Source: "mlx"}}}
	oll := &fakeOllama{err: errors.New("connection refused")}

	catalog := NewCatalog(reg, oll)
	got, err := catalog.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v, want nil even when Ollama is unreachable", err)
	}
	if len(got) != 1 || got[0].Name != "my-finetune" {
		t.Errorf("List() = %+v, want only the registry model", got)
	}
}

func TestCatalogListPropagatesRegistryError(t *testing.T) {
	reg := &fakeRegistry{err: errors.New("disk error")}
	oll := &fakeOllama{}

	catalog := NewCatalog(reg, oll)
	if _, err := catalog.List(context.Background()); err == nil {
		t.Error("List() error = nil, want the registry error to propagate")
	}
}
