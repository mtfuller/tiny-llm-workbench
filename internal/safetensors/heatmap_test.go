package safetensors

import (
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"
)

func TestFloat16ToFloat32KnownValues(t *testing.T) {
	tests := []struct {
		name string
		bits uint16
		want float32
	}{
		{"positive one", 0x3C00, 1.0},
		{"negative two", 0xC000, -2.0},
		{"one half", 0x3800, 0.5},
		{"zero", 0x0000, 0.0},
		{"negative zero", 0x8000, float32(math.Copysign(0, -1))},
		{"smallest subnormal", 0x0001, float32(math.Pow(2, -24))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := float16ToFloat32(tt.bits)
			if got != tt.want {
				t.Errorf("float16ToFloat32(0x%04X) = %v, want %v", tt.bits, got, tt.want)
			}
		})
	}
}

func TestFloat16ToFloat32Infinity(t *testing.T) {
	got := float16ToFloat32(0x7C00)
	if !math.IsInf(float64(got), 1) {
		t.Errorf("float16ToFloat32(0x7C00) = %v, want +Inf", got)
	}
}

func TestExtractHeatmapComputesStatsAndSubsamples(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.safetensors")
	// A 2x4 matrix: values 1..8. Mean = 4.5, min = 1, max = 8.
	writeTestSafetensors(t, path, map[string]testTensor{
		"w": {Dtype: "F32", Shape: []int64{2, 4}, Data: f32Bytes(1, 2, 3, 4, 5, 6, 7, 8)},
	})

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}

	heatmap, err := ExtractHeatmap(tensors[0], 4)
	if err != nil {
		t.Fatalf("ExtractHeatmap() error = %v", err)
	}

	if heatmap.Rows != 2 || heatmap.Cols != 4 {
		t.Errorf("heatmap dims = %dx%d, want 2x4 (grid size larger than the matrix)", heatmap.Rows, heatmap.Cols)
	}
	if heatmap.Min != 1 || heatmap.Max != 8 {
		t.Errorf("heatmap min/max = %v/%v, want 1/8", heatmap.Min, heatmap.Max)
	}
	if heatmap.Mean != 4.5 {
		t.Errorf("heatmap mean = %v, want 4.5", heatmap.Mean)
	}
	if len(heatmap.Grid) != 8 {
		t.Errorf("len(heatmap.Grid) = %d, want 8", len(heatmap.Grid))
	}
}

func TestExtractHeatmapSubsamplesLargerMatrix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.safetensors")

	// A 10x10 matrix subsampled down to a 3x3 grid.
	vals := make([]float32, 100)
	for i := range vals {
		vals[i] = float32(i)
	}
	writeTestSafetensors(t, path, map[string]testTensor{
		"w": {Dtype: "F32", Shape: []int64{10, 10}, Data: f32Bytes(vals...)},
	})

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}

	heatmap, err := ExtractHeatmap(tensors[0], 3)
	if err != nil {
		t.Fatalf("ExtractHeatmap() error = %v", err)
	}
	if heatmap.Rows != 3 || heatmap.Cols != 3 {
		t.Errorf("heatmap dims = %dx%d, want 3x3", heatmap.Rows, heatmap.Cols)
	}
	if len(heatmap.Grid) != 9 {
		t.Errorf("len(heatmap.Grid) = %d, want 9", len(heatmap.Grid))
	}
	// Stats must reflect all 100 elements (0..99), not just the 9 sampled.
	if heatmap.Min != 0 || heatmap.Max != 99 {
		t.Errorf("heatmap min/max = %v/%v, want 0/99 (over the full matrix, not the subsample)", heatmap.Min, heatmap.Max)
	}
}

func TestExtractHeatmapDecodesF16(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.safetensors")

	// 1.0 and -2.0 as F16 bit patterns (verified in TestFloat16ToFloat32KnownValues).
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint16(buf[0:2], 0x3C00)
	binary.LittleEndian.PutUint16(buf[2:4], 0xC000)
	writeTestSafetensors(t, path, map[string]testTensor{
		"w": {Dtype: "F16", Shape: []int64{2}, Data: buf},
	})

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}

	heatmap, err := ExtractHeatmap(tensors[0], 10)
	if err != nil {
		t.Fatalf("ExtractHeatmap() error = %v", err)
	}
	if heatmap.Min != -2.0 || heatmap.Max != 1.0 {
		t.Errorf("heatmap min/max = %v/%v, want -2/1", heatmap.Min, heatmap.Max)
	}
}

func TestExtractHeatmapUnsupportedDtype(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.safetensors")
	writeTestSafetensors(t, path, map[string]testTensor{
		"w": {Dtype: "I8", Shape: []int64{4}, Data: []byte{1, 2, 3, 4}},
	})

	tensors, err := ParseModelDir(dir)
	if err != nil {
		t.Fatalf("ParseModelDir() error = %v", err)
	}

	if _, err := ExtractHeatmap(tensors[0], 10); err == nil {
		t.Error("ExtractHeatmap() error = nil, want an error for an unsupported dtype")
	}
}
