<!-- doc-id: agents -->
# Development rules

[한국어](AGENTS.ko.md)

## Purpose

flowmark is a text database whose grammar is markdown: [polyspec/orm](https://github.com/polyspec/orm)
is the core (the markdown executor, its fourth backend) and this repository is the integration and
application layer — the sync adapters (database, GitHub, Jira), the checklist and changelog engine,
and the local board. The rulings live in `docs/plans/design-plan.md`; the execution tracker in
`docs/plans/execution-checklist.md`.

## Rules

- The user's instructions take precedence. Develop on `main` directly; a branch is
  `{type}/{shortname}-{checklist ID}` and a worktree `{project}-{shortname}-{checklist ID}`, used
  when agents or parallel work need one and removed immediately after their integration.
- `docs/plans/execution-checklist.md` is the only task list; its Korean pair holds the same item
  IDs and states. Run `make documents-check` before changing an item state. One checklist per
  repository; split sub-items rather than growing one giant unit.
- The states are `[ ]` waiting, `[~]` in progress, `[o]` done, `[!]` bypassed with `Cause:` and
  `Retry:` on the item, `[-]` impossible with `Cause:`. `[o]` is terminal: a new issue related to a
  done item spawns a suffixed item (`T8.0.14-1`) instead of reopening it. Parallel work is allowed;
  keep the number of `[~]` items from growing without completed results.
- An item becomes `[o]` in the commit that completes it, after its owning command passes. That
  commit also writes the changelog entry under `## Unreleased` in CHANGELOG.md and CHANGELOG.ko.md.
  Uncommitted changes cover one item only.
- A defect starts as a RED test that reproduces it (or a deterministic case that would), confirmed
  failing for the intended reason; the fix follows; the same case runs to GREEN. The checklist item
  records the observable criterion of correctness.
- English documents are canonical. Every document has a `.ko.md` twin with the same information;
  edit both in the same change, and a Korean-caught fix is ported to the English file before the
  next `make documents-stamp`. No interactive history edits across document commits — corrections
  are new commits with fresh stamps.
- Commit messages are English `type(scope): Subject (#task)`: a subject of at most 50 characters,
  capitalized, imperative, no trailing period; a blank line; a body wrapped near 72 characters
  explaining what changed and why; the types `feat`, `fix`, `docs`, `style`, `refactor`, `test`,
  `chore`. Commit as `min-0-max <max@blue.tools>` without additional author trailers.
- The shared tools are the vendored copy of polyspec/kit: `scripts/kit`, `tests/kit`, `kit.json`
  and `.kit/kit.lock.json` change only in kit and reach this repository with
  `make kit-sync KIT_TAG=<tag>`; `make kit-check` verifies them. A tool that kit lacks or gets
  wrong is reported to kit, never copied or edited here.
- Multi-platform is a hard requirement. No platform tricks: locks follow kit's holder-lock
  precedent, crash safety comes from journal replay, and the simplest implementation that fully
  satisfies the current requirements wins.
- Korean documents write technical terms in English; the context alone is Korean.
