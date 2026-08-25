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
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// DatasetSummary is a Dataset plus its example count, as returned by
// ListDatasets so callers don't need a second round trip to size each one.
type DatasetSummary struct {
	Dataset
	PairCount int `json:"pairCount"`
}

// Example is a single input/output training pair.
type Example struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

// CreateDataset creates a new, empty dataset named name.
func (r *Registry) CreateDataset(name string) (Dataset, error) {
	dir := r.datasetDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Dataset{}, fmt.Errorf("create dataset directory: %w", err)
	}

	dataset := Dataset{Name: name, CreatedAt: time.Now().UTC()}

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
