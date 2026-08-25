package training

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// minExamplesForSplit is the smallest dataset size SubprocessTrainer will
// carve a held-out validation slice from. Below this, mlx-lm's required
// valid.jsonl is just a copy of the training data — there isn't enough to
// usefully hold anything out.
const minExamplesForSplit = 5

// SubprocessTrainer runs training by shelling out to Python (mlx-lm's LoRA
// trainer via the bundled train.py, see scripts/train.py) and parsing its
// JSON-lines stdout.
type SubprocessTrainer struct {
	// Python is the interpreter to invoke. Defaults to "python3".
	Python string
	// ScriptPath overrides which script is run, mainly for tests. When
	// empty, the embedded scripts/train.py is written to a temp file and
	// reused for the lifetime of this SubprocessTrainer.
	ScriptPath string

	scriptOnce  sync.Once
	scriptCache string
	scriptErr   error
}

// completionExample is the per-line JSON shape mlx-lm's "completions" data
// format expects (see mlx-lm's LoRA fine-tuning docs).
type completionExample struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// scriptMessage mirrors the JSON lines train.py emits on stdout.
type scriptMessage struct {
	Type         string   `json:"type"`
	Iteration    int      `json:"iteration"`
	TrainLoss    *float64 `json:"trainLoss"`
	ValLoss      *float64 `json:"valLoss"`
	PeakMemGB    *float64 `json:"peakMemGB"`
	TokensPerSec *float64 `json:"tokensPerSec"`
	Message      string   `json:"message"`
}

// Train implements Trainer.
func (t *SubprocessTrainer) Train(ctx context.Context, cfg Config, examples []registry.Example, onProgress func(ProgressPoint)) (Result, error) {
	dataDir, err := writeDataset(examples)
	if err != nil {
		return Result{}, fmt.Errorf("write training dataset: %w", err)
	}
	defer os.RemoveAll(dataDir)

	outDir, err := os.MkdirTemp("", "tlw-adapter-*")
	if err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}

	scriptPath, err := t.scriptFile()
	if err != nil {
		return Result{}, fmt.Errorf("prepare training script: %w", err)
	}

	python := t.Python
	if python == "" {
		python = "python3"
	}

	args := append([]string{scriptPath}, buildArgs(cfg, dataDir, outDir)...)
	cmd := exec.CommandContext(ctx, python, args...)

	return runAndParse(cmd, outDir, onProgress)
}

// scriptFile returns the path to the training script to run, writing the
// embedded copy to a temp file on first use.
func (t *SubprocessTrainer) scriptFile() (string, error) {
	if t.ScriptPath != "" {
		return t.ScriptPath, nil
	}

	t.scriptOnce.Do(func() {
		f, err := os.CreateTemp("", "tlw-train-*.py")
		if err != nil {
			t.scriptErr = err
			return
		}
		defer f.Close()

		if _, err := f.WriteString(trainScript); err != nil {
			t.scriptErr = err
			return
		}
		t.scriptCache = f.Name()
	})

	return t.scriptCache, t.scriptErr
}

// buildArgs builds train.py's command-line arguments from cfg.
func buildArgs(cfg Config, dataDir, outDir string) []string {
	args := []string{
		"--model", cfg.BaseModel,
		"--data-dir", dataDir,
		"--output-dir", outDir,
		"--iters", strconv.Itoa(cfg.Iterations),
	}
	if cfg.LearningRate > 0 {
		args = append(args, "--learning-rate", strconv.FormatFloat(cfg.LearningRate, 'g', -1, 64))
	}
	return args
}

// writeDataset writes examples to a temp directory as train.jsonl and
// valid.jsonl in mlx-lm's "completions" format.
func writeDataset(examples []registry.Example) (string, error) {
	dir, err := os.MkdirTemp("", "tlw-dataset-*")
	if err != nil {
		return "", err
	}

	train, valid := splitExamples(examples)

	if err := writeJSONL(filepath.Join(dir, "train.jsonl"), train); err != nil {
		return "", err
	}
	if err := writeJSONL(filepath.Join(dir, "valid.jsonl"), valid); err != nil {
		return "", err
	}

	return dir, nil
}

// splitExamples carves out a validation slice for datasets large enough to
// spare one; smaller datasets reuse everything for both train and valid.
func splitExamples(examples []registry.Example) (train, valid []registry.Example) {
	if len(examples) < minExamplesForSplit {
		return examples, examples
	}

	n := len(examples) * 8 / 10
	if n == 0 {
		n = 1
	}
	if n == len(examples) {
		n = len(examples) - 1
	}
	return examples[:n], examples[n:]
}

func writeJSONL(path string, examples []registry.Example) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range examples {
		line, err := json.Marshal(completionExample{Prompt: e.Input, Completion: e.Output})
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}

	return nil
}

// runAndParse starts cmd, parses its stdout as scriptMessage JSON lines
// (see scripts/train.py), and reports the outcome.
func runAndParse(cmd *exec.Cmd, outputDir string, onProgress func(ProgressPoint)) (Result, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start training script: %w", err)
	}

	var scriptErr string
	succeeded := false

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var msg scriptMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // tolerate any non-JSON noise on stdout
		}

		switch msg.Type {
		case "progress":
			onProgress(ProgressPoint{
				Timestamp:    time.Now().UTC(),
				Iteration:    msg.Iteration,
				TrainLoss:    msg.TrainLoss,
				ValLoss:      msg.ValLoss,
				PeakMemGB:    msg.PeakMemGB,
				TokensPerSec: msg.TokensPerSec,
			})
		case "error":
			scriptErr = msg.Message
		case "done":
			succeeded = true
		}
	}

	waitErr := cmd.Wait()

	if scriptErr != "" {
		return Result{}, errors.New(scriptErr)
	}
	if waitErr != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return Result{}, fmt.Errorf("training script failed: %w: %s", waitErr, detail)
		}
		return Result{}, fmt.Errorf("training script failed: %w", waitErr)
	}
	if !succeeded {
		return Result{}, errors.New("training script exited without reporting completion")
	}

	return Result{OutputDir: outputDir}, nil
}
