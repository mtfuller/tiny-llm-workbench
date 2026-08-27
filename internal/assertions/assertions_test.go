package assertions

import (
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestCheckContains(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "contains", Value: "hello"}, "well hello there")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true")
	}
}

func TestCheckContainsCaseInsensitive(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "contains", Value: "hello"}, "Hello! How can I help?")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true for a case-insensitive match")
	}
}

func TestCheckContainsFails(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "contains", Value: "goodbye"}, "well hello there")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if passed {
		t.Error("Check() = true, want false")
	}
}

func TestCheckNotContains(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "not_contains", Value: "goodbye"}, "well hello there")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true")
	}
}

func TestCheckRegex(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "regex", Value: `\d{3}-\d{4}`}, "call 555-1234 now")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true")
	}
}

func TestCheckInvalidRegex(t *testing.T) {
	_, err := Check(registry.Assertion{Type: "regex", Value: `(unclosed`}, "anything")
	if err == nil {
		t.Error("Check() error = nil, want an error for invalid regex")
	}
}

func TestCheckUnknownType(t *testing.T) {
	_, err := Check(registry.Assertion{Type: "vibes"}, "anything")
	if err == nil {
		t.Error("Check() error = nil, want an error for an unknown assertion type")
	}
}

func TestCheckAllPass(t *testing.T) {
	results, allPassed := CheckAll([]registry.Assertion{
		{Type: "contains", Value: "hello"},
		{Type: "not_contains", Value: "goodbye"},
	}, "hello there")

	if !allPassed {
		t.Error("CheckAll() allPassed = false, want true")
	}
	if len(results) != 2 || !results[0].Passed || !results[1].Passed {
		t.Errorf("CheckAll() results = %+v, want both passed", results)
	}
}

func TestCheckAllOneFails(t *testing.T) {
	results, allPassed := CheckAll([]registry.Assertion{
		{Type: "contains", Value: "hello"},
		{Type: "contains", Value: "goodbye"},
	}, "hello there")

	if allPassed {
		t.Error("CheckAll() allPassed = true, want false")
	}
	if len(results) != 2 || !results[0].Passed || results[1].Passed {
		t.Errorf("CheckAll() results = %+v, want [passed, failed]", results)
	}
}

func TestCheckAllErrorCountsAsFailure(t *testing.T) {
	results, allPassed := CheckAll([]registry.Assertion{
		{Type: "regex", Value: `(unclosed`},
	}, "anything")

	if allPassed {
		t.Error("CheckAll() allPassed = true, want false for an assertion error")
	}
	if len(results) != 1 || results[0].Passed || results[0].Error == "" {
		t.Errorf("CheckAll() results = %+v, want a failed result with an error message", results)
	}
}

func TestCheckJSONSchemaValid(t *testing.T) {
	schema := `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`
	passed, err := Check(registry.Assertion{Type: "json_schema", Value: schema}, `{"name": "Ada"}`)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true for a conforming document")
	}
}

func TestCheckJSONSchemaValidWithSurroundingText(t *testing.T) {
	schema := `{"type":"object","required":["name"]}`
	reply := "Sure, here you go:\n```json\n{\"name\": \"Ada\"}\n```\nLet me know if you need anything else."
	passed, err := Check(registry.Assertion{Type: "json_schema", Value: schema}, reply)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true when the JSON is embedded in surrounding prose/markdown")
	}
}

func TestCheckJSONSchemaMissingRequired(t *testing.T) {
	schema := `{"type":"object","required":["name"]}`
	passed, err := Check(registry.Assertion{Type: "json_schema", Value: schema}, `{"age": 5}`)
	if passed {
		t.Error("Check() = true, want false for a document missing a required property")
	}
	if err == nil {
		t.Error("Check() error = nil, want a validation error explaining the mismatch")
	}
}

func TestCheckJSONSchemaNoJSONInReply(t *testing.T) {
	schema := `{"type":"object"}`
	_, err := Check(registry.Assertion{Type: "json_schema", Value: schema}, "sorry, I can't help with that")
	if err == nil {
		t.Error("Check() error = nil, want an error when the reply has no JSON value")
	}
}

func TestCheckJSONSchemaInvalidSchema(t *testing.T) {
	_, err := Check(registry.Assertion{Type: "json_schema", Value: `not json`}, `{"a": 1}`)
	if err == nil {
		t.Error("Check() error = nil, want an error for an invalid JSON schema document")
	}
}

func TestCheckJSONSchemaInvalidReplyJSON(t *testing.T) {
	schema := `{"type":"object"}`
	_, err := Check(registry.Assertion{Type: "json_schema", Value: schema}, `{"a": }`)
	if err == nil {
		t.Error("Check() error = nil, want an error when the extracted candidate isn't valid JSON")
	}
}

func TestCheckSimilarityIdentical(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "similarity", Value: "the quick brown fox", Threshold: 0.9}, "the quick brown fox")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true for identical text")
	}
}

func TestCheckSimilarityCaseInsensitive(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "similarity", Value: "Hello There", Threshold: 0.99}, "hello there")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true for a case-only difference")
	}
}

func TestCheckSimilarityCloseEnough(t *testing.T) {
	// "the quick brown fox" vs "the quick brown fix" differs by 1 char out
	// of 19 => ratio ~0.947, should pass an 0.8 threshold.
	passed, err := Check(registry.Assertion{Type: "similarity", Value: "the quick brown fox", Threshold: 0.8}, "the quick brown fix")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !passed {
		t.Error("Check() = false, want true for a small, tolerated difference")
	}
}

func TestCheckSimilarityTooDifferent(t *testing.T) {
	passed, err := Check(registry.Assertion{Type: "similarity", Value: "the quick brown fox", Threshold: 0.9}, "completely unrelated text")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if passed {
		t.Error("Check() = true, want false for very different text")
	}
}

func TestCheckSimilarityRequiresValidThreshold(t *testing.T) {
	for _, threshold := range []float64{0, -0.5, 1.5} {
		_, err := Check(registry.Assertion{Type: "similarity", Value: "x", Threshold: threshold}, "x")
		if err == nil {
			t.Errorf("Check() with threshold %v: error = nil, want an error", threshold)
		}
	}
}

func TestSimilarityRatioKnownValues(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"", "", 1},
		{"abc", "abc", 1},
		{"abc", "abd", 2.0 / 3.0},
		{"", "abc", 0},
	}
	const epsilon = 1e-9
	for _, tt := range tests {
		got := similarityRatio(tt.a, tt.b)
		if diff := got - tt.want; diff > epsilon || diff < -epsilon {
			t.Errorf("similarityRatio(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestExtractJSONValueIgnoresBracesInStrings(t *testing.T) {
	got, ok := ExtractJSONValue(`prefix {"a": "b{c}d"} suffix`)
	if !ok {
		t.Fatal("ExtractJSONValue() ok = false, want true")
	}
	want := `{"a": "b{c}d"}`
	if got != want {
		t.Errorf("ExtractJSONValue() = %q, want %q", got, want)
	}
}

func TestExtractJSONValueNoJSON(t *testing.T) {
	_, ok := ExtractJSONValue("no json here")
	if ok {
		t.Error("ExtractJSONValue() ok = true, want false")
	}
}

func TestExtractJSONValueArray(t *testing.T) {
	got, ok := ExtractJSONValue(`here: [1, 2, 3] done`)
	if !ok || got != "[1, 2, 3]" {
		t.Errorf("ExtractJSONValue() = (%q, %v), want (\"[1, 2, 3]\", true)", got, ok)
	}
}

func TestValidateJSONSchemaReturnsParsedValue(t *testing.T) {
	schema := `{"type":"object","required":["city"],"properties":{"city":{"type":"string"}}}`
	value, err := ValidateJSONSchema(schema, `Sure, here you go: {"city": "Paris", "temp": 72}`)
	if err != nil {
		t.Fatalf("ValidateJSONSchema() error = %v", err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("ValidateJSONSchema() = %T, want map[string]any", value)
	}
	if obj["city"] != "Paris" {
		t.Errorf("ValidateJSONSchema()[\"city\"] = %v, want %q", obj["city"], "Paris")
	}
}

func TestValidateJSONSchemaFailsValidation(t *testing.T) {
	schema := `{"type":"object","required":["city"]}`
	if _, err := ValidateJSONSchema(schema, `{"temp": 72}`); err == nil {
		t.Error("ValidateJSONSchema() error = nil, want an error for a document missing a required property")
	}
}

func TestValidateJSONSchemaNoJSONInReply(t *testing.T) {
	if _, err := ValidateJSONSchema(`{"type":"object"}`, "no json here"); err == nil {
		t.Error("ValidateJSONSchema() error = nil, want an error when the reply has no JSON value")
	}
}
