# Tiny LLM Workbench (TLW)

A local, interactive tool for training, validating, and running agents — powered by tiny LLMs.

TLW runs entirely on your machine: a Go CLI launches a local webserver and streams live events to a
browser UI, so you can fine-tune small models with [Apple MLX](https://github.com/ml-explore/mlx),
build agent workflows on a visual canvas, and evaluate how they perform — without shipping data to a
third-party service. Training and running models are both powered by [mlx-lm](https://github.com/ml-explore/mlx-lm)
— no other model runtime is involved, so a model trained here is a model you can actually chat with here.

**Requires macOS on Apple Silicon.** `mlx-lm` is Apple-Silicon-only and every model-backed feature
depends on it. On other platforms the CLI and UI still run, but training, chat, generation, and
benchmarks all fail.

> **Status:** early development, but every phase in the roadmap below is now built and verified live
> against real local infrastructure (Docker, and a real `mlx-lm` install for Training and running
> models). See [Roadmap](#roadmap) for specifics, and [CLAUDE.md](CLAUDE.md) for the conventions and
> working agreements that guide day-to-day (often agent-driven) development.

## Features

- **Datasets** — TLW ships with a tiny LLM fine-tuned to generate variations of training data, so you
  don't have to hand-write it. Explore and edit generated examples before training.
- **Training** — Train models locally with Apple MLX. Pick a base model (a Hugging Face MLX repo id, or
  a model you've already trained here) and a dataset, configure a run, and watch weight changes and
  training stats (duration, iterations, memory) update live.
- **Environments** — Sandboxed Docker environments give agents tools and a filesystem to do real work.
  Each environment has its own workspace page: configure its image and mounts, attach tools from a
  shared catalog, and launch an instance to try one interactively before wiring it into an agent.
- **Tools** — A shared catalog of runnable commands (a command template with a typed parameter list —
  string/number/boolean), independent of any one Environment: attach a tool to as many environments as
  you like by name, and editing it updates every attachment. Several prebuilt tools ship out of the box
  (read/write files, list a directory, a keyless DuckDuckGo web search) and come pre-attached to the
  prebuilt Environments (WebSearch, SoftwareDev, OfficeWorker), or build your own from scratch.
- **Knowledge** — Define named knowledge bases of title/content records an agent can query. Matching is
  a deterministic keyword search (every query word must appear in the record), not embeddings or a
  vector store — consistent with the rest of TLW's deterministic, non-LLM-graded decision points.
- **Agents** — Design agent *architectures* visually on a top-to-bottom canvas, not just linear
  pipelines: connect input, prompt, tool, knowledge, condition, switch, loop, state, and agent nodes —
  and wire edges into cycles — to build plan-execute-judge, Ralph loops, reflexion, and the like from
  small orthogonal pieces. A
  `condition` node routes `pass`/`fail` on a deterministic check (contains / regex / JSON schema /
  similarity) and a `switch` node routes N ways on a case match; a **loop start / loop end** pair
  brackets a loop — route a branch to the loop end to keep
  looping, anywhere else to break out — with `{{Loop.iteration}}` available inside and a max-iterations
  cap; a `state` node accumulates a scratchpad across iterations; a `say` node streams a progress
  message to the chat as the agent works (with an optional "this is the final answer" marker); an
  `agent` node runs a bounded LLM
  tool-calling loop over a chosen subset of the environment's tools plus any knowledge bases it's given,
  and can constrain its final answer to a JSON schema. A prompt or agent node with a schema can route a
  validation miss to a `fail` handle instead of ending the turn; any node whose output is JSON exposes
  `{{Node.property}}` downstream. Any node with nothing connected
  downstream of it is simply where a turn ends, so there's no separate "output" node to wire up. Each
  agent can target a specific Environment (for its tool and agent nodes). A step-by-step debugger lets
  you pause a turn, Step through one node at a time to see exactly what it produced, and Retry a node to
  get a fresh result before deciding to move on — with a live activity feed and a running-elapsed
  indicator so a node waiting on the model isn't a silent freeze. Models picked by their registry name
  (including one added from Hugging Face) are resolved wherever a model is chosen — agent nodes and the
  Training base model — so they load instead of 404ing on the org-less name.
- **Evaluations** — Define versioned test suites against your agents: a prompt, optional setup commands
  to prepare a realistic scenario (seed files, init a repo) in the agent's own Environment before its
  turn, assertions on the reply, and optional verify commands checking the environment's resulting state
  afterward — build real software-dev, knowledge-work, or office-work scenarios and see whether an agent
  actually completed the task, not just what it said. Publish versions, run against multiple agents, and
  compare durable pass@1/assertion-rate/latency results across runs. TLW ships with a tiny LLM
  fine-tuned to generate test prompt variations from a single example.
- **Benchmarks** — Define test suites (a prompt plus assertions) run directly against a set of models,
  no agent or environment involved — the main way to compare how different models actually perform.

## Getting Started

### Prerequisites

- **macOS on Apple Silicon (M-series) is required.** `mlx-lm` powers Training *and all model
  inference* (agents, chat, dataset/test-case generation, benchmarks), and it is Apple-Silicon-only. On
  Linux, Windows, or an Intel Mac the CLI and browser UI run, but every model-backed feature fails.
- Go 1.23 or higher (only to build from source)
- [Task](https://taskfile.dev) (optional, for build automation)
- Node.js 22+ and npm — only needed if you're changing the browser UI under `web/`. A prebuilt copy of
  `web/dist` is committed, so a plain `go build`/`go test` works without Node.
- Docker (e.g. Docker Desktop) running locally — only needed to launch Environments. `tlw serve` starts
  fine without it; launching an Environment will just fail with a clear "Docker daemon unreachable"
  error until it's running.
- [`mlx-lm`](https://github.com/ml-explore/mlx-lm) — needed for Training, and for anything that runs a
  model (Agents, dataset variation generation) — TLW has no other model runtime.
  Install with `pip install mlx-lm` or `brew install mlx-lm`, and make sure the resulting `mlx_lm.*`
  commands are on PATH for wherever `tlw serve` runs. `tlw serve` starts fine without it; anything that
  needs a model will just fail with a clear "not found on PATH" error until it's installed.
  Note: `brew install mtfuller/tap/tlw` does **not** pull in `mlx-lm` — install it separately.

### Install

**With Homebrew (recommended, Apple Silicon).**

```bash
brew install mtfuller/tap/tlw
```

Formula installs are not Gatekeeper-quarantined, so there's no `xattr` step. `mlx-lm` is *not* pulled
in automatically — install it separately (see Prerequisites) for Training and model inference; `tlw
serve` runs without it.

**From a GitHub Release (manual).** Grab the latest `tlw_<version>_darwin_arm64.tar.gz` from the
[Releases page](https://github.com/mtfuller/tiny-llm-workbench/releases), then:

```bash
tar -xzf tlw_*_darwin_arm64.tar.gz
xattr -d com.apple.quarantine tlw   # the raw binary is not notarized — clear Gatekeeper
./tlw serve --open
```

**With `go install`** (builds from source, needs Go 1.23+):

```bash
go install github.com/mtfuller/tiny-llm-workbench@latest
# installs as `tiny-llm-workbench` in $(go env GOPATH)/bin — rename to `tlw` if you like
```

**From source** — see below.

### Build & run

```bash
git clone git@github.com:mtfuller/tiny-llm-workbench.git
cd tiny-llm-workbench
task build
./tlw serve
```

`task build` rebuilds the browser UI (`web:build`) before compiling the Go binary. `tlw serve` starts
the local webserver, bound to `127.0.0.1` by default — open the printed URL (default
`http://localhost:8080`), or run `tlw serve --open` to launch it in your browser automatically. Pass
`--host 0.0.0.0` to deliberately expose it on your LAN (the API can run shell commands, shell out to
`mlx_lm`, and read and write files, so this is loopback-only unless you opt in), and `--port` to change
the port. Run `./tlw --help` to see everything available.

On first run the registry (`~/.tlw`) is empty and the Home page shows a short "Get started" guide: add
a base model, build a dataset, and train — or jump straight to building an agent on the canvas.

## Development

### Project structure

```
.
├── cmd/                    # Cobra command definitions (includes `serve`)
├── internal/
│   ├── eventbus/           # In-process pub/sub bridging CLI events to SSE
│   ├── server/             # HTTP server: embedded UI + JSON API + /api/events SSE stream
│   ├── registry/           # On-disk model/dataset/environment/agent/evaluation registry (~/.tlw)
│   ├── mlxrunner/          # On-demand mlx_lm.server pool — TLW's model inference backend
│   ├── datasetgen/         # Generates dataset variations via a local MLX model
│   ├── training/           # MLX training runs via the mlx_lm.lora CLI
│   ├── docker/             # Docker Engine API client (launch/stop/exec containers)
│   ├── environments/       # Environment instance lifecycle (launch/stop/exec)
│   ├── agents/             # Agent graph execution engine + chat run manager
│   ├── evaluations/        # Deterministic assertion checks + evaluation run manager
│   └── ...                 # benchmarks, deployments, knowledge, huggingface, safetensors, logger, ...
├── web/                    # React + TypeScript browser UI (Vite). web/dist is embedded
│                           # into the binary via web/embed.go.
├── tests/                  # Integration tests (full CLI invocations)
├── main.go                 # Entry point
└── Taskfile.yml            # Build/test/run automation
```

See [CLAUDE.md](CLAUDE.md) for the conventions to follow as later phases build on this.

### Common tasks

```bash
task build              # build the browser UI, then the tlw binary
task web:install        # npm install for the browser UI (web/)
task web:build          # rebuild web/dist after changing web/src
task run                # go run main.go
task test               # unit + integration tests
task test-unit          # unit tests only
task test-integration   # integration tests only
task coverage           # coverage report (coverage.html)
```

### Conventions

Go style, file layout, and testing conventions live in
[.github/instructions](.github/instructions) and apply regardless of which tool (or agent) is writing
the code:

- [STYLEGUIDE.instructions.md](.github/instructions/STYLEGUIDE.instructions.md)
- [TESTING.instructions.md](.github/instructions/TESTING.instructions.md)

## Contributing / agent-driven development

This repo is set up to be built out largely by AI coding agents across many sessions. Before picking up
work, read [CLAUDE.md](CLAUDE.md) — it covers the working agreements, open architecture decisions that
still need a human call, and the project-specific skills and hooks available in `.claude/`.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
