package registry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	datasetMetadataFile = "metadata.json"
	datasetExamplesFile = "data.jsonl"
)

// Dataset is a registry-tracked dataset's metadata.
type Dataset struct {
	Name        string    `json:"name"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// DatasetSummary is a Dataset plus its example count, as returned by
// ListDatasets so callers don't need a second round trip to size each one.
type DatasetSummary struct {
	Dataset
	PairCount int `json:"pairCount"`
}

// Example is a single input/output training pair, plus optional metadata to
// help organize a dataset (not used by training itself).
type Example struct {
	Input       string   `json:"input"`
	Output      string   `json:"output"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Source records how the example came to exist. "ai" means a local
	// model generated it (via datasetgen); empty means a human authored or
	// imported it. Approved is set once a human has reviewed an "ai"
	// example — an unapproved AI example is flagged in the UI so generated
	// data isn't trained on unchecked. NeedsReview lets a human explicitly
	// flag any example (AI or hand-written) for another look; it's
	// independent of the AI/Approved pair.
	Source      string `json:"source,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
	NeedsReview bool   `json:"needsReview,omitempty"`
}

// CreateDataset creates a new, empty dataset named name. title and
// description are optional, freeform metadata shown on the dataset's detail
// page — they don't affect training.
func (r *Registry) CreateDataset(name, title, description string) (Dataset, error) {
	dir := r.datasetDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Dataset{}, fmt.Errorf("create dataset directory: %w", err)
	}

	dataset := Dataset{Name: name, Title: title, Description: description, CreatedAt: time.Now().UTC()}

	data, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return Dataset{}, fmt.Errorf("marshal dataset metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, datasetMetadataFile), data, 0o644); err != nil {
		return Dataset{}, fmt.Errorf("write dataset metadata: %w", err)
	}

	examplesPath := filepath.Join(dir, datasetExamplesFile)
	if _, err := os.OpenFile(examplesPath, os.O_CREATE|os.O_WRONLY, 0o644); err != nil {
		return Dataset{}, fmt.Errorf("create dataset examples file: %w", err)
	}

	return dataset, nil
}

// GetDataset returns a single dataset's metadata (name, title, description,
// creation time), without its examples.
func (r *Registry) GetDataset(name string) (Dataset, error) {
	return r.readDatasetMetadata(name)
}

// ListDatasets returns all registry-tracked datasets, sorted by name.
func (r *Registry) ListDatasets() ([]DatasetSummary, error) {
	entries, err := os.ReadDir(r.datasetsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read datasets directory: %w", err)
	}

	var summaries []DatasetSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dataset, err := r.readDatasetMetadata(entry.Name())
		if err != nil {
			continue // skip directories without valid metadata
		}

		examples, err := r.ListExamples(entry.Name())
		if err != nil {
			continue
		}

		summaries = append(summaries, DatasetSummary{Dataset: dataset, PairCount: len(examples)})
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })

	return summaries, nil
}

// ListExamples returns every input/output pair in the named dataset.
func (r *Registry) ListExamples(name string) ([]Example, error) {
	f, err := os.Open(filepath.Join(r.datasetDir(name), datasetExamplesFile))
	if err != nil {
		return nil, fmt.Errorf("open dataset examples: %w", err)
	}
	defer f.Close()

	var examples []Example
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var example Example
		if err := json.Unmarshal(line, &example); err != nil {
			return nil, fmt.Errorf("parse dataset example: %w", err)
		}
		examples = append(examples, example)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dataset examples: %w", err)
	}

	return examples, nil
}

// AppendExamples adds examples to the end of the named dataset.
func (r *Registry) AppendExamples(name string, examples []Example) error {
	f, err := os.OpenFile(filepath.Join(r.datasetDir(name), datasetExamplesFile), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open dataset examples: %w", err)
	}
	defer f.Close()

	for _, example := range examples {
		data, err := json.Marshal(example)
		if err != nil {
			return fmt.Errorf("marshal dataset example: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write dataset example: %w", err)
		}
	}

	return nil
}

// UpdateExample overwrites the example at index (0-based, in the order
// ListExamples returns them). It's an error if index is out of range.
func (r *Registry) UpdateExample(name string, index int, example Example) error {
	examples, err := r.ListExamples(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(examples) {
		return fmt.Errorf("example index %d out of range (dataset has %d examples)", index, len(examples))
	}

	examples[index] = example
	return r.writeExamples(name, examples)
}

// ApproveExample marks the example at index as human-reviewed: it sets
// Approved and clears any NeedsReview flag, so it no longer shows up in the
// UI's "needs review" filter. It's an error if index is out of range.
func (r *Registry) ApproveExample(name string, index int) error {
	examples, err := r.ListExamples(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(examples) {
		return fmt.Errorf("example index %d out of range (dataset has %d examples)", index, len(examples))
	}

	examples[index].Approved = true
	examples[index].NeedsReview = false
	return r.writeExamples(name, examples)
}

// FlagExampleForReview marks the example at index as needing another human
// look. It also clears Approved, so a previously-approved example that's
// re-flagged stops reading as reviewed. It's an error if index is out of
// range.
func (r *Registry) FlagExampleForReview(name string, index int) error {
	examples, err := r.ListExamples(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(examples) {
		return fmt.Errorf("example index %d out of range (dataset has %d examples)", index, len(examples))
	}

	examples[index].NeedsReview = true
	examples[index].Approved = false
	return r.writeExamples(name, examples)
}

// DeleteExample removes the example at index (0-based, in the order
// ListExamples returns them). It's an error if index is out of range.
func (r *Registry) DeleteExample(name string, index int) error {
	examples, err := r.ListExamples(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(examples) {
		return fmt.Errorf("example index %d out of range (dataset has %d examples)", index, len(examples))
	}

	examples = append(examples[:index], examples[index+1:]...)
	return r.writeExamples(name, examples)
}

// writeExamples overwrites a dataset's entire examples file, used by
// UpdateExample/DeleteExample since editing or removing a single line means
// rewriting the rest of the file anyway (JSONL isn't seekable-in-place).
func (r *Registry) writeExamples(name string, examples []Example) error {
	f, err := os.OpenFile(filepath.Join(r.datasetDir(name), datasetExamplesFile), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open dataset examples: %w", err)
	}
	defer f.Close()

	for _, example := range examples {
		data, err := json.Marshal(example)
		if err != nil {
			return fmt.Errorf("marshal dataset example: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write dataset example: %w", err)
		}
	}

	return nil
}

// DeleteDataset removes a dataset's directory (metadata and examples). It's
// an error to delete a dataset that doesn't exist.
func (r *Registry) DeleteDataset(name string) error {
	dir := r.datasetDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("dataset %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete dataset %q: %w", name, err)
	}
	return nil
}

func (r *Registry) readDatasetMetadata(name string) (Dataset, error) {
	data, err := os.ReadFile(filepath.Join(r.datasetDir(name), datasetMetadataFile))
	if err != nil {
		return Dataset{}, err
	}

	var dataset Dataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return Dataset{}, fmt.Errorf("parse metadata for dataset %q: %w", name, err)
	}

	return dataset, nil
}
