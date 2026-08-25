// Package environments orchestrates Environment instances: launching a
// registry.Environment definition as a real Docker container, stopping it,
// and exec'ing commands into it with output streamed live over the CLI's
// event bus.
package environments

import "time"

// Instance is a running (or stopped) Environment container. Unlike Phase
// 1's training runs, instance state isn't persisted to our own JSON files —
// Docker itself is the source of truth (see Manager.ListInstances), so
// there's nothing to reconcile on a `tlw serve` restart beyond asking
// Docker what's still running.
type Instance struct {
	ID              string    `json:"id"` // the container ID
	Name            string    `json:"name"`
	EnvironmentName string    `json:"environmentName"`
	Image           string    `json:"image"`
	State           string    `json:"state"` // Docker's own state string: "running", "exited", ...
	CreatedAt       time.Time `json:"createdAt"`
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
// Environments page's SSE stream listens for.
const (
	ExecOutputEvent = "environment.exec.output"
	ExecStatusEvent = "environment.exec.status"
)
