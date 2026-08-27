// Package knowledge implements deterministic keyword search over a
// registry.KnowledgeBase's records — no embeddings/vector search, matching
// this app's established preference for deterministic mechanisms (Decision
// nodes' keyword matching, contains/regex assertions) over anything that
// would need a second ML runtime.
package knowledge

import (
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// Query returns every record in kb whose Title or Content contains every
// whitespace-separated term in query, case-insensitively — an all-terms
// substring match, not a single-phrase one, so a query like "python
// tutorial" matches a record whose content reads "tutorial for python"
// too. An empty (or all-whitespace) query matches nothing.
func Query(kb registry.KnowledgeBase, query string) []registry.KnowledgeRecord {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}

	var results []registry.KnowledgeRecord
	for _, rec := range kb.Records {
		haystack := strings.ToLower(rec.Title + " " + rec.Content)
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			results = append(results, rec)
		}
	}
	return results
}

// FormatResults renders matched records into a single plain-text digest
// suitable as an Agent Knowledge node's output — a downstream Prompt node
// can reference it directly (e.g. "Answer using this context: {{KB}}") the
// same way it would any other node's raw text output.
func FormatResults(records []registry.KnowledgeRecord) string {
	if len(records) == 0 {
		return "No matching records found."
	}

	var b strings.Builder
	for i, rec := range records {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(rec.Title)
		b.WriteString(": ")
		b.WriteString(rec.Content)
	}
	return b.String()
}
