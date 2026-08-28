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

// Environment is a registry-tracked Environment definition: a Docker image,
// the mounts it should launch with (its "file structure" — what's visible
// inside the container from the host), and the names of catalog Tools
// (see tools.go) available to run inside it once launched (see
// internal/environments). Tools is a list of names, not embedded
// definitions — attaching a tool to an environment is a live reference to
// the shared catalog entry, not a copy.
type Environment struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Tools     []string  `json:"tools"`
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
// its attached tools untouched — the "Configuration" side of the
// environment workspace page.
func (r *Registry) UpdateConfig(name, image string, mounts []Mount) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	env.Image = image
	env.Mounts = mounts
	return r.SaveEnvironment(env)
}

// AttachTool references an existing catalog tool from the named
// environment. It's idempotent (attaching an already-attached tool is a
// no-op) and validates the tool actually exists in the catalog first, so a
// typo'd name can't silently create a dangling reference at attach time.
func (r *Registry) AttachTool(name, toolName string) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	if _, err := r.GetTool(toolName); err != nil {
		return fmt.Errorf("tool %q not found in the catalog: %w", toolName, err)
	}
	for _, t := range env.Tools {
		if t == toolName {
			return nil
		}
	}
	env.Tools = append(env.Tools, toolName)
	return r.SaveEnvironment(env)
}

// DetachTool removes a tool reference from the named environment. This
// doesn't touch the catalog tool itself, only this environment's list of
// which tools it makes available. It's an error if the environment doesn't
// currently reference toolName.
func (r *Registry) DetachTool(name, toolName string) error {
	env, err := r.GetEnvironment(name)
	if err != nil {
		return err
	}
	for i, t := range env.Tools {
		if t == toolName {
			env.Tools = append(env.Tools[:i], env.Tools[i+1:]...)
			return r.SaveEnvironment(env)
		}
	}
	return fmt.Errorf("environment %q doesn't have tool %q attached", name, toolName)
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
// edit or replace them. Call EnsurePrebuiltTools first — these definitions
// reference catalog tools by name and assume they already exist.
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

// PrebuiltEnvironments returns TLW's built-in Environment definitions.
// Their images are deliberately generic (no dedicated web-search/dev/office
// container images exist yet) — they're a starting point to launch, exec
// into, and customize, not finished task-specific sandboxes. Tools are
// referenced by name from the prebuilt tool catalog (see PrebuiltTools) —
// EnsurePrebuiltTools must run first for these references to resolve.
func PrebuiltEnvironments() []Environment {
	now := time.Now().UTC()
	return []Environment{
		{
			Name:      "WebSearch",
			Image:     "curlimages/curl:8.10.1",
			Tools:     []string{"web_search", "read_file", "write_file"},
			Prebuilt:  true,
			CreatedAt: now,
		},
		{
			Name:      "SoftwareDev",
			Image:     "python:3.12-slim",
			Tools:     []string{"read_file", "write_file", "read_directory"},
			Prebuilt:  true,
			CreatedAt: now,
		},
		{
			Name:      "OfficeWorker",
			Image:     "debian:bookworm-slim",
			Tools:     []string{"read_file", "write_file", "read_directory"},
			Prebuilt:  true,
			CreatedAt: now,
		},
	}
}
