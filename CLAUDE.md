# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repo. Read this before picking up work.
For the product pitch and feature list, see [README.md](README.md) — this file covers how to work
here, not what the product does.

## What this repo is, right now

Tiny LLM Workbench (TLW) is a local tool for training, running, and evaluating agents backed by tiny
LLMs, via a Go CLI (module `github.com/mtfuller/tiny-llm-workbench`, binary `tlw`) + local webserver +
browser UI. **The product described in the README has not been built yet.** The repo currently
contains the Go CLI scaffolding it was bootstrapped from (a generic Cobra starter — `cmd/greet.go`,
`cmd/calc.go`, `cmd/process.go` are placeholder example commands that still need to be replaced with
real TLW commands).

Don't assume anything beyond `cmd/`, `internal/`, `pkg/`, `tests/`, `main.go`, and `Taskfile.yml`
exists. If you're the first to touch a given roadmap phase, you're deciding its shape, not discovering
an existing one — treat non-trivial structural choices accordingly (see below).

## Roadmap

The authoritative, checkbox-tracked roadmap lives in [README.md](README.md#roadmap). Keep it current:
when a roadmap item is actually done (builds, is tested, works end to end), check it off in the same
change that completes it. Don't check off partial work. The `roadmap-status` skill (see below) helps
with this.

Work the phases roughly in order (0 → 4) — later phases assume earlier ones exist (e.g. Environments
assumes there's a webserver and UI shell to add a nav page to). Within a phase, small, independently
testable increments beat one large change.

## Repository layout & conventions

- `cmd/` — Cobra commands, one file per command
- `internal/` — CLI-internal packages, not importable outside this module
- `pkg/` — packages that could reasonably be reused outside this CLI
- `tests/` — end-to-end integration tests that exec the built CLI
- `Taskfile.yml` — `task build`, `task test`, `task test-unit`, `task test-integration`, `task coverage`, `task run`, `task clean`, `task install`

Go style, file/command layout, and testing conventions are documented in
[.github/instructions/STYLEGUIDE.instructions.md](.github/instructions/STYLEGUIDE.instructions.md) and
[.github/instructions/TESTING.instructions.md](.github/instructions/TESTING.instructions.md). Those
apply to every change regardless of which tool is writing the code — this file doesn't restate them.

## Working agreements

- **Run `task test` and let the gofmt hook do its job before calling anything done.** See Hooks below —
  `.go` edits are auto-formatted, but you're still responsible for the code compiling and tests passing.
- **Keep the roadmap checklist honest.** Future sessions (yours or another agent's) will trust it to
  figure out what's already built — a checked box for unfinished work wastes the next session's time
  re-discovering that.
- **Prefer additive, phase-scoped changes.** Don't refactor Phase 0 scaffolding while working on Phase 2
  unless it's blocking you; note it instead (a TODO, a message to the user, or `spawn_task` if you have
  it) and stay in scope.
- **This is a local-first, privacy-sensitive tool** (it trains on and runs against the user's own data
  and models). Don't introduce telemetry, remote calls, or cloud dependencies without those being an
  explicit, discussed requirement — the whole point is that it runs locally.

## Open architecture decisions — ask, don't assume

The roadmap says *what* each phase adds, not *how*. The following are real decisions, not obvious
implementation details — if you're about to make one of these for the first time, flag it to the user
(e.g. via `AskUserQuestion`) rather than picking silently, since later phases will build on top of the
choice:

- **Webserver & SSE**: which HTTP framework (or stdlib `net/http`), and how CLI-side events get bridged
  to SSE streams for the browser (Phase 0).
  Also whether the served frontend is a Go-templated app, or a separate JS/TS SPA that is prebuilt and
  embedded via `embed.FS` (Phase 0).
- **MLX integration**: whether training shells out to a Python/MLX subprocess, or goes through cgo/FFI
  bindings, and how progress/stats get reported back to the CLI (Phase 1).
- **Model & dataset storage**: on-disk layout/format for the model registry and datasets so the Models
  and Dataset pages have something concrete to list (Phase 1).
- **Docker orchestration**: which client library, and the contract between an "Environment" definition
  and the container it launches — filesystem mounts, tool exposure, lifecycle (Phase 2).
- **Agent canvas format**: how a node/edge agent graph is serialized and persisted, and the node type
  taxonomy beyond the input/prompt/output/decision nodes the README already names (Phase 3).
- **Evaluation runner**: how assertions are expressed and checked against agent output, and how
  environment starting state is set up per-test (Phase 4).

Once the user decides one of these, record it here (a short "Decided:" note under the relevant bullet)
so it doesn't get re-litigated by a later session.

## Skills

Project-specific skills live in `.claude/skills/`:

- **`new-command`** — scaffold a new Cobra command (+ test) following this repo's conventions. Use
  instead of hand-writing command boilerplate.
- **`new-package`** — scaffold a new `internal/` or `pkg/` package with a test file skeleton.
- **`roadmap-status`** — check the README roadmap checklist against actual repo state, check off
  finished items, and summarize what's next. Use at the start of a session (what's next) and the end
  (did anything just get finished).

## Hooks

`.claude/settings.json` runs `gofmt -w` automatically after any Edit/Write to a `.go` file, and then
`go vet` on the affected package, surfacing vet errors back to the agent immediately rather than at the
next manual test run. This doesn't replace running tests — it just keeps formatting/vet noise out of
diffs and catches obvious mistakes early.

## Definition of done (per change)

- [ ] `task test` passes
- [ ] New/changed behavior has a test (unit and/or integration, per
      [TESTING.instructions.md](.github/instructions/TESTING.instructions.md))
- [ ] README roadmap checkbox updated if a roadmap item was completed
- [ ] Any architecture decision from the list above that you had to make was surfaced to the user first,
      not just picked
