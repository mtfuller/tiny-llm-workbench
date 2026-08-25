# Tiny LLM Workbench (TLW)

A local, interactive tool for training, validating, and running agents — powered by tiny LLMs.

TLW runs entirely on your machine: a Go CLI launches a local webserver and streams live events to a
browser UI, so you can fine-tune small models with [Apple MLX](https://github.com/ml-explore/mlx),
build agent workflows on a visual canvas, and evaluate how they perform — without shipping data to a
third-party service.

> **Status:** early development, but every phase in the roadmap below is now built. Phases 0, 2, 3, and 4
> are fully verified live against real local infrastructure (Ollama, Docker). Phase 1 is functionally
> complete too, though Training's happy path (an actual successful MLX run) is unverified — see the
> Roadmap note. See [Roadmap](#roadmap) for the specifics and caveats, and [CLAUDE.md](CLAUDE.md) for
> the conventions and working agreements that guide day-to-day (often agent-driven) development.

## Features

- **Datasets** — TLW ships with a tiny LLM fine-tuned to generate variations of training data, so you
  don't have to hand-write it. Explore and edit generated examples before training.
- **Training** — Train models locally with Apple MLX. Pick a base model (from Ollama or a model you've
  already trained) and a dataset, configure a run, and watch weight changes and training stats
  (duration, iterations, memory) update live.
- **Environments** — Sandboxed environments give agents tools, memory, and a filesystem to do real
  work. Several prebuilt environments (WebSearch, SoftwareDev, OfficeWorker, ...) are included, or
  build your own.
- **Agents** — Design agents visually on a canvas: connect prompt, model, tool, memory, knowledge, and
  input/output nodes into a workflow. Each agent targets a specific environment.
- **Evaluations** — Define test suites against your agents: starting environment state, an initial
  prompt, and assertions to check. TLW ships with a tiny LLM fine-tuned to generate evaluation test
  variations from a single example, and lets you compare results across agents.

## Roadmap

- [x] **Phase 0 — Initial build-out**
  - [x] Restructure the repo so the CLI launches a local webserver
  - [x] Serve the UI shell and stream CLI events to it over SSE
- [ ] **Phase 1 — Dataset and Training**
  - [x] Add Models / Dataset / Training pages to the navbar
  - [x] Models: list local models (Ollama, MLX files, binaries) — Ollama models list live; MLX models
        are registered automatically when a training run succeeds. There's still no manual "import an
        existing binary/MLX file" flow, so a model only appears in the registry by being trained here.
  - [x] Dataset: list input/output training pairs
  - [x] Training: select model + dataset, configure and run training against MLX, view results — the
        full pipeline is built and its error path (bad config, subprocess failure) is verified
        end-to-end, but the happy path (an actual successful mlx-lm run) is **unverified**: this was
        built without a working Python/MLX install available (see CLAUDE.md's MLX integration note).
        Try it on a real machine and expect to iterate on `internal/training/scripts/train.py`'s
        log-parsing if mlx-lm's output format doesn't match what it expects.
  - [x] Local LLM client for generating dataset variations
- [x] **Phase 2 — Environments**
  - [x] Add an Environments page to the navbar
  - [x] Prebuilt environments (WebSearch, SoftwareDev, OfficeWorker, ...) launched as Docker containers
  - [x] Create and save custom environments
- [x] **Phase 3 — Agents**
  - [x] Add an Agents page to the navbar
  - [x] Visual canvas for building agent workflows (input, prompt, output, decision, tool nodes) — built
        with React Flow. Decision branches on a simple keyword match against the prior node's output,
        not an LLM call. An agent can target a specific Environment in its settings; a tool node runs a
        literal shell command (with `{{input}}` templating) inside that Environment's running instance
        — deterministic, not an LLM-driven tool-calling loop. Fully verified live: a real Docker
        container was launched, a tool node's command executed inside it and its output flowed to the
        next node, and the container was cleaned up when the chat closed. No memory/knowledge nodes yet
        — that's future work.
  - [x] Run view: watch agent events live and chat with a running agent — fully verified live with a
        real local Ollama model: canvas building, saving, chatting, and the live step-by-step execution
        log all work end-to-end.
- [x] **Phase 4 — Evaluations**
  - [x] Add an Evaluations page to the navbar
  - [x] Define tests (starting state, prompt, assertions) against a set of agents in one environment —
        assertions are deterministic (`contains`/`not_contains`/`regex`) checked against an agent's
        reply, not LLM-graded. An evaluation's optional Environment is really launched for the run's
        duration (proving the Phase 2 plumbing), but agents can't act on it yet — see the Phase 3 note
        on Environments not being wired to agent execution.
  - [x] Run evaluations and compare agent performance — fully verified live: created a real evaluation,
        ran it against a real agent backed by a real local Ollama model, and got a live pass/fail
        comparison table with a per-agent score.

Check off items as they land — this list is the source of truth for "what's actually built" and future
agent sessions rely on it being current. See [CLAUDE.md](CLAUDE.md) for how it's kept in sync.

## Getting Started

### Prerequisites

- Go 1.22 or higher
- [Task](https://taskfile.dev) (optional, for build automation)
- Node.js 22+ and npm — only needed if you're changing the browser UI under `web/`. A prebuilt copy of
  `web/dist` is committed, so a plain `go build`/`go test` works without Node.
- Docker (e.g. Docker Desktop) running locally — only needed to launch Environments. `tlw serve` starts
  fine without it; launching an Environment will just fail with a clear "Docker daemon unreachable"
  error until it's running.

### Build & run

```bash
git clone git@github.com:mtfuller/tiny-llm-workbench.git
cd tiny-llm-workbench
task build
./tlw serve
```

`task build` rebuilds the browser UI (`web:build`) before compiling the Go binary. `tlw serve` starts
the local webserver — open the printed URL (default `http://localhost:8080`) to see the UI shell
receiving live events over SSE. The `greet`, `calc`, `process`, and `version` commands are still
placeholder scaffolding left over from the Go CLI template this project started from. Run `./tlw --help`
to see everything currently available.

## Development

### Project structure

```
.
├── cmd/                    # Cobra command definitions (includes `serve`)
├── internal/
│   ├── eventbus/           # In-process pub/sub bridging CLI events to SSE
│   ├── server/             # HTTP server: embedded UI + JSON API + /api/events SSE stream
│   ├── registry/           # On-disk model/dataset/environment/agent/evaluation registry (~/.tlw)
│   ├── ollama/             # Client for the local Ollama API
│   ├── models/             # Merges the registry + Ollama into one model list
│   ├── datasetgen/         # Generates dataset variations via a local LLM
│   ├── training/           # MLX training runs via a Python subprocess
│   ├── docker/             # Docker Engine API client (launch/stop/exec containers)
│   ├── environments/       # Environment instance lifecycle (launch/stop/exec)
│   ├── agents/             # Agent graph execution engine + chat run manager
│   ├── evaluations/        # Deterministic assertion checks + evaluation run manager
│   └── ...                 # logger, color, spinner, version
├── pkg/                    # Reusable packages
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
