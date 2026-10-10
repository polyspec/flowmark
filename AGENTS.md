<!-- doc-id: agents -->
# Development rules

[한국어](AGENTS.ko.md)

## Purpose

flowmark is a text database whose grammar is markdown: this repository is the engine and its thin
faces, the sync adapters (database, GitHub, Jira) and the application schema;
[polyspec/orm](https://github.com/polyspec/orm) is the query layer that reaches it by the DSN
`markdown://`. The rulings live in `docs/plans/design-plan.md` and the plans beside it; each plan
has its checklist in `docs/plans/`.

## Principles

- Everything here is the responsibility of whoever works on it. A rule is followed because it is
  right, never because it already exists: an existing rule, code path or document that is wrong
  is reported and corrected, and an oddity found while reading or changing code is reported at
  once with the question whether a rule must be made or corrected.
- Rules first. Development proceeds by stating logical rules and keeping them; behaviour that
  follows no stated rule is a defect. The measure of every decision is what is right — never that
  an existing principle, rule or precedent says so. A criterion is not eternal: it is changed when
  the situation shows it wrong, and only with its grounding stated. It is never lowered because it
  is hard to meet while testing — that is a betrayal of the criterion.
- A judgement that is hard to make, or a new line of reasoning, is verified adversarially by an
  agent, which is asked for its solutions as well as its objections; the rounds repeat until no
  disagreement remains. Its scope is fixed before the first round: the propositions under review,
  stated as written, and what lies outside them. A finding counts only when it shows a proposition
  wrong, contradicted by another document, or missing a case it claims to cover; a new question
  next to the scope becomes its own checklist item, never a further round. Without a fixed scope
  the rounds do not converge.
- No backward compatibility: an old path is removed, never kept behind a compatibility layer, a
  fallback or a migration shim.
- Errors are never hidden: no fallback that turns a wrong input into a plausible one, no silent
  drop of an input or an output, no swallowed error.
- No tricks, in any case. When a requirement cannot be met the orthodox way and only a trick
  would meet it, that is stated exactly instead of done quietly.
- The simplest implementation that fully satisfies the current requirements wins; no
  abstraction, setting or indirection for an uncertain future. Components are modular with
  their concerns clearly separated, and architecture is decided for the long term — a stopgap
  that must be replaced later is not accepted. Correct first; fast only when it stays correct.
  A problem is not solved by creating another problem.
- A wrong change is judged by harm and by correctness, not by whether it matched the intent: a
  harmful change is reverted at once; a harmless change that is wrong or unneeded is removed;
  a correct change stays, and when it is unrelated to the task it is committed on its own with
  its real reason — a record that presents an unrelated change as related is false.
- A question to the user must be needed: not already decided or evident, not asked by habit,
  never confusing, and never a way to drop something; a needed question is never left out. A
  decision that a new milestone reopens is asked again.
- A task includes its documents, AGENTS and the memory.
- A settled ruling is written down in its final form: the plan and the specifications state it,
  with its grounding, in the change that settles it. A ruling that lives only in a conversation,
  a checklist item, a commit message or an agent's report is not settled, and work that depends
  on it waits until it is written.

## Rules

- The user's instructions take precedence. Develop on `main` directly; a branch is
  `{type}/{shortname}-{checklist ID}` and a worktree `{project}-{shortname}-{checklist ID}`, used
  when agents or parallel work need one and deleted as soon as they merge into main — a merged
  branch or worktree left behind takes space, fragments the folders and confuses which branches
  are merged.
- Test code that cannot merge into main is cherry-picked when it is worth keeping and otherwise
  discarded, always before the commit. When it cannot be removed at once, a checklist sub-item
  states the condition of its removal.
- An instruction is first classified: a checklist item (work), the memory (what only the owner
  and the agent need), AGENTS (a working guideline, kept here without duplication and never in
  the checklist or the changelog), or a plain answer. Work is prioritized, the documents and the
  checklist are updated, then the work runs. The current work is finished first; it is stopped
  for a new instruction only on the owner's explicit request or a stated judgement that the new
  work must come first — otherwise the instruction is added and taken in priority order.
- A checklist is one file per plan: the work of one plan lives in one checklist file, and a
  different plan gets its own file beside its plan in `docs/plans/` — `execution-checklist.md`
  for the design plan, `<plan>-checklist.md` for any other. Every checklist file is a tracker of
  `config/checklist.json`, its Korean pair holds the same item IDs and states, and item IDs are
  unique across the files. Run `make documents-check` before changing an item state. Each
  repository runs its own checklists: work in another repository is tracked there and referenced
  here by its ID in a code span. Split sub-items rather than growing one giant unit.
- A problem becomes a new checklist item; when it relates to an existing item it is that item's
  sub-item by ID.
- The states are `[ ]` waiting, `[~]` in progress, `[o]` done, `[!]` bypassed with `Cause:` and
  `Retry:` on the item, `[-]` impossible with `Cause:`. `[o]` is terminal: a new issue related to a
  done item spawns a suffixed item (`T8.0.14-1`) instead of reopening it. Parallel work is allowed;
  keep the number of `[~]` items from growing without completed results: independent items may
  run together, but starting new items while the started ones stay partial is refused —
  completion comes before starting, so `[o]` keeps growing faster than `[~]`.
- `[!]` is used only when the next item cannot proceed without it, with an explicit reason; a
  bypass is not a completion. The item records its cause and its retry condition, and work
  resumes without waiting for permission once the retry condition holds. An audit covers only
  the `[!]` items, their causes and their retry conditions — never a rerun of unrelated tests.
- An item becomes `[o]` in the commit that completes it, after its owning command passes. That
  commit also writes the changelog entry under `## Unreleased` in CHANGELOG.md and CHANGELOG.ko.md.
  Uncommitted changes cover one item only; an unfinished item is never a reason to widen them by
  adding another. Each feature is reviewed, self-tested and recorded before its commit.
- A defect starts as a RED test that reproduces it (or a deterministic case that would), confirmed
  failing for the intended reason; the fix follows; the same case runs to GREEN. The checklist item
  records the observable criterion of correctness.
- The engine is a database, so its unit tests are dense: every grammar and constraint rule has at least one accepting and one rejecting case, table-driven, with its edge cases; every storage has P1/P2 golden fixtures covering every column type and both the NULL and the empty forms; parse and emit are fuzzed (`go test -fuzz`) with the golden fixtures as seeds, and a failing input the fuzzer finds stays as a regression case. An item that adds a rule is not done until these cases exist.
- English documents are canonical. Every document has a `.ko.md` twin with the same information;
  edit both in the same change, and a Korean-caught fix is ported to the English file before the
  next `make documents-stamp`. A correction is a new commit with fresh stamps.
- Commit messages are English `type(scope): Subject (#task)`: a subject of at most 50 characters,
  capitalized, imperative, no trailing period; a blank line; a body wrapped near 72 characters
  explaining what changed and why, not how; an optional footer after a blank line for references;
  the types `feat`, `fix`, `docs`, `style`, `refactor`, `test`,
  `chore`. Commit as `min-median-max <max@blue.tools>` without additional author trailers.
- The shared tools are the vendored copy of polyspec/kit: `scripts/kit`, `tests/kit`, `kit.json`
  and `.kit/kit.lock.json` change only in kit and reach this repository with
  `make kit-sync KIT_TAG=<tag>`; `make kit-check` verifies them. A tool that kit lacks or gets
  wrong is reported to kit, never copied or edited here.
- Multi-platform is a hard requirement. No platform tricks: locks follow kit's holder-lock
  precedent, crash safety comes from journal replay, and the simplest implementation that fully
  satisfies the current requirements wins.
- Korean documents write technical terms in English; the context alone is Korean. Code
  comments and commit messages are written in English.
