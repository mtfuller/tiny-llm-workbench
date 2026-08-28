package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// modelMetadataFile is the per-model metadata file, stored alongside the
// model's weights/files under models/<name>/.
const modelMetadataFile = "metadata.json"

// Model describes a registry-tracked model. Path points at a standalone,
// directly-loadable MLX model directory — for a trained model, this is the
// fused output of internal/training's post-training fuse step, not the raw
// LoRA adapter (which isn't runnable on its own).
type Model struct {
	Name string `json:"name"`
	// BaseModel is the MLX-format model (a Hugging Face repo id, or another
	// registry model's name) this model was fine-tuned from — empty for a
	// model that wasn't produced by a training run.
	BaseModel string    `json:"baseModel,omitempty"`
	Source    string    `json:"source"` // e.g. "mlx", "binary"
	Path      string    `json:"path,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// SaveModel writes m's metadata, creating its directory if needed.
func (r *Registry) SaveModel(m Model) error {
	dir := r.modelDir(m.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal model metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, modelMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write model metadata: %w", err)
	}

	return nil
}

// GetModel returns a single registry-tracked model's metadata.
func (r *Registry) GetModel(name string) (Model, error) {
	return r.readModelMetadata(name)
}

// ResolveModelRef turns a model reference into the string mlx-lm's --model
// flag expects. If ref names a registry model, its Path is returned — a local
// fused-model directory for a trained model, or the full "mlx-community/…"
// repo id for one added from Hugging Face. Otherwise ref is returned
// unchanged, so a raw repo id or local path given directly still works.
//
// This exists because several surfaces (the Training page's picker, an agent
// prompt/agent node's model field) let the user choose a model by its
// registry name — which mlx-lm can't resolve on its own (an org-less name
// like "Llama-3.2-1B-Instruct-4bit" makes the Hub return 401).
func (r *Registry) ResolveModelRef(ref string) string {
	if m, err := r.GetModel(ref); err == nil && m.Path != "" {
		return m.Path
	}
	return ref
}

// ListModels returns all registry-tracked models, sorted by name.
func (r *Registry) ListModels() ([]Model, error) {
	entries, err := os.ReadDir(r.modelsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read models directory: %w", err)
	}

	var models []Model
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		model, err := r.readModelMetadata(entry.Name())
		if err != nil {
			continue // skip directories without valid metadata
		}
		models = append(models, model)
	}

	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })

	return models, nil
}

// DeleteModel removes a registry-tracked model's directory (metadata and any
// files alongside it, e.g. adapter weights). It's an error to delete a model
// that doesn't exist.
func (r *Registry) DeleteModel(name string) error {
	dir := r.modelDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("model %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete model %q: %w", name, err)
	}
	return nil
}

func (r *Registry) readModelMetadata(name string) (Model, error) {
	data, err := os.ReadFile(filepath.Join(r.modelDir(name), modelMetadataFile))
	if err != nil {
		return Model{}, err
	}

	var model Model
	if err := json.Unmarshal(data, &model); err != nil {
		return Model{}, fmt.Errorf("parse metadata for model %q: %w", name, err)
	}

	return model, nil
}
