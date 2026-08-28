// Package environments orchestrates workspace sandboxes: launching a
// registry.Workspace's directory as a real Docker container (a fresh copy
// for a test workspace, a live bind mount for a real one), stopping it, and
// exec'ing commands into it with output streamed live over the CLI's event
// bus. It also renders and runs catalog Tool commands (see tools.go), used
// both by the Tools page's Playground and by an agent graph's tool nodes.
package environments

import "time"

// Instance is a running (or stopped) workspace sandbox container. Unlike
// Phase 1's training runs, instance state isn't persisted to our own JSON
// files — Docker itself is the source of truth (see Manager.ListInstances),
// so there's nothing to reconcile on a `tlw serve` restart beyond asking
// Docker what's still running.
//
// WorkspacePath is the host-side directory mounted into the container —
// for a test workspace, the throwaway copy under <registry
// root>/workspace-runs that a user can open in an editor to watch the agent
// work; for a real workspace, the user's own directory. It's only known for
// an instance this process launched (a reconciled one from ListInstances
// leaves it empty).
type Instance struct {
	ID            string    `json:"id"` // the container ID
	Name          string    `json:"name"`
	WorkspaceName string    `json:"workspaceName"`
	WorkspaceType string    `json:"workspaceType,omitempty"` // "test" | "real"
	WorkspacePath string    `json:"workspacePath,omitempty"`
	Image         string    `json:"image"`
	State         string    `json:"state"` // Docker's own state string: "running", "exited", ...
	CreatedAt     time.Time `json:"createdAt"`
}

// ExecStatus is an Exec's lifecycle state.
type ExecStatus string

const (
	ExecRunning ExecStatus = "running"
	ExecDone    ExecStatus = "done"
	ExecFailed  ExecStatus = "failed"
)

// Exec is a single command run inside an Instance. Execs are ephemeral —
// tracked in memory only for the lifetime of the `tlw serve` process, not
// persisted, since (unlike a training run) losing exec history on restart
// isn't costly.
type Exec struct {
	ID         string     `json:"id"`
	InstanceID string     `json:"instanceId"`
	Command    string     `json:"command"`
	Output     string     `json:"output"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Status     ExecStatus `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// ExecOutputEvent and ExecStatusEvent are the eventbus event types the
// Tools page's Playground SSE stream listens for.
const (
	ExecOutputEvent = "workspace.exec.output"
	ExecStatusEvent = "workspace.exec.status"
)
