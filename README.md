# Tiny LLM Workbench (TLW)

A local, interactive tool for training, validating, and running agents — powered by tiny LLMs.

TLW runs entirely on your machine: a Go CLI launches a local webserver and streams live events to a
browser UI, so you can fine-tune small models with [Apple MLX](https://github.com/ml-explore/mlx),
build agent workflows on a visual canvas, and evaluate how they perform — without shipping data to a
third-party service.

> **Status:** early development. The codebase currently contains the Go CLI scaffolding this project
> was bootstrapped from; the features below describe where the project is headed. See
> [Roadmap](#roadmap) for what's actually implemented today, and [CLAUDE.md](CLAUDE.md) for the
> conventions and working agreements that guide day-to-day (often agent-driven) development.

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

- [ ] **Phase 0 — Initial build-out**
  - [ ] Restructure the repo so the CLI launches a local webserver
  - [ ] Serve the UI shell and stream CLI events to it over SSE
- [ ] **Phase 1 — Dataset and Training**
  - [ ] Add Models / Dataset / Training pages to the navbar
  - [ ] Models: list local models (Ollama, MLX files, binaries)
  - [ ] Dataset: list input/output training pairs
  - [ ] Training: select model + dataset, configure and run training against MLX, view results
  - [ ] Local LLM client for generating dataset variations
- [ ] **Phase 2 — Environments**
  - [ ] Add an Environments page to the navbar
  - [ ] Prebuilt environments (WebSearch, SoftwareDev, OfficeWorker, ...) launched as Docker containers
  - [ ] Create and save custom environments
- [ ] **Phase 3 — Agents**
  - [ ] Add an Agents page to the navbar
  - [ ] Visual canvas for building agent workflows (input, prompt, output, decision nodes, ...)
  - [ ] Run view: watch agent events live and chat with a running agent
- [ ] **Phase 4 — Evaluations**
  - [ ] Add an Evaluations page to the navbar
  - [ ] Define tests (starting state, prompt, assertions) against a set of agents in one environment
  - [ ] Run evaluations and compare agent performance

Check off items as they land — this list is the source of truth for "what's actually built" and future
agent sessions rely on it being current. See [CLAUDE.md](CLAUDE.md) for how it's kept in sync.

## Getting Started

### Prerequisites

- Go 1.21 or higher
- [Task](https://taskfile.dev) (optional, for build automation)

### Build & run

```bash
git clone git@github.com:mtfuller/tiny-llm-workbench.git
cd tiny-llm-workbench
task build
```

This builds a `tlw` binary, but its commands (`greet`, `calc`, `process`, `version`) are still
placeholder scaffolding left over from the Go CLI template this project started from — none of the
features described above exist yet. Run `./tlw --help` to see what currently exists.

## Development

### Project structure

```
.
├── cmd/                    # Cobra command definitions
├── internal/               # CLI-internal packages (not importable externally)
├── pkg/                    # Reusable packages
├── tests/                  # Integration tests (full CLI invocations)
├── main.go                 # Entry point
└── Taskfile.yml             # Build/test/run automation
```

As Phase 0 lands, this will grow to include a webserver, an SSE event bus, and a browser UI. See
[CLAUDE.md](CLAUDE.md) for the conventions to follow as those pieces are added.

### Common tasks

```bash
task build              # build the binary
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
