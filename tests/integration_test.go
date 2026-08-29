package tests

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestCLIVersion tests the version command
func TestCLIVersion(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to run version command: %v\nOutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "tlw") {
		t.Errorf("Version output should contain 'tlw', got: %s", output)
	}
	if !strings.Contains(output, "Version:") {
		t.Errorf("Version output should contain 'Version:', got: %s", output)
	}
}

// TestCLIVersionFlag tests that `tlw --version` (the Cobra flag, not the
// subcommand) prints a one-line version.
func TestCLIVersionFlag(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run --version: %v\nOutput: %s", err, out.String())
	}

	output := strings.TrimSpace(out.String())
	if output != "tlw dev" {
		t.Errorf("--version output = %q, want %q", output, "tlw dev")
	}
}

// TestCLIVersionShort tests the version command with --short flag
func TestCLIVersionShort(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "version", "--short")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to run version --short command: %v\nOutput: %s", err, out.String())
	}

	output := strings.TrimSpace(out.String())
	// Should only output the version string
	if output != "dev" {
		t.Errorf("Version --short output should be 'dev', got: %s", output)
	}
}

// TestCLIHelp tests the help command
func TestCLIHelp(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "--help")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to run help command: %v\nOutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("Help output should contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "Available Commands:") {
		t.Errorf("Help output should contain 'Available Commands:', got: %s", output)
	}
}

// TestCLIServe tests that the serve command starts a webserver serving the
// browser UI and an SSE event stream, and shuts down cleanly on SIGTERM.
//
// This builds a temp binary rather than using `go run ../main.go`: `go run`
// does not reliably forward signals to the process it spawns, which this
// test needs in order to exercise the serve command's graceful shutdown.
func TestCLIServe(t *testing.T) {
	const port = 18123
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	binPath := filepath.Join(t.TempDir(), "tlw-test")
	build := exec.Command("go", "build", "-o", binPath, "../main.go")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build test binary: %v\nOutput: %s", err, out)
	}

	cmd := exec.Command(binPath, "serve", "--port", fmt.Sprintf("%d", port))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start serve command: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	})

	if !waitForServer(baseURL, 10*time.Second) {
		t.Fatalf("Server did not become ready in time. Output so far:\n%s", out.String())
	}

	t.Run("serves the browser UI shell", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("GET / error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET / status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("streams SSE events", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/events")
		if err != nil {
			t.Fatalf("GET /api/events error: %v", err)
		}
		defer resp.Body.Close()

		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
		}
	})

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to signal serve process: %v", err)
	}

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case err := <-waitErr:
		if err != nil {
			t.Errorf("serve command exited with error after SIGTERM: %v\nOutput:\n%s", err, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Errorf("serve command did not shut down within 5s of SIGTERM")
	}
}

// waitForServer polls baseURL until it responds or the timeout elapses.
func waitForServer(baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
