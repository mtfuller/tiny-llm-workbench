package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndListModels(t *testing.T) {
	reg := New(t.TempDir())

	want := Model{
		Name:      "my-mlx-model",
		BaseModel: "mlx-community/Qwen2.5-0.5B-Instruct-4bit",
		Source:    "mlx",
		Path:      "weights.safetensors",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := reg.SaveModel(want); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}

	models, err := reg.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("ListModels() returned %d models, want 1", len(models))
	}
	if got := models[0]; got.Name != want.Name || got.BaseModel != want.BaseModel || got.Source != want.Source ||
		got.Path != want.Path || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("ListModels()[0] = %+v, want %+v", got, want)
	}
}

func TestGetModel(t *testing.T) {
	reg := New(t.TempDir())

	want := Model{Name: "my-mlx-model", BaseModel: "mlx-community/Qwen2.5-0.5B-Instruct-4bit", Source: "mlx"}
	if err := reg.SaveModel(want); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}

	got, err := reg.GetModel("my-mlx-model")
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if got.Name != want.Name || got.BaseModel != want.BaseModel {
		t.Errorf("GetModel() = %+v, want %+v", got, want)
	}
}

func TestGetModelNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetModel("does-not-exist"); err == nil {
		t.Error("GetModel() error = nil, want an error for an unknown model")
	}
}

func TestResolveModelRef(t *testing.T) {
	reg := New(t.TempDir())
	if err := reg.SaveModel(Model{
		Name:   "Llama-3.2-1B-Instruct-4bit",
		Source: "huggingface",
		Path:   "mlx-community/Llama-3.2-1B-Instruct-4bit",
	}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}
	if err := reg.SaveModel(Model{Name: "no-path-model", Source: "mlx"}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}

	cases := map[string]string{
		// A registry model name resolves to its Path.
		"Llama-3.2-1B-Instruct-4bit": "mlx-community/Llama-3.2-1B-Instruct-4bit",
		// A registry model with no Path, and anything not in the registry,
		// passes through unchanged.
		"no-path-model": "no-path-model",
		"mlx-community/Qwen2.5-0.5B-Instruct-4bit": "mlx-community/Qwen2.5-0.5B-Instruct-4bit",
		"/some/local/dir":                          "/some/local/dir",
	}
	for ref, want := range cases {
		if got := reg.ResolveModelRef(ref); got != want {
			t.Errorf("ResolveModelRef(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestListModelsEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	models, err := reg.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 0 {
		t.Errorf("ListModels() = %v, want empty", models)
	}
}

func TestListModelsSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := reg.SaveModel(Model{Name: name, Source: "mlx"}); err != nil {
			t.Fatalf("SaveModel(%q) error = %v", name, err)
		}
	}

	models, err := reg.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	var names []string
	for _, m := range models {
		names = append(names, m.Name)
	}

	want := []string{"alpha", "mid", "zeta"}
	for i, name := range want {
		if names[i] != name {
			t.Errorf("ListModels() names = %v, want %v", names, want)
			break
		}
	}
}

func TestListModelsSkipsDirectoriesWithoutMetadata(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveModel(Model{Name: "valid", Source: "mlx"}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}

	// A directory with no metadata.json should be skipped, not error the whole list.
	if err := reg.SaveModel(Model{Name: "broken", Source: "mlx"}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}
	// Corrupt the metadata file for "broken".
	brokenPath := filepath.Join(reg.modelDir("broken"), modelMetadataFile)
	if err := os.WriteFile(brokenPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("failed to corrupt metadata: %v", err)
	}

	models, err := reg.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 || models[0].Name != "valid" {
		t.Errorf("ListModels() = %+v, want only the valid model", models)
	}
}

func TestDeleteModel(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveModel(Model{Name: "throwaway", Source: "mlx"}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}

	if err := reg.DeleteModel("throwaway"); err != nil {
		t.Fatalf("DeleteModel() error = %v", err)
	}

	models, err := reg.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 0 {
		t.Errorf("ListModels() = %+v, want empty after delete", models)
	}
}

func TestDeleteModelNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteModel("does-not-exist"); err == nil {
		t.Error("DeleteModel() error = nil, want an error for an unknown model")
	}
}
