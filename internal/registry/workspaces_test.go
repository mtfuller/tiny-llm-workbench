package registry

import (
	"testing"
)

func TestSaveAndGetEnvironment(t *testing.T) {
	reg := New(t.TempDir())

	want := Environment{
		Name:   "my-env",
		Image:  "alpine:3.20",
		Tools:  []string{"shell"},
		Mounts: []Mount{{HostPath: "/host", ContainerPath: "/container"}},
	}
	if err := reg.SaveEnvironment(want); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got.Name != want.Name || got.Image != want.Image || len(got.Mounts) != 1 || got.Mounts[0] != want.Mounts[0] {
		t.Errorf("GetEnvironment() = %+v, want %+v", got, want)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "shell" {
		t.Errorf("GetEnvironment().Tools = %+v, want [shell]", got.Tools)
	}
}

func TestGetEnvironmentUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetEnvironment("does-not-exist"); err == nil {
		t.Error("GetEnvironment() error = nil, want an error for an unknown environment")
	}
}

func TestListEnvironmentsEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	envs, err := reg.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) != 0 {
		t.Errorf("ListEnvironments() = %v, want empty", envs)
	}
}

func TestListEnvironmentsSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha"} {
		if err := reg.SaveEnvironment(Environment{Name: name, Image: "alpine:3.20"}); err != nil {
			t.Fatalf("SaveEnvironment(%q) error = %v", name, err)
		}
	}

	envs, err := reg.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) != 2 || envs[0].Name != "alpha" || envs[1].Name != "zeta" {
		t.Errorf("ListEnvironments() = %+v, want [alpha, zeta]", envs)
	}
}

func TestEnsurePrebuiltEnvironmentsSeedsOnce(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.EnsurePrebuiltEnvironments(); err != nil {
		t.Fatalf("EnsurePrebuiltEnvironments() error = %v", err)
	}

	envs, err := reg.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) != len(PrebuiltEnvironments()) {
		t.Fatalf("ListEnvironments() = %d entries, want %d prebuilt environments", len(envs), len(PrebuiltEnvironments()))
	}

	// Customize one, then ensure again — it must not be overwritten.
	customized := envs[0]
	customized.Image = "customized:latest"
	if err := reg.SaveEnvironment(customized); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.EnsurePrebuiltEnvironments(); err != nil {
		t.Fatalf("EnsurePrebuiltEnvironments() (second call) error = %v", err)
	}

	got, err := reg.GetEnvironment(customized.Name)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got.Image != "customized:latest" {
		t.Errorf("GetEnvironment().Image = %q, want the customization to survive re-seeding", got.Image)
	}
}

func TestPrebuiltEnvironmentsReferenceRealToolNames(t *testing.T) {
	toolNames := make(map[string]bool)
	for _, tool := range PrebuiltTools() {
		toolNames[tool.Name] = true
	}

	for _, env := range PrebuiltEnvironments() {
		if len(env.Tools) == 0 {
			t.Errorf("PrebuiltEnvironments() %q has no tools, want at least one", env.Name)
		}
		for _, toolName := range env.Tools {
			if !toolNames[toolName] {
				t.Errorf("PrebuiltEnvironments() %q references tool %q, which isn't in PrebuiltTools()", env.Name, toolName)
			}
		}
	}
}

func TestDeleteEnvironment(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "throwaway", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.DeleteEnvironment("throwaway"); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}

	if _, err := reg.GetEnvironment("throwaway"); err == nil {
		t.Error("GetEnvironment() error = nil, want an error after delete")
	}
}

func TestDeleteEnvironmentNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteEnvironment("does-not-exist"); err == nil {
		t.Error("DeleteEnvironment() error = nil, want an error for an unknown environment")
	}
}

func TestUpdateConfig(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	newMounts := []Mount{{HostPath: "/host", ContainerPath: "/container", ReadOnly: true}}
	if err := reg.UpdateConfig("my-env", "alpine:3.21", newMounts); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got.Image != "alpine:3.21" {
		t.Errorf("GetEnvironment().Image = %q, want alpine:3.21", got.Image)
	}
	if len(got.Mounts) != 1 || !got.Mounts[0].ReadOnly {
		t.Errorf("GetEnvironment().Mounts = %+v, want a single read-only mount", got.Mounts)
	}
}

func TestUpdateConfigUnknownEnvironment(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.UpdateConfig("does-not-exist", "alpine:3.20", nil); err == nil {
		t.Error("UpdateConfig() error = nil, want an error for an unknown environment")
	}
}

func TestAttachTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	if err := reg.SaveTool(Tool{Name: "read_file", Command: "cat {{path}}"}); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	if err := reg.AttachTool("my-env", "read_file"); err != nil {
		t.Fatalf("AttachTool() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "read_file" {
		t.Errorf("GetEnvironment().Tools = %+v, want [read_file]", got.Tools)
	}
}

func TestAttachToolUnknownTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.AttachTool("my-env", "does-not-exist"); err == nil {
		t.Error("AttachTool() error = nil, want an error when the tool isn't in the catalog")
	}
}

func TestAttachToolIsIdempotent(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	if err := reg.SaveTool(Tool{Name: "read_file", Command: "cat {{path}}"}); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := reg.AttachTool("my-env", "read_file"); err != nil {
			t.Fatalf("AttachTool() call %d error = %v", i, err)
		}
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if len(got.Tools) != 1 {
		t.Errorf("GetEnvironment().Tools = %+v, want attaching twice to still leave just one entry", got.Tools)
	}
}

func TestDetachTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20", Tools: []string{"read_file", "write_file"}}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.DetachTool("my-env", "read_file"); err != nil {
		t.Fatalf("DetachTool() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "write_file" {
		t.Errorf("GetEnvironment().Tools = %+v, want only write_file to remain", got.Tools)
	}
}

func TestDetachToolNotAttached(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.DetachTool("my-env", "read_file"); err == nil {
		t.Error("DetachTool() error = nil, want an error when the tool isn't attached")
	}
}

func TestDetachToolLeavesCatalogEntryAlone(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20", Tools: []string{"read_file"}}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	if err := reg.SaveTool(Tool{Name: "read_file", Command: "cat {{path}}"}); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	if err := reg.DetachTool("my-env", "read_file"); err != nil {
		t.Fatalf("DetachTool() error = %v", err)
	}

	if _, err := reg.GetTool("read_file"); err != nil {
		t.Errorf("GetTool() error = %v, want the catalog entry to survive detaching it from an environment", err)
	}
}
