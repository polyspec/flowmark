<!-- doc-id: readme -->
# flowmark

[한국어](README.ko.md)

A text database whose grammar is markdown. The files are the database: strict parsing,
canonical rendering, corpus-global constraints (types, keys, foreign keys, state machines)
and transactions over a folder of markdown documents — with no external database required.

- **flowmark is the engine**: one Go binary holds the markdown database engine and its thin
  faces — a server, the CLI, the MCP server and the soksak sidecar. A corpus's `dbspec/`
  catalog declares every table and how its rows are stored: one file per row, all rows in one
  file, or rows inside a parent row.
- **orm reaches it as a database**: [polyspec/orm](https://github.com/polyspec/orm) is the query
  layer; its clients reach flowmark through the DSN `markdown://` as they reach MySQL,
  PostgreSQL and SQLite.
- **On top of the engine**: bidirectional sync with databases, GitHub, Jira and other trackers
  (an adapter architecture with one contract), and an issue-tracking application schema whose
  kanban, list and gantt views run in a soksak plugin — the data is managed locally, in git,
  offline.

Status: the engine is being rebuilt on its storage grammar. The rulings live in
[docs/plans/design-plan.md](docs/plans/design-plan.md) and
[docs/plans/markdown-database-plan.md](docs/plans/markdown-database-plan.md); the trackers are
[docs/plans/execution-checklist.md](docs/plans/execution-checklist.md) and
[docs/plans/markdown-database-checklist.md](docs/plans/markdown-database-checklist.md).
The shared tooling is the vendored copy of [polyspec/kit](https://github.com/polyspec/kit)
(`make kit-sync KIT_TAG=<tag>` updates it; `make kit-check` verifies it).
