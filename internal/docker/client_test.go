package docker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

// requireDocker skips the test if the local Docker daemon isn't reachable,
// so `task test` still passes in environments without Docker running —
// only environments where it's actually up get the real smoke test.
func requireDocker(t *testing.T) *Client {
	t.Helper()

	c, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Skipf("Docker daemon not reachable, skipping: %v", err)
	}

	return c
}

func TestLaunchExecStopLifecycle(t *testing.T) {
	c := requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := "tlw-test-" + time.Now().Format("150405.000000")
	containerID, err := c.Launch(ctx, name, "test-env", "alpine:3.20", nil)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	t.Cleanup(func() {
		_ = c.Stop(context.Background(), containerID)
	})

	if containerID == "" {
		t.Fatal("Launch() returned an empty container ID")
	}

	var output strings.Builder
	exitCode, err := c.ExecStream(ctx, containerID, []string{"echo", "hello from tlw"}, func(chunk string) {
		output.WriteString(chunk)
	})
	if err != nil {
		t.Fatalf("ExecStream() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("ExecStream() exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(output.String(), "hello from tlw") {
		t.Errorf("ExecStream() output = %q, want it to contain %q", output.String(), "hello from tlw")
	}

	managed, err := c.ListManaged(ctx)
	if err != nil {
		t.Fatalf("ListManaged() error = %v", err)
	}
	found := false
	for _, m := range managed {
		if m.ID == containerID {
			found = true
			if m.WorkspaceName != "test-env" {
				t.Errorf("ListManaged() entry WorkspaceName = %q, want %q", m.WorkspaceName, "test-env")
			}
			if m.State != "running" {
				t.Errorf("ListManaged() entry State = %q, want %q", m.State, "running")
			}
		}
	}
	if !found {
		t.Errorf("ListManaged() = %+v, want it to include container %q", managed, containerID)
	}

	if err := c.Stop(ctx, containerID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	managed, err = c.ListManaged(ctx)
	if err != nil {
		t.Fatalf("ListManaged() after Stop() error = %v", err)
	}
	for _, m := range managed {
		if m.ID == containerID {
			t.Errorf("ListManaged() still lists %q after Stop() removed it", containerID)
		}
	}
}

func TestExecStreamNonZeroExitCode(t *testing.T) {
	c := requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := "tlw-test-" + time.Now().Format("150405.000000")
	containerID, err := c.Launch(ctx, name, "test-env", "alpine:3.20", nil)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	t.Cleanup(func() {
		_ = c.Stop(context.Background(), containerID)
	})

	exitCode, err := c.ExecStream(ctx, containerID, []string{"sh", "-c", "exit 7"}, func(string) {})
	if err != nil {
		t.Fatalf("ExecStream() error = %v", err)
	}
	if exitCode != 7 {
		t.Errorf("ExecStream() exit code = %d, want 7", exitCode)
	}
}

func TestPingReportsUnreachableDaemonClearly(t *testing.T) {
	// Point at a plausible-looking but unreachable socket rather than a
	// live daemon, so this test doesn't depend on Docker actually running.
	rawClient, err := client.NewClientWithOpts(client.WithHost("unix:///tmp/tlw-test-nonexistent.sock"))
	if err != nil {
		t.Fatalf("client.NewClientWithOpts() error = %v", err)
	}
	unreachable := &Client{cli: rawClient}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := unreachable.Ping(ctx); err == nil {
		t.Fatal("Ping() error = nil, want an error for an unreachable daemon")
	} else if !strings.Contains(err.Error(), "Docker") {
		t.Errorf("Ping() error = %v, want a message mentioning Docker", err)
	}
}
