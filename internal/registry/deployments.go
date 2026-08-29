package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const deploymentMetadataFile = "definition.json"

// Deployment binds one agent to one real Workspace so it can do actual,
// persisting work: starting a deployment launches a sandbox with the real
// directory bind-mounted read-write, and chatting with the agent from the
// Deployments page drives changes straight into that directory. Only the
// definition is persisted here — a running session (and its chat transcript)
// is in-memory only, like an agent chat run.
type Deployment struct {
	Name          string    `json:"name"`
	AgentName     string    `json:"agentName"`
	WorkspaceName string    `json:"workspaceName"` // must name a WorkspaceReal
	CreatedAt     time.Time `json:"createdAt"`
}

func (r *Registry) deploymentsDir() string {
	return filepath.Join(r.root, "deployments")
}

func (r *Registry) deploymentDir(name string) string {
	return filepath.Join(r.deploymentsDir(), name)
}

// SaveDeployment writes d's definition, creating or overwriting it.
// CreatedAt is set on first save and preserved on every later overwrite.
func (r *Registry) SaveDeployment(d Deployment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveDeployment(d)
}

// saveDeployment is the non-locking core of SaveDeployment; callers must hold r.mu.
func (r *Registry) saveDeployment(d Deployment) error {
	if existing, err := r.getDeployment(d.Name); err == nil {
		d.CreatedAt = existing.CreatedAt
	} else if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}

	dir := r.deploymentDir(d.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create deployment directory: %w", err)
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal deployment definition: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, deploymentMetadataFile), data, 0o644); err != nil {
		return fmt.Errorf("write deployment definition: %w", err)
	}

	return nil
}

// GetDeployment returns the named deployment's definition.
func (r *Registry) GetDeployment(name string) (Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getDeployment(name)
}

// getDeployment is the non-locking core of GetDeployment; callers must hold r.mu.
func (r *Registry) getDeployment(name string) (Deployment, error) {
	data, err := os.ReadFile(filepath.Join(r.deploymentDir(name), deploymentMetadataFile))
	if err != nil {
		return Deployment{}, fmt.Errorf("read deployment %q: %w", name, err)
	}

	var d Deployment
	if err := json.Unmarshal(data, &d); err != nil {
		return Deployment{}, fmt.Errorf("parse definition for deployment %q: %w", name, err)
	}

	return d, nil
}

// DeleteDeployment removes a deployment's directory. It's an error to delete
// one that doesn't exist.
func (r *Registry) DeleteDeployment(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := r.deploymentDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("deployment %q not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete deployment %q: %w", name, err)
	}
	return nil
}

// ListDeployments returns every registry-tracked deployment, sorted by name.
func (r *Registry) ListDeployments() ([]Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.deploymentsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deployments directory: %w", err)
	}

	var deployments []Deployment
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		d, err := r.getDeployment(entry.Name())
		if err != nil {
			continue // skip directories without a valid definition
		}
		deployments = append(deployments, d)
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Name < deployments[j].Name })

	return deployments, nil
}
