<!-- doc-id: readme -->
# flowmark

[한국어](README.ko.md)

A text database whose grammar is markdown. The files are the database: strict parsing,
canonical rendering, corpus-global constraints (types, keys, foreign keys, state machines)
and transactions over a folder of markdown documents — with no external database required.

- **orm is the core**: [polyspec/orm](https://github.com/polyspec/orm) gains markdown as its
  fourth backend beside MySQL, PostgreSQL and SQLite. Every read and write goes through the
  orm model API; CLI and MCP are thin adapters with no write logic of their own.
- **flowmark is the integration and application layer**: bidirectional sync with databases,
  GitHub, Jira and other trackers (an adapter architecture with one contract), and on top of
  it a local kanban board and issue tracker — the data is managed locally, in git, offline.
- The first application is the checklist and changelog engine this repository runs on.

Status: design phase. The rulings live in [docs/plans/design-plan.md](docs/plans/design-plan.md);
the execution tracker is [docs/plans/execution-checklist.md](docs/plans/execution-checklist.md).
The shared tooling is the vendored copy of [polyspec/kit](https://github.com/polyspec/kit)
(`make kit-sync KIT_TAG=<tag>` updates it; `make kit-check` verifies it).
