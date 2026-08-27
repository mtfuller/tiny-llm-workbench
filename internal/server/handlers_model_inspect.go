package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
	"github.com/mtfuller/tiny-llm-workbench/internal/safetensors"
)

// getModelArchitectureHandler responds with a model's derived topology —
// layer count, hidden/vocab size, per-tensor shapes — read directly from
// its .safetensors header(s) on disk (no tensor weight bytes are read).
func getModelArchitectureHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		model, err := models.GetModel(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if model.Path == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("model %q has no local files to inspect", name))
			return
		}

		tensors, err := safetensors.ParseModelDir(model.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		arch := safetensors.DeriveArchitecture(model.Path, tensors)
		writeJSON(w, http.StatusOK, arch)
	}
}

// defaultHeatmapGrid matches the design doc's suggested fixed subsample
// size for the weight heatmap.
const defaultHeatmapGrid = 200

// getModelHeatmapHandler responds with a subsampled grid (plus min/max/
// mean/std over the full tensor) for a single named tensor, read via a
// single targeted byte-range read — never the whole model file.
func getModelHeatmapHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		tensorName := r.URL.Query().Get("tensor")
		if tensorName == "" {
			writeError(w, http.StatusBadRequest, errors.New("tensor query parameter is required"))
			return
		}

		gridSize := defaultHeatmapGrid
		if raw := r.URL.Query().Get("grid"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				gridSize = n
			}
		}

		model, err := models.GetModel(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if model.Path == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("model %q has no local files to inspect", name))
			return
		}

		tensors, err := safetensors.ParseModelDir(model.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		var target *safetensors.TensorInfo
		for i := range tensors {
			if tensors[i].Name == tensorName {
				target = &tensors[i]
				break
			}
		}
		if target == nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("tensor %q not found", tensorName))
			return
		}

		heatmap, err := safetensors.ExtractHeatmap(*target, gridSize)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusOK, heatmap)
	}
}

// tokenProbabilitiesRequest is the POST /api/models/{name}/token-probabilities
// request body.
type tokenProbabilitiesRequest struct {
	Prompt      string `json:"prompt"`
	MaxTokens   int    `json:"maxTokens"`
	TopLogprobs int    `json:"topLogprobs"`
}

// tokenProbabilitiesResponse is the POST /api/models/{name}/token-probabilities
// response body.
type tokenProbabilitiesResponse struct {
	Positions []mlxrunner.TokenPosition `json:"positions"`
}

// defaultTokenProbMaxTokens/-TopLogprobs/maxTokenProbMaxTokens bound the
// token-probability tool's generation: enough to see a meaningful sequence
// without either running away in latency or (per the design doc) exceeding
// "top 10" — mlx_lm.server's own hard cap is 11.
const (
	defaultTokenProbMaxTokens = 20
	maxTokenProbMaxTokens     = 50
	defaultTokenProbTopN      = 10
	maxTokenProbTopN          = 10
)

// tokenProbabilitiesHandler generates a short completion for the request's
// prompt and returns, for each generated token, the top candidate tokens
// mlx_lm.server considered and their log-probabilities — visualizing the
// model's per-step "confidence".
func tokenProbabilitiesHandler(models modelStore, runner modelRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req tokenProbabilitiesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			writeError(w, http.StatusBadRequest, errors.New("prompt is required"))
			return
		}

		model, err := models.GetModel(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if model.Path == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("model %q has no local path to run", name))
			return
		}

		maxTokens := req.MaxTokens
		if maxTokens <= 0 {
			maxTokens = defaultTokenProbMaxTokens
		}
		if maxTokens > maxTokenProbMaxTokens {
			maxTokens = maxTokenProbMaxTokens
		}

		topLogprobs := req.TopLogprobs
		if topLogprobs <= 0 {
			topLogprobs = defaultTokenProbTopN
		}
		if topLogprobs > maxTokenProbTopN {
			topLogprobs = maxTokenProbTopN
		}

		positions, err := runner.TokenProbabilities(r.Context(), model.Path, req.Prompt, maxTokens, topLogprobs)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if positions == nil {
			positions = []mlxrunner.TokenPosition{}
		}

		writeJSON(w, http.StatusOK, tokenProbabilitiesResponse{Positions: positions})
	}
}
