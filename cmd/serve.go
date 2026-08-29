package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/benchmarks"
	"github.com/mtfuller/tiny-llm-workbench/internal/color"
	"github.com/mtfuller/tiny-llm-workbench/internal/datasetgen"
	"github.com/mtfuller/tiny-llm-workbench/internal/deployments"
	"github.com/mtfuller/tiny-llm-workbench/internal/docker"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/huggingface"
	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/server"
	"github.com/mtfuller/tiny-llm-workbench/internal/testcasegen"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

const shutdownTimeout = 5 * time.Second

var (
	servePort int
	serveHost string
	serveOpen bool
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the local TLW webserver and browser UI",
	Long: `Start the local webserver that serves the browser UI and streams CLI
events to it over Server-Sent Events. Leave this running and open the printed
URL in a browser. Stop it with Ctrl+C.

By default the server binds to 127.0.0.1 (loopback only), since the API can run
shell commands in Docker containers, shell out to mlx_lm, and read and write
files on this machine. Pass --host 0.0.0.0 (or a specific interface address) to
deliberately expose it on your LAN.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bus := eventbus.New()

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		reg, err := registry.Open()
		if err != nil {
			return fmt.Errorf("open registry: %w", err)
		}

		// runner's context (ctx, the whole server's lifetime) is deliberately
		// not tied to any single HTTP request — a model's mlx_lm.server
		// subprocess is started lazily and reused across many requests, and
		// must outlive whichever request happened to trigger starting it.
		runner := mlxrunner.New(ctx)
		generator := datasetgen.New(runner, reg)
		testCaseGenerator := testcasegen.New(runner, reg)
		hfClient := huggingface.New()

		// trainingMgr's context (ctx, not the per-request context an HTTP
		// handler would otherwise capture) bounds how long a run can keep
		// training in the background after StartRun's caller gets its
		// response — it must last for the server's lifetime, and runs are
		// cancelled when the server shuts down.
		trainingMgr := training.NewManager(ctx, filepath.Join(reg.Root(), "runs"), bus, reg, reg, &training.SubprocessTrainer{})
		if err := trainingMgr.LoadRuns(); err != nil {
			logger.Warn("Failed to load training run history: %v", err)
		}

		if err := reg.EnsurePrebuiltTools(); err != nil {
			logger.Warn("Failed to seed prebuilt tools: %v", err)
		}

		dockerClient, err := docker.New()
		if err != nil {
			return fmt.Errorf("build docker client: %w", err)
		}
		if pingErr := dockerClient.Ping(ctx); pingErr != nil {
			logger.Warn("%v — workspace sandboxes will fail to launch until it is", pingErr)
		}
		environmentsMgr := environments.NewManager(ctx, dockerClient, reg, bus, filepath.Join(reg.Root(), "workspace-runs"))

		agentsMgr := agents.NewManager(ctx, reg, runner, environmentsMgr, reg, reg, bus)

		evaluationsMgr := evaluations.NewManager(ctx, reg, agentsMgr, environmentsMgr, bus, filepath.Join(reg.Root(), "evaluation-results"))

		benchmarksMgr := benchmarks.NewManager(ctx, reg, reg, runner, bus, filepath.Join(reg.Root(), "benchmark-results"))

		deploymentsMgr := deployments.NewManager(ctx, reg, reg, environmentsMgr, agentsMgr)

		handler, err := server.New(server.Deps{
			Bus:                bus,
			Models:             reg,
			ModelRunner:        runner,
			HuggingFace:        hfClient,
			Datasets:           reg,
			Generator:          generator,
			Training:           trainingMgr,
			Workspaces:         reg,
			Instances:          environmentsMgr,
			Tools:              reg,
			Knowledge:          reg,
			Agents:             reg,
			AgentRuns:          agentsMgr,
			Evaluations:        reg,
			EvalRuns:           evaluationsMgr,
			Benchmarks:         reg,
			BenchRuns:          benchmarksMgr,
			Deployments:        reg,
			DeploymentSessions: deploymentsMgr,
			TestCaseGen:        testCaseGenerator,
			RegistryRoot:       reg.Root(),
		})
		if err != nil {
			return fmt.Errorf("build server: %w", err)
		}

		addr := net.JoinHostPort(serveHost, strconv.Itoa(servePort))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			if errors.Is(err, syscall.EADDRINUSE) {
				return fmt.Errorf("port %d is already in use — start with --port to pick another", servePort)
			}
			return fmt.Errorf("listen on %s: %w", addr, err)
		}

		go publishHeartbeats(ctx, bus)

		url := fmt.Sprintf("http://%s:%d", displayHost(serveHost), listener.Addr().(*net.TCPAddr).Port)
		if serveOpen {
			go openBrowser(url)
		}

		return runServer(ctx, &http.Server{Handler: handler}, listener)
	},
}

// runServer serves on listener until ctx is cancelled, then shuts down
// gracefully.
func runServer(ctx context.Context, httpServer *http.Server, listener net.Listener) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.Serve(listener)
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	color.Success("TLW is running at http://%s:%d", displayHost(serveHost), port)
	logger.Info("Webserver listening on %s", listener.Addr())

	select {
	case <-ctx.Done():
		logger.Info("Shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

// openBrowser tries to open url in the user's default browser, after a short
// delay so the server is already accepting connections. A failure is logged,
// not fatal — the URL is printed regardless.
func openBrowser(url string) {
	time.Sleep(400 * time.Millisecond)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		logger.Warn("Could not open a browser automatically (%v) — open %s yourself", err, url)
	}
}

// displayHost turns a bind address into something usable in a browser URL:
// a wildcard bind (0.0.0.0 / ::) is reachable via localhost on the same
// machine, and an empty host means loopback.
func displayHost(host string) string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "localhost"
	default:
		return host
	}
}

// publishHeartbeats periodically publishes a heartbeat event so connected
// browser clients can see the SSE stream is alive.
func publishHeartbeats(ctx context.Context, bus *eventbus.Bus) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			bus.Publish(eventbus.Event{Type: "heartbeat", Data: t.Format(time.RFC3339)})
		}
	}
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "port to serve the webserver on")
	serveCmd.Flags().StringVar(&serveHost, "host", "127.0.0.1", "address to bind to (use 0.0.0.0 to expose on your LAN)")
	serveCmd.Flags().BoolVar(&serveOpen, "open", false, "open the UI in your default browser once the server is up")
}
