package environments

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirRecursiveWithModeBits(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "a.txt"), "top", 0o644)
	if err := os.MkdirAll(filepath.Join(src, "nested", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(src, "nested", "deep", "b.sh"), "#!/bin/sh\n", 0o755)

	dst := filepath.Join(t.TempDir(), "out")
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	if got := readFile(t, filepath.Join(dst, "a.txt")); got != "top" {
		t.Errorf("a.txt = %q, want %q", got, "top")
	}
	if got := readFile(t, filepath.Join(dst, "nested", "deep", "b.sh")); got != "#!/bin/sh\n" {
		t.Errorf("b.sh = %q, want the script contents", got)
	}
	info, err := os.Stat(filepath.Join(dst, "nested", "deep", "b.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("b.sh mode = %v, want the executable bit preserved", info.Mode())
	}
}

func TestCopyDirMissingSourceMakesEmptyDir(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	if err := copyDir(filepath.Join(t.TempDir(), "does-not-exist"), dst); err != nil {
		t.Fatalf("copyDir() error = %v, want a missing source to be tolerated", err)
	}
	info, err := os.Stat(dst)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected an empty dir at %q, stat err = %v", dst, err)
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
