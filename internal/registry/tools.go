package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const toolMetadataFile = "definition.json"

// ToolParameterType is a Tool parameter's value type — used to render the
// right form control when trying a tool, and for light validation before a
// value is substituted into the tool's command.
type ToolParameterType string

const (
	ToolParamString  ToolParameterType = "string"
	ToolParamNumber  ToolParameterType = "number"
	ToolParamBoolean ToolParameterType = "boolean"
)

// ToolParameter is one named, typed input a Tool's Command template expects
// as a "{{name}}" placeholder.
type ToolParameter struct {
	Name        string            `json:"name"`
	Type        ToolParameterType `json:"type"`
	Description string            `json:"description,omitempty"`
	Required    bool              `json:"required"`
}

// Tool is a named, runnable capability: a shell command template with
// "{{name}}" placeholders, one per declared Parameter, substituted (each
// value shell-quoted) when it actually runs inside an Environment's
// container — see internal/environments.RenderToolCommand. A template
// places a placeholder directly where its value belongs (e.g.
// "cat {{path}}") without adding its own quotes around it, since
// substitution already quotes values.
//
// Tools live in their own global, top-level catalog (like Models/Datasets),
// not embedded in each Environment — an Environment instead names which
// catalog tools it makes available (Environment.Tools []string). Attaching
// is a live reference, not a copy: editing a catalog tool's definition
// changes it everywhere it's attached, the same way a Training run
// references a Model/Dataset by name rather than copying it in.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Command     string          `json:"command"`
	Parameters  []ToolParameter `json:"parameters"`
	Prebuilt    bool            `json:"prebuilt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

func (r *Registry) toolDir(name string) string {
	return filepath.Join(r.toolsDir(), name)
}

func (r *Registry) toolsDir() string {
	return filepath.Join(r.root, "tools")
}

// SaveTool writes tool's definition to the catalog, creating or overwriting
// it. CreatedAt is set on first save and preserved on later overwrites,
// same reasoning as SaveAgent/SaveBenchmark.
func (r *Registry) SaveTool(tool Tool) error {
	if existing, err := r.GetTool(tool.Name); err == nil {
		tool.CreatedAt = existing.CreatedAt
	} else if tool.CreatedAt.IsZero() {
		tool.CreatedAt = time.Now().UTC()
	}

	dir := r.toolDir(tool.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create tool directory: %w", err)
	}

	data, err := json.MarshalIndent(tool, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tool definition: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, toolMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write tool definition: %w", err)
	}

	return nil
}

// GetTool returns the named catalog tool's definition.
func (r *Registry) GetTool(name string) (Tool, error) {
	data, err := os.ReadFile(filepath.Join(r.toolDir(name), toolMetadataFile))
	if err != nil {
		return Tool{}, fmt.Errorf("read tool %q: %w", name, err)
	}

	var tool Tool
	if err := json.Unmarshal(data, &tool); err != nil {
		return Tool{}, fmt.Errorf("parse definition for tool %q: %w", name, err)
	}

	return tool, nil
}

// DeleteTool removes a tool from the catalog. This doesn't touch any
// Environment that names it in its Tools list — a dangling reference is
// handled the same way an Agent tool node already handles a tool that's no
// longer on its bound Environment (a clear "not found" surfaced in the UI),
// rather than a cascading delete across every environment.
func (r *Registry) DeleteTool(name string) error {
	dir := r.toolDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("tool %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete tool %q: %w", name, err)
	}
	return nil
}

// ListTools returns every catalog tool, sorted by name.
func (r *Registry) ListTools() ([]Tool, error) {
	entries, err := os.ReadDir(r.toolsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tools directory: %w", err)
	}

	var tools []Tool
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		tool, err := r.GetTool(entry.Name())
		if err != nil {
			continue // skip directories without a valid definition
		}
		tools = append(tools, tool)
	}

	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	return tools, nil
}

// readFileTool, writeFileTool, and readDirectoryTool are shared by more than
// one prebuilt environment — general file-manipulation capabilities any
// container with a shell can run.
func readFileTool() Tool {
	return Tool{
		Name:        "read_file",
		Description: "Read a file's contents",
		Command:     "cat {{path}}",
		Parameters:  []ToolParameter{{Name: "path", Type: ToolParamString, Description: "Path to the file", Required: true}},
	}
}

func writeFileTool() Tool {
	return Tool{
		Name:        "write_file",
		Description: "Write content to a file, overwriting it if it already exists",
		Command:     "printf '%s' {{content}} > {{path}}",
		Parameters: []ToolParameter{
			{Name: "path", Type: ToolParamString, Description: "Path to write to", Required: true},
			{Name: "content", Type: ToolParamString, Description: "Content to write", Required: true},
		},
	}
}

func readDirectoryTool() Tool {
	return Tool{
		Name:        "read_directory",
		Description: "List a directory's contents",
		Command:     "ls -la {{path}}",
		Parameters:  []ToolParameter{{Name: "path", Type: ToolParamString, Description: "Directory path", Required: true}},
	}
}

// webSearchTool searches the web via DuckDuckGo's free, keyless Instant
// Answer API — chosen over a real search API specifically to avoid needing
// an API key/signup, consistent with this app's local-first stance. Results
// are instant-answer/infobox style, not full web search results. The query
// is passed to curl via --data-urlencode rather than embedded in a
// hand-built URL string, so curl (not this app) handles URL-encoding it —
// and {{query}} sits directly after the literal "q=" prefix in the same
// shell word, with no extra quotes, so command substitution's own
// shell-quoting composes correctly (see RenderToolCommand's doc comment).
func webSearchTool() Tool {
	return Tool{
		Name:        "web_search",
		Description: "Search the web via DuckDuckGo's Instant Answer API (free, no API key)",
		Command:     "curl -s -G --data-urlencode q={{query}} --data format=json --data no_html=1 --data skip_disambig=1 https://api.duckduckgo.com/",
		Parameters:  []ToolParameter{{Name: "query", Type: ToolParamString, Description: "Search query", Required: true}},
	}
}

// PrebuiltTools returns TLW's built-in Tool catalog entries.
func PrebuiltTools() []Tool {
	return []Tool{readFileTool(), writeFileTool(), readDirectoryTool(), webSearchTool()}
}

// EnsurePrebuiltTools seeds the registry's built-in tool catalog entries if
// they don't already exist. It never overwrites an existing definition, so
// a user is free to edit or delete them — deleting one just means it won't
// be reseeded until the next `tlw serve` start finds it still missing.
func (r *Registry) EnsurePrebuiltTools() error {
	for _, tool := range PrebuiltTools() {
		if _, err := r.GetTool(tool.Name); err == nil {
			continue // already present
		}
		tool.Prebuilt = true
		if err := r.SaveTool(tool); err != nil {
			return fmt.Errorf("seed prebuilt tool %q: %w", tool.Name, err)
		}
	}
	return nil
}
