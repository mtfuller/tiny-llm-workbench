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

// Model describes a registry-tracked model. Ollama models are intentionally
// not part of this registry — they're listed live from Ollama's own API and
// merged in by internal/models.Catalog.
type Model struct {
	Name      string    `json:"name"`
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
