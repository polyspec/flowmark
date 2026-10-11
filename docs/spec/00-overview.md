<!-- doc-id: spec-overview -->
# Overview

[한국어](00-overview.ko.md)

flowmark is a text database whose grammar is markdown. The files are the database: reading and
writing go through one validated path, and the storage format stays a document that humans read.
git keeps the audit trail, the branches and the rollback.

## Scope

- **flowmark is the engine.** One Go binary holds the markdown database engine — the codec, the
  validator, the IR executor, the write path with its lock and journal, the history — and its
  thin faces: the server, the CLI, the MCP server and the soksak sidecar (protocol v1 of the
  soksak core specification). Every face runs the same engine; none has a write path of its own.
- **orm reaches flowmark as an IR executor.** [polyspec/orm](https://github.com/polyspec/orm) is
  the query layer: its clients validate and plan in their own process and reach a database
  through the driver their DSN scheme selects. The scheme `markdown://` selects an IR executor —
  the client validates the IR request, does not plan, and sends it to the flowmark server, which
  answers rows or one orm error. orm contains no markdown engine; grounding: an engine inside orm
  would be carried by each of its five clients, a separate engine is one implementation.
- **The catalog is the schema.** A corpus root holds `dbspec/*.dbs`, the dbspec document set
  that declares every table, its storage and its settings ([schema](30-schema.md)). Documents
  carry no schema copy, as a database keeps its schema once and never per row. flowmark reads
  the catalog and the IR with orm's Go `engine/dbspec` and `engine/ir` packages as libraries and
  never reimplements them.
- **The corpus is the truth.** A corpus is the directory tree of markdown documents that the
  catalog describes. Every write flows through the codec and the validator inside a transaction;
  every read goes through the same executor. An external database, when present at all, is a
  derived or owning side of a sync adapter — never a hidden mirror.
- **Views are consumers.** The GUI ships as a soksak plugin in its own repository; kanban, list
  and gantt are views over the same rows, stored as rows themselves, with no write path of
  their own.

## The enforcement promise

A document format with database-level force: a value outside its type, a dangling reference, a
broken key, an illegal state transition or a state change that bypassed the engine is a located
diagnostic `{file, line, column, rule, message}` — for the engine's own writes and for a human's
edits alike. Idempotency is a stated proposition, not a convention: emitting a parsed canonical
document reproduces it, and repeating an operation with the same input yields the same result
and state.

## Glossary

| Term | Meaning |
| --- | --- |
| corpus | the directory tree of markdown documents that is the database |
| corpus root | the directory given to a command or the server; it holds `dbspec/` and `.flowmark/` |
| catalog | the `dbspec/*.dbs` document set of a corpus root |
| storage | how a table's rows sit in files: `files`, `document` or `block` |
| row scope | the part of a document that belongs to one row: its file, or its detail section |
| codec | the strict parser and the canonical renderer of the storages |
| canonical form | the byte form the renderer owns; see [canonical format](10-canonical-format.md) |
| post-image | the corpus as it would read after a transaction commits |
| prepare record | the journal record, written before the renames, that makes a commit recoverable |
| IR executor | an orm DSN target that takes the validated IR request instead of rendered SQL |
| truth ownership | which side of a sync pair holds the authority for a table |
| adapter | an external system bound by the one sync contract (a database, a tracker) |
| face | one of the binary's entry points: server, CLI, MCP server, soksak sidecar |
| key | a row's identity; the file name of a `files` row (`<table>/<key>.flowmark.md`) |

## Non-goals

- No SQL text is produced or accepted; the executor consumes the IR request.
- No derived query mirror: a mirror, if ever admitted, must prove full-rebuild equivalence and
  have no write path. In-memory indexes are accelerators, not mirrors.
- No remote access in v1: the server binds loopback only and authenticates with a token.
- No general wiki engine: the grammar serves catalog-described documents, and free prose lives
  where the catalog puts it.

## Document map

The rulings live in [the design plan](../plans/design-plan.md) and the
[markdown database plan](../plans/markdown-database-plan.md); this specification set pins them:
[canonical format](10-canonical-format.md), [document model](20-document-model.md),
[schema](30-schema.md), [operations](40-ops.md), [constraints](50-constraints.md),
[capability table](60-dialect.md), [sync](70-sync.md), [migration](80-migration.md),
[conformance](90-conformance.md), [the application schema](100-checklist.md) and
[open questions](110-open-questions.md).
