package registry

import "testing"

func TestSaveAndGetDeployment(t *testing.T) {
	reg := New(t.TempDir())

	want := Deployment{Name: "prod", AgentName: "coder", WorkspaceName: "my-project"}
	if err := reg.SaveDeployment(want); err != nil {
		t.Fatalf("SaveDeployment() error = %v", err)
	}

	got, err := reg.GetDeployment("prod")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if got.Name != want.Name || got.AgentName != want.AgentName || got.WorkspaceName != want.WorkspaceName {
		t.Errorf("GetDeployment() = %+v, want %+v", got, want)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetDeployment().CreatedAt is zero, want it stamped on first save")
	}
}

func TestSaveDeploymentPreservesCreatedAt(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveDeployment(Deployment{Name: "d", AgentName: "a", WorkspaceName: "w"}); err != nil {
		t.Fatalf("SaveDeployment() error = %v", err)
	}
	first, _ := reg.GetDeployment("d")

	if err := reg.SaveDeployment(Deployment{Name: "d", AgentName: "a2", WorkspaceName: "w2"}); err != nil {
		t.Fatalf("SaveDeployment() re-save error = %v", err)
	}
	second, _ := reg.GetDeployment("d")

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed on re-save: %v -> %v", first.CreatedAt, second.CreatedAt)
	}
	if second.AgentName != "a2" {
		t.Errorf("AgentName = %q, want the overwrite applied", second.AgentName)
	}
}

func TestGetDeploymentUnknown(t *testing.T) {
	reg := New(t.TempDir())
	if _, err := reg.GetDeployment("nope"); err == nil {
		t.Error("GetDeployment() error = nil, want an error for an unknown deployment")
	}
}

func TestListDeploymentsSorted(t *testing.T) {
	reg := New(t.TempDir())
	for _, n := range []string{"gamma", "alpha", "beta"} {
		if err := reg.SaveDeployment(Deployment{Name: n, AgentName: "a", WorkspaceName: "w"}); err != nil {
			t.Fatalf("SaveDeployment(%q) error = %v", n, err)
		}
	}

	got, err := reg.ListDeployments()
	if err != nil {
		t.Fatalf("ListDeployments() error = %v", err)
	}
	want := []string{"alpha", "beta", "gamma"}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("ListDeployments() = %v, want order %v", got, want)
		}
	}
}

func TestDeleteDeployment(t *testing.T) {
	reg := New(t.TempDir())
	if err := reg.SaveDeployment(Deployment{Name: "d", AgentName: "a", WorkspaceName: "w"}); err != nil {
		t.Fatalf("SaveDeployment() error = %v", err)
	}
	if err := reg.DeleteDeployment("d"); err != nil {
		t.Fatalf("DeleteDeployment() error = %v", err)
	}
	if _, err := reg.GetDeployment("d"); err == nil {
		t.Error("GetDeployment() after delete = nil error, want not-found")
	}
	if err := reg.DeleteDeployment("d"); err == nil {
		t.Error("DeleteDeployment() twice = nil error, want an error")
	}
}
