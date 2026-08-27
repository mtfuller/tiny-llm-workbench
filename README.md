# Tiny LLM Workbench (TLW)

A local, interactive tool for training, validating, and running agents — powered by tiny LLMs.

TLW runs entirely on your machine: a Go CLI launches a local webserver and streams live events to a
browser UI, so you can fine-tune small models with [Apple MLX](https://github.com/ml-explore/mlx),
build agent workflows on a visual canvas, and evaluate how they perform — without shipping data to a
third-party service. Training and running models are both powered by [mlx-lm](https://github.com/ml-explore/mlx-lm)
— no other model runtime is involved, so a model trained here is a model you can actually chat with here.

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
- **Environments** — Sandboxed Docker environments give agents tools, memory, and a filesystem to do
  real work. Each environment has its own workspace page: configure its image and mounts, define tools
  (a command template with a typed parameter list — string/number/boolean), and launch an instance to
  try a tool interactively before wiring it into an agent. Several prebuilt environments (WebSearch,
  SoftwareDev, OfficeWorker) ship with real tools already defined (read/write files, list a directory;
  WebSearch also gets a keyless web-search tool), or build your own from scratch.
- **Agents** — Design agents visually on a canvas: connect prompt, model, tool, memory, knowledge, and
  input/output nodes into a workflow. Each agent targets a specific environment.
- **Evaluations** — Define test suites against your agents: starting environment state, an initial
  prompt, and assertions to check. TLW ships with a tiny LLM fine-tuned to generate evaluation test
  variations from a single example, and lets you compare results across agents.
- **Benchmarks** — Define test suites (a prompt plus assertions) run directly against a set of models,
  no agent or environment involved — the main way to compare how different models actually perform.

## Roadmap

- [x] **Phase 0 — Initial build-out**
  - [x] Restructure the repo so the CLI launches a local webserver
  - [x] Serve the UI shell and stream CLI events to it over SSE
- [x] **Phase 1 — Dataset and Training**
  - [x] Add Models / Dataset / Training pages to the navbar
  - [x] Models: list local models — registered automatically when a training run succeeds (fused into a
        standalone, directly-runnable model, not just the raw LoRA adapter). There's still no manual
        "import an existing model file" flow, so a model only appears in the registry by being trained
        here; any other MLX-format Hugging Face repo id can still be used anywhere a model is picked,
        downloaded automatically on first use.
  - [x] Dataset: list input/output training pairs
  - [x] Training: select model + dataset, configure and run training against MLX, view results — the
        full pipeline (including the happy path, a real successful `mlx_lm.lora` run producing a
        registered, runnable model) is verified end-to-end against a real mlx-lm install (see
        CLAUDE.md's MLX integration note).
  - [x] Local MLX model client for generating dataset variations
- [x] **Phase 2 — Environments**
  - [x] Add an Environments page to the navbar
  - [x] Prebuilt environments (WebSearch, SoftwareDev, OfficeWorker, ...) launched as Docker containers
  - [x] Create and save custom environments
  - [x] Per-environment workspace page (Configuration / Tools / Playground tabs): edit image and mounts
        (with per-mount read-only), define tools as a command template plus a typed parameter list
        (string/number/boolean), and launch an instance to try a tool interactively with live streamed
        output. Prebuilt environments ship with real tools (read/write file, list directory; WebSearch
        also gets a DuckDuckGo-backed web search tool) rather than just descriptive labels. Fully
        verified live against a real Docker container: launched an instance, ran the real `web_search`
        tool and got a real API response, and ran `read_file` against a path containing a space to
        confirm argument quoting is correct.
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
        real local MLX model (served via `mlx_lm.server`, TLW's inference backend — see CLAUDE.md's MLX
        integration note): canvas building, saving, chatting, and the live step-by-step execution log
        all work end-to-end.
- [x] **Phase 4 — Evaluations**
  - [x] Add an Evaluations page to the navbar
  - [x] Define tests (starting state, prompt, assertions) against a set of agents in one environment —
        assertions are deterministic (`contains`/`not_contains`/`regex`) checked against an agent's
        reply, not LLM-graded. An evaluation's optional Environment is really launched for the run's
        duration (proving the Phase 2 plumbing), but agents can't act on it yet — see the Phase 3 note
        on Environments not being wired to agent execution.
  - [x] Run evaluations and compare agent performance — fully verified live: created a real evaluation,
        ran it against a real agent backed by a real local MLX model, and got a live pass/fail
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
- [`mlx-lm`](https://github.com/ml-explore/mlx-lm) (Apple Silicon only) — needed for Training, and for
  anything that runs a model (Agents, dataset variation generation) — TLW has no other model runtime.
  Install with `pip install mlx-lm` or `brew install mlx-lm`, and make sure the resulting `mlx_lm.*`
  commands are on PATH for wherever `tlw serve` runs. `tlw serve` starts fine without it; anything that
  needs a model will just fail with a clear "not found on PATH" error until it's installed.

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
│   ├── mlxrunner/          # On-demand mlx_lm.server pool — TLW's model inference backend
│   ├── datasetgen/         # Generates dataset variations via a local MLX model
│   ├── training/           # MLX training runs via the mlx_lm.lora CLI
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
