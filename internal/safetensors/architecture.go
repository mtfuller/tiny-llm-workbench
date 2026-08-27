package safetensors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Architecture is a derived summary of a model's topology, for the Models
// page's architecture visualizer.
type Architecture struct {
	ModelType      string          `json:"modelType,omitempty"`
	NumLayers      int             `json:"numLayers"`
	HiddenSize     int64           `json:"hiddenSize,omitempty"`
	VocabSize      int64           `json:"vocabSize,omitempty"`
	NumParameters  int64           `json:"numParameters"`
	EstimatedBytes int64           `json:"estimatedBytes"`
	Tensors        []TensorSummary `json:"tensors"`
}

// TensorSummary is one tensor's shape/dtype plus which logical block of the
// architecture it belongs to, for grouping in the UI.
type TensorSummary struct {
	Name        string  `json:"name"`
	Dtype       string  `json:"dtype"`
	Shape       []int64 `json:"shape"`
	NumElements int64   `json:"numElements"`
	Block       string  `json:"block"` // "embedding" | "layer" | "norm" | "lm_head" | "other"
	Layer       *int    `json:"layer,omitempty"`
}

var layerRe = regexp.MustCompile(`\.layers\.(\d+)\.`)

// DeriveArchitecture summarizes tensors (as returned by ParseModelDir) into
// an Architecture. dir's config.json, if present, overrides the topology
// figures (layer count, hidden size, vocab size) derived by inspecting
// tensor names/shapes — config.json is the authoritative source when a
// model ships one; tensor-name regex matching is the fallback for models
// that don't.
func DeriveArchitecture(dir string, tensors []TensorInfo) Architecture {
	arch := Architecture{Tensors: make([]TensorSummary, len(tensors))}

	maxLayer := -1
	var totalParams, totalBytes int64

	for i, t := range tensors {
		n := t.NumElements()
		totalParams += n
		totalBytes += n * bytesPerElement(t.Dtype)

		block, layer := classify(t.Name)
		if layer != nil && *layer > maxLayer {
			maxLayer = *layer
		}

		arch.Tensors[i] = TensorSummary{
			Name:        t.Name,
			Dtype:       t.Dtype,
			Shape:       t.Shape,
			NumElements: n,
			Block:       block,
			Layer:       layer,
		}

		if block == "embedding" && len(t.Shape) == 2 {
			arch.VocabSize = t.Shape[0]
			arch.HiddenSize = t.Shape[1]
		}
	}

	sort.SliceStable(arch.Tensors, func(i, j int) bool {
		a, b := arch.Tensors[i], arch.Tensors[j]
		if ra, rb := blockRank(a.Block), blockRank(b.Block); ra != rb {
			return ra < rb
		}
		if a.Layer != nil && b.Layer != nil && *a.Layer != *b.Layer {
			return *a.Layer < *b.Layer
		}
		return a.Name < b.Name
	})

	arch.NumLayers = maxLayer + 1
	arch.NumParameters = totalParams
	arch.EstimatedBytes = totalBytes

	if cfg, err := readConfig(filepath.Join(dir, "config.json")); err == nil {
		if cfg.ModelType != "" {
			arch.ModelType = cfg.ModelType
		}
		if cfg.NumHiddenLayers > 0 {
			arch.NumLayers = cfg.NumHiddenLayers
		}
		if cfg.HiddenSize > 0 {
			arch.HiddenSize = cfg.HiddenSize
		}
		if cfg.VocabSize > 0 {
			arch.VocabSize = cfg.VocabSize
		}
	}

	return arch
}

func blockRank(block string) int {
	switch block {
	case "embedding":
		return 0
	case "layer":
		return 1
	case "norm":
		return 2
	case "lm_head":
		return 3
	default:
		return 4
	}
}

func classify(name string) (block string, layer *int) {
	if m := layerRe.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return "layer", &n
	}
	switch {
	case strings.Contains(name, "embed_tokens"):
		return "embedding", nil
	case strings.Contains(name, "lm_head"):
		return "lm_head", nil
	case strings.Contains(name, "norm"):
		return "norm", nil
	default:
		return "other", nil
	}
}

// bytesPerElement estimates on-disk size per element for common safetensors
// dtypes. Unknown dtypes (e.g. a packed quantized format) fall back to 2
// bytes — a rough estimate rather than a hard failure, since this only
// affects the displayed size estimate, not correctness of anything else.
func bytesPerElement(dtype string) int64 {
	switch strings.ToUpper(dtype) {
	case "F64", "I64", "U64":
		return 8
	case "F32", "I32", "U32":
		return 4
	case "F16", "BF16", "I16", "U16":
		return 2
	case "I8", "U8", "BOOL":
		return 1
	default:
		return 2
	}
}

type modelConfig struct {
	ModelType       string `json:"model_type"`
	HiddenSize      int64  `json:"hidden_size"`
	NumHiddenLayers int    `json:"num_hidden_layers"`
	VocabSize       int64  `json:"vocab_size"`
}

func readConfig(path string) (modelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return modelConfig{}, err
	}
	var cfg modelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return modelConfig{}, err
	}
	return cfg, nil
}
