package agents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// nodeResult is one node's output for a single turn, kept so a later node's
// template can reference it by name. parsed is non-nil only when the node
// declared (and its reply satisfied) an output JSON Schema — see
// registry.NodeData.OutputSchema.
type nodeResult struct {
	raw    string
	parsed any
}

// runContext accumulates a nodeResult per node name as Engine.Run walks the
// graph, keyed by registry.NodeData.Name (not the node's internal ID) —
// Name is the stable, user-chosen token a template references. A node with
// no Name simply never becomes referenceable.
//
// Because the engine only ever walks one path per turn (branching at a
// Decision node still follows a single edge), this map naturally holds
// exactly the set of nodes that "came before" the one currently resolving a
// template — no separate reachability analysis is needed. A template
// referencing a node that hasn't executed yet this turn (a typo, or a node
// on a branch not taken) simply isn't found, and render reports that
// clearly rather than silently substituting nothing.
type runContext struct {
	results map[string]nodeResult
}

func newRunContext() *runContext {
	return &runContext{results: make(map[string]nodeResult)}
}

// set records name's result for later templates to reference. A no-op for
// an unnamed node.
func (rc *runContext) set(name, raw string, parsed any) {
	if name == "" {
		return
	}
	rc.results[name] = nodeResult{raw: raw, parsed: parsed}
}

// templateExprPattern matches a single {{...}} placeholder, capturing its
// inner text verbatim (further split on the first "." by render) — this
// intentionally avoids a more elaborate regex trying to parse the name and
// property path in one pass, since Go's RE2 engine doesn't backtrack and a
// combined optional-group pattern can't reliably prefer the longest name
// match the way a name containing spaces needs.
var templateExprPattern = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// render substitutes every {{name}} / {{name.path}} reference in template
// against rc: {{name}} resolves to that node's raw text output; {{name.path}}
// navigates a dot-separated path into that node's parsed JSON output (only
// available if the node declared an OutputSchema). The first unresolvable
// reference — an unknown node name, a path into a node with no parsed JSON,
// or an unknown property — fails the whole substitution with a clear error,
// rather than silently leaving a gap or an empty string.
func (rc *runContext) render(template string) (string, error) {
	if !strings.Contains(template, "{{") {
		return template, nil
	}

	var firstErr error
	result := templateExprPattern.ReplaceAllStringFunc(template, func(match string) string {
		if firstErr != nil {
			return ""
		}

		inner := strings.TrimSpace(templateExprPattern.FindStringSubmatch(match)[1])
		name, path, _ := strings.Cut(inner, ".")
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)

		nr, ok := rc.results[name]
		if !ok {
			firstErr = fmt.Errorf("template references unknown node %q", name)
			return ""
		}
		if path == "" {
			return nr.raw
		}
		if nr.parsed == nil {
			firstErr = fmt.Errorf("template references %q.%s, but node %q has no output schema configured (only {{%s}} is available)", name, path, name, name)
			return ""
		}
		value, ok := lookupJSONPath(nr.parsed, path)
		if !ok {
			firstErr = fmt.Errorf("template references unknown property %q on node %q", path, name)
			return ""
		}
		return stringifyJSONValue(value)
	})

	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
}

// lookupJSONPath navigates a dot-separated path of object keys into value
// (as decoded by jsonschema.UnmarshalJSON / assertions.ValidateJSONSchema).
// Array indexing isn't supported — a first-pass keeps this to plain object
// property access, the common case for a Prompt node's structured reply.
func lookupJSONPath(value any, path string) (any, bool) {
	current := value
	for _, key := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// stringifyJSONValue renders a parsed JSON value (string/json.Number/bool/
// nil/object/array) as plain text suitable for substituting into a prompt,
// tool argument, or decision match target.
func stringifyJSONValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
