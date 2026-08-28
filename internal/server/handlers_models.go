package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/huggingface"
	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

// modelJSON is a model as returned by GET /api/models — deliberately
// slimmer than registry.Model (drops Path/CreatedAt, which are internal
// filesystem details, not part of the public API).
type modelJSON struct {
	Name      string `json:"name"`
	BaseModel string `json:"baseModel,omitempty"`
	Source    string `json:"source"`
}

// listModelsHandler responds with every registry-tracked model.
func listModelsHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := models.ListModels()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		out := make([]modelJSON, len(list))
		for i, m := range list {
			out[i] = modelJSON{Name: m.Name, BaseModel: m.BaseModel, Source: m.Source}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// modelDetailJSON is the GET /api/models/{name} response body: the model's
// own metadata plus the training run that produced it, if any.
type modelDetailJSON struct {
	Name        string        `json:"name"`
	BaseModel   string        `json:"baseModel,omitempty"`
	Source      string        `json:"source"`
	TrainingRun *training.Run `json:"trainingRun,omitempty"`
}

// getModelHandler responds with a single model's metadata and the training
// run that produced it (the most recent run whose output name matches, if
// any — a model can also exist without one, e.g. hand-registered).
func getModelHandler(models modelStore, trainingMgr trainingManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		model, err := models.GetModel(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}

		detail := modelDetailJSON{Name: model.Name, BaseModel: model.BaseModel, Source: model.Source}
		for _, run := range trainingMgr.ListRuns() { // most recently started first
			if run.Config.OutputName == name {
				detail.TrainingRun = run
				break
			}
		}

		writeJSON(w, http.StatusOK, detail)
	}
}

// hfModelJSON is one Hugging Face search result as returned by
// GET /api/huggingface/models.
type hfModelJSON struct {
	RepoID       string   `json:"repoId"`
	Name         string   `json:"name"` // the short name it'd be registered under
	Downloads    int      `json:"downloads"`
	Likes        int      `json:"likes"`
	Tags         []string `json:"tags"`
	LastModified string   `json:"lastModified"`
	Added        bool     `json:"added"` // already in the local registry
}

// searchHuggingFaceModelsHandler proxies a user-initiated search of the
// mlx-community org on the Hugging Face Hub. Results are marked `added` if a
// registry model already tracks that repo, so the UI can disable its button.
func searchHuggingFaceModelsHandler(hf hfSearcher, models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hf == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("Hugging Face search is not configured"))
			return
		}

		results, err := hf.SearchModels(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		// Which repos are already tracked locally (by repo id, stored as the
		// model's Path when added from the Hub).
		added := map[string]bool{}
		if existing, err := models.ListModels(); err == nil {
			for _, m := range existing {
				if m.Source == "huggingface" && m.Path != "" {
					added[m.Path] = true
				}
			}
		}

		out := make([]hfModelJSON, len(results))
		for i, m := range results {
			out[i] = hfModelJSON{
				RepoID:       m.ID,
				Name:         huggingface.RepoShortName(m.ID),
				Downloads:    m.Downloads,
				Likes:        m.Likes,
				Tags:         m.Tags,
				LastModified: m.LastModified.Format(time.RFC3339),
				Added:        added[m.ID],
			}
			if out[i].Tags == nil {
				out[i].Tags = []string{}
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// addHuggingFaceModelRequest is the POST /api/huggingface/models body.
type addHuggingFaceModelRequest struct {
	RepoID string `json:"repoId"`
}

// addHuggingFaceModelHandler registers an mlx-community repo as a local
// model. No weights are fetched here — the model's Path is the repo id, and
// mlx_lm downloads it on first use (chat, an agent run, a benchmark, …),
// exactly as it does for a repo id typed straight into a model picker.
func addHuggingFaceModelHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addHuggingFaceModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if !huggingface.IsMLXCommunityRepo(req.RepoID) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%q is not an mlx-community model repo", req.RepoID))
			return
		}

		name := huggingface.RepoShortName(req.RepoID)
		if _, err := models.GetModel(name); err == nil {
			writeError(w, http.StatusConflict, fmt.Errorf("a model named %q already exists", name))
			return
		}

		model := registry.Model{
			Name:      name,
			BaseModel: req.RepoID,
			Source:    "huggingface",
			Path:      req.RepoID,
			CreatedAt: time.Now().UTC(),
		}
		if err := models.SaveModel(model); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, modelJSON{Name: model.Name, BaseModel: model.BaseModel, Source: model.Source})
	}
}

// deleteModelHandler removes a registry-tracked model.
func deleteModelHandler(models modelStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := models.DeleteModel(r.PathValue("name")); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// chatMessageJSON is one turn in the POST /api/models/{name}/chat request
// body's message history.
type chatMessageJSON struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatWithModelRequest is the POST /api/models/{name}/chat request body —
// the full conversation so far (including the newest user turn), since
// mlx_lm.server itself is stateless between requests and keeps no memory of
// earlier turns on its own.
type chatWithModelRequest struct {
	Messages []chatMessageJSON `json:"messages"`
}

// chatWithModelResponse is the POST /api/models/{name}/chat response body.
type chatWithModelResponse struct {
	Completion string `json:"completion"`
}

// chatWithModelHandler runs a full conversation against a registry model,
// starting (or reusing) its mlx_lm.server process as needed. Backs the
// Models detail page's chat modal, letting a model be tried out without a
// full Agent/Evaluation setup.
func chatWithModelHandler(models modelStore, runner modelRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req chatWithModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if len(req.Messages) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("messages must include at least one message"))
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

		messages := make([]mlxrunner.ChatMessage, len(req.Messages))
		for i, m := range req.Messages {
			messages[i] = mlxrunner.ChatMessage{Role: m.Role, Content: m.Content}
		}

		completion, err := runner.Chat(r.Context(), model.Path, messages)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		writeJSON(w, http.StatusOK, chatWithModelResponse{Completion: completion})
	}
}
