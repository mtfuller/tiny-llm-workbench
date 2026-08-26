package training

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// minExamplesForSplit is the smallest dataset size SubprocessTrainer will
// carve a held-out validation slice from. Below this, mlx-lm's required
// valid.jsonl is just a copy of the training data — there isn't enough to
// usefully hold anything out.
const minExamplesForSplit = 5

// defaultBatchSize matches mlx_lm.lora's own default --batch-size. Passed
// explicitly (see Train) rather than left to mlx_lm.lora's default because
// that default doesn't adapt to a small dataset: confirmed against a real
// run that a 12-example dataset (train=9/valid=3 after the 80/20 split
// below) fails with "Dataset must have at least batch_size=4 examples but
// only has 3" — mlx_lm.lora requires batch_size <= the smaller of the two
// splits, and small-but-not-tiny datasets (as few as 5, or as many as 15,
// examples) routinely produce a split smaller than 4 either way.
const defaultBatchSize = 4

// defaultCommand is the standalone mlx-lm CLI command SubprocessTrainer
// invokes when Command is left empty. Deliberately not `<python> -m
// mlx_lm.lora`: mlx-lm is commonly installed in a way that isn't importable
// from an arbitrary Python (e.g. `brew install mlx-lm` vendors its own
// isolated environment and only exposes the `mlx_lm.*` command wrappers on
// PATH), so this only ever needs the command itself resolvable on PATH —
// no Python interpreter involved at all.
const defaultCommand = "mlx_lm.lora"

// defaultFuseCommand is the mlx-lm CLI command Fuse invokes when FuseCommand
// is left empty.
const defaultFuseCommand = "mlx_lm.fuse"

// SubprocessTrainer runs training by shelling out directly to mlx-lm's
// `mlx_lm.lora` CLI command and regex-parsing its textual stdout.
type SubprocessTrainer struct {
	// Command overrides which executable to run, mainly for tests. Empty
	// means defaultCommand, resolved via PATH.
	Command string
	// FuseCommand overrides which executable Fuse runs, mainly for tests.
	// Empty means defaultFuseCommand, resolved via PATH.
	FuseCommand string
}

// completionExample is the per-line JSON shape mlx-lm's "completions" data
// format expects (see mlx-lm's LoRA fine-tuning docs).
type completionExample struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// Verified against a real `mlx_lm.lora` run (mlx-lm on an Apple M1, 2026-08).
// Example lines:
//
//	"Iter 10: Train loss 1.242, Learning Rate 1.000e-05, It/sec 4.857, " +
//	  "Tokens/sec 871.760, Trained Tokens 1795, Peak mem 1.106 GB"
//	"Iter 10: Val loss 0.083, Val took 0.207s"
//
// Tokens/sec and Peak mem are matched with their own independent regexes
// rather than folded into trainLineRe as trailing optional groups: an
// earlier attempt did that with a single combined pattern using a
// non-greedy `.*?` between optional groups, which regex is free to satisfy
// by matching zero characters — so it matched successfully while never
// actually reading those fields, always returning no match for them even
// when the line had a value.
var (
	trainLineRe    = regexp.MustCompile(`Iter (\d+): Train loss ([\d.]+)`)
	valLineRe      = regexp.MustCompile(`Iter (\d+): Val loss ([\d.]+)`)
	tokensPerSecRe = regexp.MustCompile(`Tokens/sec ([\d.]+)`)
	peakMemGBRe    = regexp.MustCompile(`Peak mem ([\d.]+) GB`)
)

// Train implements Trainer.
func (t *SubprocessTrainer) Train(ctx context.Context, cfg Config, examples []registry.Example, onProgress func(ProgressPoint)) (Result, error) {
	dataDir, trainCount, validCount, err := writeDataset(examples)
	if err != nil {
		return Result{}, fmt.Errorf("write training dataset: %w", err)
	}
	defer os.RemoveAll(dataDir)

	outDir, err := os.MkdirTemp("", "tlw-adapter-*")
	if err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}

	command := t.Command
	if command == "" {
		command = defaultCommand
	}

	if err := lookPathOrFriendlyError(command); err != nil {
		return Result{}, err
	}

	batchSize := batchSizeFor(trainCount, validCount)
	cmd := exec.CommandContext(ctx, command, buildArgs(cfg, dataDir, outDir, batchSize)...)
	return runAndParse(cmd, outDir, onProgress)
}

// batchSizeFor picks the largest batch size (up to defaultBatchSize) that
// both the train and validation splits can actually support — mlx_lm.lora
// requires --batch-size <= len(split) for both.
func batchSizeFor(trainCount, validCount int) int {
	batchSize := defaultBatchSize
	if trainCount < batchSize {
		batchSize = trainCount
	}
	if validCount < batchSize {
		batchSize = validCount
	}
	if batchSize < 1 {
		batchSize = 1
	}
	return batchSize
}

// Fuse implements Trainer.
func (t *SubprocessTrainer) Fuse(ctx context.Context, baseModel, adapterDir, savePath string) error {
	command := t.FuseCommand
	if command == "" {
		command = defaultFuseCommand
	}

	if err := lookPathOrFriendlyError(command); err != nil {
		return err
	}

	// --dequantize matters: fusing a LoRA adapter into an already-quantized
	// base model without it is a silent no-op (confirmed against a real
	// mlx-lm install — the fused model came out byte-for-byte behaviorally
	// identical to the un-fine-tuned base, no error or warning). Re-quantizing
	// the result afterward is deliberately not done here either: verified
	// against a real (heavily overfit) adapter that re-quantizing to 4-bit
	// can wash out the fine-tuning signal entirely. The tradeoff is a fused
	// model roughly 3-4x the size of the original quantized base, which is
	// worth it for actually reflecting what was trained.
	args := []string{
		"--model", baseModel,
		"--adapter-path", adapterDir,
		"--save-path", savePath,
		"--dequantize",
	}

	cmd := exec.CommandContext(ctx, command, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return fmt.Errorf("%s failed: %w: %s", command, err, detail)
		}
		return fmt.Errorf("%s failed: %w", command, err)
	}

	return nil
}

// lookPathOrFriendlyError checks that command is resolvable on PATH,
// returning a clear, actionable error naming both common install methods
// if not.
func lookPathOrFriendlyError(command string) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf(
			"%s not found on PATH — install it with `pip install mlx-lm` or `brew install mlx-lm`, "+
				"and make sure the environment tlw serve runs in has that installation's bin directory on PATH",
			command,
		)
	}
	return nil
}

// buildArgs builds mlx_lm.lora's command-line arguments from cfg.
func buildArgs(cfg Config, dataDir, outDir string, batchSize int) []string {
	args := []string{
		"--model", cfg.BaseModel,
		"--train",
		"--data", dataDir,
		"--iters", strconv.Itoa(cfg.Iterations),
		"--adapter-path", outDir,
		"--batch-size", strconv.Itoa(batchSize),
	}
	if cfg.LearningRate > 0 {
		args = append(args, "--learning-rate", strconv.FormatFloat(cfg.LearningRate, 'g', -1, 64))
	}
	return args
}

// writeDataset writes examples to a temp directory as train.jsonl and
// valid.jsonl in mlx-lm's "completions" format, returning the directory and
// how many examples ended up in each split (so Train can size --batch-size
// to fit both).
func writeDataset(examples []registry.Example) (dir string, trainCount, validCount int, err error) {
	dir, err = os.MkdirTemp("", "tlw-dataset-*")
	if err != nil {
		return "", 0, 0, err
	}

	train, valid := splitExamples(examples)

	if err := writeJSONL(filepath.Join(dir, "train.jsonl"), train); err != nil {
		return "", 0, 0, err
	}
	if err := writeJSONL(filepath.Join(dir, "valid.jsonl"), valid); err != nil {
		return "", 0, 0, err
	}

	return dir, len(train), len(valid), nil
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

// parseProgressLine matches a single line of mlx_lm.lora's stdout against
// the train/val progress patterns, returning the parsed point and whether
// the line matched at all.
func parseProgressLine(line string) (ProgressPoint, bool) {
	if m := trainLineRe.FindStringSubmatch(line); m != nil {
		iter, _ := strconv.Atoi(m[1])
		trainLoss, _ := strconv.ParseFloat(m[2], 64)
		point := ProgressPoint{Timestamp: time.Now().UTC(), Iteration: iter, TrainLoss: &trainLoss}
		if tm := tokensPerSecRe.FindStringSubmatch(line); tm != nil {
			tps, _ := strconv.ParseFloat(tm[1], 64)
			point.TokensPerSec = &tps
		}
		if mm := peakMemGBRe.FindStringSubmatch(line); mm != nil {
			mem, _ := strconv.ParseFloat(mm[1], 64)
			point.PeakMemGB = &mem
		}
		return point, true
	}

	if m := valLineRe.FindStringSubmatch(line); m != nil {
		iter, _ := strconv.Atoi(m[1])
		valLoss, _ := strconv.ParseFloat(m[2], 64)
		return ProgressPoint{Timestamp: time.Now().UTC(), Iteration: iter, ValLoss: &valLoss}, true
	}

	return ProgressPoint{}, false
}

// runAndParse starts cmd, regex-parses its stdout for mlx_lm.lora's
// progress lines, and reports the outcome. A non-zero exit is reported with
// cmd's stderr (mlx_lm.lora prints tracebacks/errors there, confirmed
// against a real install) for real diagnostic detail instead of a bare exit
// code.
func runAndParse(cmd *exec.Cmd, outputDir string, onProgress func(ProgressPoint)) (Result, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start mlx_lm.lora: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if point, ok := parseProgressLine(scanner.Text()); ok {
			onProgress(point)
		}
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return Result{}, fmt.Errorf("mlx_lm.lora failed: %w: %s", waitErr, detail)
		}
		return Result{}, fmt.Errorf("mlx_lm.lora failed: %w", waitErr)
	}

	return Result{OutputDir: outputDir}, nil
}
