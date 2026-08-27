package registry

import (
	"strings"
	"testing"
)

func TestSaveAndGetEnvironment(t *testing.T) {
	reg := New(t.TempDir())

	want := Environment{
		Name:  "my-env",
		Image: "alpine:3.20",
		Tools: []Tool{
			{Name: "shell", Command: "{{cmd}}", Parameters: []ToolParameter{{Name: "cmd", Type: ToolParamString, Required: true}}},
		},
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
	if len(got.Tools) != 1 || got.Tools[0].Name != "shell" {
		t.Errorf("GetEnvironment().Tools = %+v, want a single 'shell' tool", got.Tools)
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

func TestPrebuiltEnvironmentsHaveRealTools(t *testing.T) {
	for _, env := range PrebuiltEnvironments() {
		if len(env.Tools) == 0 {
			t.Errorf("PrebuiltEnvironments() %q has no tools, want at least one", env.Name)
		}
		for _, tool := range env.Tools {
			if tool.Command == "" {
				t.Errorf("PrebuiltEnvironments() %q tool %q has no command", env.Name, tool.Name)
			}
			for _, p := range tool.Parameters {
				if !strings.Contains(tool.Command, "{{"+p.Name+"}}") {
					t.Errorf("PrebuiltEnvironments() %q tool %q declares parameter %q but its command has no {{%s}} placeholder", env.Name, tool.Name, p.Name, p.Name)
				}
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

func TestAddTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	tool := Tool{Name: "read_file", Command: "cat {{path}}", Parameters: []ToolParameter{{Name: "path", Type: ToolParamString, Required: true}}}
	if err := reg.AddTool("my-env", tool); err != nil {
		t.Fatalf("AddTool() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "read_file" {
		t.Errorf("GetEnvironment().Tools = %+v, want a single read_file tool", got.Tools)
	}
}

func TestUpdateTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20", Tools: []Tool{{Name: "read_file", Command: "cat {{path}}"}}}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.UpdateTool("my-env", 0, Tool{Name: "read_file_v2", Command: "cat -A {{path}}"}); err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "read_file_v2" {
		t.Errorf("GetEnvironment().Tools = %+v, want the tool renamed to read_file_v2", got.Tools)
	}
}

func TestUpdateToolOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.UpdateTool("my-env", 0, Tool{Name: "x"}); err == nil {
		t.Error("UpdateTool() error = nil, want an error for an out-of-range index")
	}
}

func TestDeleteTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{
		Name:  "my-env",
		Image: "alpine:3.20",
		Tools: []Tool{{Name: "read_file", Command: "cat {{path}}"}, {Name: "write_file", Command: "printf '%s' {{content}} > {{path}}"}},
	}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.DeleteTool("my-env", 0); err != nil {
		t.Fatalf("DeleteTool() error = %v", err)
	}

	got, err := reg.GetEnvironment("my-env")
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "write_file" {
		t.Errorf("GetEnvironment().Tools = %+v, want only write_file to remain", got.Tools)
	}
}

func TestDeleteToolOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveEnvironment(Environment{Name: "my-env", Image: "alpine:3.20"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := reg.DeleteTool("my-env", 0); err == nil {
		t.Error("DeleteTool() error = nil, want an error for an out-of-range index")
	}
}
