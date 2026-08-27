package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const knowledgeMetadataFile = "definition.json"

// KnowledgeRecord is one queryable entry in a KnowledgeBase — a title plus
// free-text content an Agent's Knowledge node searches against.
type KnowledgeRecord struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
}

// KnowledgeBase is a registry-tracked, independent collection of records an
// Agent can query via a "knowledge" canvas node — see internal/knowledge.
// Deliberately not tied to an Environment/container: querying is plain
// in-process text matching, nothing to launch. Querying is deterministic
// keyword/substring matching against Title/Content (see
// internal/knowledge.Query), not embeddings/semantic search — consistent
// with the rest of this app's deterministic mechanisms (Decision node
// keyword matching, contains/regex assertions), and avoiding a second
// ML runtime dependency.
type KnowledgeBase struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Records     []KnowledgeRecord `json:"records"`
	CreatedAt   time.Time         `json:"createdAt"`
}

func (r *Registry) knowledgeDir(name string) string {
	return filepath.Join(r.knowledgeBasesDir(), name)
}

func (r *Registry) knowledgeBasesDir() string {
	return filepath.Join(r.root, "knowledge")
}

// SaveKnowledgeBase writes kb's definition, creating or overwriting it.
// CreatedAt is set on first save and preserved on later overwrites, same
// reasoning as SaveAgent/SaveBenchmark.
func (r *Registry) SaveKnowledgeBase(kb KnowledgeBase) error {
	if existing, err := r.GetKnowledgeBase(kb.Name); err == nil {
		kb.CreatedAt = existing.CreatedAt
	} else if kb.CreatedAt.IsZero() {
		kb.CreatedAt = time.Now().UTC()
	}

	dir := r.knowledgeDir(kb.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create knowledge base directory: %w", err)
	}

	data, err := json.MarshalIndent(kb, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal knowledge base: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, knowledgeMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write knowledge base: %w", err)
	}

	return nil
}

// GetKnowledgeBase returns the named knowledge base.
func (r *Registry) GetKnowledgeBase(name string) (KnowledgeBase, error) {
	data, err := os.ReadFile(filepath.Join(r.knowledgeDir(name), knowledgeMetadataFile))
	if err != nil {
		return KnowledgeBase{}, fmt.Errorf("read knowledge base %q: %w", name, err)
	}

	var kb KnowledgeBase
	if err := json.Unmarshal(data, &kb); err != nil {
		return KnowledgeBase{}, fmt.Errorf("parse definition for knowledge base %q: %w", name, err)
	}

	return kb, nil
}

// DeleteKnowledgeBase removes a knowledge base's directory. It's an error to
// delete one that doesn't exist.
func (r *Registry) DeleteKnowledgeBase(name string) error {
	dir := r.knowledgeDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("knowledge base %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete knowledge base %q: %w", name, err)
	}
	return nil
}

// ListKnowledgeBases returns every registry-tracked knowledge base, sorted
// by name.
func (r *Registry) ListKnowledgeBases() ([]KnowledgeBase, error) {
	entries, err := os.ReadDir(r.knowledgeBasesDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read knowledge directory: %w", err)
	}

	var bases []KnowledgeBase
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		kb, err := r.GetKnowledgeBase(entry.Name())
		if err != nil {
			continue // skip directories without a valid definition
		}
		bases = append(bases, kb)
	}

	sort.Slice(bases, func(i, j int) bool { return bases[i].Name < bases[j].Name })

	return bases, nil
}

// AddRecords appends new records to the named knowledge base, assigning
// each a fresh server-side ID (mirrors AppendExamples/AddTestCases) —
// ignoring any ID the caller sent.
func (r *Registry) AddRecords(name string, records []KnowledgeRecord) error {
	kb, err := r.GetKnowledgeBase(name)
	if err != nil {
		return err
	}
	for i, rec := range records {
		rec.ID = fmt.Sprintf("rec-%d-%d", time.Now().UnixNano(), i)
		kb.Records = append(kb.Records, rec)
	}
	return r.SaveKnowledgeBase(kb)
}

// UpdateRecord overwrites the record at index (0-based, in the order
// GetKnowledgeBase returns them), preserving its existing ID. It's an error
// if index is out of range.
func (r *Registry) UpdateRecord(name string, index int, record KnowledgeRecord) error {
	kb, err := r.GetKnowledgeBase(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(kb.Records) {
		return fmt.Errorf("record index %d out of range (knowledge base has %d records)", index, len(kb.Records))
	}
	record.ID = kb.Records[index].ID
	kb.Records[index] = record
	return r.SaveKnowledgeBase(kb)
}

// DeleteRecord removes the record at index (0-based, in the order
// GetKnowledgeBase returns them). It's an error if index is out of range.
func (r *Registry) DeleteRecord(name string, index int) error {
	kb, err := r.GetKnowledgeBase(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(kb.Records) {
		return fmt.Errorf("record index %d out of range (knowledge base has %d records)", index, len(kb.Records))
	}
	kb.Records = append(kb.Records[:index], kb.Records[index+1:]...)
	return r.SaveKnowledgeBase(kb)
}
