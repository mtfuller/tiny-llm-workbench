# Pre-Release Capability & Polish Audit

_Audit date: 2026-08-28. Scope: CLI, HTTP server, registry, frontend routes, README, tests, CI._

**Verdict:** The feature surface is broad and every roadmap phase (0–4) is built and verified live.
The gaps below are almost entirely cross-cutting release / polish concerns, not missing capabilities.
A handful are genuine blockers.

---

## Resolution status (2026-08-29)

Worked through on branch `pre-release-blockers`:

| # | Item | Status |
|---|------|--------|
| 1 | Placeholder scaffolding commands | ✅ removed (greet/calc/process, pkg/example, their tests) |
| 2 | Server binds to `0.0.0.0` | ✅ defaults to `127.0.0.1`; opt-in `--host` |
| 3 | Stale landing page | ✅ dashboard rebuilt (10 cards, real event feed) |
| 4 | No 404 route | ✅ `NotFound` catch-all added |
| 5 | Working tree / `web/dist` / README | ✅ committed, dist rebuilt, roadmap synced (release tag: still your call) |
| 6 | Platform lock-in undersold | ✅ "macOS on Apple Silicon" now top-line in intro + prerequisites |
| 7 | First-run onboarding | ✅ `tlw serve --open` + empty-registry "Get started" guide on Home |
| 8 | No install path | ✅ README Install section (GitHub Release + Gatekeeper step, `go install`); release workflow builds the binary |
| 9 | No CI | ✅ `ci.yml` (go build/vet/`test -race` + web lint/tsc/build + `web/dist` freshness) and `release.yml` (tag → draft release, unsigned darwin/arm64) |
| 10 | Registry has no locking | ✅ `sync.Mutex` on `*Registry`, non-reentrant, guard test under `-race` |
| 11 | `testcasegen.parsePrompts` fragile | ✅ ported datasetgen's tolerant parser + regression tests |
| 12 | Global SSE stream has no reconnect UX | ✅ explicit backoff + "connection lost" banner + `reconnect()`, verified live |

Green items: `tlw --version` ✅, busy-port hint ✅.

Also fixed: the pre-existing `go test -race ./...` failures — `GetRun`/`ListRuns`/`GetExec`/`Get`/`List`
in the training, benchmarks, evaluations, environments, agents, and deployments managers handed out the
live `*Run`/`*Exec`/`*Session` pointer that a background goroutine mutates under the manager mutex, so
any concurrent reader (a handler marshaling to JSON, a poll loop) raced it. They now return
snapshot copies. CI runs `go test -race ./...` repo-wide.

Not done: `CONTRIBUTING.md`, coverage gate/badge, in-app `~/.tlw` export.

---

## 🔴 Release blockers

### 1. Placeholder scaffolding commands still ship
- `cmd/greet.go`, `cmd/calc.go`, `cmd/process.go` are Cobra-template leftovers.
- They appear in `tlw --help`; `cmd/root.go`'s long help text says outright: _"the commands below
  (greet, calc, process, version) are scaffolding placeholders."_
- `pkg/example` exists only for them.
- 5 of 7 tests in `tests/integration_test.go` (`TestCLIGreet`, `TestCLICalc`, `TestCLIProcess`,
  `TestCLIVerboseFlag`, and half of the version tests) exist only for them.
- **Action:** delete the three commands, `pkg/example`, and the placeholder integration tests. Keep
  `serve`, `version`, and `TestCLIServe`.

### 2. Server binds to `0.0.0.0` with no auth
- `cmd/serve.go` — `net.Listen("tcp", fmt.Sprintf(":%d", servePort))` listens on **every** network
  interface.
- The API runs arbitrary shell commands in Docker containers, shells out to `mlx_lm.*`, reads/writes
  files via tool nodes, and enumerates the host filesystem (`GET /api/fs/list`).
- On any shared / untrusted network, anyone on the LAN can drive the entire API.
- **Action:** default the bind address to `127.0.0.1`; add an opt-in `--host` flag for deliberate LAN
  use. This is a real security issue for a "runs entirely on your machine, private" tool, not just
  polish.

### 3. Stale landing page (`web/src/pages/Home.tsx`)
- Dashboard card links to `/environments` (line ~44) — that route was renamed to `/workspaces` and
  no longer exists (`web/src/App.tsx`).
- Shows only 6 of ~11 top-level sections — missing **Benchmarks, Deployments, Knowledge, Tools,
  Workspaces**.
- Card label still reads "Environments".
- The "live activity log" only subscribes to `heartbeat` (every 5s) — no real activity (training
  progress, agent steps, workspace exec) is surfaced.
- **Action:** rebuild the dashboard against the current nav; drop the dead link; feed it real events.

### 4. No 404 / catch-all route
- `web/src/App.tsx` has no `<Route path="*">`.
- Any unknown URL (including the broken Home link above) renders the app chrome with an empty
  `<Outlet />` — a blank page.
- **Action:** add a catch-all `NotFound` component with a "page not found → go home" message.

### 5. Working tree can't ship as-is
- ~35 uncommitted files (recent feature work: dataset AI-provenance, model-picker modal, benchmark
  review workflow, node preview, agent prompt template).
- `web/dist` is out of sync in the working tree (new untracked `index-*.css` / `.js`, old ones
  deleted but not committed).
- README roadmap doesn't list several shipped features: model-picker browse modal, dataset/benchmark
  "needs review" workflow, agent/prompt node "Preview model", agent prompt template.
- **Action:** commit, `task web:build`, sync the README roadmap, then tag a release.

---

## 🟡 Polish a public user will hit fast

### 6. Platform lock-in is undersold in the README
- `mlx-lm` is Apple-Silicon-only, and it powers **Training _and all inference_** (agents, chat,
  dataset/test-case generation, benchmarks).
- On Linux / Windows / Intel Mac, the app launches and the UI works, but everything model-related
  fails.
- README mentions "(Apple Silicon only)" once, in a sub-bullet.
- **Action:** make "Requires: macOS on Apple Silicon" a top-line requirement.

### 7. First-run onboarding is thin
- `tlw serve` prints a URL. It does not: auto-open the browser, run an upfront "mlx-lm not found /
  Docker daemon unreachable" summary, or greet a brand-new (empty-registry) user with any guidance.
- The Home page with an empty registry just shows zeros.
- **Action:** a "Get started" checklist (add a model → make a dataset → train, _or_ → build an agent)
  and an optional `--open` browser launch.

### 8. No install path other than "clone + build"
- No `go install`, no Homebrew tap, no GitHub release binaries, no `curl | sh`.
- An unsigned macOS binary hits Gatekeeper.
- **Action:** GitHub Releases with a notarized macOS/arm64 binary, and/or a Homebrew tap. At minimum
  document the Gatekeeper workaround.

### 9. No CI
- `.github/` has instruction docs but no `workflows/`.
- Nothing runs `go test ./...`, `npm run lint`, or `tsc` on a PR.
- Nothing fails when `web/dist` wasn't rebuilt after a `web/src` change — a recurring footgun (it is
  stale right now).
- **Action:** a PR workflow (Go tests + lint + tsc + a `web/dist` freshness check) and a release
  workflow.

### 10. Registry has no locking
- The registry is flat files (`internal/registry/`) with no mutex or file lock.
- Two browser tabs — or parallel requests from one tab — editing the same dataset/agent can lose
  writes mid load-mutate-save.
- CLAUDE.md acknowledges this ("don't add UUIDs unless it becomes a complaint"), but public users
  _will_ open multiple tabs.
- **Action:** a per-resource `sync.Mutex` (or a single registry `sync.RWMutex`) around
  load-mutate-save is cheap insurance against corruption.

### 11. `testcasegen.parsePrompts` still has the fragile JSON parser
- `internal/testcasegen/generator.go` still uses `strings.IndexByte('[')` +
  `strings.LastIndexByte(']')` + `json.Unmarshal`.
- Dataset generation (`internal/datasetgen`) was hardened for this (trailing comma / stray bracket /
  per-object salvage); benchmark & evaluation test-case generation was not.
- Same "generate errors out with `invalid character ']'` on Llama 3.2 1B" bug class.
- **Action:** port the `datasetgen/generator.go` hardening.

### 12. Global SSE stream has no reconnect UX
- `web/src/eventStream.tsx` relies on the browser's native `EventSource` retry; on error it flips a
  status value to `'closed'` and shows a dot.
- After a server restart or laptop sleep, long-running training/eval runs won't visibly recover in
  the UI (per-feature poll fallbacks exist, but the stream itself has no banner or explicit
  backoff).
- **Action:** a "connection lost, retrying…" banner and an explicit reconnect.

---

## 🟢 Smaller

- `tlw --version` does not work — only the `version` subcommand. One-line Cobra fix (`rootCmd.Version`
  + `SetVersionTemplate`).
- No `CONTRIBUTING.md` — README points contributors at the 146 KB, agent-oriented `CLAUDE.md`.
- `tlw serve` on a busy port errors (`bind: address already in use`) and exits with no "try `--port`"
  hint or auto-increment.
- No in-app export/backup of `~/.tlw` (a directory, so `tar` works, but nothing surfaced).
- `task coverage` / `coverage.html` exist but there's no coverage gate or badge.

---

## What is NOT a gap

- **Capability coverage.** Datasets, Training, Models (+ architecture / heatmap / token-probability
  viz, wired into `ModelDetail.tsx`), Workspaces, Tools, Knowledge, Agents (cyclic canvas, all node
  types, debugger, preview, editable prompt template), Evaluations (versioned, setup/verify),
  Benchmarks, Deployments — all built and, per CLAUDE.md, verified live against real MLX + Docker.
- **Error messages for missing external deps.** `mlxrunner` gives a clear "not found on PATH — install
  with `pip install mlx-lm`…"; `tlw serve` warns (does not hard-fail) when the Docker daemon is
  unreachable or training history can't load.
- **Graceful shutdown.** `runServer` handles SIGINT/SIGTERM with a 5s drain.
- **Code hygiene.** No `TODO`/`FIXME`/`HACK` markers in the codebase; `LICENSE` (MIT) present;
  `web/dist` committed so `go build`/`go test` work without Node.
