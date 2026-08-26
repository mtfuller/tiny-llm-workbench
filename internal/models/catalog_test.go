package models

import (
	"context"
	"errors"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/ollama"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeRegistry struct {
	models    []registry.Model
	err       error
	deleteErr error
	deleted   []string
}

func (f *fakeRegistry) ListModels() ([]registry.Model, error) {
	return f.models, f.err
}

func (f *fakeRegistry) DeleteModel(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeOllama struct {
	models    []ollama.ModelInfo
	err       error
	deleteErr error
	deleted   []string
}

func (f *fakeOllama) ListModels(ctx context.Context) ([]ollama.ModelInfo, error) {
	return f.models, f.err
}

func (f *fakeOllama) DeleteModel(ctx context.Context, name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
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

func TestCatalogDeleteRoutesOllamaSourceToOllama(t *testing.T) {
	reg := &fakeRegistry{}
	oll := &fakeOllama{}

	catalog := NewCatalog(reg, oll)
	if err := catalog.Delete(context.Background(), "qwen2.5:0.5b", "ollama"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(oll.deleted) != 1 || oll.deleted[0] != "qwen2.5:0.5b" {
		t.Errorf("ollama.deleted = %v, want [qwen2.5:0.5b]", oll.deleted)
	}
	if len(reg.deleted) != 0 {
		t.Errorf("registry.deleted = %v, want empty for an ollama-sourced delete", reg.deleted)
	}
}

func TestCatalogDeleteRoutesOtherSourcesToRegistry(t *testing.T) {
	reg := &fakeRegistry{}
	oll := &fakeOllama{}

	catalog := NewCatalog(reg, oll)
	if err := catalog.Delete(context.Background(), "my-finetune", "mlx"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(reg.deleted) != 1 || reg.deleted[0] != "my-finetune" {
		t.Errorf("registry.deleted = %v, want [my-finetune]", reg.deleted)
	}
	if len(oll.deleted) != 0 {
		t.Errorf("ollama.deleted = %v, want empty for a registry-sourced delete", oll.deleted)
	}
}
