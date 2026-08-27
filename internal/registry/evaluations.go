package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const evaluationMetadataFile = "definition.json"

// Assertion is a deterministic check against a reply's text. Type is one of
// "contains", "not_contains", "regex", "json_schema", "similarity".
//
// Value's meaning depends on Type: the substring/pattern to check for
// contains/not_contains/regex, the JSON Schema document for json_schema, or
// the reference text to compare against for similarity. Threshold is only
// used by "similarity" (the minimum required similarity ratio, in (0, 1]).
type Assertion struct {
	Type      string  `json:"type"`
	Value     string  `json:"value"`
	Threshold float64 `json:"threshold,omitempty"`
}

// TestCase is one prompt + the assertions its reply must satisfy. Tags are
// optional, freeform labels (only used by Benchmarks today, for filtering
// the test case list — same role as Example.Tags for datasets).
type TestCase struct {
	ID         string      `json:"id"`
	Prompt     string      `json:"prompt"`
	Assertions []Assertion `json:"assertions"`
	Tags       []string    `json:"tags,omitempty"`
}

// Evaluation is a registry-tracked test suite run against a set of agents.
// Environment is optional: if set, running the evaluation launches a real
// instance of it for the run's duration (see internal/evaluations) — but
// agents can't act on it yet, since Phase 3 agents have no way to invoke an
// Environment's tools.
type Evaluation struct {
	Name        string     `json:"name"`
	Environment string     `json:"environment,omitempty"`
	TestCases   []TestCase `json:"testCases"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (r *Registry) evaluationDir(name string) string {
	return filepath.Join(r.evaluationsDir(), name)
}

func (r *Registry) evaluationsDir() string {
	return filepath.Join(r.root, "evaluations")
}

// SaveEvaluation writes eval's definition, creating or overwriting it.
func (r *Registry) SaveEvaluation(eval Evaluation) error {
	dir := r.evaluationDir(eval.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create evaluation directory: %w", err)
	}

	data, err := json.MarshalIndent(eval, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evaluation definition: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, evaluationMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write evaluation definition: %w", err)
	}

	return nil
}

// GetEvaluation returns the named evaluation's definition.
func (r *Registry) GetEvaluation(name string) (Evaluation, error) {
	data, err := os.ReadFile(filepath.Join(r.evaluationDir(name), evaluationMetadataFile))
	if err != nil {
		return Evaluation{}, fmt.Errorf("read evaluation %q: %w", name, err)
	}

	var eval Evaluation
	if err := json.Unmarshal(data, &eval); err != nil {
		return Evaluation{}, fmt.Errorf("parse definition for evaluation %q: %w", name, err)
	}

	return eval, nil
}

// DeleteEvaluation removes an evaluation's directory (its definition). It's
// an error to delete an evaluation that doesn't exist.
func (r *Registry) DeleteEvaluation(name string) error {
	dir := r.evaluationDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("evaluation %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete evaluation %q: %w", name, err)
	}
	return nil
}

// ListEvaluations returns every registry-tracked evaluation, sorted by name.
func (r *Registry) ListEvaluations() ([]Evaluation, error) {
	entries, err := os.ReadDir(r.evaluationsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read evaluations directory: %w", err)
	}

	var evals []Evaluation
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		eval, err := r.GetEvaluation(entry.Name())
		if err != nil {
			continue // skip directories without a valid definition
		}
		evals = append(evals, eval)
	}

	sort.Slice(evals, func(i, j int) bool { return evals[i].Name < evals[j].Name })

	return evals, nil
}
