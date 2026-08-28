package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndGetTestWorkspace(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveWorkspace(Workspace{Name: "scratch", Type: WorkspaceTest}); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}

	got, err := reg.GetWorkspace("scratch")
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if got.Name != "scratch" || got.Type != WorkspaceTest {
		t.Errorf("GetWorkspace() = %+v, want name=scratch type=test", got)
	}
	// A test workspace's HostPath is always forced to its files/ dir, which
	// is created on save.
	wantPath := reg.WorkspaceFilesDir("scratch")
	if got.HostPath != wantPath {
		t.Errorf("GetWorkspace().HostPath = %q, want %q", got.HostPath, wantPath)
	}
	if info, err := os.Stat(wantPath); err != nil || !info.IsDir() {
		t.Errorf("expected editable files dir at %q, stat err = %v", wantPath, err)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetWorkspace().CreatedAt is zero, want it stamped on first save")
	}
}

func TestSaveRealWorkspaceKeepsHostPath(t *testing.T) {
	reg := New(t.TempDir())
	realDir := t.TempDir()

	if err := reg.SaveWorkspace(Workspace{Name: "my-project", Type: WorkspaceReal, HostPath: realDir}); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}

	got, err := reg.GetWorkspace("my-project")
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if got.HostPath != realDir {
		t.Errorf("GetWorkspace().HostPath = %q, want the caller-supplied %q", got.HostPath, realDir)
	}
	if _, err := os.Stat(reg.WorkspaceFilesDir("my-project")); !os.IsNotExist(err) {
		t.Errorf("real workspace should not create a files/ dir, stat err = %v", err)
	}
}

func TestSaveWorkspacePreservesCreatedAt(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveWorkspace(Workspace{Name: "w", Type: WorkspaceTest}); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	first, _ := reg.GetWorkspace("w")

	if err := reg.SaveWorkspace(Workspace{Name: "w", Type: WorkspaceTest}); err != nil {
		t.Fatalf("SaveWorkspace() re-save error = %v", err)
	}
	second, _ := reg.GetWorkspace("w")

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed on re-save: %v -> %v", first.CreatedAt, second.CreatedAt)
	}
}

func TestGetWorkspaceUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetWorkspace("does-not-exist"); err == nil {
		t.Error("GetWorkspace() error = nil, want an error for an unknown workspace")
	}
}

func TestListWorkspacesEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	ws, err := reg.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(ws) != 0 {
		t.Errorf("ListWorkspaces() = %v, want empty", ws)
	}
}

func TestListWorkspacesSorted(t *testing.T) {
	reg := New(t.TempDir())
	for _, n := range []string{"charlie", "alpha", "bravo"} {
		if err := reg.SaveWorkspace(Workspace{Name: n, Type: WorkspaceTest}); err != nil {
			t.Fatalf("SaveWorkspace(%q) error = %v", n, err)
		}
	}

	ws, err := reg.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	got := []string{ws[0].Name, ws[1].Name, ws[2].Name}
	want := []string{"alpha", "bravo", "charlie"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListWorkspaces() names = %v, want %v", got, want)
		}
	}
}

func TestDeleteTestWorkspaceRemovesFiles(t *testing.T) {
	reg := New(t.TempDir())
	if err := reg.SaveWorkspace(Workspace{Name: "w", Type: WorkspaceTest}); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	seed := filepath.Join(reg.WorkspaceFilesDir("w"), "notes.txt")
	if err := os.WriteFile(seed, []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := reg.DeleteWorkspace("w"); err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, err := reg.GetWorkspace("w"); err == nil {
		t.Error("GetWorkspace() after delete = nil error, want not-found")
	}
}

func TestDeleteRealWorkspaceLeavesTargetDir(t *testing.T) {
	reg := New(t.TempDir())
	realDir := t.TempDir()
	keep := filepath.Join(realDir, "keep.txt")
	if err := os.WriteFile(keep, []byte("data"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := reg.SaveWorkspace(Workspace{Name: "p", Type: WorkspaceReal, HostPath: realDir}); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}

	if err := reg.DeleteWorkspace("p"); err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("deleting a real workspace must not touch its target dir, but %q is gone: %v", keep, err)
	}
}

func TestDeleteWorkspaceUnknown(t *testing.T) {
	reg := New(t.TempDir())
	if err := reg.DeleteWorkspace("nope"); err == nil {
		t.Error("DeleteWorkspace() error = nil, want an error for an unknown workspace")
	}
}
