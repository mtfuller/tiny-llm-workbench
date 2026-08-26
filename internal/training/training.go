// Package training orchestrates MLX fine-tuning runs: it shells out to a
// bundled Python script (see scripts/train.py) that drives mlx-lm's LoRA
// trainer, parses its JSON-lines progress on stdout, and republishes that
// progress on the CLI's event bus for the Training page's SSE stream.
package training

import (
	"context"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// Config describes a single training run.
type Config struct {
	// BaseModel is a Hugging Face repo id or local path to an MLX-format
	// model (what mlx-lm's --model flag expects). Ollama-pulled models are
	// not MLX-compatible and can't be used here directly.
	BaseModel string `json:"baseModel"`
	// Dataset is the name of a registry dataset (internal/registry) to
	// train on.
	Dataset string `json:"dataset"`
	// OutputName is the name the resulting LoRA adapter is registered
	// under in the model registry once training succeeds.
	OutputName string `json:"outputName"`
	// Iterations is the number of training iterations (mlx-lm's --iters).
	Iterations int `json:"iterations"`
	// LearningRate is mlx-lm's --learning-rate. Zero means "use the
	// script's default".
	LearningRate float64 `json:"learningRate"`
}

// Status is a Run's lifecycle state.
type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// ProgressPoint is one progress update parsed from the training script.
// Pointer fields are omitted by the script when a given line doesn't report
// that stat (e.g. validation loss is only reported periodically).
type ProgressPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	Iteration    int       `json:"iteration"`
	TrainLoss    *float64  `json:"trainLoss,omitempty"`
	ValLoss      *float64  `json:"valLoss,omitempty"`
	PeakMemGB    *float64  `json:"peakMemGB,omitempty"`
	TokensPerSec *float64  `json:"tokensPerSec,omitempty"`
}

// Run is a single training run's full state, persisted to disk so history
// survives a restart of `tlw serve`.
type Run struct {
	ID         string          `json:"id"`
	Config     Config          `json:"config"`
	Status     Status          `json:"status"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	Progress   []ProgressPoint `json:"progress"`
	Error      string          `json:"error,omitempty"`
}

// Result is what a Trainer returns when a run completes successfully.
type Result struct {
	// OutputDir is where the trained LoRA adapter was written.
	OutputDir string
}

// Trainer runs a single training job, invoking onProgress as new progress
// points become available. Implementations must stop promptly when ctx is
// cancelled.
type Trainer interface {
	Train(ctx context.Context, cfg Config, examples []registry.Example, onProgress func(ProgressPoint)) (Result, error)
}
