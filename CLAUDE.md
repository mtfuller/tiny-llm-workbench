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
  - **Inference** (Agents' prompt nodes, dataset variation generation, the Models page's chat modal):
    `internal/mlxrunner.Runner` manages a pool of on-demand `mlx_lm.server` subprocesses — one per
    distinct model actually in use, started lazily on first request (a free port via a brief `:0` bind,
    then polled on `/v1/models` until ready), reused across requests, and stopped after 15 minutes idle.
    `Generate(ctx, model, prompt) (string, error)` (the same shape `ollama.Client` used to, so
    `internal/agents` and `internal/datasetgen` needed zero code changes when Ollama was swapped out —
    only what gets injected in `cmd/serve.go` changed) wraps a single implicit user turn; `Chat(ctx,
    model string, messages []ChatMessage) (string, error)` takes an explicit multi-turn history for the
    Models page's chat modal — mlx_lm.server itself is stateless between requests, so the full
    conversation has to be resent every call. Both funnel through the same internal `complete()`, which
    talks to `mlx_lm.server`'s OpenAI-compatible `POST /v1/chat/completions`, capping `max_tokens` at
    `Runner.MaxTokens` (default 256, `mlxrunner.defaultMaxTokens`).

    **Real, confirmed root cause of small models producing repeated-gibberish output** (e.g. the same
    sentence looping for hundreds of tokens): reading the installed `mlx_lm.server` 0.31.3 source
    directly showed two compounding defaults. First, `repetition_penalty` on its request body defaults to
    `0.0` (disabled) unless a caller sets it — TLW now always sends `Runner.RepetitionPenalty` (default
    1.3, `mlxrunner.defaultRepetitionPenalty`) to suppress the token loops small models are prone to
    under low-temperature decoding. Second, and more fundamental: TLW originally hit the plain-text
    `/v1/completions` endpoint, which (confirmed by reading the source) is the *one* endpoint that does
    **not** call the model's own `tokenizer.apply_chat_template` — an Instruct-tuned model fed a raw
    prompt string with no role markers isn't being asked to answer anything, it's just continuing
    whatever text pattern statistically follows. Switching to `/v1/chat/completions` (wrapping the prompt
    as a `{"role": "user", ...}` message) fixed this at the source rather than papering over it with a
    lower token cap. Verified by hand against a real repeated-gibberish case (a plain "What do you like
    to do?" prompt) — the same prompt through the old path degenerated into a looping sentence; through
    the new path it produced a normal, on-topic, non-repeating reply.
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

  **2026-08-27 addendum — workspace revamp: real tools, tool I/O schema, per-environment playground:**
  the user asked for Environments to feel like "a project workspace to build out and construct
  environments" rather than a flat definitions/instances list, plus real (not just descriptive) tools on
  the prebuilt environments and the ability to launch an environment and try a tool interactively. Two
  design decisions were surfaced to the user first (both answered via `AskUserQuestion`, both
  "Recommended" options): tool I/O schema is a **simple typed parameter list**
  (`registry.ToolParameter{Name, Type: string|number|boolean, Description, Required}`), not full JSON
  Schema — consistent with the project's existing preference for deterministic, non-LLM-graded surfaces
  (keyword-match decision nodes, `contains`/`regex` assertions) over anything requiring a model to emit
  complex structured output; and the built-in `web_search` tool hits the **DuckDuckGo Instant Answer API**
  (`https://api.duckduckgo.com/`), chosen specifically for being free and keyless, fitting the "no cloud
  dependency without discussion" project ethos.

  `registry.Environment.Tools` changed from `[]string` (purely descriptive labels) to `[]registry.Tool`
  (`Name, Description, Command, Parameters []ToolParameter`) — a real, runnable definition. `Mount`
  gained `ReadOnly bool`, wired all the way through `internal/docker.Client.Launch`'s
  `mount.Mount{ReadOnly: ...}`. New index-addressed CRUD on the registry (`UpdateConfig`, `AddTool`,
  `UpdateTool`, `DeleteTool`) mirrors the established Benchmark-TestCase/Dataset-Example
  load-mutate-save pattern exactly. The three prebuilt environments (`OfficeWorker`, `SoftwareDev`,
  `WebSearch`) were given real tools: `read_file` (`cat {{path}}`), `write_file`
  (`printf '%s' {{content}} > {{path}}`), `read_directory` (`ls -la {{path}}`), and (WebSearch only)
  `web_search` (`curl -s -G --data-urlencode q={{query}} --data format=json ... https://api.duckduckgo.com/`).

  New `internal/environments/tools.go`: `RenderToolCommand(tool, args)` validates args against the
  tool's declared parameters (required-ness, and type-checking number/boolean values) and substitutes
  them into the command template. Substitution always wraps each value in single quotes (escaping
  embedded single quotes via `'\''`), and — this took some care to get right — **templates must never
  add their own quotes around a placeholder**: a placeholder is either a whole standalone shell word
  (`cat {{path}}`) or a literal prefix sharing the same shell word with no space (`q={{query}}`), since
  adjacent quoted/unquoted parts of one POSIX shell word concatenate correctly on their own. An earlier
  draft of `web_search`'s command wrapped the placeholder in its own double quotes
  (`--data-urlencode "q={{query}}"`) and was caught and fixed before ever running — nesting a
  single-quoted substituted value inside an already-double-quoted template string doesn't compose.
  `Manager.TryTool(instanceID, tool, args)` renders and runs the command via the existing `StartExec`
  (same async, eventbus-streamed mechanism the plain ad hoc exec box already used), so trying a tool in
  the UI looks and behaves identically to running a raw command, just with arguments collected as a form.
  This is deliberately separate from `RunToolSync` (the Agent "tool" node's blocking mechanism, driven by
  a single unnamed `{{input}}` placeholder) — the two were not merged; an Agent's tool node still doesn't
  invoke a named `registry.Tool` at all, out of scope for this change.

  New routes: `GET /api/environments/{name}`, `PUT /api/environments/{name}/config`,
  `POST /api/environments/{name}/tools`, `PUT|DELETE /api/environments/{name}/tools/{index}`,
  `POST /api/environments/{name}/tools/{index}/try`. `POST /api/environments` now creates an
  environment with no tools — matching the Benchmarks/Datasets precedent of starting a resource empty
  and adding to it afterward from its own page, rather than cramming tool definitions into the create
  form.

  The frontend gained a new per-environment workspace page (`EnvironmentDetail.tsx`, routed at
  `/environments/:name`) with three tabs: **Configuration** (edit image + mounts, each mount with a
  read-only checkbox), **Tools** (list/add/edit/delete tools — `panel-flush`/toolbar/pagination matching
  the Datasets/Benchmarks list convention, a modal with a parameter editor extracted into
  `ToolEditor.tsx`'s `ToolParameterFields`, mirroring how `TestCaseEditor.tsx`'s `AssertionFields` was
  extracted), and **Playground** (launch/stop instances of this environment, pick one + a tool, render a
  form from the tool's declared parameters — text/number input or a true/false/unset select for
  boolean — and run it, showing live output via the same `environment.exec.output`/`environment.exec.status`
  SSE subscription the old inline exec box used). The list page (`Environments.tsx`) was simplified to
  just Launch/Delete actions plus links into each environment's/instance's workspace; the old inline
  "Run a command" exec box was removed from it (that capability now lives in the Playground tab, scoped
  to tool-shaped commands rather than an arbitrary free-text one — Playground doesn't expose the "give it
  a raw command line" affordance the old box had).

  **A real bug found via live browser + Docker verification, not just unit tests:** the Playground's own
  tool-try flow raced the SSE "done" event ahead of the `tryTool` HTTP response the UI seeds `activeExec`
  from — for a fast-finishing tool (most of them; `cat`/`printf`/`ls` inside a container complete in
  milliseconds), the "done" event could arrive and be dropped (no matching `activeExec.id` yet) before
  the UI ever started listening for it, leaving the status stuck at "running" forever with no further
  event to correct it. This is the exact class of bug already on record (see
  `feedback_sse_needs_poll_fallback` in memory) applied here for the first time to Environments. Fixed
  the same way Training/Evaluations/Benchmarks do: a `useEffect` polls `getExec` every second while
  `activeExec.status === 'running'`, reconciling regardless of whether the SSE event was seen. Verified
  live end-to-end against real Docker containers (not mocked): launched a real `WebSearch` instance, ran
  the real `web_search` tool and got a real DuckDuckGo API JSON response back from inside the container;
  ran `read_file` against a path containing a space (`/tmp/my file.txt`) and confirmed the rendered
  command was correctly quoted (`cat '/tmp/my file.txt'`) and returned the exact file contents — first
  reproducing the stuck-at-"running" race live, then confirming the poll-fallback fix resolved it.

  **2026-08-27 addendum — global Tool catalog, independent Knowledge bases, dedicated nav section:** the
  user asked for a new top-level "Environments" nav section (Environments, Knowledge, Tools) so a user has
  "a space to easily design flexible and powerful environments that their agents can interact with" —
  tools as a shared catalog rather than something defined per-environment, plus a new Knowledge concept:
  queryable records an agent can pull from. Four real design forks were surfaced via `AskUserQuestion`
  (all "Recommended" options chosen): **Tool catalog model** is a live reference, not a copy — an
  Environment names which catalog tools it makes available, so editing a tool in the catalog changes it
  everywhere it's attached (the same reference-by-name relationship Training already has with
  Models/Datasets); **file structure scope** stays exactly what it already was — the Environment's
  existing host↔container mount configuration, just repositioned under the new nav section, no new
  capability; **Knowledge query mechanism** is deterministic keyword/substring matching, consistent with
  the project's established avoidance of embeddings/vector search/second ML runtimes (the same reasoning
  as keyword-match Decision nodes and `contains`/`regex` assertions); **Knowledge binding** is independent
  of Environment — a KnowledgeBase needs no Docker container, so an Agent's new "knowledge" node type
  queries one directly with no environment/instance involved at all.

  `registry.Tool` moved out of `Environment.Tools` (which was `[]Tool`) into its own top-level registry
  resource (`tools/<name>/definition.json`, alongside Models/Datasets/Environments/Agents/Evaluations/
  Benchmarks) with its own `SaveTool`/`GetTool`/`DeleteTool`/`ListTools`, a `Prebuilt bool` field, and
  `EnsurePrebuiltTools` (the same idempotent-seed pattern as `EnsurePrebuiltEnvironments`, and called
  before it in `cmd/serve.go` since the prebuilt Environments now reference prebuilt Tools by name).
  `Environment.Tools` became `[]string` (name references only); `AttachTool`/`DetachTool` replaced the old
  index-addressed `AddTool`/`UpdateTool`/`DeleteTool` — `AttachTool` is idempotent and validates the name
  resolves against the catalog before appending, `DetachTool` only removes the reference and never touches
  the catalog entry. Deleting a catalog tool does **not** cascade to environments that reference it — a
  dangling name is silently skipped wherever tools are resolved (`agents.Manager.SendMessage`'s
  environment-tools lookup, `EnvironmentDetail.tsx`'s attached-tools list), the same graceful-degradation
  UX already established for "tool removed from environment" before this change existed at all.

  New `registry.KnowledgeBase{Name, Description, Records []KnowledgeRecord, CreatedAt}` at
  `knowledge/<name>/definition.json`, with `KnowledgeRecord{ID, Title, Content, Tags}` and the same
  `AddRecords`/`UpdateRecord`/`DeleteRecord` index-addressed CRUD shape as Dataset examples and Benchmark
  test cases (`AddRecords` always assigns a fresh server-side ID, ignoring any client-supplied one). New
  `internal/knowledge` package: `Query(kb, query string) []KnowledgeRecord` lowercases both sides and
  requires every whitespace-separated query term to appear somewhere in `Title + " " + Content` — an
  order-independent all-terms match (query "python tutorial" matches a record containing "tutorial for
  python"), not a literal-phrase substring match; `FormatResults` renders matches as `"Title: Content"`
  joined by blank lines, or a literal "No matching records found." string.

  A fifth Agent node type, "knowledge", was added alongside input/prompt/decision/tool/output:
  `registry.NodeData` gained `KnowledgeBaseName` and `KnowledgeQuery` (templated; empty falls back to the
  previous node's raw output, the same convention as `PromptTemplate`/`MatchTemplate`). `agents.Engine`
  gained a `knowledgeReader` interface (`GetKnowledgeBase(name) (registry.KnowledgeBase, error)`) and a
  `"knowledge"` case that renders the query template, calls `knowledge.Query`+`FormatResults`, and stores
  the result under the node's Name in the same `runContext` every other node type uses — so a Knowledge
  node's output is referenceable downstream as `{{NodeName}}` with zero special-casing needed in
  `runcontext.go`, the cross-node templating mechanism built earlier in this project. `NewEngine` grew to
  3 params (`llm, tools, kb`) and `agents.Manager` gained a `toolReader` dependency and grew to 8 params
  (`ctx, agentsReader, llm, envs, envReader, toolStore, kb, bus`) — both mechanical signature-growth
  additions following this codebase's established constructor-param convention rather than introducing an
  options-struct pattern for the first time here.

  New routes: `/api/tools` (list/create) and `/api/tools/{name}` (get/update/delete) for the catalog;
  `/api/knowledge` (list/create) and `/api/knowledge/{name}` (get/delete) plus `/api/knowledge/{name}/records`
  (add) and `/api/knowledge/{name}/records/{index}` (update/delete) for KnowledgeBases; environment tool
  routes changed from index-addressed (`PUT/DELETE /api/environments/{name}/tools/{index}`) to
  name-addressed (`POST /api/environments/{name}/tools` with `{toolName}` body to attach, `DELETE
  /api/environments/{name}/tools/{toolName}` to detach, `POST .../tools/{toolName}/try` to run).

  The frontend gained a new top-level "Environments" nav section (Environments, Knowledge, Tools),
  replacing Environments' old spot under "Automation" (which now holds just Agents/Evaluations): a new
  `Tools.tsx` list page for the global catalog (reusing `ToolEditor.tsx`'s `ToolParameterFields`/
  `DraftTool`/`toDraftTool` unchanged from when they were built for the old per-environment tool editor);
  new `Knowledge.tsx` (list) and `KnowledgeDetail.tsx` (per-base record CRUD, mirroring
  `DatasetDetail.tsx`'s examples pattern — search, tag filter via `FilterMenu`, pagination) pages;
  `EnvironmentDetail.tsx`'s Tools tab rewritten to show attached tools (resolved by joining
  `environment.tools: string[]` against a fetched catalog) with a "+" opening an "Attach tool" modal
  (a `<select>` of catalog tools not yet attached, with a link to the Tools page to create a new one) and
  a detach (unlink icon) action per row instead of the old edit/delete; `AgentEditor.tsx` gained a
  "Knowledge" palette entry (a new, previously-unused violet `#7c5cbf`, distinct from every other node
  type's color) with inspector fields for knowledge-base selection and a templated query field using the
  existing `VariableMenuButton` insert-variable picker.

  Fully verified live end-to-end: attached a newly-created catalog tool to the real `WebSearch`
  environment and ran it against a real launched Docker container from the Playground tab (`DONE (EXIT
  0)` / `hello World`); built a real `faq` KnowledgeBase with a "Refunds" record, then an Input →
  Knowledge → Prompt → Output agent (`{{Knowledge 1}}` referenced in the Prompt node's template) and ran a
  real chat turn against `mlx-community/Qwen2.5-0.5B-Instruct-4bit` over `mlx_lm.server` — confirmed via
  the Run view's live step log that the Knowledge node correctly matched the "Refunds" record for the
  message "refunds" and the Prompt node's template correctly substituted its content, producing a real,
  on-topic, refund-policy-accurate reply — not a mock.
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

  **2026-08-27 addendum — structured Tool node picker, settings modal, redesigned inspector:** the user
  asked for the Tool node to let you "select a tool" rather than type a raw shell command, for Agent
  settings to move out of the always-visible right sidebar into a settings button + modal on the left
  workspace sidebar, and for the node inspector to look and feel like a professional configuration panel.
  Three design decisions were surfaced via `AskUserQuestion` (all "Recommended" options): the list view
  gained Environment/Created columns (matching Evaluations' pattern), the settings modal holds
  Environment + a new free-text `registry.Agent.Description` field, and — the big one — the Tool node
  became a **structured picker**, not a cosmetic-only restyle or a "insert template into a raw field"
  hybrid.

  `registry.NodeData.Command` (a raw shell string + `{{input}}` templating) was removed entirely and
  replaced with `ToolName string`, `ToolArgs map[string]string`, and `ToolInputParam string` — the same
  simple-typed-parameter-list schema `registry.Tool` already uses for the Environments workspace's
  Playground tab. A tool node now names one of the bound Environment's real tools; `ToolArgs` holds a
  literal value per parameter; `ToolInputParam`, if set, names the *one* parameter that instead receives
  the previous node's output at run time (the UI enforces "at most one bound parameter" with a radio
  button per parameter, labeled "Use previous output"). `internal/agents.Engine.Run` gained a
  `tools []registry.Tool` parameter (the bound Environment's declared tools) and its tool-node case now
  resolves `ToolName` against that list, merges `ToolArgs` with `{ToolInputParam: previousOutput}`, and
  renders the command via `environments.RenderToolCommand` — the exact same validating/quoting function
  the Environments Playground already uses — before calling `RunToolSync`, instead of a bare
  `strings.ReplaceAll(command, "{{input}}", output)`. `agents.Manager` gained a new `environmentReader`
  dependency (`GetEnvironment(name) (registry.Environment, error)`, satisfied by `registry.Registry`
  itself, wired in `cmd/serve.go`) so `SendMessage` can look up the bound Environment's tools before
  calling `Engine.Run` — `internal/agents` importing `internal/environments` for `RenderToolCommand`
  introduced no cycle (environments doesn't know about agents). This was a clean cutover with no
  backward-compat shim for old `Command`-shaped tool nodes — acceptable at this stage since there are no
  real users' saved agents to migrate, only local dev/test data.

  While adding the Created column, the same pre-existing "never actually sets CreatedAt" bug already
  found and fixed in Benchmarks/Evaluations was found in `saveAgentHandler`/`SaveAgent` too and fixed the
  same way: `SaveAgent` now preserves an existing agent's `CreatedAt` on overwrite and sets it to now on
  first save, rather than leaving it perpetually zero.

  The frontend's `AgentEditor.tsx` right sidebar was rebuilt: a node-type-colored header bar (reusing the
  exact same border colors as the canvas's own `.flow-node-*` classes, so the inspector visually matches
  whatever's selected on the canvas), a scrollable body, and a pinned-to-bottom "Delete node" footer,
  replacing the old flat "h3 + raw fields" layout. The always-visible "Agent settings" block (Environment
  select) was removed from the inspector entirely; a gear-icon "Agent settings" button was added to the
  bottom of the left node palette instead, opening an `AgentSettingsModal` (Environment + Description) —
  changes there are held in the same component state the main Save button already persists, so nothing
  new to wire up for saving. Fully verified live end-to-end against a real Docker container and the real
  DuckDuckGo API: built an Input → Tool(`web_search`, `query` bound to previous output) → Output agent
  bound to the `WebSearch` environment, ran a real chat turn, and confirmed the tool node correctly
  substituted the user's message into the `query` parameter, executed inside the real launched container,
  and returned genuine API JSON to the output node — the exact case a raw-`{{input}}`-string design would
  have handled anyway, but now going through the same validated, safely-quoted rendering path everywhere
  a tool runs in the app (Environments Playground, Agent tool nodes) instead of two divergent mechanisms.

  **2026-08-27 addendum — cross-node templating and JSON-schema-constrained node output:** the user asked
  for two related capabilities: referencing an upstream node's output inside a downstream node's config
  (e.g. a Prompt node's text containing `"please solve user problem: {{output}}"`), and forcing one node's
  output to comply with a JSON Schema so downstream nodes can pull specific properties out of it. Before
  building anything, the installed `mlx_lm.server` (0.31.3) source was checked directly for a
  `response_format`/`json_schema`/grammar-constrained-decoding request parameter (the OpenAI-API
  equivalent) — **it has none** (confirmed by grepping `server.py`; its only per-request
  generation-shaping knobs are `repetition_penalty` and `top_logprobs`, and `logits_processors` are
  fixed server-side args, not something a request can supply). So "forcing" a schema can only ever mean
  best-effort prompting plus after-the-fact parsing/validation — the same conclusion already reached for
  `testcasegen`'s JSON generation. Four real design forks were surfaced via `AskUserQuestion` (all
  "Recommended" options chosen): named references reachable from **any** upstream node (not just the
  immediate predecessor), fail-the-turn-immediately on schema validation failure (no retry, no soft-fail),
  templating on Prompt + Tool + Decision nodes, and an inspector "insert variable" picker UI.

  `registry.Node` — really `registry.NodeData` — gained `Name string`, replacing `Label` for every node
  type (a clean cutover, not additive): it's both the canvas display title and the token a downstream
  node's template references (`{{Name}}` for raw text, `{{Name.property}}` for a JSON property). Prompt
  nodes gained `PromptTemplate` (templated user-turn text; empty falls back to the previous node's raw
  output verbatim, preserving old behavior with zero migration needed) and `OutputSchema` (a JSON Schema
  the reply must validate against). Decision nodes gained `MatchTemplate` (templated text to keyword-match
  against; same empty-falls-back-to-previous-output convention). Tool nodes' `ToolInputParam` (the old
  "bind exactly one parameter to the previous output" mechanism) was removed entirely — every `ToolArgs`
  value can now itself contain a `{{...}}` template, which strictly subsumes what `ToolInputParam` did.

  New `internal/agents/runcontext.go`: a `runContext` accumulates a `{raw, parsed}` result per node
  **Name** (not ID) as `Engine.Run` walks the graph — since the engine only ever walks one path per turn
  (a Decision node's branching still follows a single edge), this map is naturally exactly the set of
  nodes that ran earlier in the turn, so no separate graph-reachability analysis is needed; a reference to
  a node that hasn't run yet (a typo, or a node on the branch not taken) is simply absent and reported as
  a clear error. `render(template)` finds every `{{...}}` block, splits its inner text on the first `.`
  (deliberately not a single combined regex — Go's RE2 doesn't backtrack the way a name-with-spaces
  followed by an optional path group would need), and resolves the name against the context, then
  optionally walks a dot-separated path into that node's parsed JSON value (only present if it declared an
  `OutputSchema`) via `lookupJSONPath`; `stringifyJSONValue` renders the result back to plain text
  (`json.Number.String()`, `strconv.FormatBool`, or a compact JSON re-marshal for a nested object/array).
  `internal/assertions.checkJSONSchema` (the existing `json_schema` assertion type) was refactored into a
  thin wrapper around a newly-exported `ValidateJSONSchema`, which returns the parsed value instead of
  just pass/fail — `internal/agents` reuses this exact function for a Prompt node's `OutputSchema` check,
  and `internal/environments.RenderToolCommand` for a Tool node's arg substitution+quoting, so a template
  reference and a tool-parameter value go through the same already-tested paths rather than new ones.
  `Engine.Run` gained node-name-uniqueness validation up front (two nodes sharing a Name is a template
  ambiguity, reported before any node runs, not discovered mid-turn).

  The frontend's `TemplateField.tsx` is new: `upstreamVariableOptions(nodes, edges, nodeId)` walks edges
  backward from a node to find every named ancestor (a real BFS over the graph, not "just the immediate
  predecessor"), offering `{{Name}}` for each plus `{{Name.property}}` for every top-level key an
  ancestor Prompt node's `OutputSchema` declares (parsed client-side with a try/catch — a malformed schema
  just contributes no property options, it doesn't break the picker); `insertAtCursor` splices a chosen
  snippet into a field at its actual cursor position (via `selectionStart`/`selectionEnd`) rather than
  just appending to the end, and `VariableMenuButton` is a small popover (mirroring
  `TagFilterDropdown`'s portal-to-`document.body` pattern) that lists the options and calls back into it.
  **A real positioning bug found live, not just in code review:** the popover's trigger button sits at the
  right edge of the (narrow, fixed-width) inspector sidebar, and the popover was originally positioned
  `left: <button's left edge>` — for a button already near the right edge of the viewport, the popover's
  own width pushed its right side off-screen, clipping every option. Fixed by anchoring from the right
  instead (`right: window.innerWidth - button.right`), which is always on-screen by construction since
  it's computed from a coordinate the visible button itself occupies.

  While building the Created-date column for this session's earlier Agents list-view work, `saveAgentHandler`
  turned out to have a variant of the same "response doesn't reflect what was actually persisted" bug
  already fixed elsewhere: it echoed back its own local `registry.Agent` struct — built straight from the
  request, `CreatedAt` never set — instead of what `SaveAgent` had actually stamped (Go passes structs by
  value, so `SaveAgent`'s own `agent.CreatedAt = ...` mutates its private copy, not the caller's). The
  persisted file was always correct (confirmed via a direct `GET` returning the real timestamp); only the
  immediate `POST` response was wrong, and nothing in the frontend happened to read that field from it —
  still fixed the same way `createEnvironmentHandler` already does it: re-read via `GetAgent` after
  saving, and return that instead.

  Fully verified live against a real local MLX model (`mlx-community/Qwen2.5-0.5B-Instruct-4bit` via
  `mlx_lm.server`): built Input → `Classifier` (Prompt, `OutputSchema` requiring a `city` string) →
  `Responder` (Prompt, `PromptTemplate: "please solve user problem: recommend one food to try in
  {{Classifier.city}}"`) → Output, sent "I want to visit Paris next month.", and confirmed via the Run
  view's live step log that Classifier's real reply `{"city": "Paris"}` validated against the schema and
  Responder's actual prompt had `{{Classifier.city}}` correctly resolved to "Paris" (the final reply was
  specifically about Parisian food, not a generic answer) — the core cross-node-reference and
  schema-property-access mechanism working end-to-end against a real model's real output, not a mock.
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

  **2026-08-27 addendum — richer assertion types, shared checker:** the assertion set grew beyond
  `contains`/`not_contains`/`regex` to also cover `json_schema` (validate that the reply contains a JSON
  value conforming to a user-supplied JSON Schema) and `similarity` (pass if the reply is at least a
  given normalized-Levenshtein similarity ratio — `registry.Assertion.Threshold`, in (0, 1] — to a
  reference text; a "close diff" check, not exact match). The user wanted both Evaluations and Benchmarks
  to have the same richer set, which changed the earlier duplication tradeoff (see the **Benchmarks**
  bullet below): the checker was extracted into a new `internal/assertions` package (`Check`/`CheckAll`),
  and both `internal/evaluations` and `internal/benchmarks` import it instead of keeping their own
  copies. `json_schema` validation uses `github.com/santhosh-tekuri/jsonschema/v6` (new dependency, no
  transitive deps, chosen over hand-rolling JSON Schema semantics) — compiled fresh per check, since
  usage is one-off per test case, not a hot loop worth caching. Because tiny local models often preface a
  JSON reply with commentary or wrap it in a markdown code fence, `json_schema` doesn't require the whole
  reply to be JSON: `extractJSONValue` scans for the first balanced `{...}`/`[...]` substring (tracking
  string-literal state so braces inside a JSON string don't throw off the balance count) and validates
  that. `similarity` lowercases both sides before comparing, matching `contains`'s existing
  case-insensitivity reasoning. The frontend's `TestCaseFields` (`web/src/TestCaseEditor.tsx`) grew a
  multi-line textarea for `json_schema` (schema text is rarely one line) and a small numeric threshold
  input for `similarity`, auto-defaulted to 0.85 the moment that type is selected so the field is never
  silently invalid; a new `formatAssertion` helper renders a one-line human-readable summary
  (`similar to "..." (≥ 85%)`, `matches JSON schema`) everywhere assertions are displayed read-only.
  Verified live: created a benchmark with one `similarity` and one `json_schema` assertion, ran it against
  the real fused Qwen2 model, and confirmed both failed for the right, precisely-surfaced reasons against
  a real generated reply ("That's one wrong!" is neither similar to the reference text nor contains any
  JSON — the UI showed the exact `reply does not contain a JSON value` error from `internal/assertions`).

  **2026-08-27 addendum — Evaluations rebuilt to mirror Benchmarks, plus real setup/verify scenarios:**
  the user asked for Evaluations to become "like Benchmarks, but slightly different" — every test case
  gets setup work to prepare the environment before the agent's turn, and assertions checked at the end —
  so evaluators can build real software-dev/knowledge-work/office-work scenarios and verify the agent
  actually completed them, not just what it said. Three real forks were surfaced via `AskUserQuestion`
  first (all "Recommended" chosen): **setup is a list of literal shell commands** run in sequence (reusing
  `RunToolSync`, the exact mechanism Tool nodes/the Playground already use) rather than a structured
  catalog-tool-call list; **each test case gets its own fresh instance per agent** (not one shared instance
  for the whole run) so one scenario's file changes can never leak into another's; and **assertions can
  check environment state**, not just the agent's reply — a test case can declare `VerifyCommands` (a
  shell command run after the agent's turn, checked with the same assertion types against its own output).
  A fourth, deeper fork emerged only once the design got concrete: for setup/verify to mean anything they
  have to run in the *same* container the agent's own Tool nodes act in during its turn — asked again, and
  the user chose to **keep Evaluation's own separate `Environment` field** (rather than just reusing
  whichever Environment the agent itself happens to be bound to), which meant `agents.Manager` needed a way
  to run a turn inside an instance somebody else already launched.

  `registry.TestCase` (shared with Benchmarks) gained `Setup []string` and `VerifyCommands []VerifyStep`
  (`VerifyStep{Command string, Assertions []Assertion}`), both `omitempty` and left unset by Benchmarks —
  there's no Environment to prepare or verify when testing a model directly. A `VerifyStep`'s command exit
  code is deliberately *not* itself pass/fail (a command like `grep` legitimately exits non-zero for "not
  found") — only its assertions against the captured output decide pass/fail, keeping the same
  deterministic-assertion philosophy as everywhere else.

  `registry.Evaluation` was rebuilt to mirror `registry.Benchmark`'s draft/published-version split exactly
  (`Version int`, `EvaluationVersion` immutable snapshots, `PublishEvaluationVersion`/
  `ListEvaluationVersions`/`GetEvaluationVersion`, `AddEvaluationTestCases`/`UpdateEvaluationTestCase`/
  `DeleteEvaluationTestCase`) — named distinctly from Benchmark's identically-shaped methods only because
  both types share the `*Registry` receiver in the same package, so `PublishVersion` etc. can't be declared
  twice. `Environment` stays a live setting on the mutable `Evaluation`, not part of a published version's
  snapshot — the same reasoning as an Agent's own `Environment` binding not being part of its graph.

  `agents.Manager` gained `StartRunInInstance(agentName, instanceID string) (*Run, error)` — starts a chat
  run reusing an already-launched instance instead of launching its own, for exactly this case. `Run`
  gained an unexported `ownsInstance bool` (never serialized) so `StopRun` only calls `envs.Stop` for a run
  that actually launched its own instance via `StartRun`; a `StartRunInInstance` run's instance is left
  alone since the caller (Evaluations) owns its lifecycle. This is the one piece of this feature that isn't
  a mechanical Benchmarks-mirror — a small, deliberate widening of `agents.Manager`'s contract to support a
  caller-supplied instance.

  `internal/evaluations.Manager.runTestCase` is the new core: per (agent, test case), it launches a fresh
  instance from the evaluation's `Environment` (skipped entirely if unset — a test case that declares
  setup/verify but has no Environment configured fails immediately with a clear misconfiguration error
  rather than silently skipping them), runs `Setup` commands in it (a failed setup command fails the whole
  test case without ever starting the agent — a broken starting scenario can't produce a trustworthy
  result), starts the agent's turn via `StartRunInInstance` against that same instance so its Tool nodes
  act on exactly what Setup prepared, checks the reply against `Assertions` as before, then runs
  `VerifyCommands` against the same instance and checks each one's own assertions, before stopping the
  instance and the agent run. Durable per-agent results now persist the same way Benchmarks' do
  (`RunResult` keyed by `(EvaluationVersion, AgentName)`, upserted to `<registry
  root>/evaluation-results/<name>.json`) — the ephemeral `Run`/`ListRuns`/`GetRun` API is now purely
  progress-tracking, mirroring Benchmarks' split between live-run and durable-result data exactly.

  New routes mirror Benchmarks' 1:1: `POST .../test-cases`, `PUT/DELETE .../test-cases/{index}`,
  `POST .../test-cases/generate`, `POST .../versions`, `GET /api/evaluation-versions/{name}` (a sibling of
  `/api/evaluations/{name}`, same ServeMux-ambiguity reasoning as `/api/benchmark-versions`), and
  `GET /api/evaluation-results/{name}` (ditto, sibling of `/api/evaluations/{name}` rather than nested
  under `/api/evaluations/{name}/results`, which would collide with `/api/evaluations/runs/{id}`). One new
  route Benchmarks has no equivalent of: `PUT /api/evaluations/{name}/config`, updating only the
  `Environment` binding — a live setting, so it never touches `TestCases`/`Version`/`CreatedAt`.

  The frontend's `TestCaseEditor.tsx` lost its old bulk multi-test-case editor (`TestCaseFields`/
  `DraftTestCase`/`emptyTestCase`/`toDraftTestCases`/`toPayloadTestCases`) — Evaluations moved to the same
  per-test-case add/edit/delete modal pattern Benchmarks already uses (`AssertionFields` reused directly,
  no wrapper), so the bulk editor had no remaining callers. It gained `VerifyStepFields` (a new component
  nesting `AssertionFields` one level deeper, mirroring how a test case's own assertions nest, but one
  step per verify command) plus `DraftVerifyStep`/`toDraftVerifySteps`/`toPayloadVerifySteps`. `Setup`
  itself needed no shared component — it's a single plain multi-line textarea (one shell command per line)
  local to `EvaluationDetail.tsx`'s test case modal. `EvaluationDetail.tsx` and `Evaluations.tsx` were
  otherwise rebuilt to mirror `BenchmarkDetail.tsx`/`Benchmarks.tsx` structurally (versioned info card with
  a publish button, Test cases/Run results tabs, sortable results table with the same
  pass@1/assertion-rate/error-rate/avg-latency metrics computed client-side — extended here to also count
  verify-step assertions, not just reply assertions), plus the Environment-binding edit control Benchmarks
  has no equivalent of, and a result detail flow one level deeper than Benchmarks' (clicking a test case
  card opens its own modal showing the reply, its assertions, and every verify step's command/output/
  assertions, since a single inline card no longer has room for all of that).

  Fully verified live end-to-end against a real Docker container — no mocks: bound a `file-writer` agent
  (Environment `SoftwareDev`, graph Input → Tool `write_file` with `content: "{{Input}}"` → Output) to a
  new evaluation also configured with `Environment: SoftwareDev`; the test case's prompt was "hello from
  setup test", `Setup: ["mkdir -p /repo"]`, and one `VerifyCommand` (`cat /repo/output.txt`, asserting
  `contains "hello from setup test"`), with zero reply assertions since `write_file` itself produces no
  stdout. Published v1 and ran it against `file-writer`: the run succeeded, and the persisted result showed
  the verify step's actual captured output — `"hello from setup test"` — reading back exactly what the
  agent's Tool node had written into the *same* container Setup had just prepared, with the assertion
  correctly passing. This is the core, previously-unbuilt claim of the whole feature (setup → agent action
  → verify all sharing one real instance) confirmed against real infrastructure, not test doubles.
- **Model visualization tools**: how the Models detail page inspects a model's actual weight file and
  probes its behavior — architecture topology, a per-tensor weight heatmap, and a token-probability
  ("confidence") tool.
  **Decided:** a design doc for this proposed reading `.safetensors` files entirely client-side, using
  the browser's `File.slice()` API against a user-picked file — deliberately not followed here. TLW
  already knows a registry model's on-disk path server-side (`registry.Model.Path`), so there's nothing
  to gain from asking the user to re-locate a file the app already has, and reading exact byte ranges via
  `os.File.ReadAt` achieves the same "don't load the whole file" goal from the backend instead. New
  `internal/safetensors` package: `ParseModelDir` reads only the 8-byte length prefix + JSON header of a
  model's `.safetensors` file(s) (handling both a single file and a sharded
  `model.safetensors.index.json` + multiple shard files) to locate every tensor's byte range without
  reading any weight data; `DeriveArchitecture` turns that into layer count / hidden size / vocab size /
  parameter count (preferring `config.json`'s own fields when present, falling back to regex-matching
  `\.layers\.(\d+)\.` against tensor names — sorted numerically, not alphabetically, so layer 10 doesn't
  sort before layer 2); `ExtractHeatmap` reads one tensor's exact bytes via a single `ReadAt`, decodes
  F32/F16/BF16 (hand-verified float16↔float32 bit math, including subnormals), and subsamples down to a
  fixed grid (default 200×200) plus min/max/mean/std computed over every element. Verified by hand
  against a real fused/dequantized TLW-trained model (Qwen2 0.5B, F16, 494M params) — the derived
  parameter count and estimated byte size matched that model's own `model.safetensors.index.json`
  metadata exactly. New `GET /api/models/{name}/architecture` and `GET /api/models/{name}/heatmap`
  endpoints call this package directly (no mockable interface — it's pure filesystem I/O, tested with
  real hand-built `.safetensors` files in `internal/safetensors`'s own tests, the same way
  `internal/registry` tests real files rather than mocking `os`).
  For token probabilities: `mlxrunner.Runner` gained `TokenProbabilities`, wrapping the same
  `/v1/chat/completions` endpoint `Chat`/`Generate` use with `top_logprobs`/`logprobs` request fields —
  confirmed by reading mlx_lm.server's source (0.31.3) that both endpoints share the same
  logprobs-computing code path, and that a response entry's top-level `id`/`token`/`logprob` is always
  the single most-likely candidate (not necessarily the token actually generated, though the two are the
  same whenever decoding is greedy — mlx_lm.server's own default and one TLW never overrides), with the
  full top-N list available under `top_logprobs`. New `POST /api/models/{name}/token-probabilities`
  endpoint clamps `maxTokens`/`topLogprobs` server-side (50 / 10 — mlx_lm.server's own hard cap is 11)
  regardless of what the frontend requests, so a page reload or a modified client request can't spawn a
  runaway generation. The heatmap's color scale intentionally uses the app's own `--ok`/`--accent` theme
  tokens (green/negative ↔ terracotta/positive) rather than a generic scientific blue-red scale, read via
  `getComputedStyle` at render time so it adapts to light/dark mode automatically.

- **Benchmarks**: how a test suite is defined and run when the target is a raw model rather than an
  agent, and how results get compared across models.
  **Decided:** Benchmarks is Evaluations' sibling, not something built on top of it — same shape of test
  case (a prompt plus assertions, reusing `registry.TestCase`/`registry.Assertion` directly, no new type
  needed there). Assertion-checking itself originally lived duplicated in
  `internal/evaluations/assertions.go` and `internal/benchmarks/assertions.go` (deliberately — the two
  features are conceptually independent, one checks agent replies, the other model replies, and the
  original `contains`/`not_contains`/`regex` checker was small enough that duplicating it beat a
  cross-package dependency). That call was reversed once the user asked for the same richer assertion set
  in both places — see the **Evaluation runner** bullet above for the resulting shared
  `internal/assertions` package; duplicating an increasingly non-trivial checker that must now behave
  identically in both features stopped being the cheaper option. `registry.Benchmark`
  (`benchmarks/<name>/definition.json`) mirrors `registry.Evaluation` minus the `Environment` field —
  there's no agent-run lifecycle or environment to launch, so a benchmark run
  (`internal/benchmarks.Manager`, mirroring `internal/evaluations.Manager`'s async
  StartRun/ListRuns/GetRun/eventbus-progress shape) sends each test case's prompt straight to
  `mlxrunner.Runner.Generate(ctx, model.Path, prompt)` for each selected registry model — one independent
  generation per test case, no chat history, no agent or environment overhead. A model with no local
  `Path` (or an unresolvable name) fails every one of its test cases with a recorded per-result error
  rather than failing the whole run, so a bad model selection doesn't hide the results for the other
  selected models in the comparison. Routes/handlers/frontend (`Benchmarks`/`BenchmarkDetail` pages,
  reusing `AssertionFields` from `TestCaseEditor.tsx` — already target-agnostic) directly mirror
  Evaluations' equivalents, including the results table shape (rows =
  target, columns = test cases, a score column) since that layout already reads as a side-by-side model
  comparison without changes. Fully verified live: created a real benchmark, ran it against the real
  fused Qwen2 model over a real `mlx_lm.server` subprocess, and got back its actual generated reply
  (checked against the `contains "hello"` assertion, correctly failing on a real "Good morning!" reply).
  The sidebar also gained two `sidebar-nav-label` section headings at this point — "Workbench" (Models,
  Datasets, Training) and "Automation" (Environments, Agents, Evaluations, Benchmarks) — grouping what
  had been one flat nav list; `.sidebar-nav-label`/`.sidebar-nav-section` already existed in `index.css`
  (plural, evidently anticipating this) but were unused until now.

  **2026-08-27 addendum — versioning, durable per-model results, redesigned detail page:**
  `registry.Benchmark` gained a `Version int` field, computed inside `SaveBenchmark` itself (not taken
  from the caller): a first save gets Version 1, and a later save only bumps it if `TestCases` actually
  changed (`reflect.DeepEqual` against the stored definition) — a no-op re-save (open the editor, save
  without changing anything) leaves the version untouched, so it's a meaningful "this test suite changed"
  signal rather than an edit-count. `SaveBenchmark` also now sets/preserves `CreatedAt` itself for the
  same reason (this uncovered a real pre-existing bug: neither `saveBenchmarkHandler` nor
  `saveEvaluationHandler` ever actually set `CreatedAt` — it always serialized as the zero time. Fixed
  here for Benchmarks since it's directly load-bearing for versioning's "first save vs. later save"
  branch; the identical bug in Evaluations is untouched, out of scope for this change).

  Run results changed from "one ephemeral, in-memory `Run` covering N models, shown once and then gone on
  restart" to a durable, queryable comparison: `internal/benchmarks.Manager` gained a `resultsDir`
  (`NewManager`'s new final parameter; wired in `cmd/serve.go` to `<registry root>/benchmark-results`) and
  persists one `RunResult` per (BenchmarkVersion, ModelName) to a `<resultsDir>/<benchmarkName>.json` file
  — `persistResult` upserts by that key (a matching existing entry is replaced, not appended), so
  re-running the same benchmark version against the same model overwrites its previous result exactly as
  asked, while a different version or a different model gets its own row. This is a Manager-owned
  persistence directory, not something routed through `registry` — the natural `registry.Benchmark`-owning
  location would be a results.json file colocated with `definition.json`, but `RunResult` embeds
  `internal/assertions.Result` (via `benchmarks.TestCaseResult`), and `internal/assertions` already
  imports `registry` for `registry.Assertion`, so `registry` importing `internal/benchmarks`/
  `internal/assertions` back would cycle. Mirrors `internal/training.Manager`'s existing
  own-directory-under-registry-root persistence pattern instead. The ephemeral `Run`/`ListRuns`/`GetRun`
  API is unchanged in spirit and kept — it's now purely a progress-tracking mechanism (is a run active,
  live per-test-case progress over the eventbus) since the durable comparison data lives in
  `ListResults`/`GET /api/benchmark-results/{name}` instead. That route is a sibling of
  `/api/benchmarks/{name}`, not nested under it as `/api/benchmarks/{name}/results` — Go's `net/http`
  `ServeMux` refused that shape as genuinely ambiguous against the already-existing
  `GET /api/benchmarks/runs/{id}` (both are 2-segment GET patterns with the wildcard in a different
  position; `/api/benchmarks/runs/results` would satisfy either).

  The benchmark detail page was restructured into two views per the user's request: a "Test cases" tab
  (the test suite, editable) and a "Run results" tab (every persisted `RunResult` across every version and
  model ever tried, sortable by model/version/pass rate/duration/started-at — defaulting to pass rate
  descending, which is the actual point: "see all the models you've tested and sort on more performant").
  A result whose version doesn't match the benchmark's current version is visually flagged "(outdated)"
  rather than hidden — deliberately not filtered out, so edit history stays visible, but clearly marked so
  a stale comparison isn't mistaken for a fair one. Both tabs' list chrome (search input, `panel-flush` +
  `panel-toolbar`, empty/loading states, `Pagination`) was matched to the existing Datasets/Evaluations
  list-page convention per explicit user request, rather than the plainer unpaginated tables the page
  first shipped with.

  A further pass moved the page's always-visible "Run against" card (model checklist + Run button) out of
  the layout entirely: the first card is now plain "Benchmark info" (version, test case count, created
  date — general info about the resource, nothing actionable), and starting a run is a `Plus` icon-button
  in the **Run results** tab's own list-toolbar (matching the "+" pattern every other list page already
  uses for its create action), opening a `RunBenchmarkModal` with the model checklist. The icon-button
  disables itself (not the modal's submit button) while a run is active, since the point is to stop a
  second run from being started at all, not to let the modal open and then block submission.

  **2026-08-27 addendum — publish/immutable-version model, standard metrics, pagination everywhere:**
  the earlier "Version auto-increments whenever TestCases changes" design was replaced entirely: a
  benchmark's `TestCases` is now explicitly a *draft* (freely add/edit/delete/generate, never affects
  `Version`), and `Version` only ever changes via a new, explicit `PublishVersion` call, which snapshots
  the current draft into an immutable `registry.BenchmarkVersion` (`versions.json`, alongside
  `definition.json`) and is the *only* thing a run can target — `internal/benchmarks.Manager.StartRun`
  now takes an explicit `version int` and calls `GetVersion(name, version)`, not `GetBenchmark`. This
  guarantees a run's results always trace back to an exact, unchanging test suite: editing the draft
  after publishing has zero effect on any already-published version (verified live — published v1 with
  "Say hello.", edited the draft to "Say hello. Please.", ran v1 again, and the result modal showed the
  real model reply against the original unedited "Say hello." prompt). `SaveBenchmark` was simplified to
  just preserve `Version`/`CreatedAt` on every draft save, full stop — no more diffing. New routes:
  `POST /api/benchmarks/{name}/versions` (publish) and `GET /api/benchmark-versions/{name}` (list, a
  sibling of `/api/benchmarks/{name}` for the same ServeMux-ambiguity reason as `/api/benchmark-results`).
  The frontend gained a `Tag`-icon "Publish version" button in the Test cases tab toolbar (disabled once
  the draft is empty), a banner on the info card when the draft differs from the latest published version
  (or none has ever been published), and the Run modal gained a version `<select>` defaulting to latest.

  Benchmark results also gained standard-style metrics, computed entirely client-side from each
  `RunResult`'s existing per-test-case data — no backend/persisted-shape change, so historical results get
  them for free: **Pass@1** (identical to the existing pass rate by definition, since each test case runs
  exactly once — k=1 pass@k *is* the pass rate, not an approximation of it; a real, distinct pass@k would
  need actual multi-sample generation, deliberately out of scope per explicit user decision), **assertion
  rate** (fraction of individual assertions passed across all test cases — finer-grained than whole-test-
  case pass/fail), **error rate** (fraction of test cases that couldn't even get a reply), and **avg
  latency** (the run's own wall-clock duration ÷ test case count — exact, not approximate, since test
  cases run strictly sequentially, confirmed by reading `internal/benchmarks.Manager.run`). The results
  table's "Duration" column was replaced by "Avg latency"; total run duration moved into the per-result
  detail modal's new metrics summary instead. The table gained a `.table-scroll` (`overflow-x: auto`)
  wrapper since `.panel-flush`'s own `overflow: hidden` would otherwise just clip the extra columns.

  Per explicit user request ("all list views need pagination"), every list on the site was audited:
  `EvaluationDetail.tsx` was the one remaining gap (its test cases table, agent-comparison results table,
  and run history list had none) and got the same `usePagination`/`Pagination` treatment every other list
  page already uses. Live/streaming feeds (Home's live activity log, a running agent's chat/step log, a
  training run's live event log) were deliberately treated as exempt — they're bounded live tails of an
  event stream, not a browsable historical resource list, so pagination doesn't apply to them.

  **2026-08-27 addendum — per-test-case CRUD + generation, mirroring Datasets exactly:** creating a
  benchmark no longer requires any test cases upfront (`saveBenchmarkHandler`'s "at least one test case"
  validation was removed) — `Benchmarks.tsx`'s create modal is now just a name field, and submitting
  navigates straight to the new benchmark's (empty) detail page, same reasoning as a Dataset starting
  empty. Test cases are managed the same way Dataset examples are: `registry.TestCase` gained `Tags
  []string`, and three new registry methods (`AddTestCases`, `UpdateTestCase`, `DeleteTestCase` — index-
  addressed, mirroring `AppendExamples`/`UpdateExample`/`DeleteExample`) each load-mutate-save through the
  existing `SaveBenchmark`, so version-bump detection keeps working for free rather than needing its own
  copy of the diffing logic. `AddTestCases` always assigns a fresh server-side ID
  (`tc-<unixnano>-<i>`), ignoring any ID the client sent; `UpdateTestCase` preserves the existing ID at
  that index. New routes: `POST .../test-cases`, `PUT .../test-cases/{index}`,
  `DELETE .../test-cases/{index}`.

  A new `internal/testcasegen` package mirrors `internal/datasetgen` for "generate test case prompt
  variations" (`POST .../test-cases/generate`), but deliberately only varies the **prompt** — a
  generated variation reuses the seed's own assertions/tags unchanged, rather than asking the model to
  also invent new pass/fail criteria. This isn't a shortcut; tiny local models are unreliable at
  emitting structured/parseable output (the same reasoning already behind Phase 3's keyword-match
  decision nodes and the deterministic assertion types), so having the model generate JSON Schema
  documents or regex patterns it can't validate itself isn't a fight worth having — paraphrasing a
  prompt while assertions stay fixed is what "test the same thing, asked differently" actually means.
  Confirmed live that this caution is warranted: asked for 2 variations with an explicit "respond with
  ONLY a JSON array of strings" instruction, and Qwen2.5-0.5B-Instruct returned a JSON *object* with
  typo'd keys instead (`{"promt1": "...", "promt2": "..."}`) — correctly rejected by
  `testcasegen.parsePrompts` with a clear "no JSON array found" error rather than silently accepting
  garbage, and that error surfaced legibly in the UI. The feature's plumbing (validation, the real
  mlx_lm.server round trip, and clean error propagation all the way to the modal) is fully verified;
  getting a specific tiny model to reliably nail the exact output format on every attempt is a separate,
  unbounded problem this project already declines to fight elsewhere.

  The frontend reused rather than duplicated: `AssertionFields` was extracted out of `TestCaseFields`
  (the multi-test-case bulk editor, still used by Evaluations) into its own component so a
  single-test-case modal could reuse the same assertion-row UI without the "Test case N / Remove"
  wrapper; `toPayloadAssertions`/`toDraftAssertions` were exported alongside it. `TagFilterDropdown` was
  extracted from `DatasetDetail.tsx` (where it was previously a page-local component) into its own
  `web/src/TagFilterDropdown.tsx` so `BenchmarkDetail.tsx` could use the identical tag-filter popover
  rather than a second copy — this was a pure extraction, no behavior change to Datasets.

  **Not fixed here (flagged separately):** both the new `GenerateTestCasesModal` and the pre-existing
  Dataset `GenerateVariationsModal` populate their model suggestion dropdown from registry models'
  `.name`, but `mlxrunner.Runner` needs a trained model's `.path` (no name→path resolution exists inside
  it) — typing a registry model's display name into either modal makes `mlx_lm.server` hang (observed
  live: ~0% CPU, no progress, had to be killed manually) rather than failing fast, because it treats the
  name as an unresolvable Hugging Face repo id. This is a pre-existing gap in the already-shipped Dataset
  feature that the new Benchmarks feature simply inherited by mirroring it — worth fixing in both places
  together, not in scope for this change.

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
