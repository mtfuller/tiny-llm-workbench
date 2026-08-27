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
- **Agents** — Design agents visually on a canvas: connect prompt, model, tool, knowledge, decision, and
  input/output nodes into a workflow. Each agent can target a specific Environment (for its tool nodes).
- **Evaluations** — Define versioned test suites against your agents: a prompt, optional setup commands
  to prepare a realistic scenario (seed files, init a repo) in the agent's own Environment before its
  turn, assertions on the reply, and optional verify commands checking the environment's resulting state
  afterward — build real software-dev, knowledge-work, or office-work scenarios and see whether an agent
  actually completed the task, not just what it said. Publish versions, run against multiple agents, and
  compare durable pass@1/assertion-rate/latency results across runs. TLW ships with a tiny LLM
  fine-tuned to generate test prompt variations from a single example.
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
  - [x] Add an Environments page to the navbar — since expanded into its own "Environments" nav section
        (Environments, Knowledge, Tools)
  - [x] Prebuilt environments (WebSearch, SoftwareDev, OfficeWorker, ...) launched as Docker containers
  - [x] Create and save custom environments
  - [x] Per-environment workspace page (Configuration / Tools / Playground tabs): edit image and mounts
        (with per-mount read-only), attach tools from a shared global catalog (a command template plus a
        typed parameter list — string/number/boolean, editable on its own Tools page and referenced by
        name, so editing a tool updates every environment it's attached to), and launch an instance to try
        a tool interactively with live streamed output. Prebuilt environments come with real tools already
        attached (read/write file, list directory; WebSearch also gets a DuckDuckGo-backed web search
        tool) rather than just descriptive labels. Fully verified live against a real Docker container:
        attached a catalog tool to a running environment and ran it for a real result, and ran `read_file`
        against a path containing a space to confirm argument quoting is correct.
  - [x] Knowledge — a separate page (independent of any Environment/Docker) for defining named knowledge
        bases of title/content records, queryable by deterministic keyword match. An Agent's new
        "knowledge" canvas node queries one directly.
- [x] **Phase 3 — Agents**
  - [x] Add an Agents page to the navbar
  - [x] Visual canvas for building agent workflows (input, prompt, output, decision, tool, knowledge
        nodes) — built with React Flow. Agent settings (which Environment it targets, plus a free-text
        description) live
        in a settings modal opened from the left node palette, not an always-visible sidebar block. A
        professional-looking, node-type-colored right-sidebar inspector configures each selected node; a
        tool node picks one of the bound Environment's real tools from a dropdown and fills in a
        generated form for its typed parameters — deterministic, not an LLM-driven tool-calling loop.
        Every node has an editable Name, and any downstream node's text fields (a prompt's system
        prompt/prompt template, a tool's parameters, a decision's match text) can reference an earlier
        node's output — not just its immediate predecessor — as `{{NodeName}}`, with an "insert variable"
        picker in the inspector so you don't have to type it by hand. A prompt node can optionally declare
        a JSON Schema its reply must satisfy (best-effort: the model is instructed via the prompt, then
        the reply is parsed and validated — there's no true constrained decoding available from the local
        MLX server, so a non-conforming reply fails the turn rather than being silently accepted), and once
        it does, downstream nodes can pull out a specific property with `{{NodeName.property}}`. Decision
        branches on a simple keyword match against a node's output (optionally a specific property, via the
        same templating), not an LLM call. Fully verified live end-to-end, twice: a real Docker container
        was launched, a tool node's chosen tool (a real DuckDuckGo web search) executed inside it with the
        previous node's output correctly templated into its query parameter; and a real local model
        extracted `{"city": "Paris"}` into a schema-checked node, with a downstream node's prompt template
        correctly resolving `{{Classifier.city}}` to produce a reply specifically about Parisian food. A
        knowledge node queries a named KnowledgeBase (deterministic keyword match, no Environment
        involved) and its result is referenceable downstream the same way any other node's output is —
        verified live with a real KnowledgeBase record correctly matched and templated into a prompt node
        that produced an accurate, on-topic reply from a real local model.
  - [x] Run view: watch agent events live and chat with a running agent — fully verified live with a
        real local MLX model (served via `mlx_lm.server`, TLW's inference backend — see CLAUDE.md's MLX
        integration note): canvas building, saving, chatting, and the live step-by-step execution log
        all work end-to-end.
- [x] **Phase 4 — Evaluations**
  - [x] Add an Evaluations page to the navbar
  - [x] Define tests (prompt, assertions) against a set of agents in one environment — assertions are
        deterministic (`contains`/`not_contains`/`regex`/`json_schema`/`similarity`) checked against an
        agent's reply, not LLM-graded.
  - [x] Run evaluations and compare agent performance — fully verified live: created a real evaluation,
        ran it against a real agent backed by a real local MLX model, and got a live pass/fail
        comparison table with a per-agent score.
  - [x] Rebuilt to mirror Benchmarks: draft/publish versioning (a run always targets an immutable
        published version), durable per-agent results across runs, and per-test-case setup/verify
        commands so a test case can prepare a realistic scenario (setup commands run before the agent's
        turn) and check the environment's resulting state afterward (verify commands with their own
        assertions), not just the reply — the point being to actually verify a software-dev/
        knowledge-work/office-work task got done, not just what the agent said about it. Setup, the
        agent's own Tool-node actions, and verify commands all run in the exact same freshly-launched
        Environment instance for that (agent, test case) pair. Fully verified live against a real Docker
        container: an agent's Tool node wrote a value into a file inside a container a test case's setup
        step had just prepared, and the test case's verify command read that exact value back out of the
        same container, with its assertion correctly passing.

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
