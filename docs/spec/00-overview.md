<!-- doc-id: spec-overview -->
# Overview

[한국어](00-overview.ko.md)

flowmark is a text database whose grammar is markdown. The files are the database: reading and
writing go through one validated path, and the storage format stays a document that humans read.
git keeps the audit trail, the branches and the rollback.

## Scope

- **orm is the core.** [polyspec/orm](https://github.com/polyspec/orm) executes the markdown
  corpus as its fourth backend beside MySQL, PostgreSQL and SQLite. The shared planner validates
  the IR request; the markdown executor consumes the request directly and renders no SQL.
- **flowmark is the integration and application layer.** One Go binary carries three thin faces —
  the CLI, the MCP server and the soksak sidecar (protocol v1 of the soksak core specification) —
  and the repository holds the sync adapters (database, GitHub, Jira) and the checklist
  application. The GUI ships as a soksak plugin in its own repository; every view is one more
  thin consumer with no write path of its own.
- **The corpus is the truth.** A corpus is a directory tree of markdown documents described by a
  dbspec schema. Every write flows through codec and validator inside a transaction; every read
  goes through the same model API. An external database, when present at all, is a derived or
  owning side of a sync adapter — never a hidden mirror.

## The enforcement promise

A document format with database-level force: a key outside the grammar, a dangling reference, a
type error or an illegal state transition is a located diagnostic `{file, line, column, rule,
message}` — for the tool's own writes and for a human's edits alike. Idempotency is a stated
proposition, not a convention: emitting a parsed canonical document reproduces it, and repeating
an operation with the same input yields the same result and state.

## Glossary

| Term | Meaning |
| --- | --- |
| corpus | the directory tree of markdown documents that is the database |
| codec | the strict parser and the canonical renderer of one document |
| canonical form | the byte form the renderer owns; see [canonical format](10-canonical-format.md) |
| skeleton | the structural parts of a document the renderer may rewrite |
| post-image | the corpus as it would read after a transaction commits |
| prepare record | the journal record, written before the renames, that makes a commit recoverable |
| truth ownership | which side of a sync pair holds the authority for a table |
| adapter | an external system bound by the one sync contract (a database, a tracker) |
| face | one of the binary's entry points: CLI, MCP server, soksak sidecar |
| twin | the Korean projection of an English document, bound by `source-sha256` |
| doc-id | the unique document identity carried in the header comment |

## Non-goals

- No SQL text is produced or accepted; the plan layer renders nothing for markdown.
- No derived query mirror: a mirror, if ever admitted, must prove full-rebuild equivalence and
  have no write path. In-memory indexes and the parsed-row cache are accelerators, not mirrors.
- No resident daemon of flowmark's own; a face runs inside its host's lifetime (a shell, an MCP
  host, the soksak host) and holds no lock between operations.
- No general wiki engine: the grammar serves schema-carrying documents, and free prose lives
  where the schema puts it.

## Document map

The rulings live in [the design plan](../plans/design-plan.md); this specification set pins them:
[canonical format](10-canonical-format.md), [document model](20-document-model.md),
[schema](30-schema.md), [operations](40-ops.md), [constraints](50-constraints.md),
[capability table](60-dialect.md), [sync](70-sync.md), [migration](80-migration.md),
[conformance](90-conformance.md), [the checklist application](100-checklist.md) and
[open questions](110-open-questions.md).
