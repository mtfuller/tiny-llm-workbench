package evaluations

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// AssertionResult is one assertion's outcome against a test case's reply.
type AssertionResult struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

// checkAssertion evaluates a against output, returning whether it passed.
// An unrecognized type or invalid regex is reported as an error, not a
// silent failure, so a broken assertion definition is visible in results.
func checkAssertion(a registry.Assertion, output string) (bool, error) {
	switch a.Type {
	case "contains":
		// Case-insensitive: an LLM's capitalization is arbitrary (e.g. a
		// sentence-initial "Hello") and shouldn't decide pass/fail, same
		// reasoning as Phase 3's decision-node keyword match.
		return strings.Contains(strings.ToLower(output), strings.ToLower(a.Value)), nil
	case "not_contains":
		return !strings.Contains(strings.ToLower(output), strings.ToLower(a.Value)), nil
	case "regex":
		re, err := regexp.Compile(a.Value)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", a.Value, err)
		}
		return re.MatchString(output), nil
	default:
		return false, fmt.Errorf("unknown assertion type %q", a.Type)
	}
}

// checkAssertions evaluates every assertion in a test case against output,
// returning each result plus whether all of them passed.
func checkAssertions(assertions []registry.Assertion, output string) ([]AssertionResult, bool) {
	results := make([]AssertionResult, 0, len(assertions))
	allPassed := true

	for _, a := range assertions {
		passed, err := checkAssertion(a, output)
		result := AssertionResult{Type: a.Type, Value: a.Value, Passed: passed}
		if err != nil {
			result.Error = err.Error()
			passed = false
		}
		if !passed {
			allPassed = false
		}
		results = append(results, result)
	}

	return results, allPassed
}
