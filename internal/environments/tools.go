package environments

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

// RenderToolCommand validates args against tool's declared parameters and
// substitutes them into its command template, each value wrapped in single
// quotes (with embedded single quotes escaped) so it's always treated as one
// literal shell argument regardless of spaces or special characters.
//
// A template places a placeholder directly where its value belongs (e.g.
// "cat {{path}}", or "q={{query}}" for a literal prefix sharing the same
// shell word) without adding its own quotes around it — substitution already
// quotes the value, and wrapping it again would either break composition
// (nesting single quotes inside double quotes doesn't mean what it looks
// like) or just be redundant.
func RenderToolCommand(tool registry.Tool, args map[string]string) (string, error) {
	for _, p := range tool.Parameters {
		value, ok := args[p.Name]
		if !ok || value == "" {
			if p.Required {
				return "", fmt.Errorf("missing required parameter %q", p.Name)
			}
			continue
		}
		if err := validateToolParameter(p, value); err != nil {
			return "", err
		}
	}

	command := tool.Command
	for _, p := range tool.Parameters {
		value, ok := args[p.Name]
		if !ok {
			continue
		}
		command = strings.ReplaceAll(command, "{{"+p.Name+"}}", shellQuote(value))
	}

	return command, nil
}

func validateToolParameter(p registry.ToolParameter, value string) error {
	switch p.Type {
	case registry.ToolParamNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("parameter %q must be a number", p.Name)
		}
	case registry.ToolParamBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("parameter %q must be \"true\" or \"false\"", p.Name)
		}
	}
	return nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// (by closing the quote, emitting an escaped literal quote, then reopening
// it), so it's always treated as one literal shell argument no matter its
// contents — spaces, newlines, and shell metacharacters included.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
