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
  **Decided:** stdlib `net/http` (go.mod bumped to 1.22 for method-matching `ServeMux` patterns like
  `"GET /api/events"`), no router library. Frontend is a React + TypeScript SPA built with Vite under
  `web/`, built to `web/dist` and embedded via `web/embed.go` (`//go:embed all:dist`); `web/dist` is
  committed to git so `go build`/`go test` work without Node installed — run `task web:build` after
  changing `web/src` and commit the result. CLI events are bridged to SSE via an in-process pub/sub
  broker (`internal/eventbus`), since the webserver runs in the same process as the CLI command that
  starts it (`tlw serve`); there is no cross-process bridge to design. `internal/server` wires the
  bus's `/api/events` endpoint and the SPA handler (with an index.html fallback for client-side
  routing) together.
- **MLX integration**: whether training shells out to a Python/MLX subprocess, or goes through cgo/FFI
  bindings, and how progress/stats get reported back to the CLI (Phase 1). Later expanded to cover model
  *inference* too, once Ollama (the original inference backend) turned out to be a dead end for anything
  MLX-trained (see below).
  **Decided:** mlx-lm is TLW's only model runtime, for both training and running models — no Python
  involved on our side at all, and (as of the Ollama removal below) no second inference engine either.
  - **Training**: `internal/training.SubprocessTrainer` execs `mlx_lm.lora` directly (resolved via PATH;
    overridable via `Command`, mainly for tests) and regex-parses its textual stdout for
    `Iter N: Train/Val loss ...` progress lines in Go (`subprocess_trainer.go`), republishing them on
    `internal/eventbus` (`training.progress`/`training.status`). A successful run's LoRA adapter isn't
    runnable on its own (see below), so `Manager` (`manager.go`) immediately fuses it into the base
    model via `SubprocessTrainer.Fuse` (`mlx_lm.fuse --dequantize`, writing to `registry.ModelDir(name)`)
    and only registers *that* as the model — a fuse failure fails the whole run, since a run that can't
    produce something runnable didn't really succeed. Runs persist to `<registry root>/runs/<id>.json`
    and survive a `tlw serve` restart.
  - **Inference** (Agents' prompt nodes, dataset variation generation): `internal/mlxrunner.Runner`
    manages a pool of on-demand `mlx_lm.server` subprocesses — one per distinct model actually in use,
    started lazily on first request (a free port via a brief `:0` bind, then polled on `/v1/models`
    until ready), reused across requests, and stopped after 15 minutes idle. It implements the same
    `Generate(ctx, model, prompt) (string, error)` shape `ollama.Client` used to, so `internal/agents`
    and `internal/datasetgen` needed zero code changes — only what gets injected in `cmd/serve.go`
    changed. Talks to `mlx_lm.server`'s OpenAI-compatible `POST /v1/completions`, capping `max_tokens`
    at `Runner.MaxTokens` (default 256, `mlxrunner.defaultMaxTokens`) — confirmed by hand that
    `mlx_lm.server`'s own default (512, no repetition penalty engaged by default) routinely rambled into
    long repetitive text for a short chat/dataset-gen prompt.
  - The "model" string throughout is either a Hugging Face MLX repo id (e.g.
    `mlx-community/Qwen2.5-0.5B-Instruct-4bit`, downloaded automatically on first use) or a registry
    model's `Path` (a local, standalone, fused model directory) — both load the same way, so the
    frontend doesn't need to distinguish them; model pickers are a free-text input + `<datalist>`
    combobox seeded with registry models, not a restrictive `<select>`.

  **A real upstream bug drove the fuse-not-adapter design**: `mlx_lm.server`'s `--adapter-path` flag
  (and the request-body `"adapters"` override) is broken in the installed version (0.31.3, confirmed by
  hand) — a base-model-path-remapping bug means the CLI-supplied adapter is silently never applied, so
  the server just serves the un-fine-tuned base with no error. Fusing the adapter into a standalone model
  first (via `mlx_lm.fuse`) sidesteps the buggy code path entirely. Two more things confirmed by hand
  while building this fuse step, both load-bearing: `mlx_lm.fuse` needs `--dequantize` when the base
  model is quantized, or it's a **silent no-op** (the fused model comes out behaviorally identical to
  the un-fine-tuned base, no error/warning at all); and re-quantizing the dequantized result afterward
  (to save disk space) can **wash out the fine-tuning signal** — verified against a real overfit adapter
  where re-quantizing to 4-bit lost the trained behavior entirely. So fused models are deliberately left
  dequantized (~3-4x the size of the original quantized base) rather than trying to shrink them back
  down — reliability over disk space, and these are small models anyway.

  **This went through three earlier shapes before landing here** — worth knowing if this ever needs
  revisiting, since the same mistake (defaulting to a Python subprocess, or a second runtime, when a
  direct CLI exec would do) happened more than once: (1) originally training was a Python subprocess
  running a bundled `train.py` that itself drove `python -m mlx_lm.lora` and re-emitted JSON-lines
  progress, reasoned as "MLX's actively maintained surface is its Python API" — true of the *library*,
  irrelevant once the integration point is a CLI command; (2) after a real user hit
  `No module named 'mlx_lm'` because their `python3` resolved to Xcode Command Line Tools' stub
  interpreter, `train.py` was changed to invoke `mlx_lm.lora` as a standalone command instead of
  `-m mlx_lm.lora`, plus a `TLW_PYTHON` env var — then, once `train.py` was CLI-only, it wasn't doing
  anything Go's `os/exec`/`bufio.Scanner`/`regexp` couldn't do directly, so `train.py`, `embed.go`, and
  `TLW_PYTHON` were all removed; (3) inference was originally Ollama (`internal/ollama.Client`,
  `internal/models.Catalog` merging it with the registry) — reasonable for chat/dataset-gen against
  small general-purpose models, until the user asked directly whether an MLX-trained model could be run
  through Ollama. Verified by hand it can't, on two independent levels: a LoRA adapter alone isn't a
  full model (`ollama create` fails with `open config.json: no such file or directory`), and even a full
  MLX-format model fails GGUF conversion (`unknown data type: U32` — MLX's quantization packing isn't a
  layout Ollama's converter recognizes). That made Ollama a dead end for the product's actual point
  (train here, then run what you trained), so it — `internal/ollama`, `internal/models` — was removed
  entirely in favor of `internal/mlxrunner`. **Don't reintroduce Ollama, or any Python layer, for either
  training or inference** unless something genuinely needs a capability mlx-lm's own CLI/server can't
  provide — every prior addition of a second runtime or language layer here turned out to be avoidable.

  **Verified 2026-08-26** against a real `mlx-lm` install (Homebrew, Apple M1) — the full happy path
  (`tlw serve` → `POST /api/training/runs` → `SubprocessTrainer` → real `mlx_lm.lora` → regex-parsed
  progress → `internal/eventbus` → `registry.SaveModel`) was driven end-to-end multiple times (including
  after the Go-only rewrite) and produced real LoRA adapters registered as models. That first real run
  caught a real regex bug: the train-loss pattern folded `Tokens/sec`/`Peak mem` into the same pattern as
  trailing *optional* groups separated by non-greedy `.*?` — which regex can (and did) satisfy by
  matching zero characters, so the match always succeeded but silently returned nothing for both fields
  even when the real log line had them. Fixed by matching `Tokens/sec`/`Peak mem` with their own
  independent regexes instead of folding them into one pattern (see the comment above `trainLineRe` in
  `subprocess_trainer.go`, and `TestParseProgressLineTrainLossWithTokensAndMem`). No unverified-happy-path
  gap remains for this integration.

  **A second real bug, found via a real user's dataset (2026-08-26):** training with 12 examples failed
  with `Dataset must have at least batch_size=4 examples but only has 3` — `mlx_lm.lora`'s own default
  `--batch-size` (4) doesn't adapt to dataset size, and `SubprocessTrainer`'s 80/20 train/valid split
  (`splitExamples`) routinely produces a split smaller than 4 for datasets as small as 1-3 or anywhere
  in the 5-15 range — a wide, realistic swath of exactly the tiny custom datasets this tool is for. Fixed
  by always passing `--batch-size` explicitly, computed as `batchSizeFor(len(train), len(valid))` — the
  largest size (capped at the default 4) both splits can actually support. Verified for real: the exact
  reported 12-example case, plus 3 and 5 (both previously broken too), now train successfully end-to-end.
  See `TestBatchSizeForShrinksToSmallestSplit` for the regression guard.
- **Model & dataset storage**: on-disk layout/format for the model registry and datasets so the Models
  and Dataset pages have something concrete to list (Phase 1).
  **Decided:** a plain directory registry, no database. Root directory defaults to `~/.tlw` (overridable
  via the `TLW_HOME` env var, mainly for tests) with `models/<name>/metadata.json` (plus the model's own
  files alongside it — for a trained model, the fused output of `mlx_lm.fuse`, not a raw adapter) and
  `datasets/<name>/metadata.json` + `data.jsonl`. Since the Ollama removal (see the MLX integration
  entry above), the registry is the *only* model source — no external catalog to merge with. See
  `internal/registry`.
- **Docker orchestration**: which client library, and the contract between an "Environment" definition
  and the container it launches — filesystem mounts, tool exposure, lifecycle (Phase 2).
  **Decided:** the official Docker SDK for Go (`github.com/docker/docker/client`) talking to the local
  Docker daemon — not shelling out to the `docker` CLI. An Environment definition
  (`internal/registry.Environment`: name, image, host↔container mounts, a plain descriptive `tools`
  list) lives in the registry like Models/Datasets. "Tools" was originally just descriptive metadata;
  it's now wired up (see the Agent canvas format entry below) — an Agent can be bound to an Environment,
  and its canvas can include a "tool" node that runs a literal shell command inside that Environment's
  running instance via `environments.Manager.RunToolSync`. Launching an Environment starts a real
  labeled container (tracked instance, not just the definition); instances also support exec-into with
  output streamed live over the eventbus/SSE (same pattern as Phase 1's training progress), since a
  quick way to verify "does this environment actually work" was worth the extra surface area. Instance
  state on `tlw serve` restart is reconciled by listing Docker containers with TLW's management label
  (Docker is the source of truth), not by persisting our own run-history JSON like Phase 1's training
  runs — the container already *is* the durable record. See `internal/docker` and
  `internal/environments`.
- **Agent canvas format**: how a node/edge agent graph is serialized and persisted, and the node type
  taxonomy beyond the input/prompt/output/decision nodes the README already names (Phase 3).
  **Decided:** the canvas UI is built with React Flow (`@xyflow/react`) rather than a hand-rolled
  drag/pan/zoom implementation. The graph itself (`registry.Agent`: name + `Graph{Nodes, Edges}`) is
  stored in the registry like Models/Datasets/Environments, at `agents/<name>/definition.json`. Node
  taxonomy has a fifth type beyond the README's original four (input, prompt, decision, output): a
  "tool" node, added to wire Phase 3 Agents up to Phase 2 Environments. An Agent optionally names one
  Environment (`registry.Agent.Environment`); starting a chat run launches an instance of it
  (`agents.Manager.StartRun` → `environments.Manager.Launch`), and a "tool" node's `Data.Command` (with
  a `{{input}}` placeholder templated to the previous node's output) is run inside that instance via a
  new synchronous `RunToolSync` method — deliberately separate from the existing async
  `StartExec`/`GetExec`/eventbus-streaming path used by the Environments page, because the agent
  engine's `Run` call is itself synchronous end-to-end and needs the command's output before it can
  continue walking the graph. This is a deterministic, literal-shell-command mechanism, not an
  LLM-driven tool-calling loop — consistent with Decision nodes' keyword matching and Evaluations'
  assertion checks, and chosen because tiny local models are unreliable at emitting
  structured/parseable tool-call syntax. The run's Environment instance is stopped when the chat ends
  (`agents.Manager.StopRun`, called by the frontend's chat modal `onClose`) — idempotent, and using a
  fresh `context.Background()` rather than the manager's own context so cleanup survives server
  shutdown. A Decision node picks its branch via a simple case-insensitive
  substring/keyword match against the previous node's output (not an LLM call) — deterministic and
  testable, at the cost of being a fairly blunt instrument; the node has one keyword and two outgoing
  edges distinguished by `sourceHandle` ("yes"/"no"). Running an agent is a synchronous chat turn
  (`internal/agents.Manager.SendMessage` walks the graph start-to-finish and returns the reply directly)
  rather than fire-and-forget-plus-poll like Phase 1/2's long-running jobs — a turn is only a couple of
  local model calls, fast enough to just await (each capped at `mlxrunner.defaultMaxTokens` (256) to
  bound latency — see the MLX integration entry above). Step-by-step execution is still published on the
  eventbus (`agent.step`) for the Run view's live event log even though the caller doesn't need to poll
  for the final result. Chat run history is in-memory only (not persisted to disk), same reasoning as
  Phase 2's execs. See `internal/agents`.
  **Note:** `web/package.json` pins `@xyflow/react` to the exact version `12.10.2` (no `^` range) —
  `12.11.4` is a genuinely broken publish (its bundled `@xyflow/system@0.0.80` is missing an export
  `@xyflow/react`'s own code references, breaking the production build). Don't let a routine
  `npm update`/`npm install @xyflow/react@latest` drift onto it; check the release notes/changelog for a
  fix before upgrading past 12.10.2.
- **Evaluation runner**: how assertions are expressed and checked against agent output, and how
  environment starting state is set up per-test (Phase 4).
  **Decided:** assertions are deterministic rules — `contains` / `not_contains` / `regex` — checked
  directly against an agent's reply text, no LLM grading (consistent with Phase 3's keyword-match
  decision nodes). An `registry.Evaluation` (`evaluations/<name>/definition.json`) holds a list of test
  cases (prompt + assertions) and an optional Environment name. Running an evaluation
  (`internal/evaluations.Manager`) is async like Phase 1/2's long jobs (POST returns immediately with a
  "running" status; progress publishes on the eventbus, `evaluation.progress`/`evaluation.status`), not
  synchronous like Phase 3's single agent chat turn — a full run can be many agents × many test cases ×
  LLM calls, too slow to block an HTTP request on. If the evaluation names an Environment, the runner
  launches a real instance for the run's duration and stops it after — but **agents cannot act on it
  yet** (Phase 3 agents have no way to invoke Environment tools), so this only exercises the Phase 2
  launch/stop plumbing and gives "starting environment state" a real hook for later, not present
  functionality. Each test case runs as a fresh `agents.Manager` chat run (`StartRun` + one
  `SendMessage`) per agent, so agents don't share conversation history across test cases within a run.
  See `internal/evaluations`.

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
