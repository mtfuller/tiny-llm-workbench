// Package registry implements TLW's on-disk model and dataset registry: a
// plain directory tree (no database) rooted at ~/.tlw by default, so its
// contents stay inspectable and hand-editable.
package registry

import (
	"os"
	"path/filepath"
)

// homeEnvVar overrides the registry root, mainly for tests.
const homeEnvVar = "TLW_HOME"

// Registry reads and writes the model/dataset registry rooted at Root.
type Registry struct {
	root string
}

// New creates a Registry rooted at root.
func New(root string) *Registry {
	return &Registry{root: root}
}

// Open creates a Registry rooted at the default location: $TLW_HOME if set,
// otherwise ~/.tlw.
func Open() (*Registry, error) {
	if dir := os.Getenv(homeEnvVar); dir != "" {
		return New(dir), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return New(filepath.Join(home, ".tlw")), nil
}

// Root returns the registry's root directory.
func (r *Registry) Root() string {
	return r.root
}

func (r *Registry) modelsDir() string {
	return filepath.Join(r.root, "models")
}

func (r *Registry) datasetsDir() string {
	return filepath.Join(r.root, "datasets")
}

func (r *Registry) modelDir(name string) string {
	return filepath.Join(r.modelsDir(), name)
}

// ModelDir returns the directory a model named name lives in (or would live
// in), so callers can write files there (e.g. a fused, servable model)
// before calling SaveModel.
func (r *Registry) ModelDir(name string) string {
	return r.modelDir(name)
}

func (r *Registry) datasetDir(name string) string {
	return filepath.Join(r.datasetsDir(), name)
}
