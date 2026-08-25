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

// writeFakeScript writes a shell script to a temp file and returns its path.
// Tests run these with "sh" as the interpreter so they don't depend on a
// working Python install (this sandbox's python3 is broken).
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
	args := buildArgs(cfg, "/data", "/out")

	want := []string{
		"--model", "mlx-community/test-model",
		"--data-dir", "/data",
		"--output-dir", "/out",
		"--iters", "500",
		"--learning-rate", "1e-05",
	}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("buildArgs() = %v, want %v", args, want)
	}
}

func TestBuildArgsOmitsZeroLearningRate(t *testing.T) {
	args := buildArgs(Config{BaseModel: "m", Iterations: 10}, "/data", "/out")
	for _, a := range args {
		if a == "--learning-rate" {
			t.Errorf("buildArgs() = %v, want no --learning-rate flag when unset", args)
		}
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

func TestRunAndParseSuccess(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo '{"type":"progress","iteration":1,"trainLoss":2.5}'
echo '{"type":"progress","iteration":2,"trainLoss":1.5,"peakMemGB":4.2}'
echo '{"type":"done"}'
`)

	var progress []ProgressPoint
	cmd := exec.CommandContext(context.Background(), "sh", script)
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

func TestRunAndParseScriptReportedError(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo '{"type":"error","message":"mlx_lm not installed"}'
exit 1
`)

	cmd := exec.CommandContext(context.Background(), "sh", script)
	_, err := runAndParse(cmd, "/tmp/out", func(ProgressPoint) {})
	if err == nil || !strings.Contains(err.Error(), "mlx_lm not installed") {
		t.Errorf("runAndParse() error = %v, want it to surface the script's reported message", err)
	}
}

func TestRunAndParseExitsWithoutDone(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo '{"type":"progress","iteration":1,"trainLoss":2.5}'
`)

	cmd := exec.CommandContext(context.Background(), "sh", script)
	_, err := runAndParse(cmd, "/tmp/out", func(ProgressPoint) {})
	if err == nil {
		t.Error("runAndParse() error = nil, want an error when the script exits without a done message")
	}
}

func TestRunAndParseNonZeroExitSurfacesStderr(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo "boom: no module named mlx_lm" >&2
exit 7
`)

	cmd := exec.CommandContext(context.Background(), "sh", script)
	_, err := runAndParse(cmd, "/tmp/out", func(ProgressPoint) {})
	if err == nil || !strings.Contains(err.Error(), "no module named mlx_lm") {
		t.Errorf("runAndParse() error = %v, want it to include the script's stderr", err)
	}
}

func TestSubprocessTrainerEndToEnd(t *testing.T) {
	script := writeFakeScript(t, `#!/bin/sh
echo '{"type":"progress","iteration":1,"trainLoss":2.5}'
echo '{"type":"done"}'
`)

	trainer := &SubprocessTrainer{Python: "sh", ScriptPath: script}
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
