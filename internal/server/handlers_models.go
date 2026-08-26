package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
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
