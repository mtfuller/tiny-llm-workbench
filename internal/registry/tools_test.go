package registry

import (
	"strings"
	"testing"
)

func TestSaveAndGetTool(t *testing.T) {
	reg := New(t.TempDir())

	want := Tool{
		Name:        "read_file",
		Description: "Read a file's contents",
		Command:     "cat {{path}}",
		Parameters:  []ToolParameter{{Name: "path", Type: ToolParamString, Required: true}},
	}
	if err := reg.SaveTool(want); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	got, err := reg.GetTool("read_file")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if got.Name != want.Name || got.Command != want.Command || len(got.Parameters) != 1 {
		t.Errorf("GetTool() = %+v, want %+v", got, want)
	}
}

func TestSaveToolSetsCreatedAtOnFirstSave(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveTool(Tool{Name: "t", Command: "echo hi"}); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	got, err := reg.GetTool("t")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetTool().CreatedAt is zero, want it set on first save")
	}
}

func TestSaveToolPreservesCreatedAtOnOverwrite(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveTool(Tool{Name: "t", Command: "echo hi"}); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}
	first, err := reg.GetTool("t")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}

	if err := reg.SaveTool(Tool{Name: "t", Command: "echo bye"}); err != nil {
		t.Fatalf("SaveTool() (update) error = %v", err)
	}
	second, err := reg.GetTool("t")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed on overwrite: first = %v, second = %v", first.CreatedAt, second.CreatedAt)
	}
	if second.Command != "echo bye" {
		t.Errorf("Command = %q, want %q", second.Command, "echo bye")
	}
}

func TestGetToolUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetTool("does-not-exist"); err == nil {
		t.Error("GetTool() error = nil, want an error for an unknown tool")
	}
}

func TestListToolsEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	tools, err := reg.ListTools()
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("ListTools() = %v, want empty", tools)
	}
}

func TestListToolsSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha"} {
		if err := reg.SaveTool(Tool{Name: name, Command: "echo hi"}); err != nil {
			t.Fatalf("SaveTool(%q) error = %v", name, err)
		}
	}

	tools, err := reg.ListTools()
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "alpha" || tools[1].Name != "zeta" {
		t.Errorf("ListTools() = %+v, want [alpha, zeta]", tools)
	}
}

func TestDeleteTool(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveTool(Tool{Name: "throwaway", Command: "echo hi"}); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	if err := reg.DeleteTool("throwaway"); err != nil {
		t.Fatalf("DeleteTool() error = %v", err)
	}

	if _, err := reg.GetTool("throwaway"); err == nil {
		t.Error("GetTool() error = nil, want an error after delete")
	}
}

func TestDeleteToolNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteTool("does-not-exist"); err == nil {
		t.Error("DeleteTool() error = nil, want an error for an unknown tool")
	}
}

func TestEnsurePrebuiltToolsSeedsOnce(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.EnsurePrebuiltTools(); err != nil {
		t.Fatalf("EnsurePrebuiltTools() error = %v", err)
	}

	tools, err := reg.ListTools()
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != len(PrebuiltTools()) {
		t.Fatalf("ListTools() = %d entries, want %d prebuilt tools", len(tools), len(PrebuiltTools()))
	}

	// Customize one, then ensure again — it must not be overwritten.
	customized := tools[0]
	customized.Command = "echo customized"
	if err := reg.SaveTool(customized); err != nil {
		t.Fatalf("SaveTool() error = %v", err)
	}

	if err := reg.EnsurePrebuiltTools(); err != nil {
		t.Fatalf("EnsurePrebuiltTools() (second call) error = %v", err)
	}

	got, err := reg.GetTool(customized.Name)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if got.Command != "echo customized" {
		t.Errorf("GetTool().Command = %q, want the customization to survive re-seeding", got.Command)
	}
}

func TestPrebuiltToolsHaveMatchingPlaceholders(t *testing.T) {
	for _, tool := range PrebuiltTools() {
		if tool.Command == "" {
			t.Errorf("PrebuiltTools() %q has no command", tool.Name)
		}
		for _, p := range tool.Parameters {
			if !strings.Contains(tool.Command, "{{"+p.Name+"}}") {
				t.Errorf("PrebuiltTools() %q declares parameter %q but its command has no {{%s}} placeholder", tool.Name, p.Name, p.Name)
			}
		}
	}
}
