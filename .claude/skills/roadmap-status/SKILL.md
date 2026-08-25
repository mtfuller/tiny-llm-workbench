---
name: roadmap-status
description: Check the README.md roadmap checklist against actual repo state, check off items that are genuinely finished, and summarize what to work on next. Use at the start of a tiny-llm-workbench session to pick up work, and at the end of one to record what got finished. Triggers on "what's next", "update the roadmap", "what's left", "is Phase X done".
---

# Roadmap status

[README.md](../../../README.md) has a checkbox roadmap (Phase 0–4) that is the source of truth for what
this project has actually built, per [CLAUDE.md](../../../CLAUDE.md). Because development spans many
independent agent sessions, this checklist only stays useful if it's kept honest — don't trust it
blindly, and don't leave it stale.

## Checking status (start of session)

1. Read the roadmap section of `README.md`.
2. For each unchecked item in the current or earlier phases, spot-check whether it's actually still
   unbuilt (search the repo — code may have landed without the checkbox being updated, or vice versa).
   Don't assume the checklist is accurate; verify against real files/tests.
3. Report: which phase is the active one, which items in it are genuinely done vs. not, and what the
   next unstarted item is. Flag any checked box that doesn't match reality so it can be corrected.
4. If the next item touches one of the open architecture decisions listed in `CLAUDE.md`, surface that
   decision to the user before starting rather than assuming an answer.

## Updating status (end of session / after finishing work)

1. Only check off an item once it's actually done: it builds, it's tested, and it works end to end (per
   the Definition of Done in `CLAUDE.md`) — not "mostly written."
2. Edit the checkbox in `README.md`'s Roadmap section (`- [ ]` → `- [x]`). Don't reword the roadmap
   items themselves unless the scope genuinely changed — keep diffs to the checkbox.
3. If a decision from `CLAUDE.md`'s "Open architecture decisions" list was made during this session, add
   a short "Decided: ..." note under that bullet in `CLAUDE.md` so it isn't re-litigated later.
4. If new sub-work was discovered that isn't reflected in the roadmap (e.g. a phase turned out to need
   an item the README doesn't list), mention it to the user rather than silently editing the roadmap's
   scope.
