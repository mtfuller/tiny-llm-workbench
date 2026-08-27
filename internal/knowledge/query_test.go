package knowledge

import (
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func testBase() registry.KnowledgeBase {
	return registry.KnowledgeBase{
		Name: "faq",
		Records: []registry.KnowledgeRecord{
			{ID: "1", Title: "Refunds", Content: "Refunds take 3-5 business days to process."},
			{ID: "2", Title: "Shipping", Content: "We ship internationally via a tutorial for python packaging."},
			{ID: "3", Title: "Python tutorial", Content: "Learn the basics of the language."},
		},
	}
}

func TestQueryMatchesContent(t *testing.T) {
	results := Query(testBase(), "refunds")
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("Query() = %+v, want just record 1", results)
	}
}

func TestQueryIsCaseInsensitive(t *testing.T) {
	results := Query(testBase(), "REFUNDS")
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("Query() = %+v, want just record 1", results)
	}
}

func TestQueryMatchesTitle(t *testing.T) {
	results := Query(testBase(), "shipping")
	if len(results) != 1 || results[0].ID != "2" {
		t.Errorf("Query() = %+v, want just record 2", results)
	}
}

func TestQueryRequiresAllTermsRegardlessOfOrder(t *testing.T) {
	// Both records 2 and 3 contain "python" and "tutorial" somewhere across
	// title+content, in different orders — an all-terms-present match (not
	// a single-phrase substring match) should find both.
	results := Query(testBase(), "python tutorial")
	if len(results) != 2 {
		t.Fatalf("Query() = %+v, want both records 2 and 3 (order-independent all-terms match)", results)
	}
	ids := map[string]bool{results[0].ID: true, results[1].ID: true}
	if !ids["2"] || !ids["3"] {
		t.Errorf("Query() ids = %v, want [2, 3]", ids)
	}
}

func TestQueryNoMatches(t *testing.T) {
	results := Query(testBase(), "nonexistent term")
	if len(results) != 0 {
		t.Errorf("Query() = %+v, want no matches", results)
	}
}

func TestQueryEmptyQueryMatchesNothing(t *testing.T) {
	results := Query(testBase(), "   ")
	if len(results) != 0 {
		t.Errorf("Query() = %+v, want no matches for an empty query", results)
	}
}

func TestFormatResultsEmpty(t *testing.T) {
	got := FormatResults(nil)
	want := "No matching records found."
	if got != want {
		t.Errorf("FormatResults() = %q, want %q", got, want)
	}
}

func TestFormatResultsJoinsMultipleRecords(t *testing.T) {
	records := []registry.KnowledgeRecord{
		{Title: "A", Content: "first"},
		{Title: "B", Content: "second"},
	}
	got := FormatResults(records)
	want := "A: first\n\nB: second"
	if got != want {
		t.Errorf("FormatResults() = %q, want %q", got, want)
	}
}
