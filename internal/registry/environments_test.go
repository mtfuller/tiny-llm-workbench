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
