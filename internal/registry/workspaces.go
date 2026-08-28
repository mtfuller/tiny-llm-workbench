package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const workspaceMetadataFile = "metadata.json"

// WorkspaceType distinguishes a throwaway sandbox from a real project
// directory.
type WorkspaceType string

const (
	// WorkspaceTest is a throwaway workspace: an editable directory kept
	// under the registry root that is *copied* into a fresh sandbox
	// container per agent run / debug session / evaluation test case. The
	// agent's changes never flow back to the source, so a test workspace is
	// safe to experiment against repeatedly. Only test workspaces are
	// selectable for an Agent or an Evaluation test case.
	WorkspaceTest WorkspaceType = "test"
	// WorkspaceReal points at a real directory on the user's machine,
	// bind-mounted read-write so the agent's changes persist. Used by
	// Deployments for actual work — real workspaces are filtered out of the
	// agent editor.
	WorkspaceReal WorkspaceType = "real"
)

// Workspace is a registry-tracked workspace: just a directory plus a
// test/real flag. There is no image or tool list here — the sandbox image
// is a fixed default (see internal/docker) and an agent's tools/knowledge
// are chosen on the agent now, not on the workspace.
//
// For a test workspace, HostPath is always <root>/workspaces/<name>/files,
// created on save and meant to be edited directly (VS Code, etc.). For a
// real workspace, HostPath is a caller-supplied absolute path on the user's
// machine (the handler validates it exists and is a directory).
type Workspace struct {
	Name      string        `json:"name"`
	Type      WorkspaceType `json:"type"`
	HostPath  string        `json:"hostPath"`
	CreatedAt time.Time     `json:"createdAt"`
}

func (r *Registry) workspacesDir() string {
	return filepath.Join(r.root, "workspaces")
}

func (r *Registry) workspaceDir(name string) string {
	return filepath.Join(r.workspacesDir(), name)
}

// WorkspaceFilesDir returns the on-disk directory a test workspace's
// starting files live in — the folder a user edits to set up a scenario.
func (r *Registry) WorkspaceFilesDir(name string) string {
	return filepath.Join(r.workspaceDir(name), "files")
}

// SaveWorkspace writes w's metadata, creating its directory if needed. For a
// test workspace it also ensures the editable files/ subdirectory exists and
// forces HostPath to point at it; a real workspace keeps its caller-supplied
// HostPath. CreatedAt is set on first save and preserved on overwrite.
func (r *Registry) SaveWorkspace(w Workspace) error {
	if existing, err := r.GetWorkspace(w.Name); err == nil {
		w.CreatedAt = existing.CreatedAt
	} else if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now().UTC()
	}

	dir := r.workspaceDir(w.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}

	if w.Type == WorkspaceTest {
		filesDir := r.WorkspaceFilesDir(w.Name)
		if err := os.MkdirAll(filesDir, 0o755); err != nil {
			return fmt.Errorf("create workspace files directory: %w", err)
		}
		w.HostPath = filesDir
	}

	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, workspaceMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write workspace metadata: %w", err)
	}

	return nil
}

// GetWorkspace returns the named workspace.
func (r *Registry) GetWorkspace(name string) (Workspace, error) {
	data, err := os.ReadFile(filepath.Join(r.workspaceDir(name), workspaceMetadataFile))
	if err != nil {
		return Workspace{}, fmt.Errorf("read workspace %q: %w", name, err)
	}

	var w Workspace
	if err := json.Unmarshal(data, &w); err != nil {
		return Workspace{}, fmt.Errorf("parse metadata for workspace %q: %w", name, err)
	}

	return w, nil
}

// DeleteWorkspace removes a workspace's registry directory. For a test
// workspace this also deletes its files/ subtree; a real workspace's target
// directory on the user's machine is never touched, only the registry
// pointer to it. It's an error to delete one that doesn't exist.
func (r *Registry) DeleteWorkspace(name string) error {
	dir := r.workspaceDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("workspace %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete workspace %q: %w", name, err)
	}
	return nil
}

// ListWorkspaces returns every registry-tracked workspace, sorted by name.
func (r *Registry) ListWorkspaces() ([]Workspace, error) {
	entries, err := os.ReadDir(r.workspacesDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspaces directory: %w", err)
	}

	var workspaces []Workspace
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		w, err := r.GetWorkspace(entry.Name())
		if err != nil {
			continue // skip directories without valid metadata
		}
		workspaces = append(workspaces, w)
	}

	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].Name < workspaces[j].Name })

	return workspaces, nil
}
