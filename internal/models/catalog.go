// Package models combines Ollama's locally-pulled models with TLW's own
// model registry into the single list the Models page shows.
package models

import (
	"context"
	"sort"

	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
	"github.com/mtfuller/tiny-llm-workbench/internal/ollama"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// Model is one entry in the merged model list.
type Model struct {
	Name   string `json:"name"`
	Source string `json:"source"` // "ollama", "mlx", "binary", ...
	Size   int64  `json:"size,omitempty"`
}

// ollamaLister is the subset of ollama.Client that Catalog needs; an
// interface so tests can supply a fake without spinning up an HTTP server.
type ollamaLister interface {
	ListModels(ctx context.Context) ([]ollama.ModelInfo, error)
	DeleteModel(ctx context.Context, name string) error
}

// modelRegistry is the subset of registry.Registry that Catalog needs.
type modelRegistry interface {
	ListModels() ([]registry.Model, error)
	DeleteModel(name string) error
}

// Catalog merges TLW's model registry with Ollama's local models.
type Catalog struct {
	registry modelRegistry
	ollama   ollamaLister
}

// NewCatalog builds a Catalog over reg and an Ollama client.
func NewCatalog(reg modelRegistry, ollamaClient ollamaLister) *Catalog {
	return &Catalog{registry: reg, ollama: ollamaClient}
}

// List returns every known model, sorted by name. If Ollama isn't reachable
// (e.g. not installed or not running), its models are simply omitted rather
// than failing the whole list — TLW's own registry is still authoritative.
func (c *Catalog) List(ctx context.Context) ([]Model, error) {
	var models []Model

	registryModels, err := c.registry.ListModels()
	if err != nil {
		return nil, err
	}
	for _, m := range registryModels {
		models = append(models, Model{Name: m.Name, Source: m.Source})
	}

	ollamaModels, err := c.ollama.ListModels(ctx)
	if err != nil {
		logger.Debug("Ollama unavailable, omitting its models from the catalog: %v", err)
	} else {
		for _, m := range ollamaModels {
			models = append(models, Model{Name: m.Name, Source: "ollama", Size: m.Size})
		}
	}

	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })

	return models, nil
}

// Delete removes a model, dispatching to Ollama or the registry depending on
// source (as reported by List).
func (c *Catalog) Delete(ctx context.Context, name, source string) error {
	if source == "ollama" {
		return c.ollama.DeleteModel(ctx, name)
	}
	return c.registry.DeleteModel(name)
}
