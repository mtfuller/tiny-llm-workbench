package server

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/safetensors"
)

// writeTestSafetensors writes a minimal real safetensors file for handler
// tests to point a fake model's Path at.
func writeTestSafetensors(t *testing.T, path string, tensors map[string]struct {
	Dtype string
	Shape []int64
	Data  []byte
}) {
	t.Helper()

	header := map[string]any{}
	var data []byte
	for name, tv := range tensors {
		start := int64(len(data))
		data = append(data, tv.Data...)
		header[name] = map[string]any{
			"dtype":        tv.Dtype,
			"shape":        tv.Shape,
			"data_offsets": []int64{start, int64(len(data))},
		}
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal test header: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test safetensors file: %v", err)
	}
	defer f.Close()

	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(headerBytes)))
	f.Write(lenBuf[:])
	f.Write(headerBytes)
	f.Write(data)
}

func f32Bytes(vs ...float32) []byte {
	buf := make([]byte, 4*len(vs))
	for i, v := range vs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func TestGetModelArchitecture(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), map[string]struct {
		Dtype string
		Shape []int64
		Data  []byte
	}{
		"model.embed_tokens.weight": {Dtype: "F32", Shape: []int64{10, 4}, Data: f32Bytes(make([]float32, 40)...)},
		"model.layers.0.mlp.weight": {Dtype: "F32", Shape: []int64{4, 4}, Data: f32Bytes(make([]float32, 16)...)},
	})

	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-model", Path: dir, Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/my-model/architecture", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../architecture status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var arch safetensors.Architecture
	if err := json.Unmarshal(rec.Body.Bytes(), &arch); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if arch.NumLayers != 1 || arch.HiddenSize != 4 || arch.VocabSize != 10 {
		t.Errorf("arch = %+v, want NumLayers=1 HiddenSize=4 VocabSize=10", arch)
	}
	if len(arch.Tensors) != 2 {
		t.Errorf("len(arch.Tensors) = %d, want 2", len(arch.Tensors))
	}
}

func TestGetModelArchitectureModelNotFound(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{getErr: errors.New("model not found")}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/missing/architecture", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET .../architecture (missing model) status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetModelHeatmap(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), map[string]struct {
		Dtype string
		Shape []int64
		Data  []byte
	}{
		"w": {Dtype: "F32", Shape: []int64{2, 2}, Data: f32Bytes(1, 2, 3, 4)},
	})

	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-model", Path: dir, Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/my-model/heatmap?tensor=w&grid=10", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET .../heatmap status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var heatmap safetensors.HeatmapData
	if err := json.Unmarshal(rec.Body.Bytes(), &heatmap); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if heatmap.Min != 1 || heatmap.Max != 4 {
		t.Errorf("heatmap min/max = %v/%v, want 1/4", heatmap.Min, heatmap.Max)
	}
}

func TestGetModelHeatmapRequiresTensorParam(t *testing.T) {
	dir := t.TempDir()
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-model", Path: dir, Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/my-model/heatmap", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET .../heatmap (no tensor param) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetModelHeatmapTensorNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTestSafetensors(t, filepath.Join(dir, "model.safetensors"), map[string]struct {
		Dtype string
		Shape []int64
		Data  []byte
	}{
		"w": {Dtype: "F32", Shape: []int64{2}, Data: f32Bytes(1, 2)},
	})

	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-model", Path: dir, Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/my-model/heatmap?tensor=does-not-exist", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET .../heatmap (unknown tensor) status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestTokenProbabilitiesHandler(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-model", Path: "/tlw/models/my-model", Source: "mlx"}}}
	runner := &fakeModelRunner{positions: []mlxrunner.TokenPosition{
		{Token: "hi", LogProb: -0.1, TopCandidates: []mlxrunner.TokenProbability{{Token: "hi", LogProb: -0.1}}},
	}}
	deps.ModelRunner = runner

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(tokenProbabilitiesRequest{Prompt: "say hi", MaxTokens: 100, TopLogprobs: 50})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/my-model/token-probabilities", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../token-probabilities status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got tokenProbabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got.Positions) != 1 || got.Positions[0].Token != "hi" {
		t.Errorf("positions = %+v, want the fake runner's single position", got.Positions)
	}

	if len(runner.tokenProbCalls) != 1 {
		t.Fatalf("runner.tokenProbCalls = %+v, want 1 call", runner.tokenProbCalls)
	}
	call := runner.tokenProbCalls[0]
	if call.model != "/tlw/models/my-model" || call.prompt != "say hi" {
		t.Errorf("call = %+v, want model/prompt passed through", call)
	}
	if call.maxTokens != maxTokenProbMaxTokens {
		t.Errorf("call.maxTokens = %d, want it clamped to %d", call.maxTokens, maxTokenProbMaxTokens)
	}
	if call.topN != maxTokenProbTopN {
		t.Errorf("call.topN = %d, want it clamped to %d", call.topN, maxTokenProbTopN)
	}
}

func TestTokenProbabilitiesHandlerRequiresPrompt(t *testing.T) {
	deps := testDeps()
	deps.Models = &fakeModelStore{list: []registry.Model{{Name: "my-model", Path: "/tlw/models/my-model", Source: "mlx"}}}

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, _ := json.Marshal(tokenProbabilitiesRequest{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/my-model/token-probabilities", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST .../token-probabilities (no prompt) status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
