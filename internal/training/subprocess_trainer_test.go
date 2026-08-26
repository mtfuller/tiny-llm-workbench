package training

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// writeFakeScript writes an executable shell script to a temp file and
// returns its path. Tests run these directly (no interpreter wrapper) as a
// stand-in for mlx_lm.lora, so they don't depend on any Python or MLX
// install.
func writeFakeScript(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake.sh")
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("failed to write fake script: %v", err)
	}
	return path
}

func TestBuildArgs(t *testing.T) {
	cfg := Config{BaseModel: "mlx-community/test-model", Iterations: 500, LearningRate: 1e-5}
	args := buildArgs(cfg, "/data", "/out", 4)

	want := []string{
		"--model", "mlx-community/test-model",
		"--train",
		"--data", "/data",
		"--iters", "500",
		"--adapter-path", "/out",
		"--batch-size", "4",
		"--learning-rate", "1e-05",
	}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("buildArgs() = %v, want %v", args, want)
	}
}

func TestBuildArgsOmitsZeroLearningRate(t *testing.T) {
	args := buildArgs(Config{BaseModel: "m", Iterations: 10}, "/data", "/out", 4)
	for _, a := range args {
		if a == "--learning-rate" {
			t.Errorf("buildArgs() = %v, want no --learning-rate flag when unset", args)
		}
	}
}

func TestBuildArgsPassesBatchSize(t *testing.T) {
	args := buildArgs(Config{BaseModel: "m", Iterations: 10}, "/data", "/out", 3)
	want := "--batch-size 3"
	if !strings.Contains(strings.Join(args, " "), want) {
		t.Errorf("buildArgs() = %v, want it to contain %q", args, want)
	}
}

func TestBatchSizeForCapsAtDefault(t *testing.T) {
	if got := batchSizeFor(9, 20); got != defaultBatchSize {
		t.Errorf("batchSizeFor(9, 20) = %d, want the default %d when both splits exceed it", got, defaultBatchSize)
	}
}

// TestBatchSizeForShrinksToSmallestSplit guards the exact bug reported by a
// real user: a 12-example dataset splits into train=9/valid=3 (the 80/20
// split below), and mlx_lm.lora's own default --batch-size (4) is larger
// than the 3-example validation split, so it refuses to run at all
// ("Dataset must have at least batch_size=4 examples but only has 3").
func TestBatchSizeForShrinksToSmallestSplit(t *testing.T) {
	train, valid := splitExamples(make([]registry.Example, 12))
	if len(valid) != 3 {
		t.Fatalf("splitExamples(12 examples) valid = %d, want 3 (the split this test guards against changing silently)", len(valid))
	}
	if got := batchSizeFor(len(train), len(valid)); got != 3 {
		t.Errorf("batchSizeFor(%d, %d) = %d, want 3 (the smaller split)", len(train), len(valid), got)
	}
}

func TestBatchSizeForNeverGoesBelowOne(t *testing.T) {
	if got := batchSizeFor(1, 1); got != 1 {
		t.Errorf("batchSizeFor(1, 1) = %d, want 1", got)
	}
}

func TestSplitExamplesSmallDatasetReusesAll(t *testing.T) {
	examples := []registry.Example{{Input: "a"}, {Input: "b"}}
	train, valid := splitExamples(examples)
	if len(train) != 2 || len(valid) != 2 {
		t.Errorf("splitExamples() = train:%d valid:%d, want both to reuse all examples for a tiny dataset", len(train), len(valid))
	}
}

func TestSplitExamplesLargeDatasetHoldsOutSlice(t *testing.T) {
	examples := make([]registry.Example, 10)
	for i := range examples {
		examples[i] = registry.Example{Input: string(rune('a' + i))}
	}
	train, valid := splitExamples(examples)
	if len(train)+len(valid) != len(examples) {
		t.Errorf("splitExamples() = train:%d valid:%d, want them to partition all %d examples", len(train), len(valid), len(examples))
	}
	if len(valid) == 0 {
		t.Error("splitExamples() valid is empty, want a held-out slice for a large dataset")
	}
}

func TestParseProgressLineTrainLossOnly(t *testing.T) {
	point, ok := parseProgressLine("Iter 1: Train loss 2.5")
	if !ok {
		t.Fatal("parseProgressLine() ok = false, want true")
	}
	if point.Iteration != 1 || point.TrainLoss == nil || *point.TrainLoss != 2.5 {
		t.Errorf("parseProgressLine() = %+v, want iteration=1 trainLoss=2.5", point)
	}
	if point.TokensPerSec != nil || point.PeakMemGB != nil {
		t.Errorf("parseProgressLine() = %+v, want nil TokensPerSec/PeakMemGB when absent from the line", point)
	}
}

// TestParseProgressLineTrainLossWithTokensAndMem guards against a real bug:
// an earlier version folded Tokens/sec and Peak mem into TRAIN_LINE itself
// as trailing optional groups separated by a non-greedy `.*?`, which regex
// can satisfy by matching zero characters — so it matched successfully but
// silently returned no value for either field even when the real
// mlx_lm.lora line had them (confirmed against a real run).
func TestParseProgressLineTrainLossWithTokensAndMem(t *testing.T) {
	line := "Iter 10: Train loss 1.242, Learning Rate 1.000e-05, It/sec 4.857, " +
		"Tokens/sec 871.760, Trained Tokens 1795, Peak mem 1.106 GB"
	point, ok := parseProgressLine(line)
	if !ok {
		t.Fatal("parseProgressLine() ok = false, want true")
	}
	if point.Iteration != 10 || point.TrainLoss == nil || *point.TrainLoss != 1.242 {
		t.Errorf("parseProgressLine() = %+v, want iteration=10 trainLoss=1.242", point)
	}
	if point.TokensPerSec == nil || *point.TokensPerSec != 871.760 {
		t.Errorf("parseProgressLine() TokensPerSec = %v, want 871.760", point.TokensPerSec)
	}
	if point.PeakMemGB == nil || *point.PeakMemGB != 1.106 {
		t.Errorf("parseProgressLine() PeakMemGB = %v, want 1.106", point.PeakMemGB)
	}
}

func TestParseProgressLineValLoss(t *testing.T) {
	point, ok := parseProgressLine("Iter 10: Val loss 0.083, Val took 0.207s")
	if !ok {
		t.Fatal("parseProgressLine() ok = false, want true")
	}
	if point.Iteration != 10 || point.ValLoss == nil || *point.ValLoss != 0.083 {
		t.Errorf("parseProgressLine() = %+v, want iteration=10 valLoss=0.083", point)
	}
	if point.TrainLoss != nil {
		t.Errorf("parseProgressLine() TrainLoss = %v, want nil for a Val loss line", point.TrainLoss)
	}
}

func TestParseProgressLineNoMatch(t *testing.T) {
	if _, ok := parseProgressLine("Loading pretrained model"); ok {
		t.Error("parseProgressLine() ok = true, want false for a non-progress line")
	}
}

func TestRunAndParseSuccess(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo "Iter 1: Train loss 2.5"
echo "Iter 2: Train loss 1.5, Tokens/sec 100.0, Peak mem 4.2 GB"
`)

	var progress []ProgressPoint
	cmd := exec.CommandContext(context.Background(), script)
	result, err := runAndParse(cmd, "/tmp/out", func(p ProgressPoint) { progress = append(progress, p) })
	if err != nil {
		t.Fatalf("runAndParse() error = %v", err)
	}
	if result.OutputDir != "/tmp/out" {
		t.Errorf("result.OutputDir = %q, want %q", result.OutputDir, "/tmp/out")
	}
	if len(progress) != 2 {
		t.Fatalf("progress = %+v, want 2 points", progress)
	}
	if *progress[0].TrainLoss != 2.5 || progress[0].Iteration != 1 {
		t.Errorf("progress[0] = %+v, want iteration=1 trainLoss=2.5", progress[0])
	}
	if progress[1].PeakMemGB == nil || *progress[1].PeakMemGB != 4.2 {
		t.Errorf("progress[1].PeakMemGB = %v, want 4.2", progress[1].PeakMemGB)
	}
}

// TestRunAndParseSuccessWithNoProgressLines documents an intentional
// difference from the old JSON-protocol design: success is now determined
// purely by exit code, not a synthetic "done" sentinel, so a clean exit
// with no recognizable progress output is still a success.
func TestRunAndParseSuccessWithNoProgressLines(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo "Loading pretrained model"
`)

	cmd := exec.CommandContext(context.Background(), script)
	_, err := runAndParse(cmd, "/tmp/out", func(ProgressPoint) {})
	if err != nil {
		t.Errorf("runAndParse() error = %v, want nil for a clean exit with no progress lines", err)
	}
}

func TestRunAndParseNonZeroExitSurfacesStderr(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo "boom: no module named mlx_lm" >&2
exit 7
`)

	cmd := exec.CommandContext(context.Background(), script)
	_, err := runAndParse(cmd, "/tmp/out", func(ProgressPoint) {})
	if err == nil || !strings.Contains(err.Error(), "no module named mlx_lm") {
		t.Errorf("runAndParse() error = %v, want it to include the command's stderr", err)
	}
}

func TestSubprocessTrainerEndToEnd(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo "Iter 1: Train loss 2.5"
`)

	trainer := &SubprocessTrainer{Command: script}
	examples := []registry.Example{{Input: "hi", Output: "hello!"}}

	var progress []ProgressPoint
	result, err := trainer.Train(context.Background(), Config{BaseModel: "m", Iterations: 10}, examples, func(p ProgressPoint) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}
	if result.OutputDir == "" {
		t.Error("result.OutputDir is empty, want the temp adapter directory")
	}
	if len(progress) != 1 {
		t.Errorf("progress = %+v, want 1 point", progress)
	}
}

func TestSubprocessTrainerCommandNotFound(t *testing.T) {
	trainer := &SubprocessTrainer{Command: "tlw-definitely-not-a-real-command"}
	examples := []registry.Example{{Input: "hi", Output: "hello!"}}

	_, err := trainer.Train(context.Background(), Config{BaseModel: "m", Iterations: 10}, examples, func(ProgressPoint) {})
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("Train() error = %v, want a clear \"not found on PATH\" message", err)
	}
}

func TestFuseSuccess(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
exit 0
`)

	trainer := &SubprocessTrainer{FuseCommand: script}
	if err := trainer.Fuse(context.Background(), "mlx-community/test-model", "/tmp/adapter", "/tmp/fused"); err != nil {
		t.Fatalf("Fuse() error = %v", err)
	}
}

func TestFuseNonZeroExitSurfacesStderr(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo "unknown data type: U32" >&2
exit 1
`)

	trainer := &SubprocessTrainer{FuseCommand: script}
	err := trainer.Fuse(context.Background(), "mlx-community/test-model", "/tmp/adapter", "/tmp/fused")
	if err == nil || !strings.Contains(err.Error(), "unknown data type: U32") {
		t.Errorf("Fuse() error = %v, want it to include the command's stderr", err)
	}
}

func TestFuseCommandNotFound(t *testing.T) {
	trainer := &SubprocessTrainer{FuseCommand: "tlw-definitely-not-a-real-command"}
	err := trainer.Fuse(context.Background(), "mlx-community/test-model", "/tmp/adapter", "/tmp/fused")
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("Fuse() error = %v, want a clear \"not found on PATH\" message", err)
	}
}
