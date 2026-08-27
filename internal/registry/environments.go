package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const environmentMetadataFile = "metadata.json"

// Mount is a host↔container bind mount an Environment's container should
// have when launched.
type Mount struct {
	HostPath      string `json:"hostPath"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly,omitempty"`
}

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

// Tool is a named, runnable capability inside an Environment's container: a
// shell command template with "{{name}}" placeholders, one per declared
// Parameter, substituted (each value shell-quoted) when the tool actually
// runs — see internal/environments.RenderToolCommand. A template places a
// placeholder directly where its value belongs (e.g. "cat {{path}}") without
// adding its own quotes around it, since substitution already quotes values.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Command     string          `json:"command"`
	Parameters  []ToolParameter `json:"parameters"`
}

// Environment is a registry-tracked Environment definition: a Docker image,
// the mounts it should launch with, and the Tools available to run inside
// it once launched (see internal/environments).
type Environment struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Tools     []Tool    `json:"tools"`
	Mounts    []Mount   `json:"mounts"`
	Prebuilt  bool      `json:"prebuilt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (r *Registry) environmentDir(name string) string {
	return filepath.Join(r.environmentsDir(), name)
}

func (r *Registry) environmentsDir() string {
	return filepath.Join(r.root, "environments")
}

// SaveEnvironment writes e's metadata, creating its directory if needed.
func (r *Registry) SaveEnvironment(e Environment) error {
	dir := r.environmentDir(e.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create environment directory: %w", err)
	}

	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal environment metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, environmentMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write environment metadata: %w", err)
	}

	return nil
}

// GetEnvironment returns the named environment definition.
func (r *Registry) GetEnvironment(name string) (Environment, error) {
	data, err := os.ReadFile(filepath.Join(r.environmentDir(name), environmentMetadataFile))
	if err != nil {
		return Environment{}, fmt.Errorf("read environment %q: %w", name, err)
	}

	var env Environment
	if err := json.Unmarshal(data, &env); err != nil {
		return Environment{}, fmt.Errorf("parse metadata for environment %q: %w", name, err)
	}

	return env, nil
}

// DeleteEnvironment removes an environment definition's directory. Deleting
// a prebuilt definition is allowed — it won't be reseeded until the next
// `tlw serve` start. It's an error to delete one that doesn't exist.
func (r *Registry) DeleteEnvironment(name string) error {
	dir := r.environmentDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("environment %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete environment %q: %w", name, err)
	}
	return nil
}

// UpdateConfig overwrites the named environment's image and mounts, leaving
// its tools untouched — the "Configuration" side of the environment
// workspace page.
func (r *Registry) UpdateConfig(name, image string, mounts []Mount) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	env.Image = image
	env.Mounts = mounts
	return r.SaveEnvironment(env)
}

// AddTool appends a new tool to the named environment.
func (r *Registry) AddTool(name string, tool Tool) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	env.Tools = append(env.Tools, tool)
	return r.SaveEnvironment(env)
}

// UpdateTool overwrites the tool at index (0-based, in the order
// GetEnvironment returns them). It's an error if index is out of range.
func (r *Registry) UpdateTool(name string, index int, tool Tool) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(env.Tools) {
		return fmt.Errorf("tool index %d out of range (environment has %d tools)", index, len(env.Tools))
	}
	env.Tools[index] = tool
	return r.SaveEnvironment(env)
}

// DeleteTool removes the tool at index (0-based, in the order
// GetEnvironment returns them). It's an error if index is out of range.
func (r *Registry) DeleteTool(name string, index int) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(env.Tools) {
		return fmt.Errorf("tool index %d out of range (environment has %d tools)", index, len(env.Tools))
	}
	env.Tools = append(env.Tools[:index], env.Tools[index+1:]...)
	return r.SaveEnvironment(env)
}

// ListEnvironments returns every registry-tracked environment, sorted by
// name.
func (r *Registry) ListEnvironments() ([]Environment, error) {
	entries, err := os.ReadDir(r.environmentsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read environments directory: %w", err)
	}

	var environments []Environment
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		env, err := r.GetEnvironment(entry.Name())
		if err != nil {
			continue // skip directories without valid metadata
		}
		environments = append(environments, env)
	}

	sort.Slice(environments, func(i, j int) bool { return environments[i].Name < environments[j].Name })

	return environments, nil
}

// EnsurePrebuiltEnvironments seeds the registry's built-in environment
// definitions (WebSearch, SoftwareDev, OfficeWorker) if they don't already
// exist. It never overwrites an existing definition, so a user is free to
// edit or replace them.
func (r *Registry) EnsurePrebuiltEnvironments() error {
	for _, env := range PrebuiltEnvironments() {
		if _, err := r.GetEnvironment(env.Name); err == nil {
			continue // already present
		}
		if err := r.SaveEnvironment(env); err != nil {
			return fmt.Errorf("seed prebuilt environment %q: %w", env.Name, err)
		}
	}
	return nil
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

// PrebuiltEnvironments returns TLW's built-in Environment definitions.
// Their images are deliberately generic (no dedicated web-search/dev/office
// container images exist yet) — they're a starting point to launch, exec
// into, and customize, not finished task-specific sandboxes.
func PrebuiltEnvironments() []Environment {
	now := time.Now().UTC()
	return []Environment{
		{
			Name:      "WebSearch",
			Image:     "curlimages/curl:8.10.1",
			Tools:     []Tool{webSearchTool(), readFileTool(), writeFileTool()},
			Prebuilt:  true,
			CreatedAt: now,
		},
		{
			Name:      "SoftwareDev",
			Image:     "python:3.12-slim",
			Tools:     []Tool{readFileTool(), writeFileTool(), readDirectoryTool()},
			Prebuilt:  true,
			CreatedAt: now,
		},
		{
			Name:      "OfficeWorker",
			Image:     "debian:bookworm-slim",
			Tools:     []Tool{readFileTool(), writeFileTool(), readDirectoryTool()},
			Prebuilt:  true,
			CreatedAt: now,
		},
	}
}
