---
name: new-package
description: Scaffold a new internal/ or pkg/ Go package with a test file skeleton, following this repo's layering conventions. Use when adding new business logic that a Cobra command will call into, e.g. "add an internal package for the model registry", "create a pkg for dataset loading".
---

# New internal/pkg package

Add a new package under `internal/` (CLI-internal) or `pkg/` (reusable outside this CLI), matching the
layering in [STYLEGUIDE.instructions.md](../../../.github/instructions/STYLEGUIDE.instructions.md) and
[CLAUDE.md](../../../CLAUDE.md).

## Steps

1. **Decide `internal/` vs `pkg/`.**
   - `internal/` — logic specific to this CLI (config, logging, terminal output, orchestration between
     commands). This is almost always the right choice for new business logic in Phase 0–4 features.
   - `pkg/` — only if the package would make sense imported by a *different* Go module (a standalone
     client library, a format parser someone might reuse). Default to `internal/` unless you have a
     concrete reason to reach for `pkg/`.
2. **Pick a lowercase, single-word package name** matching existing packages (`logger`, `color`,
   `spinner`, `version`). Create `internal/<name>/<name>.go` (or `pkg/<name>/<name>.go`).
3. **File layout inside the package**: package clause → imports (stdlib, then external, then internal)
   → constants → types → `init()` if needed → constructors → methods → helpers. See
   [STYLEGUIDE.instructions.md](../../../.github/instructions/STYLEGUIDE.instructions.md) for the exact
   pattern.
4. **Write `<name>_test.go` in the same package** (white-box, table-driven, using `testify`), per
   [TESTING.instructions.md](../../../.github/instructions/TESTING.instructions.md). Aim for the
   coverage targets there (>80% overall, >85% for business logic).
5. **Wire it up from `cmd/`** — commands should call into this package rather than embedding logic
   themselves (see the `new-command` skill).
6. Run `task test-unit` (or `task coverage`) to confirm the new package is tested and covered before
   moving on.

Don't create a package for a single trivial helper function — put it in the closest existing package
first, and only split it out once there's enough surface area to justify a new one.
