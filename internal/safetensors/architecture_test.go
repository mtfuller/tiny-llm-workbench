package safetensors

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDeriveArchitectureFromTensorNames(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), map[string]testTensor{
		"model.embed_tokens.weight": {Dtype: "F32", Shape: []int64{100, 8}, Data: make([]byte, 100*8*4)},
		"model.layers.0.mlp.weight": {Dtype: "F32", Shape: []int64{8, 8}, Data: make([]byte, 8*8*4)},
		"model.layers.1.mlp.weight": {Dtype: "F32", Shape: []int64{8, 8}, Data: make([]byte, 8*8*4)},
		"model.norm.weight":         {Dtype: "F32", Shape: []int64{8}, Data: make([]byte, 8*4)},
		"lm_head.weight":            {Dtype: "F32", Shape: []int64{100, 8}, Data: make([]byte, 100*8*4)},
	})

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}

	arch := DeriveArchitecture(dir, tensors)

	if arch.NumLayers != 2 {
		t.Errorf("NumLayers = %d, want 2 (layers 0 and 1)", arch.NumLayers)
	}
	if arch.HiddenSize != 8 {
		t.Errorf("HiddenSize = %d, want 8 (from embed_tokens shape)", arch.HiddenSize)
	}
	if arch.VocabSize != 100 {
		t.Errorf("VocabSize = %d, want 100 (from embed_tokens shape)", arch.VocabSize)
	}
	wantParams := int64(100*8 + 8*8 + 8*8 + 8 + 100*8)
	if arch.NumParameters != wantParams {
		t.Errorf("NumParameters = %d, want %d", arch.NumParameters, wantParams)
	}

	// Tensors should come back in architecture order: embedding, then
	// layers in numeric order, then norm, then lm_head — not alphabetical
	// (which would misorder layers 10+ before layer 2).
	wantOrder := []string{
		"model.embed_tokens.weight",
		"model.layers.0.mlp.weight",
		"model.layers.1.mlp.weight",
		"model.norm.weight",
		"lm_head.weight",
	}
	if len(arch.Tensors) != len(wantOrder) {
		t.Fatalf("got %d tensor summaries, want %d", len(arch.Tensors), len(wantOrder))
	}
	for i, want := range wantOrder {
		if arch.Tensors[i].Name != want {
			t.Errorf("Tensors[%d].Name = %q, want %q", i, arch.Tensors[i].Name, want)
		}
	}
}

func TestDeriveArchitectureSortsLayersNumerically(t *testing.T) {
	dir := t.TempDir()
	tensors := map[string]testTensor{
		"model.embed_tokens.weight": {Dtype: "F32", Shape: []int64{2, 2}, Data: f32Bytes(0, 0, 0, 0)},
	}
	// Layer 10 should sort after layer 2, even though "10" < "2" as strings.
	for _, n := range []int{2, 10} {
		name := "model.layers." + strconv.Itoa(n) + ".weight"
		tensors[name] = testTensor{Dtype: "F32", Shape: []int64{2}, Data: f32Bytes(0, 0)}
	}
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), tensors)

	parsed, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}
	arch := DeriveArchitecture(dir, parsed)

	if len(arch.Tensors) != 3 {
		t.Fatalf("got %d tensors, want 3", len(arch.Tensors))
	}
	if arch.Tensors[1].Name != "model.layers.2.weight" || arch.Tensors[2].Name != "model.layers.10.weight" {
		t.Errorf("layer order = [%s, %s], want layer 2 before layer 10",
			arch.Tensors[1].Name, arch.Tensors[2].Name)
	}
	if arch.NumLayers != 11 {
		t.Errorf("NumLayers = %d, want 11 (max layer index 10 + 1)", arch.NumLayers)
	}
}

func TestDeriveArchitecturePrefersConfigJSON(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), map[string]testTensor{
		"model.embed_tokens.weight": {Dtype: "F32", Shape: []int64{4, 4}, Data: make([]byte, 4*4*4)},
	})
	config := `{"model_type": "qwen2", "hidden_size": 896, "num_hidden_layers": 24, "vocab_size": 151936}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}
	arch := DeriveArchitecture(dir, tensors)

	if arch.ModelType != "qwen2" || arch.NumLayers != 24 || arch.HiddenSize != 896 || arch.VocabSize != 151936 {
		t.Errorf("arch = %+v, want config.json's values to win over tensor-derived ones", arch)
	}
}
