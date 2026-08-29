package server

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// listDatasetsHandler responds with every registry-tracked dataset and its
// example count.
func listDatasetsHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := datasets.ListDatasets()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if list == nil {
			list = []registry.DatasetSummary{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// createDatasetRequest is the POST /api/datasets request body.
type createDatasetRequest struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// createDatasetHandler creates a new, empty dataset.
func createDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDatasetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}

		dataset, err := datasets.CreateDataset(req.Name, req.Title, req.Description)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, dataset)
	}
}

// deleteDatasetHandler removes a dataset and all its examples.
func deleteDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := datasets.DeleteDataset(r.PathValue("name")); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// datasetDetail is the GET /api/datasets/{name} response body.
type datasetDetail struct {
	Name        string             `json:"name"`
	Title       string             `json:"title,omitempty"`
	Description string             `json:"description,omitempty"`
	Examples    []registry.Example `json:"examples"`
}

// getDatasetHandler responds with a single dataset's metadata and
// input/output pairs.
func getDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		examples, err := datasets.ListExamples(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if examples == nil {
			examples = []registry.Example{}
		}

		// Metadata lives in the same directory ListExamples just proved
		// exists, so a failure here is unexpected — fall back to Name-only
		// metadata rather than failing the whole request over it.
		dataset, _ := datasets.GetDataset(name)

		writeJSON(w, http.StatusOK, datasetDetail{
			Name:        name,
			Title:       dataset.Title,
			Description: dataset.Description,
			Examples:    examples,
		})
	}
}

// addExamplesRequest is the POST /api/datasets/{name}/examples request
// body — used for both manually adding one example (a single-element
// Examples) and bulk import from the frontend's parsed CSV/JSONL.
type addExamplesRequest struct {
	Examples []registry.Example `json:"examples"`
}

// addExamplesHandler appends one or more manually-entered examples to a
// dataset.
func addExamplesHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req addExamplesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if len(req.Examples) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("at least one example is required"))
			return
		}

		if err := datasets.AppendExamples(name, req.Examples); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, req.Examples)
	}
}

// updateExampleHandler overwrites a single example, addressed by its
// position in the dataset (as returned by GET /api/datasets/{name}).
func updateExampleHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid example index: %w", err))
			return
		}

		var example registry.Example
		if err := json.NewDecoder(r.Body).Decode(&example); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		if err := datasets.UpdateExample(name, index, example); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusOK, example)
	}
}

// approveExampleHandler marks a single AI-generated example as
// human-approved, addressed by its position in the dataset.
func approveExampleHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid example index: %w", err))
			return
		}

		if err := datasets.ApproveExample(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// flagExampleHandler marks a single example as needing another human
// review, addressed by its position in the dataset.
func flagExampleHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid example index: %w", err))
			return
		}

		if err := datasets.FlagExampleForReview(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteExampleHandler removes a single example, addressed by its position
// in the dataset (as returned by GET /api/datasets/{name}).
func deleteExampleHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid example index: %w", err))
			return
		}

		if err := datasets.DeleteExample(name, index); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// exportDatasetHandler streams a dataset's examples as a downloadable file,
// in either JSONL (default) or CSV, chosen via the "format" query param.
func exportDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		examples, err := datasets.ListExamples(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}

		format := r.URL.Query().Get("format")
		if format == "" {
			format = "jsonl"
		}

		switch format {
		case "jsonl":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.jsonl"`, name))
			for _, example := range examples {
				data, err := json.Marshal(example)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				if _, err := w.Write(append(data, '\n')); err != nil {
					return
				}
			}
		case "csv":
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, name))
			cw := csv.NewWriter(w)
			if err := cw.Write([]string{"input", "output", "description", "tags"}); err != nil {
				return
			}
			for _, example := range examples {
				row := []string{example.Input, example.Output, example.Description, strings.Join(example.Tags, ";")}
				if err := cw.Write(row); err != nil {
					return
				}
			}
			cw.Flush()
		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported export format %q", format))
		}
	}
}

// importDatasetRequest is the POST /api/datasets/{name}/import request
// body — content is the raw file text, parsed server-side so CSV parsing
// (quoted fields, escaped quotes) only has one correct implementation
// rather than duplicating it in the browser.
type importDatasetRequest struct {
	Format  string `json:"format"`
	Content string `json:"content"`
}

// importDatasetHandler parses a CSV or JSONL file's examples and appends
// them to a dataset.
func importDatasetHandler(datasets datasetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req importDatasetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		var examples []registry.Example
		var err error
		switch req.Format {
		case "csv":
			examples, err = parseCSVExamples(req.Content)
		case "jsonl", "":
			examples, err = parseJSONLExamples(req.Content)
		default:
			err = fmt.Errorf("unsupported import format %q", req.Format)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if len(examples) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("no examples found in the imported file"))
			return
		}

		if err := datasets.AppendExamples(name, examples); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, examples)
	}
}

// parseCSVExamples parses content as CSV. The header must include "input"
// and "output" columns (case-insensitive, any order); optional "description"
// and "tags" columns are picked up if present ("tags" is semicolon-separated
// within its cell, since commas are already CSV's own separator). Missing
// "input"/"output" columns are rejected so a malformed file fails loudly
// instead of silently importing garbage.
func parseCSVExamples(content string) ([]registry.Example, error) {
	reader := csv.NewReader(strings.NewReader(content))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("CSV file is empty")
	}

	col := make(map[string]int, len(rows[0]))
	for i, name := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(name))] = i
	}
	inputCol, hasInput := col["input"]
	outputCol, hasOutput := col["output"]
	if !hasInput || !hasOutput {
		return nil, errors.New(`CSV header must include "input" and "output" columns`)
	}
	descriptionCol, hasDescription := col["description"]
	tagsCol, hasTags := col["tags"]

	get := func(row []string, i int) string {
		if i < len(row) {
			return row[i]
		}
		return ""
	}

	examples := make([]registry.Example, 0, len(rows)-1)
	for _, row := range rows[1:] {
		example := registry.Example{Input: get(row, inputCol), Output: get(row, outputCol)}
		if hasDescription {
			example.Description = get(row, descriptionCol)
		}
		if hasTags {
			example.Tags = splitTags(get(row, tagsCol))
		}
		examples = append(examples, example)
	}
	return examples, nil
}

// splitTags parses a semicolon-separated tags cell into a trimmed,
// non-empty tag list (nil if the cell is blank).
func splitTags(cell string) []string {
	if strings.TrimSpace(cell) == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(cell, ";") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// parseJSONLExamples parses content as newline-delimited JSON objects.
func parseJSONLExamples(content string) ([]registry.Example, error) {
	var examples []registry.Example
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var example registry.Example
		if err := json.Unmarshal([]byte(line), &example); err != nil {
			return nil, fmt.Errorf("parse JSONL line: %w", err)
		}
		examples = append(examples, example)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("read JSONL content: %w", err)
	}
	return examples, nil
}

// generateVariationsRequest is the POST /api/datasets/{name}/variations
// request body.
type generateVariationsRequest struct {
	Model string           `json:"model"`
	Seed  registry.Example `json:"seed"`
	Count int              `json:"count"`
}

// generateVariationsHandler asks a local LLM for variations on a seed
// example and appends the results to the named dataset.
func generateVariationsHandler(datasets datasetStore, generator variationGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var req generateVariationsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Model == "" {
			writeError(w, http.StatusBadRequest, errors.New("model is required"))
			return
		}
		if req.Count <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("count must be positive"))
			return
		}

		examples, err := generator.Variations(r.Context(), req.Model, req.Seed, req.Count)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}

		if err := datasets.AppendExamples(name, examples); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, examples)
	}
}
