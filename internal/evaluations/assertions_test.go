package evaluations

import (
	"testing"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

func TestCheckAssertionContains(t *testing.T) {
	passed, err := checkAssertion(registry.Assertion{Type: "contains", Value: "hello"}, "well hello there")
	if err != nil {
		t.Fatalf("checkAssertion() error = %v", err)
	}
	if !passed {
		t.Error("checkAssertion() = false, want true")
	}
}

func TestCheckAssertionContainsCaseInsensitive(t *testing.T) {
	passed, err := checkAssertion(registry.Assertion{Type: "contains", Value: "hello"}, "Hello! How can I help?")
	if err != nil {
		t.Fatalf("checkAssertion() error = %v", err)
	}
	if !passed {
		t.Error("checkAssertion() = false, want true for a case-insensitive match")
	}
}

func TestCheckAssertionContainsFails(t *testing.T) {
	passed, err := checkAssertion(registry.Assertion{Type: "contains", Value: "goodbye"}, "well hello there")
	if err != nil {
		t.Fatalf("checkAssertion() error = %v", err)
	}
	if passed {
		t.Error("checkAssertion() = true, want false")
	}
}

func TestCheckAssertionNotContains(t *testing.T) {
	passed, err := checkAssertion(registry.Assertion{Type: "not_contains", Value: "goodbye"}, "well hello there")
	if err != nil {
		t.Fatalf("checkAssertion() error = %v", err)
	}
	if !passed {
		t.Error("checkAssertion() = false, want true")
	}
}

func TestCheckAssertionRegex(t *testing.T) {
	passed, err := checkAssertion(registry.Assertion{Type: "regex", Value: `\d{3}-\d{4}`}, "call 555-1234 now")
	if err != nil {
		t.Fatalf("checkAssertion() error = %v", err)
	}
	if !passed {
		t.Error("checkAssertion() = false, want true")
	}
}

func TestCheckAssertionInvalidRegex(t *testing.T) {
	_, err := checkAssertion(registry.Assertion{Type: "regex", Value: `(unclosed`}, "anything")
	if err == nil {
		t.Error("checkAssertion() error = nil, want an error for invalid regex")
	}
}

func TestCheckAssertionUnknownType(t *testing.T) {
	_, err := checkAssertion(registry.Assertion{Type: "vibes"}, "anything")
	if err == nil {
		t.Error("checkAssertion() error = nil, want an error for an unknown assertion type")
	}
}

func TestCheckAssertionsAllPass(t *testing.T) {
	results, allPassed := checkAssertions([]registry.Assertion{
		{Type: "contains", Value: "hello"},
		{Type: "not_contains", Value: "goodbye"},
	}, "hello there")

	if !allPassed {
		t.Error("checkAssertions() allPassed = false, want true")
	}
	if len(results) != 2 || !results[0].Passed || !results[1].Passed {
		t.Errorf("checkAssertions() results = %+v, want both passed", results)
	}
}

func TestCheckAssertionsOneFails(t *testing.T) {
	results, allPassed := checkAssertions([]registry.Assertion{
		{Type: "contains", Value: "hello"},
		{Type: "contains", Value: "goodbye"},
	}, "hello there")

	if allPassed {
		t.Error("checkAssertions() allPassed = true, want false")
	}
	if len(results) != 2 || !results[0].Passed || results[1].Passed {
		t.Errorf("checkAssertions() results = %+v, want [passed, failed]", results)
	}
}

func TestCheckAssertionsErrorCountsAsFailure(t *testing.T) {
	results, allPassed := checkAssertions([]registry.Assertion{
		{Type: "regex", Value: `(unclosed`},
	}, "anything")

	if allPassed {
		t.Error("checkAssertions() allPassed = true, want false for an assertion error")
	}
	if len(results) != 1 || results[0].Passed || results[0].Error == "" {
		t.Errorf("checkAssertions() results = %+v, want a failed result with an error message", results)
	}
}
