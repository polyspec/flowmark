<!-- doc-id: agents -->
# Development rules

[한국어](AGENTS.ko.md)

## Purpose

flowmark is a text database whose grammar is markdown: this repository is the engine and its thin
faces, the sync adapters (database, GitHub, Jira) and the application schema;
[polyspec/orm](https://github.com/polyspec/orm) is the query layer that reaches it by the DSN
`markdown://`. The rulings live in `docs/plans/flowmark/plan.md` and the plans beside it; each plan
has its checklist in `docs/plans/`.

## Principles

The working principles of every polyspec repository are [PRINCIPLES.md](PRINCIPLES.md), which kit
vendors here byte for byte; they bind all work in this repository. This file states only
flowmark's own rules: the ones below refine a principle for the engine, and the section after
them holds the rules of this repository.

- Events, not polling, as the engine applies it: a file change is learned from the operating
  system's notification; a notification that reports lost events (an overflow) is itself an
  event, and its consumer reads the affected tree again — that path exists because the
  notification contract requires it, and it is stated. A file system that sends no
  notifications is not supported, and that is stated too. Polling or a timer is chosen only when
  no orthodox mechanism exists, stated as such, never the main path.
  Enforced by: none, a judgement at review.
- Idempotence and observability, as the engine applies them: a journal replay, a canon, an
  install and a write of the same bytes are idempotent; a request that is not idempotent by
  nature, such as an insert, is refused on its repetition as its rule says. That an event
  occurred is known from what it publishes, not found by searching for it. A process that
  crashed publishes nothing, so on start the engine reads the declared places of its own state
  — the lock and the journal — once; that is a read of a declared place, not a search.
  Enforced by: none, a judgement at review.
- Paths, as the engine applies them: the engine never creates a symlink and never follows one;
  a symlink found in a corpus is reported as its rule says.
  Enforced by: none, a judgement at review.

## Rules

- The shared tools are the package `@polyspec/kit`, a devDependency of the root `package.json` pinned to the asset URL of its
  release with its integrity. The files of its `kit.copies` (`scripts/kit/kit-bootstrap.mk`, `scripts/kit/bootstrap/`, the
  release workflow, `PRINCIPLES.md`, `config/layout.kit.yml` and the lint configuration) are copies that `make kit-sync`
  writes from the installed package and `make kit-check` compares; they are changed in kit, never here. flowmark runs a kit
  tool through its make target or `$(KIT)/<tool>.$(KIT_EXT)`, and takes a generic need from its ecosystem library declared
  in its own `package.json`. A tool that kit lacks or gets wrong is reported to kit, never copied or edited here.
  Enforced by: `make kit-check`
- `make flowmark-test` is the full suite of the repository's own code: it runs the node tests of `tests/` and
  `scripts/go-test.mjs`, which provisions the case-sensitive folder some Go tests need (on macOS a Case-sensitive APFS image
  under `var/`, on Windows `fsutil`, elsewhere a folder it probes) and runs `go test ./...` with it in each Go module of the
  repository (`packages/go`, `examples`, `tests/conformance/go`). A bare `go test ./...` on
  a file system that folds case fails those tests by design and names the variable they need. The CI workflow runs the same
  two commands on Linux, Windows and macOS.
  Enforced by: `.github/workflows/test.yml`
- The engine is a database, so its unit tests are dense: every grammar and constraint rule has at least one accepting and one
  rejecting case, table-driven, with its edge cases; every storage has P1/P2 golden fixtures covering every column type and
  both the NULL and the empty forms; parse and emit are fuzzed (`go test -fuzz`) with the golden fixtures as seeds, and a
  failing input the fuzzer finds stays as a regression case. An item that adds a rule is not done until these cases exist.
  Enforced by: none, a judgement at review.
- Multi-platform is a hard requirement: the platform-specific code of the lock, the journal, the watch and the file system
  runs on each platform of the CI matrix, and no platform trick stands in for a portable implementation; locks follow kit's
  holder-lock precedent, and the simplest implementation that satisfies the current requirements wins.
  Enforced by: `.github/workflows/test.yml`
- Each repository runs its own checklists: work in another repository is tracked there and referenced here by its ID in a
  code span, so that `make refs-check` reads no other repository's item as one of this repository.
  Enforced by: `make refs-check`
- Git files name no outside project that is not a dependency of flowmark; the polyspec repositories (kit, orm, hyper, crudui,
  template and others) are internal libraries and may be cited.
  Enforced by: none, a judgement at review.
