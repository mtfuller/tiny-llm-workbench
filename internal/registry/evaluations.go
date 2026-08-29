package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	evaluationMetadataFile = "definition.json"
	evaluationVersionsFile = "versions.json"
)

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

// VerifyStep is a shell command run in a test case's sandbox instance after
// the agent's turn finishes, checked against its own combined stdout/stderr
// output with the same assertion types used against a reply. Evaluations-only:
// a Benchmark tests a model directly with no sandbox to verify against. The
// command's exit code is
// deliberately not itself pass/fail — a command like `grep` legitimately
// exits non-zero for "not found," so whether that counts as success is
// entirely up to the assertions checked against its output text, the same
// deterministic-assertion philosophy used everywhere else in this project.
type VerifyStep struct {
	Command    string      `json:"command"`
	Assertions []Assertion `json:"assertions"`
}

// TestCase is one prompt + the assertions its reply must satisfy, plus two
// fields only Evaluations use (a Benchmark's test cases always leave them
// empty, since there's no sandbox to set up or verify): Workspace names a
// TEST workspace (see registry.Workspace) whose files are copied into a
// fresh sandbox for the test case, so a scenario's starting state is edited
// as real files rather than scripted; VerifyCommands are run after the
// agent's turn to check the sandbox's resulting state. Tags are optional,
// freeform labels for filtering the test case list.
type TestCase struct {
	ID             string       `json:"id"`
	Prompt         string       `json:"prompt"`
	Workspace      string       `json:"workspace,omitempty"`
	Assertions     []Assertion  `json:"assertions"`
	VerifyCommands []VerifyStep `json:"verifyCommands,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
	// Source is "ai" for a test case whose prompt a local model generated
	// (via testcasegen), empty for a hand-written one. Approved is set once
	// a human has reviewed an "ai" case. NeedsReview lets a human flag any
	// case (generated or hand-written) for another look before the draft is
	// published — same review model as a dataset's Example. Currently only
	// the Benchmarks UI surfaces these.
	Source      string `json:"source,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
	NeedsReview bool   `json:"needsReview,omitempty"`
}

// Evaluation is a registry-tracked test suite run against a set of agents.
// There is no evaluation-level environment binding: each test case names its
// own TEST workspace (TestCase.Workspace), and a fresh copy of it is the
// sandbox that case's setup, the agent's turn, and its VerifyCommands all
// share.
//
// TestCases here is the *draft*: freely editable (Add/Update/DeleteTestCase)
// and never run directly — mirrors registry.Benchmark exactly. Version is
// the number of the most recently published EvaluationVersion (0 if none
// has ever been published); only PublishVersion changes it.
type Evaluation struct {
	Name      string     `json:"name"`
	Version   int        `json:"version"`
	TestCases []TestCase `json:"testCases"`
	CreatedAt time.Time  `json:"createdAt"`
}

// EvaluationVersion is an immutable snapshot of an evaluation's test cases,
// created by PublishVersion. Once published, a version's TestCases never
// change — editing the evaluation's draft afterward has no effect on
// versions already published.
type EvaluationVersion struct {
	Version     int        `json:"version"`
	TestCases   []TestCase `json:"testCases"`
	PublishedAt time.Time  `json:"publishedAt"`
}

func (r *Registry) evaluationDir(name string) string {
	return filepath.Join(r.evaluationsDir(), name)
}

func (r *Registry) evaluationsDir() string {
	return filepath.Join(r.root, "evaluations")
}

// SaveEvaluation writes eval's definition, creating or overwriting it.
// Version and CreatedAt are always computed here, not taken from eval: a
// first save starts at Version 0 (nothing published yet) with CreatedAt set
// to now; a later save preserves both regardless of what eval's caller set
// — draft edits (Add/Update/DeleteTestCase) never change Version. Only
// PublishVersion changes Version.
func (r *Registry) SaveEvaluation(eval Evaluation) error {
	if existing, err := r.GetEvaluation(eval.Name); err == nil {
		eval.CreatedAt = existing.CreatedAt
		eval.Version = existing.Version
	} else {
		eval.CreatedAt = time.Now().UTC()
		eval.Version = 0
	}

	return r.writeEvaluationDefinition(eval)
}

func (r *Registry) writeEvaluationDefinition(eval Evaluation) error {
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

// DeleteEvaluation removes an evaluation's directory (its definition, and
// any published versions). It's an error to delete an evaluation that
// doesn't exist.
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

// PublishEvaluationVersion snapshots the evaluation's current draft
// TestCases into a new, immutable EvaluationVersion (number = the previous
// latest + 1) and advances the evaluation's Version to it. It's an error to
// publish an evaluation with no draft test cases — there'd be nothing for a
// run to exercise. Named distinctly from registry.Benchmark's identically-
// shaped PublishVersion (same for ListEvaluationVersions/GetEvaluationVersion
// below) only because both types share the *Registry receiver in this one
// package — a plain "PublishVersion(name string)" can't be declared twice.
func (r *Registry) PublishEvaluationVersion(evaluationName string) (EvaluationVersion, error) {
	eval, err := r.GetEvaluation(evaluationName)
	if err != nil {
		return EvaluationVersion{}, err
	}
	if len(eval.TestCases) == 0 {
		return EvaluationVersion{}, fmt.Errorf("evaluation %q has no test cases to publish", evaluationName)
	}

	versions, err := r.ListEvaluationVersions(evaluationName)
	if err != nil {
		return EvaluationVersion{}, err
	}

	newVersion := EvaluationVersion{
		Version:     eval.Version + 1,
		TestCases:   append([]TestCase(nil), eval.TestCases...),
		PublishedAt: time.Now().UTC(),
	}

	if err := r.writeEvaluationVersions(evaluationName, append(versions, newVersion)); err != nil {
		return EvaluationVersion{}, err
	}

	eval.Version = newVersion.Version
	if err := r.writeEvaluationDefinition(eval); err != nil {
		return EvaluationVersion{}, err
	}

	return newVersion, nil
}

// ListEvaluationVersions returns every published version of the named
// evaluation, oldest first. An evaluation with nothing published yet
// returns an empty slice, not an error.
func (r *Registry) ListEvaluationVersions(evaluationName string) ([]EvaluationVersion, error) {
	data, err := os.ReadFile(filepath.Join(r.evaluationDir(evaluationName), evaluationVersionsFile))
	if os.IsNotExist(err) {
		return []EvaluationVersion{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read evaluation versions: %w", err)
	}

	var versions []EvaluationVersion
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("parse evaluation versions: %w", err)
	}

	return versions, nil
}

// GetEvaluationVersion returns one published version of the named
// evaluation.
func (r *Registry) GetEvaluationVersion(evaluationName string, version int) (EvaluationVersion, error) {
	versions, err := r.ListEvaluationVersions(evaluationName)
	if err != nil {
		return EvaluationVersion{}, err
	}

	for _, v := range versions {
		if v.Version == version {
			return v, nil
		}
	}

	return EvaluationVersion{}, fmt.Errorf("evaluation %q has no version %d", evaluationName, version)
}

func (r *Registry) writeEvaluationVersions(evaluationName string, versions []EvaluationVersion) error {
	dir := r.evaluationDir(evaluationName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create evaluation directory: %w", err)
	}

	data, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evaluation versions: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, evaluationVersionsFile), data, 0o644); err != nil {
		return fmt.Errorf("write evaluation versions: %w", err)
	}

	return nil
}

// AddEvaluationTestCases appends one or more manually-entered (or
// generated) test cases to the named evaluation's draft, ignoring any ID
// the caller set on tcs (a fresh one is always assigned) — mirrors
// registry.Benchmark's AddTestCases (named distinctly for the same
// same-package-receiver-collision reason as PublishEvaluationVersion
// above). This never touches any published version.
func (r *Registry) AddEvaluationTestCases(evaluationName string, tcs []TestCase) error {
	eval, err := r.GetEvaluation(evaluationName)
	if err != nil {
		return err
	}

	now := time.Now().UnixNano()
	for i, tc := range tcs {
		tc.ID = fmt.Sprintf("tc-%d-%d", now, i)
		eval.TestCases = append(eval.TestCases, tc)
	}

	return r.SaveEvaluation(eval)
}

// UpdateEvaluationTestCase overwrites the draft test case at index
// (0-based, in the order GetEvaluation returns them), keeping its existing
// ID. It's an error if index is out of range. This never touches any
// published version.
func (r *Registry) UpdateEvaluationTestCase(evaluationName string, index int, tc TestCase) error {
	eval, err := r.GetEvaluation(evaluationName)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(eval.TestCases) {
		return fmt.Errorf("test case index %d out of range (evaluation has %d test cases)", index, len(eval.TestCases))
	}

	tc.ID = eval.TestCases[index].ID
	eval.TestCases[index] = tc
	return r.SaveEvaluation(eval)
}

// DeleteEvaluationTestCase removes the draft test case at index (0-based,
// in the order GetEvaluation returns them). It's an error if index is out
// of range. This never touches any published version.
func (r *Registry) DeleteEvaluationTestCase(evaluationName string, index int) error {
	eval, err := r.GetEvaluation(evaluationName)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(eval.TestCases) {
		return fmt.Errorf("test case index %d out of range (evaluation has %d test cases)", index, len(eval.TestCases))
	}

	eval.TestCases = append(eval.TestCases[:index], eval.TestCases[index+1:]...)
	return r.SaveEvaluation(eval)
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
