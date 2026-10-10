<!-- doc-id: plans-markdown-database -->
# The markdown database plan

[한국어](markdown-database-plan.ko.md)

The plan of checklist item T12: the storage grammar, the relations, the layering and the views of
the markdown database. Each ruling below is promoted into its specification by T12.1; when a
specification and this plan disagree, the specification wins and this plan gets the correction.

## Context

The kanban desync fixes (T10-1 to T10-4) exposed that the markdown database had cases, not rules:

- **The three shapes were demo cases.** The list, table and detailed demos each grew a shape that
  no rule defines. The `## T2: title` sections of the detailed demo are read by no code, and the
  copied title has already drifted from the item (`작성1` against `작성`).
- **The model lacks its basic parts.** An item has no body column, no parent field, no relation
  and no history record; a child is guessed from the ID string, so `MoveItem` leaves the children
  behind and `EditTitle` drops the `@assignee` and due tokens.
- **The layering ruling is wrong.** The design plan (§1) placed the markdown executor in the orm
  repository, but orm is the query layer above databases (see the rulings below), and the code
  never followed that ruling: orm holds two dbspec settings and flowmark imports nothing of orm.
- **Behaviours are hardcoded** (see the hardcoding inventory).

**Goal.** Markdown works as a real database:

- a finite and complete storage grammar maps every database concept to a markdown construct;
- each table chooses, as database normalization does, whether its rows live together in one
  file (aggregate) or one file per row (split);
- orm reaches the markdown database by DSN as it reaches any other database;
- kanban, list and gantt are views over the same rows.

**Settled.** The `.dbs` file is the only origin of the schema. A title must not be empty; the
body column is required and may hold the empty text. No backward compatibility, no fallback, no
hidden error, no trick; rules first.

## Rulings

### flowmark is the engine; orm reaches it through a driver

- Grounding (orm `docs/protocol.md`, `docs/config.md`): an orm client plans the IR in its own
  process and reaches an external database through the native driver its DSN scheme selects.
  orm contains no database engine; MySQL, PostgreSQL and SQLite all live outside it.
- The markdown engine — parse, constraints, transactions, lock, journal — is storage-engine work.
  Inside orm it would break orm's own layering, and each of the five clients (Go, PHP, Python,
  Rust, TypeScript) would carry its own engine; the "markdown is Go-only" caveat of the design
  plan is the symptom.
- As a separate engine there is one implementation, and each orm client gains only a thin driver
  that connects, sends the IR and reads the rows.
- The earlier recommendation to move the engine into orm was made without checking this and is
  withdrawn; the design plan's seam ruling is corrected.

### The `.dbs` file generates the skeleton; documents carry no schema copy

- The `.dbs` set is the schema, as a database catalog is. `flowmark install` generates the
  storage skeleton from it — folders, parent documents, empty block sections — as `CREATE TABLE`
  creates storage.
- A database keeps its schema once, never per row. A data document carries the manifest hash
  stamp only: a schema copy per document makes the drift of the copies a bug class of its own.
- **T8.5 is withdrawn**: the Mermaid block inside a document is no authority. `ParseMermaid`,
  `ApplySchema`, the `schema.mermaid` operation and the board's diagram strip are removed.
- Mermaid views (orm's `erDiagram` included) are not a present requirement and are out of scope.

### Introspection

- orm introspects MySQL, PostgreSQL and SQLite into a dbspec document
  (`dialects.md#introspection`); the manifest settings (`state_machine`, `markdown`) are not
  recoverable from a database.
- Markdown introspects into dbspec exactly when the corpus carries the stamp. Without it,
  structure is inferable but type, key and constraint are not: the result is an import tool that
  returns the list of what it did not carry over, never a silently confirmed guess.

## The storage grammar

Every markdown-specific declaration is a dbspec **manifest setting**, after the precedent of the
existing `markdown <column>` setting: the SQL backends ignore it and introspection returns a
plain column. One grammar means one parser — flowmark uses orm's Go `engine/dbspec` package as a
library and never reimplements it.

| Concept | Rule |
| --- | --- |
| Table storage | Exactly one per table: `store file <dir>` (split, one file per row) or `store block <fk> list` / `store block <fk> table` (aggregate, a block inside the parent row). A table without one is a schema error; no default is guessed. |
| Row key | A markdown table has a single-column primary key. A file row's key is its `<!-- doc-id: -->` value — the file name carries no meaning and no copy. A block row writes its key in the row. |
| Title | `title <column>`: non-null and refusing the empty text, required on every stored table. The H1 of a file row, the line text of a list row, a cell of a table row. |
| Body | `body <column>`: a `markdown` column, required on every stored table, the empty text allowed. The preamble of a file row; the text of a block row's detail section. |
| Other markdown columns | A section of the row scope whose heading is the column name; an undeclared heading is a diagnostic. |
| Scalar columns | A frontmatter key in a file row; a cell under the column-named header in a table row, every column present; a trailing inline field `column:value` in a list row — only declared columns are fields, a value with spaces is written `"..."`, and the renderer escapes a title token that would read as a field. The `@` and date sigils are removed. |
| State column | The `state_machine` column. The checkbox glyph of a list row is declared by `checkbox <column> <state> '<glyph>'`. |
| Row scope | A file row's scope is its file. A block row's scope is the `<heading+1> <key>` detail section under its block section; the heading holds the key only and never a title copy. Without a detail section the body is the empty text. |
| Block placement | The block of a child table C lives in the parent row's scope under the section `<next level> C`, recursively; past H6 is a schema error. |
| Hierarchy | A self foreign key `parent`. In a list the indentation is the parent and no column is written; a table or a file writes the column. No parent is guessed from an ID string; a cycle is refused. |
| Order | Block rows keep their document order; file rows follow a declared order column, or the key order without one. |
| Relations | Every foreign key is restrict-only. A one-to-many across files stores the child table with `store file` and writes the foreign key in its frontmatter. Aggregate or split is a storage declaration; the logical relation is the same. |
| Many-to-many and links | A link table, such as `item_links(from, to, kind in (blocks, relates, duplicates))`; the gantt dependency edge is `blocks`. |
| History | The engine performs the `audit into` meaning itself; the history table is a block under the audited row, and the `require` columns of a transition (evidence, cause, retry) persist as its row. |
| Block shape | The schema decides the shape of a block section; a different shape is a diagnostic. |

### The application schema and the views

- `flowmark.dbs` is the standard application schema, extended by adding columns: `items` (id,
  parent, state, title, body, assignee → people, start, due, priority), `people`, `item_links`,
  `item_history` and `views`.
- A view (kanban, list, gantt) is **data**: a row of `views`, so a filter saved in the GUI never
  edits the schema.
- The WIP limit is enforced by the engine at transition time and so belongs to the schema:
  `state_machine <column> limit <state> <n>`. The relative rule `doing < done + 2` is wrong — its
  limit vanishes as done items accumulate — and is removed.

## Layering

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── markdown driver ── DSN markdown:// ──┐
                                                                                               ▼
flowmark = the markdown database engine (one Go binary)
   engine: codec · validator · IR executor · write path (lock + journal, spec 40) · introspection
   app:    application schema (flowmark.dbs) · application rules · install (.dbs → skeleton) · sync
   faces:  server (for the orm drivers) · CLI · MCP · soksak sidecar — thin faces of one engine
   uses:   orm engine/dbspec (parser, manifest hash) · engine/ir (request types) — as libraries
                                                                                               ▲
soksak plugin ── sidecar protocol v1 ─────────────────────────────────────────────────────────┘
   views (kanban/list/gantt) · detail modal · no write path of its own
```

- **Server.** Binds loopback TCP; the DSN is `markdown://127.0.0.1:<port>`; the wire is JSON
  lines of orm IR requests (with the manifest hash) and row answers. A manifest-hash mismatch is
  refused. Remote access and authentication are outside v1, and the specification says so.
- **Concurrency.** Humans, git, the CLI and the server touch the same files, so every operation
  takes the per-operation portable lock of spec 40.
- **orm gains** the dbspec settings `store`, `title`, `body`, `checkbox` and `state_machine …
  limit`, plus a `markdown` driver per client with its capability table; orm's interface parity
  rule gives each client its own orm checklist item.
- **flowmark rewrites** the engine on the storage grammar and removes `app/schema.go`,
  `firstFlowmarkFile`, the sigil grammar and the relative WIP rule, with no compatibility layer.

## Steps

1. **T12.1 Rulings and specifications.** Correct the design plan's seam ruling and withdraw T8.5,
   recording the facts of the divergence; revise the specifications 20 (storage), 30 (settings),
   10 (grammar), 40 (server and lock), 50 (relations and cycles), 60 (capability) and 100
   (application schema, views, WIP); write the working principles and the branch, worktree and
   test-disposal rules into AGENTS; update the memory.
2. **T12.2 orm dbspec settings**, red then green, with the orm document pairs and vectors.
3. **T12.3 The engine rewrite** on orm `engine/dbspec` and `engine/ir`: the storage codec with
   P1/P2, the validator, the full IR executor, the write path with lock, journal and history,
   the introspection; the old engine paths removed.
4. **T12.4 The faces**: the DSN server, table-scoped operations (`schema`, `rows`, `row.get`,
   `row.insert`, `row.update`, `row.move`, `row.delete`, `transition`, `views`) on the CLI, MCP
   and sidecar, `flowmark install`, and the demos as one dataset in three storages.
5. **T12.5 The orm Go `markdown` driver** with the conformance against the flowmark server; the
   other clients as their own orm items.
6. **T12.6 The plugin**: view switching (kanban, list with group/sort/filter, gantt with start,
   due and `blocks` edges), the detail modal (every column, body editing, children, links,
   history), the hierarchy, separated UI strings, and answers judged by structure.

Each step runs red test, green, `[o]` with the changelog pair and the commit; uncommitted changes
cover one item; a branch or worktree is deleted as soon as it merges into main. The checklist of
this repository stays managed by hand.

## Findings removed by the rewrite (never patched)

- `EditTitle` drops `@assignee` and the due; `MoveItem` leaves the children; the continuation
  check of `ApplySchema` is always false (`app/schema.go:100`); a table document bypasses the WIP
  gate; the validator registers no table-form ID, so duplicates pass and prose references read as
  dangling; transition evidence is never stored.
- An unknown required field passes `requiredMet` unchecked; the frontmatter is parsed and read by
  no code.
- The lock and the journal of spec 40 are not implemented.

## Hardcoding inventory (every item read from the schema)

| Place | Hardcoded |
| --- | --- |
| `app/apply.go:15,40,178` | the WIP glyphs `[~]` and `[o]`, and `WIPHeadroom = 2` |
| `app/apply.go:159-167` | the names `evidence`, `cause`, `retry` and substring checks |
| `app/apply.go:315`, `corpus/validate.go:24` | the ID prefix `T`; the ID grammar becomes a column check |
| `codec/blocks.go:71`, `executor/executor.go:112` | table kinds guessed from column names |
| `codec/blocks.go:24-45` | the `@assignee` and date sigils |
| `app/schema.go` | the in-document Mermaid (removed) |
| `sidecar/loop.go:351` | `firstFlowmarkFile` |
| `corpus/validate.go:51`, `**/*.md` | the twin suffix and the selection, decided in one configuration |
| `ui/board.js` (plugin) | UI strings in code, answers matched by sentence |

## Verification

- orm: the dbspec rule tests pass; the Go driver passes the conformance over every IR kind
  against the flowmark server.
- flowmark: P1/P2 round trips for the three storages; one dataset in three storages yields the
  same rows; the conformance vectors (corpus before, request, corpus after) pass.
- Introspection: a stamped corpus yields its `.dbs`; an unstamped one yields the list of what
  was not carried over.
- Write path: `go test -race`, multi-process concurrent writes and crash simulation with journal
  replay.
- GUI: kanban, list, gantt and the modal captured through the observation server and headless
  Chrome; a refused transition returns to its place.
- Documents and hardcoding: `make documents-check`, `make refs-check` and `make kit-check` pass,
  and a grep of the inventory finds nothing.
