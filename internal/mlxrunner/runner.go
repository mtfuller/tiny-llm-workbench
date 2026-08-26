// Package mlxrunner runs MLX models for inference (agent chat, dataset
// variation generation) by managing a pool of on-demand `mlx_lm.server`
// subprocesses — one per distinct model actually in use, started lazily on
// first request and stopped after a period of inactivity to free memory.
// This replaced Ollama as TLW's inference backend: Ollama can't load
// MLX-trained models at all (different weight format/quantization scheme —
// confirmed by hand against a real Ollama install), so training and running
// now both go through mlx-lm.
package mlxrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/logger"
)

// defaultCommand is the mlx-lm CLI command Runner execs when Command is
// left empty.
const defaultCommand = "mlx_lm.server"

// defaultIdleTimeout is how long a model's server can sit unused before
// Runner stops it to free memory.
const defaultIdleTimeout = 15 * time.Minute

// readyTimeout bounds how long Runner waits for a freshly started server to
// start responding — loading a model (and possibly downloading it from
// Hugging Face on first use) can take a while.
const readyTimeout = 3 * time.Minute

// reapInterval is how often the idle reaper checks for servers to stop.
const reapInterval = time.Minute

// Runner manages a pool of `mlx_lm.server` subprocesses, one per model
// actually in use, and proxies Generate calls to them over HTTP. It
// implements the same interface ollama.Client used to (Generate(ctx, model,
// prompt) (string, error)), so it's a drop-in replacement wherever that was
// injected (internal/agents, internal/datasetgen).
type Runner struct {
	// Command overrides which executable to run, mainly for tests. Empty
	// means defaultCommand, resolved via PATH.
	Command string
	// IdleTimeout overrides how long an unused server stays up. Zero means
	// defaultIdleTimeout.
	IdleTimeout time.Duration
	// MaxTokens overrides how many tokens a single Generate call may
	// produce. Zero means defaultMaxTokens.
	MaxTokens int
	// RepetitionPenalty overrides how strongly repeated tokens are
	// discouraged during generation. Zero means defaultRepetitionPenalty.
	RepetitionPenalty float64

	// ctx bounds the lifetime of every server process this Runner starts —
	// it must outlive any single HTTP request (a server is reused across
	// many requests), so New takes the app's shutdown context, not a
	// per-request one.
	ctx        context.Context
	httpClient *http.Client

	mu      sync.Mutex
	servers map[string]*serverProc // keyed by model
	closed  bool
}

// serverProc is one running (or starting) mlx_lm.server instance.
type serverProc struct {
	cmd      *exec.Cmd
	baseURL  string
	lastUsed time.Time
	ready    chan struct{} // closed once the server responds, or fails to
	readyErr error
	done     chan struct{} // closed when the process exits
}

// New builds a Runner whose server subprocesses are tied to ctx: cancelling
// ctx (e.g. on `tlw serve` shutdown) kills every running server.
func New(ctx context.Context) *Runner {
	r := &Runner{
		ctx:        ctx,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		servers:    make(map[string]*serverProc),
	}
	go r.reapLoop()
	go func() {
		<-ctx.Done()
		r.shutdown()
	}()
	return r
}

// Generate runs model on prompt and returns its full completion, starting
// (or reusing) that model's server as needed. model is an MLX-format model:
// a Hugging Face repo id, or a local directory (e.g. a TLW-trained model,
// already fused into a standalone model by internal/training).
func (r *Runner) Generate(ctx context.Context, model, prompt string) (string, error) {
	return r.chat(ctx, model, []chatMessage{{Role: "user", Content: prompt}})
}

// ChatMessage is one turn in a multi-turn conversation passed to Chat.
type ChatMessage struct {
	Role    string
	Content string
}

// Chat runs a full conversation (every prior turn plus the latest one)
// against model in a single call, for multi-turn chat UIs. Unlike Generate's
// single implicit user turn, callers must resend the whole history every
// time — mlx_lm.server itself is stateless between requests, so nothing
// persists between calls on TLW's side either.
func (r *Runner) Chat(ctx context.Context, model string, messages []ChatMessage) (string, error) {
	msgs := make([]chatMessage, len(messages))
	for i, m := range messages {
		msgs[i] = chatMessage{Role: m.Role, Content: m.Content}
	}
	return r.chat(ctx, model, msgs)
}

func (r *Runner) chat(ctx context.Context, model string, messages []chatMessage) (string, error) {
	srv, err := r.ensure(model)
	if err != nil {
		return "", err
	}

	text, err := r.complete(ctx, srv.baseURL, messages)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	srv.lastUsed = time.Now()
	r.mu.Unlock()

	return text, nil
}

// chatMessage/chatCompletionRequest/-Response mirror mlx_lm.server's
// OpenAI-compatible POST /v1/chat/completions API. "default_model" asks for
// whatever model the server was started with — mlx_lm.server's
// --adapter-path flag has a real bug where it's silently ignored for the
// server's own default model (confirmed against mlx-lm 0.31.3: the
// CLI-supplied adapter never gets applied), which is exactly why training
// fuses adapters into a standalone model first instead of ever asking a
// server to load base+adapter together.
//
// This hits /v1/chat/completions rather than the plain-text /v1/completions
// endpoint deliberately: reading mlx_lm.server's source (0.31.3) shows only
// the chat endpoint calls the model's own tokenizer.apply_chat_template —
// /v1/completions feeds prompt straight to the model with no role markers
// or stop tokens at all. For an Instruct-tuned model that means it isn't
// being asked to answer anything, just to keep writing whatever text
// pattern statistically follows the raw prompt string — a real, confirmed
// contributor (alongside repetition_penalty below) to the repeated-gibberish
// output seen from small models on this endpoint.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model             string        `json:"model"`
	Messages          []chatMessage `json:"messages"`
	MaxTokens         int           `json:"max_tokens"`
	RepetitionPenalty float64       `json:"repetition_penalty"`
}

// defaultMaxTokens caps how long a single completion can run. Confirmed by
// hand: mlx_lm.server's own default (512 tokens) routinely rambled into
// long text for a short chat/dataset-gen prompt — this both bounds latency
// and keeps replies from running away.
const defaultMaxTokens = 256

// defaultRepetitionPenalty discourages the token-loop degeneration small
// models are prone to under greedy decoding (mlx_lm.server's own default
// temperature is 0.0). Confirmed by reading mlx_lm.server's source (0.31.3):
// its repetition_penalty request field defaults to 0.0, i.e. off, unless a
// caller sets it — matching a real observed case where a 0.5B model's reply
// to a plain prompt degenerated into the same sentence repeated dozens of
// times. 1.3 is a fairly assertive but standard value for suppressing that
// on small models without visibly hurting coherent replies.
const defaultRepetitionPenalty = 1.3

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (r *Runner) complete(ctx context.Context, baseURL string, messages []chatMessage) (string, error) {
	maxTokens := r.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	repetitionPenalty := r.RepetitionPenalty
	if repetitionPenalty == 0 {
		repetitionPenalty = defaultRepetitionPenalty
	}

	body, err := json.Marshal(chatCompletionRequest{
		Model:             "default_model",
		Messages:          messages,
		MaxTokens:         maxTokens,
		RepetitionPenalty: repetitionPenalty,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request mlx_lm.server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("mlx_lm.server returned status %d: %s", resp.StatusCode, respBody)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return "", fmt.Errorf("decode mlx_lm.server response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("mlx_lm.server returned no choices")
	}

	return completion.Choices[0].Message.Content, nil
}

// ensure returns a ready server for model, starting one if none is running
// (or the previous one has died).
func (r *Runner) ensure(model string) (*serverProc, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("mlxrunner: shutting down")
	}

	srv, ok := r.servers[model]
	if ok {
		select {
		case <-srv.done:
			delete(r.servers, model)
			ok = false
		default:
		}
	}
	if !ok {
		var err error
		srv, err = r.start(model)
		if err != nil {
			r.mu.Unlock()
			return nil, err
		}
		r.servers[model] = srv
	}
	r.mu.Unlock()

	<-srv.ready
	if srv.readyErr != nil {
		return nil, srv.readyErr
	}
	return srv, nil
}

// start launches a new mlx_lm.server for model and begins polling it for
// readiness in the background; it returns as soon as the process is
// spawned, without waiting for it to finish loading.
func (r *Runner) start(model string) (*serverProc, error) {
	command := r.Command
	if command == "" {
		command = defaultCommand
	}
	if _, err := exec.LookPath(command); err != nil {
		return nil, fmt.Errorf(
			"%s not found on PATH — install it with `pip install mlx-lm` or `brew install mlx-lm`, "+
				"and make sure the environment tlw serve runs in has that installation's bin directory on PATH",
			command,
		)
	}

	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("allocate a port for mlx_lm.server: %w", err)
	}

	cmd := exec.CommandContext(r.ctx, command, "--model", model, "--host", "127.0.0.1", "--port", strconv.Itoa(port))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	srv := &serverProc{
		cmd:      cmd,
		baseURL:  fmt.Sprintf("http://127.0.0.1:%d", port),
		lastUsed: time.Now(),
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
	}

	go func() {
		waitErr := cmd.Wait()
		close(srv.done)
		if waitErr != nil {
			logger.Debug("mlx_lm.server for %q exited: %v", model, waitErr)
		}
	}()

	go waitReady(srv, model, &stderr)

	return srv, nil
}

// waitReady polls srv until it responds to an HTTP request, the process
// exits first, or readyTimeout elapses — closing srv.ready (with readyErr
// set on failure) when it's resolved one way or another.
func waitReady(srv *serverProc, model string, stderr *bytes.Buffer) {
	defer close(srv.ready)

	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(readyTimeout)

	for {
		select {
		case <-srv.done:
			detail := strings.TrimSpace(stderr.String())
			if detail == "" {
				detail = "process exited before becoming ready"
			}
			srv.readyErr = fmt.Errorf("mlx_lm.server for %q failed to start: %s", model, detail)
			return
		case <-deadline:
			srv.readyErr = fmt.Errorf("mlx_lm.server for %q did not become ready within %s", model, readyTimeout)
			return
		case <-ticker.C:
			req, err := http.NewRequest(http.MethodGet, srv.baseURL+"/v1/models", nil)
			if err != nil {
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
	}
}

// freePort asks the OS for an unused TCP port by briefly binding to :0.
// There's a small unavoidable race between releasing it here and
// mlx_lm.server binding it itself, acceptable for a local dev tool.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// reapLoop periodically stops servers that have been idle past the idle
// timeout, freeing their memory.
func (r *Runner) reapLoop() {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.reapIdle()
		}
	}
}

func (r *Runner) reapIdle() {
	idleTimeout := r.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = defaultIdleTimeout
	}

	r.mu.Lock()
	var toStop []*serverProc
	for model, srv := range r.servers {
		select {
		case <-srv.ready:
		default:
			continue // still starting up, leave it alone
		}
		if srv.readyErr == nil && time.Since(srv.lastUsed) > idleTimeout {
			toStop = append(toStop, srv)
			delete(r.servers, model)
		}
	}
	r.mu.Unlock()

	for _, srv := range toStop {
		stopServer(srv)
	}
}

// shutdown stops every running server. Called automatically when the
// Runner's context is cancelled.
func (r *Runner) shutdown() {
	r.mu.Lock()
	r.closed = true
	servers := r.servers
	r.servers = make(map[string]*serverProc)
	r.mu.Unlock()

	for _, srv := range servers {
		stopServer(srv)
	}
}

func stopServer(srv *serverProc) {
	if srv.cmd.Process != nil {
		_ = srv.cmd.Process.Kill()
	}
	<-srv.done
}
