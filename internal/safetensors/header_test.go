package safetensors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseModelDirSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), map[string]testTensor{
		"model.embed_tokens.weight": {Dtype: "F32", Shape: []int64{4, 2}, Data: f32Bytes(1, 2, 3, 4, 5, 6, 7, 8)},
		"model.layers.0.mlp.weight": {Dtype: "F32", Shape: []int64{2, 2}, Data: f32Bytes(1, 2, 3, 4)},
	})

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}
	if len(tensors) != 2 {
		t.Fatalf("ParseModelDir() returned %d tensors, want 2", len(tensors))
	}

	byName := map[string]TensorInfo{}
	for _, ti := range tensors {
		byName[ti.Name] = ti
	}

	embed, ok := byName["model.embed_tokens.weight"]
	if !ok {
		t.Fatal("missing model.embed_tokens.weight")
	}
	if embed.Dtype != "F32" || len(embed.Shape) != 2 || embed.Shape[0] != 4 || embed.Shape[1] != 2 {
		t.Errorf("embed tensor = %+v, want dtype F32 shape [4 2]", embed)
	}
	if got := embed.DataEnd - embed.DataStart; got != 32 {
		t.Errorf("embed tensor byte range = %d bytes, want 32 (8 float32s)", got)
	}
}

func TestParseModelDirNoFilesFound(t *testing.T) {
	dir := t.TempDir()

	if _, err := ParseModelDir(dir); err == nil {
		t.Error("ParseModelDir() error = nil, want an error when no .safetensors files exist")
	}
}

func TestParseModelDirSharded(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model-00001-of-00002.safetensors"), map[string]testTensor{
		"model.embed_tokens.weight": {Dtype: "F32", Shape: []int64{2, 2}, Data: f32Bytes(1, 2, 3, 4)},
	})
	writeTestSafetensors(t, filepath.Join(dir, "model-00002-of-00002.safetensors"), map[string]testTensor{
		"lm_head.weight": {Dtype: "F32", Shape: []int64{2, 2}, Data: f32Bytes(5, 6, 7, 8)},
	})

	index := map[string]any{
		"metadata": map[string]any{"total_size": 32},
		"weight_map": map[string]string{
			"model.embed_tokens.weight": "model-00001-of-00002.safetensors",
			"lm_head.weight":            "model-00002-of-00002.safetensors",
		},
	}
	indexBytes, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(dir, "model.safetensors.index.json"), indexBytes, 0o644); err != nil {
		t.Fatalf("write index.json: %v", err)
	}

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}
	if len(tensors) != 2 {
		t.Fatalf("ParseModelDir() returned %d tensors, want 2", len(tensors))
	}

	for _, ti := range tensors {
		wantFile := "model-00001-of-00002.safetensors"
		if ti.Name == "lm_head.weight" {
			wantFile = "model-00002-of-00002.safetensors"
		}
		if filepath.Base(ti.File) != wantFile {
			t.Errorf("tensor %q File = %q, want it to point at shard %q", ti.Name, ti.File, wantFile)
		}
	}
}
