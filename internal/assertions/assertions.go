// Package assertions checks a registry.Assertion against a piece of text —
// an agent's reply (internal/evaluations) or a model's raw generation
// (internal/benchmarks). It's shared by both, rather than duplicated,
// because both callers need the exact same, increasingly non-trivial set of
// checks (including JSON Schema validation and text similarity) to behave
// identically; that outweighs the earlier reasoning for keeping
// evaluations' and benchmarks' assertion logic independent, back when it was
// just a ~40-line contains/regex checker.
package assertions

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// Result is one assertion's outcome against a piece of text.
type Result struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

// Check evaluates a against output, returning whether it passed. An
// unrecognized type, invalid regex, invalid JSON Schema, or a reply that
// doesn't satisfy a json_schema/similarity assertion's preconditions is
// reported as an error, not a silent failure, so the reason is visible in
// results rather than just a bare "fail".
func Check(a registry.Assertion, output string) (bool, error) {
	switch a.Type {
	case "contains":
		// Case-insensitive: an LLM's capitalization is arbitrary (e.g. a
		// sentence-initial "Hello") and shouldn't decide pass/fail.
		return strings.Contains(strings.ToLower(output), strings.ToLower(a.Value)), nil
	case "not_contains":
		return !strings.Contains(strings.ToLower(output), strings.ToLower(a.Value)), nil
	case "regex":
		re, err := regexp.Compile(a.Value)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", a.Value, err)
		}
		return re.MatchString(output), nil
	case "json_schema":
		return checkJSONSchema(a.Value, output)
	case "similarity":
		return checkSimilarity(a.Value, a.Threshold, output)
	default:
		return false, fmt.Errorf("unknown assertion type %q", a.Type)
	}
}

// CheckAll evaluates every assertion against output, returning each result
// plus whether all of them passed.
func CheckAll(list []registry.Assertion, output string) ([]Result, bool) {
	results := make([]Result, 0, len(list))
	allPassed := true

	for _, a := range list {
		passed, err := Check(a, output)
		result := Result{Type: a.Type, Value: a.Value, Passed: passed}
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

// checkJSONSchema validates the first complete JSON value found in output
// against the JSON Schema document schemaText.
func checkJSONSchema(schemaText, output string) (bool, error) {
	_, err := ValidateJSONSchema(schemaText, output)
	if err != nil {
		return false, err
	}
	return true, nil
}

// ValidateJSONSchema extracts the first complete JSON value found in output,
// validates it against the JSON Schema document schemaText, and returns the
// parsed value (as decoded by jsonschema.UnmarshalJSON: map[string]any for
// an object, []any for an array, json.Number for a number, etc.) on success.
// Models often preface a JSON reply with a sentence or wrap it in a markdown
// code fence, so this looks for the first balanced {...}/[...] substring
// rather than requiring output to be JSON in its entirety.
//
// Exported so internal/agents can reuse the exact same extraction/validation
// path for a Prompt node's optional output schema — that feature needs the
// parsed value itself (to expose properties to downstream nodes), not just
// a pass/fail like an assertion.
func ValidateJSONSchema(schemaText, output string) (any, error) {
	candidate, ok := ExtractJSONValue(output)
	if !ok {
		return nil, errors.New("reply does not contain a JSON value")
	}

	instance, err := jsonschema.UnmarshalJSON(strings.NewReader(candidate))
	if err != nil {
		return nil, fmt.Errorf("reply is not valid JSON: %w", err)
	}

	schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaText))
	if err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}

	if err := schema.Validate(instance); err != nil {
		return nil, err
	}
	return instance, nil
}

// ExtractJSONValue returns the first balanced {...} or [...] substring in s,
// ignoring braces/brackets that appear inside JSON string literals.
func ExtractJSONValue(s string) (string, bool) {
	start := strings.IndexAny(s, "{[")
	if start == -1 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]

		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}

	return "", false
}

// checkSimilarity passes if output is at least threshold similar to
// reference, by normalized Levenshtein distance (case-insensitive, same
// reasoning as "contains"). threshold must be in (0, 1].
func checkSimilarity(reference string, threshold float64, output string) (bool, error) {
	if threshold <= 0 || threshold > 1 {
		return false, fmt.Errorf("similarity assertion requires a threshold in (0, 1], got %v", threshold)
	}
	return similarityRatio(strings.ToLower(output), strings.ToLower(reference)) >= threshold, nil
}

// similarityRatio returns 1 for identical strings, scaling down toward 0 as
// their Levenshtein edit distance approaches the length of the longer one.
func similarityRatio(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	maxLen := len(ar)
	if len(br) > maxLen {
		maxLen = len(br)
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ar, br))/float64(maxLen)
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
