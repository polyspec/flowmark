<!-- doc-id: plans-design-plan -->
# flowmark Design Plan

Revision 2 — folds the findings of the three-lane adversarial audit (engine core, orm integration, operations) and the multi-platform directive into the rulings below. Every audit blocker became a ruling; the audit's unbroken claims are listed at the end.

Revision 3 — corrects the rulings that the [markdown database plan](../markdown-database/plan.md) withdrew, with the facts that withdrew them:

- **The seam.** This plan placed the markdown executor inside orm. orm is the query layer: its clients plan in their own process and reach every database through a driver (orm `docs/protocol.md`, `docs/config.md`), so an executor inside it would exist once per client — the "Go-only" caveat below was the symptom. The code never followed the ruling either: orm received two dbspec settings and flowmark imported nothing of orm. flowmark is the engine, and orm reaches it as an IR executor by DSN.
- **The schema stamp and standalone fragments.** A stamp in every document copies the schema once per row; the `dbspec/` catalog of a corpus root is the only schema.
- **The task-ID grammar.** Keys are opaque; hierarchy is a `parent` column and supersede a link, so the ID grammar, its ordering and the renumber codemod are gone.
- **`identity` rejection, advisory `audit`, the WIP gate as a column limit, the per-operation-only lock and the corpus revision** are replaced as the plan states.
- **T8.5** — a Mermaid block inside a document as the transition authority — was never this plan's ruling and is withdrawn with it.
- **The readable document.** A corpus document is a `*.flowmark.md` file that reads as an ordinary markdown document: the doc-id header, the frontmatter of a row file, the provenance header, twins and `dbspec/flowmark.json` are withdrawn; a `files` row's key is its file name and foreign keys are links (markdown database plan, "The readable document").

The sections below carry the corrected rulings; the specifications hold them in full.

## Context

The user manages engineering specs as markdown and runs development through checklists (five states `[ ]` `[~]` `[o]` `[!]` `[-]`, dotted hierarchical IDs, prerequisite/follow-up dependencies), changelogs, and commit discipline. The problem: document-based operation has **no idempotent, database-level enforcement** — format violations, dangling foreign keys, and illegal status transitions stop no one.

**flowmark's identity (user-confirmed)**: a **text database whose grammar is markdown**. The files themselves are the database and it runs fully standalone without an external database. **flowmark is the engine** (Revision 3 — orm is the query layer that reaches it as an IR executor) **and the integration and application layer** — bidirectional sync with databases, GitHub, Jira and other trackers, and on top of it a local kanban board and issue tracker (GUI as one more thin consumer): GitHub and Jira have their own boards, but here the data is managed locally, in git, offline. Core value: the storage format is a document humans read, and git provides audit, branching, and rollback for free.

**User decisions**:

1. polyspec/orm reaches markdown as a **fourth database** beside mysql/pgsql/sqlite — through the DSN `markdown://`, which selects an IR executor served by flowmark (Revision 3). The faces are thin adapters with no write logic of their own.
2. Schema direction: **dbspec is the root grammar and the only authority** — the `dbspec/` catalog of a corpus root declares every table and its storage; documents carry no schema copy (detail in §4, Revision 3).
3. Documents are **EN/KO pairs from day one** (orm house style, kit-compatible).
4. **Vendor polyspec/kit now** — the fifth state `[-]` is **upstreamed in the polyspec/kit repository** (orm's `scripts/kit` is a hash-locked vendor copy that must never be edited in orm; it reaches orm only through `make kit-sync KIT_TAG=<tag>`). The user delegated authority over kit.
5. This session's deliverable = **design documents + repository bootstrap** (implementation is tracked as later phases on the checklist). Per the audit's scope finding, the spec documents themselves split into a second session (T1b); this session closes T1a.
6. Sync projection files: real markdown files exist on disk. `readonly` (default) or `writable` option (detail in §7).
7. The repository lives at **github.com/polyspec/flowmark** — a standalone repository inside the polyspec organization.
8. **Multi-platform is a hard requirement. No platform tricks.** The simplest implementation that fully satisfies the current requirements wins (kit's own rule, applied here): no OS file lock, no POSIX-only durability assumptions — locks follow kit's `holder-lock` precedent (lock file + holder identity + stale detection), with the platform primitives each platform needs named in spec 40 (the change notification that drives the lock wait, the process start and machine identifier that judge a holder, the write-through move that makes a name durable on Windows), and crash safety comes from the journal replay, with each step's files and names made durable before the next step as spec 40 states per platform.
9. **The GUI ships as a soksak plugin in its own repository; flowmark serves soksak as a sidecar.** One Go binary carries three thin faces — the CLI, the MCP server, and the soksak sidecar (protocol v1 of the soksak core specification: JSON lines over standard input and output, surface and root routing, a host-owned lifecycle). The repository is single-language Go with a `package.json` and a `sidecar.json` manifest; per-language packages appear only when a requirement names them.

## Verified grounding (checked directly in the polyspec checkouts)

- `orm/docs/protocol.md:12,96` — IR kinds are bounded: `one|all|count|group_count|sum|avg|paginate|insert|update|delete|restore`. The request carries no SQL text → a **native markdown executor consuming the IR request is a bounded problem**. The audit confirmed expressiveness: every kind plus the Order/Pred/Func surface is computable over an in-memory corpus; the open questions were specification, not expressiveness.
- `orm/docs/dialects.md:184` — the three databases unify collation to binary codepoint order. This is the rule for **text comparison**, which the engine follows (see the ordering ruling in §2).
- `kit/scripts/kit/documents-stamp.mjs`, `check-documents.mjs` — `source-sha256` is an **EN→KO translation sync marker** (the Korean twin holds the sha256 of the raw English file bytes). Data drift detection is therefore designed on `.flowmark/` sidecar state, and flowmark adopts **raw-file-bytes hashing** for the same marker (kit-compatible; canonicalization belongs in the check, not the hash).
- `kit` is its own repository (`polyspec/kit`); consumers hold a hash-locked vendor copy of `scripts/kit` + `tests/kit` written by `make kit-sync KIT_TAG=<tag>` and verified by `make kit-check`. A vendored file is changed in kit, never in the consumer.
- `kit/scripts/kit/checklist.mjs` — states are per-tracker data; the `[-]` impossible state is now read at both levels (upstreamed as kit K13, branch `feat/fifth-state-K13` pending merge).
- kit document rules: every English document needs a Korean twin (doc-id match, explicit anchors match, fenced blocks match, the twin holds source-sha256); plans/ and CHANGELOG are twin-checked too.

## Core design (what the spec documents will pin down)

### 1. Architecture and the seam

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── IR executor ── DSN markdown:// ──┐
                                                                                          ▼
flowmark = the markdown database engine (one Go binary)
   ├─ codec: strict parser + canonical renderer of the storages (two-clause idempotency, §2)
   ├─ validator: catalog constraints (type/PK/UNIQUE/FK/CHECK/state machine and history)
   ├─ IR executor over in-memory indexes derived from the parsed corpus
   ├─ write path: lock → conflict check → validate → stage → prepare record → renames → commit
   ├─ .flowmark/: lock, tx journal, server token
   ├─ faces: server · CLI · MCP · soksak sidecar
   └─ sync engine: md→db export / db→md materialize (§7)
```

**The seam ruling (Revision 3)**: orm is the query layer; its clients plan in their own process and reach each database through the driver their DSN scheme selects. The scheme `markdown://` selects an **IR executor**: the client validates the IR request, does not plan, and sends it to the flowmark server, which answers rows or one orm error — `getQuery()`, statement events, `subscribe()` and `force_index` are `CAPABILITY_UNSUPPORTED` (orm's precedent for unsupported capability). The engine lives in flowmark, once; each orm client carries a thin executor client, the Go client first. flowmark uses orm's `engine/dbspec` and `engine/ir` packages as libraries. Revision 2 placed the executor in orm, which would have put an engine in each of the five clients.

- A SQLite mirror is rejected: mirror↔corpus equivalence becomes a permanent bug class. If a mirror is ever admitted, its conformance conditions are (a) full-rebuild equivalence and (b) no write path.
- The IR-boundedness claim covers the request layer. **API parity is not claimed** — the capability table (§6) is the contract, in orm's `CAPABILITY_UNSUPPORTED`/`OPERATOR_NOT_ALLOWED` tradition.

**The examples ruling (T77)**: flowmark holds the engine and only the dependencies the engine needs: its Go module and the orm Go module whose `engine/dbspec` and `engine/ir` it uses. The examples — the tracker and board corpora, their Go tests, and the board web application, which pins hyper, crudui, orm, template, php-serialize and ordered-json and needs PHP and Composer — live in the repository flowmark-examples, which consumes the tagged Go module of flowmark.

### 2. Canonical format (`10-canonical-format.md`)

- **Idempotency is the dbspec two-clause proposition**, not a single byte-identity claim: `emit(parse(s)) == s` for canonical input `s`, and `emit(parse(emit(d))) == emit(d)` for any valid input `d`. The renderer owns exactly the **structural skeleton** — the field list of a `files` row, heading lines, and the item and table lines of the rows (Revision 3) — and never rewrites payload bytes in preserved runs (prose, fenced content). Non-canonical input is a `format.noncanonical` diagnostic plus a separate one-shot `flowmark canon` migration transaction, never a side effect of a logical write.
- The body and every `markdown` column value are sections of the row scope, bounded by heading level outside fenced blocks; an absent section is NULL, a present empty one the empty text (Revision 3).
- **Ordering**: an IR `order` sorts by the column's type, text by codepoint; a read without one returns storage order — the pre-order of the tree with siblings in their order column or key order. `group_count` groups NULLs together (SQL parity). Keys are opaque: the task-ID grammar, its segment-wise ordering, the suffix rule and `flowmark next-id` are withdrawn (Revision 3); `identity` and `key_prefix` allocate keys.
- **List rows** carry the state glyph, the key, the title and trailing `column:value` fields parsed from the right; a `markdown` column is never a field or a cell, and a value with a line feed in a single-line place is refused.
- **Readable values** (Revision 3): a corpus document has no frontmatter and no comment; it reads as an ordinary markdown document. A `files` row's scalars are a field list after its H1, written in a readable form (dates, uuids, `bytes` as padded base64) rather than dbspec literals, and every relation is a link.
- Corpus documents have no twins (Revision 3): a row file carries no doc-id and no `source-sha256`. The EN/KO pairs and `source-sha256` (sha256 of the raw English file bytes, kit-compatible) belong to this repository's own documents.

### 3. Document-table model (`20-document-model.md`)

- Each table declares one storage (Revision 3): `store files` (one file per row in the table's folder), `store document list|table` (every row in `<table>.flowmark.md`) or `store block <fk> list|table` (the rows inside the parent row). Aggregate or split is a storage declaration; the logical relations are the same.
- A `files` row is the file `<table>/<key>.flowmark.md`: its H1 is the title and its scalar columns are a field list after it (Revision 3). Its key is its file name; hierarchy is a `parent` column, written by indentation in a list.

### 4. Schema (`30-schema.md`) — the catalog is the schema

- **dbspec is the root grammar and the only authority.** The `dbspec/*.dbs` set of a corpus root is the catalog (Revision 3). Documents carry no schema copy and no stamp, and standalone documents with embedded fragments are withdrawn: schema is law and data is rows, and a copy of the law per document makes the drift of the copies a bug class of its own. orm's engine, IR and conformance are bound to the dbspec manifest hash, and dbspec's parser and type system are reused as a library.
- **Markdown-specific schema surface is manifest-level settings, never rendered types** (introspection coherence, the `updated`/`soft_delete` precedent): `store`, `key_prefix`, `title`, `body`, `order`, `checkbox`, `markdown` and the state machine's `initial`, `history` and `limit` are settings that the SQL backends ignore; a real database introspects them back as plain columns.
- **`identity` is allowed** only on an `i64` primary key of a markdown table, allocated under the lock one more than the larger of the largest key and the table's AUTOINCREMENT counter (Revision 3, and A refresh of the changed rows (T42) for the counter; the other key types have no maximum to add one to); key reuse under git rewind is accepted, since the rewind rewinds the counter too — git history is the allocator of record.

### 5. Ops and transactions (`40-ops.md`)

- **Commit protocol** (order is normative): write a synced copy `.flowmark/tx/<id>.<n>.<hash>` of each post-image as the source of rename n, so two identical targets stay two files and no target shares a file with the stage; write a **prepare record** `<id>.prepare` with the generation it takes, every rename with its pre- and post-hashes (an absent file is the empty hash) and the bytes of both recorded states, synced with its folder; perform the renames through the runtime's atomic rename, a delete being a rename into the journal, and sync each folder of a target once; write the generation; collect the stage (T58, Ruling: a commit waits for the disk as few times as its atomicity needs). **Recovery runs on every corpus open**: a prepare record completes, its absence discards the stage, and a target whose current hash matches neither recorded hash is refused and reported, naming the stage files that hold both recorded states (a change landed mid-crash — never silently overwritten). Durability across power loss needs each step's files and names on disk before the next step: a folder sync on Linux and macOS, a write-through move on Windows (spec 40).
- **One portable lock** (multi-platform ruling): every operation — read or write, from any face — takes the same lock file `.flowmark/lock` with holder identity and stale detection (kit `holder-lock` precedent); a held lock is waited for, driven by the OS change notification, up to a deadline that reports the holder, and a stale lock is removed only by `flowmark unlock`. The lock is held for one operation, or for one server transaction from `begin` to `commit` or `rollback` (Revision 3); a closed connection or an idle transaction rolls back and releases it. Conflicts on commit (a target changed since read) fail with the file and line — a face re-reads and retries once, an orm client receives `WRITE_CONFLICT` — never a silent merge.
- **The server** (Revision 3) is a resident face bound to loopback with a kept token; the DSN is `markdown://127.0.0.1:<port>/<corpus>?token=<token>`.
- **Validation runs at commit against the corpus freshly re-read under the lock**, after the conflict check — not against a BEGIN snapshot, and not against a predicted touched-file set.
- The corpus revision and `CORPUS_CHANGED` are withdrawn (Revision 3): `paginate` is one request, and no face reads across calls.
- **Cost model**: a per-command p95 latency target is declared in 40-ops (order: hundreds of milliseconds at the corpus envelope); how an operation avoids reading unchanged files — a long-lived corpus kept current by file notifications, with the commit targets always re-read under the lock — is owned by `T12.5-2`, since a cache keyed by content hash must still read every file; the commit checks every constraint the transaction can change, on the corpus re-read under the lock, with full `flowmark validate` on demand and on catalog change.
- Walk rules: only `*.flowmark.md` files outside `dbspec/`, `.flowmark/` and hidden entries are read; a symlinked document or `files` folder is `corpus.walk`, any other symlink is outside the database; a hard link is not a rule (document model).

### 6. Constraints and the capability table (`50-constraints.md`, `60-dialect.md`)

- Strict parse, type/PK/UNIQUE/FK (exact byte match; keys unique without regard to case), CHECK. Soft-deleted rows still occupy unique keys and still satisfy FKs (SQL parity — stated so a soft delete never trips a spurious ruling). `aes`, `aes_version` and `blind_index` are declared on a corpus: the client encrypts as on SQL, and the engine takes its keys by injection (spec 30 Keys).
- The **state machine is a dbspec-level manifest setting** (transitions, terminals, required columns, and — Revision 3 — `initial`, `history` and `limit`). Every engine transition writes a history row; a state changed outside the engine is reported as a history gap or a broken chain, with a repair command. The application schema supersedes a done item with a `supersedes` link; the engine enforces no WIP rule — the relative gate is removed, and the owner's rule stays a working rule.
- **Human writes void the database's writer-monopoly premise; each feature rules on it**: FK actions are **restrict-only** (`cascade`/`set_null` are rejected at catalog load — they are unimplementable for a writer that bypasses the engine); `audit` and `immutable` are schema errors on a corpus, replaced by the state history (Revision 3); every non-null column is written, the engine writing defaults on insert.
- **The capability table** (`60-dialect.md`) is the integration contract under the IR-executor wire: `update|share` locks and their `*_nowait` forms are documented no-ops; every `isolation` level is accepted, since a transaction holds the corpus lock; `timeoutMs` is `CAPABILITY_UNSUPPORTED` and `fulltext` `OPERATOR_NOT_ALLOWED`; an `on_duplicate` insert has exactly one conflict key (orm's rule); a changed target fails `WRITE_CONFLICT`; `getQuery`, statement events, `subscribe` and `force_index` are `CAPABILITY_UNSUPPORTED`; savepoints are nested stages.
- The markdown plan lifecycle (orm's dbplan/apply/recover machinery) is **scoped out of v1**: v1 supports corpus creation and additive column addition; dbplan chains on markdown corpora are rejected (80-migration states the scope and the reason).

### 7. Sync and adapters (optional, `70-sync.md`)

- The sync engine is an **adapter architecture with one contract**; the database is the first adapter, GitHub (issues/projects) and Jira follow: per-table truth ownership, a sync provenance kept outside the document, its location and form decided by open question 5 (70-sync), git-aware drift handling, a recorded materialized-from revision as the merge base, and explicit one-shot commands (no live daemon). An external tracker's items materialize as corpus rows — the checklist app's item model is the local issue model, so the local board and the remote tracker are two projections of the same rows.
- md→db: derived export (rebuildable). db→md (and every adapter→md): materialize. The validator rejects edits to adapter-owned documents (readonly default); `writable` re-ingests after validation.
- **Readonly drift is git-aware**: the working tree differing from HEAD restores to HEAD and reports; a clean tree whose HEAD differs from the last ingested version accepts and re-baselines. Merge conflicts on adapter-owned paths are resolved by discarding both sides and re-materializing — a hand merge there is a diagnostic.
- The export renders through orm's SQL dialect renderer from the catalog (Revision 3). A changelog projection derives from the history rows — on-file data — never from the gitignored journal (a fresh clone must be able to rebuild every projection).

### 8. Migration (`80-migration.md`)

- v1 scope: corpus creation by `flowmark install` from the catalog and additive column additions; a storage move is one transaction. The renumber codemod is withdrawn with the ID grammar (Revision 3). Additive-first; renames are explicit steps; dbplan chains are `CAPABILITY_UNSUPPORTED` (see §6).

### 9. The application schema (`100-checklist.md`)

- `flowmark.dbs` (Revision 3): `items` (parent, the five-state machine with its history, title, body, assignee, start and due as date/datetime pairs, priority, position), `people`, `item_links` (blocks, relates, duplicates, supersedes), `item_history` and `views`.
- **Views are data and thin consumers**: a kanban, list or gantt view is a row of `views`; a board reads through the engine and writes through the same operations — an illegal drag fails exactly like an illegal CLI write. No view-specific write path exists, and no WIP limit is invented.
- This repository is not a corpus (Revision 3): its checklists stay kit documents managed by hand, so the cutover is withdrawn; `make validate` and the prose ID checker are removed with the old engine; `make refs-check` keeps checking the kit-tracked checklists.

## Session plan (T1a this session; T1b next session)

**T1a — audit-folded design + bootstrap** (this session)

1. `git init` flowmark, `.gitignore` (`.flowmark/`), `README.md`+`README.ko.md`, `AGENTS.md`+`AGENTS.ko.md` (operating rules: checklist discipline, conventional commits, the [o]↔changelog↔commit unit, the twin workflow — a Korean-caught fix is ported to the English file before the next stamp, no interactive history edits across document commits), `CHANGELOG.md`+`CHANGELOG.ko.md` (`## Unreleased`). Commit: `chore: Bootstrap flowmark repository`.
2. **kit `[-]` upstream** — done on branch `feat/fifth-state-K13` (worktree `kit-fifth-state-K13`, red vectors → FIVE_STATES + cause rules → green, K13 row + AGENTS twins + stamped KO checklist). Merge into kit's `main` once the parallel work there stabilizes, then remove the worktree and branch. orm adopts later via `make kit-sync` when the user tags a kit release.
3. **Vendor kit whole-directory** into flowmark (`scripts/kit` + `tests/kit` + `kit.json` + `.kit/kit.lock.json` following the kit-sync contract; the lock records the kit commit) and author `config/checklist.json` (list format, `T<n>(.<n>)*(-<n>)?` ID pattern, five states, active `[~]`) and `config/documents.json`. Commit: `chore: Vendor polyspec/kit tooling`.
4. `docs/plans/flowmark/checklist.md`+`.ko.md` — the T1a/T1b/T2–T9 items in the user's format (doc-id, prerequisite/follow-up, cause/completion-criteria/evidence), maintained throughout.
5. This design plan (revision 2) and its Korean twin — committed with the audit folded in.

**T1b — spec documents** (next session; EN/KO pairs, one commit per document, twins stamped): `00-overview`, `10-canonical-format` (§2), `20-document-model` (§3), `30-schema` (§4), `40-ops` (§5), `50-constraints` (§6), then skeletons `60-dialect` (the capability table as full content), `70-sync`, `80-migration`, `90-conformance`, full `100-checklist`, `110-open-questions`. Acceptance criterion per document: every ruling appears as a proposition with its grounding (decision count, not prose volume).

**Ruling: orm reaches a corpus by SQL.** An orm markdown connection is a SQLite connection whose statements the server answers ([SQL statements](../../spec/40-ops.md#sql-statements)), so a recipe of orm runs on flowmark as on any database and a runtime ports one driver. This replaces the earlier ruling that a request carries no SQL text: that ruling asked every runtime to port a second executor client and left most recipes out. The IR stays for the engine's own faces and as the writer of a statement's difference. Performance is not a database's, and a database's behaviour is required.

**Ruling: an operation costs what it touches.** A statement, and every operation of the engine's faces, costs the files it
changes and the rows it touches, not the size of the corpus; reads run beside a writer as on SQLite in WAL mode. The code
of T31 contradicts this in every layer an operation passes through, and none of it was measured:

- the corpus read under the lock (`writepath.Corpus.read`) runs `validate.Read`, two passes over every file, the
  availability rules over every row, and `irexec.Open` with `index` over every row, at `Begin` and again at the commit;
- every write request deep-copies every row (`irexec.Corpus.clone`), and the commit compares every file
  (`Corpus.Differs`);
- a SQL write reads every row of every table before and after it (`execute`, `allRows`, `diff`), `placeBack` reads the
  whole order table, `allocate` every key of the prefix, and a changed corpus is copied into a new database whole;
- a read writes a lock record, a read whose rows go on holds the corpus lock until it is closed, and a second read waits
  for the first one's cursor; the database is `:memory:` behind `database/sql`, which may replace its connection with an
  empty database.

Each layer is a row, in this order, and each row's Red is a count of the cost counters of T36:

1. **Cost counters (T36).** The engine counts the files it reads, parses and compares, the rows it indexes and copies, and
   the rows a statement reads and writes in its database. A test runs a point read, and a point insert, update and delete
   on an order table and on a `key_prefix` table with sibling groups of a fixed size, and an external edit of one file,
   waited for by its notification, at 1,000 and at 100,000 rows; each count is equal at both sizes, and
   `testing.AllocsPerRun` differs by no more than a stated constant. These tests are the Red of the rows below and turn
   green row by row; benchmarks report the times and gate nothing.
2. **The commit generation (T37).** The journal publishes a generation in its declared place: a counter, and the paths of
   each commit for a stated number of generations back. A commit increments it at its commit record, after its renames and
   before its stage is collected, with that record's fsync, and so does a recovery that completes a commit. A corpus carries the generation it was
   read at. An external edit goes through no commit and has no generation; it is learned from the tree's notifications.
3. **An incremental corpus read (T38).** A read takes the paths the tree's notifications name since the last read and the
   paths of the commits since its generation, which it forgets in the tree first; a generation older than the stated
   number, and an overflow, read the tree again. `validate.Live` reads again only those folders and documents and keeps
   what makes one read depend on another in indexes it updates for them: the link targets of every key with the
   documents whose links asked for each (a changed target parses again exactly those documents), and the duplicate keys
   and unreadable entries that make a table unavailable. `irexec.FromSnapshot` shares every unchanged document with the
   corpus before. The incremental read equals the read from nothing, table-driven and fuzzed. The cross-file rules that
   are no read result — parent cycles, positions, references — are the commit's checks, which T40 reads by key.
4. **A copy-on-write post-image (T39).** A write's post-image shares the documents of the corpus it read and copies a
   document's rows when it first changes them. The commit reads its targets from disk, whatever the notifications said;
   two corpora read or written one from the other are compared where they differ, along the paths of the ordered map of
   their documents, whose shape is a function of its keys, so `Differs` walks the logarithm of the documents.
5. **Keyed row access (T40).** A stored row of a table is read by its key without reading the table; `placeBack` places
   the sibling group a write moved; `allocate` takes the largest key of a prefix from an index that the corpus keeps; and
   the commit's checks of the keys, references, parent cycles, positions, limits and unique columns of its target rows read
   the rows they need through indexes the corpus keeps per document and shares between corpora, not every row of a table.
6. **The difference of a statement is published (T41).** The database's writer connection is one `*sql.Conn` held for its life,
   on which SQLite's preupdate hook records which rows a statement, its triggers and its foreign key actions touched: table,
   operation, old and new rowid, and the old and new key. A key the hook reports is read with its length
   (`sqlite3_value_bytes`, through a fork of the driver, whose release stops reading a text at a NUL); a key whose text is not
   whole, or that breaks the rule of keys (spec 20 Keys, `store.keyFault`), fails the statement before any write is
   formed. The driver and its fork are Go, and the build needs no cgo. The temp schema is ignored, and the hook records nothing during a fill, a refresh or
   `placeBack`. The rows are coalesced by their final key, in the order the hook reports them, which is the order the rows
   a statement inserts are placed in; the old row is the corpus's stored row of its key (T40) and the new row is read by key
   with the projection of `rowsOf`; a key that changed is a delete and an insert. Each statement runs under a savepoint of
   the writer transaction, and a failure the callback records rolls back to that savepoint when the statement returns and
   fails it. The writes formed from the hook equal the writes `diff` forms, which stays as the oracle of a differential
   test, table-driven and fuzzed, over triggers, CASCADE, REPLACE, UPSERT, WITHOUT ROWID, rowid tables, key changes and
   statements that fail midway.
7. **A refresh of the changed rows (T42).** A database whose corpus differs takes the rows of the paths of the commits since
   its generation and of the paths the notifications named since its last refresh, and, when a table's availability
   changed, that whole table; a new catalog builds a new database. The refresh runs outside a transaction with foreign
   keys off, the standing triggers dropped and checks ignored, as the fill does. A row keyed by rowid keeps its stored rowid;
   the rowid of another row is the database's own, so a `SELECT` without `ORDER BY` has no stated order, as in SQLite. An
   AUTOINCREMENT counter is part of the corpus: after a write the statement reads `sqlite_sequence`, which the hook does not
   report, one row for each AUTOINCREMENT table, and the counters that rose go into the commit, to `dbspec/sequences.json`;
   a fill and a refresh take the larger of the stored counter and the largest key in the corpus, so no key is given twice,
   a key written by hand included, as SQLite's AUTOINCREMENT gives none. The refreshed
   database equals the database filled from the same corpus from nothing, table for table, by key, `sqlite_sequence`
   included.
8. **Reads without the lock (T43).** A read runs its statement on a reader connection and holds no corpus lock while it
   runs; a cursor holds a reader connection and no lock. The database is brought to the corpus before the read: when the
   lock is free, the read takes it for the refresh alone, so a lock record is written once for each change and not for each
   read; when a live holder has the lock, the read refreshes without it, consistent when the generation is equal before and
   after and no prepare record is present at either check, and otherwise answers from the database's generation, which
   the spec states, and reads again on the generation's notification, never on a timer; a prepared stage of a holder that
   has ended is recovered by the read, which takes the lock to do it, as a SQLite reader rolls back a hot journal, and the
   spec states this one write of a read. A database behind the generation a read found is refreshed once, by the read that
   finds it first, when the writer connection is free; a local transaction holding the writer connection means no other
   process can commit, so a read then answers from the database's generation, as stated. The database is a file private to
   the opening at `.flowmark/sql/<token>.db`, beside `<token>.holder`, the holder record of its process (host, pid and start
   time), so two openings of one process have two databases, with its `-wal` and `-shm`, in WAL mode
   with SQLite's automatic checkpoint (`wal_autocheckpoint` stated), one writer connection and a pool of reader connections
   of a stated size, a read that finds the pool full waiting on a release. The engine creates `.flowmark/sql` with access for its
   owner alone and checks it, the database's path and its `-wal` and `-shm` with lstat before the open, refusing a link and,
   on Windows, a reparse point; the driver opens with fixed flags and has no `SQLITE_OPEN_NOFOLLOW`, and a link planted
   between the check and the open needs the owner's own access to the folder, as stated. The recovery at open
   removes the files of processes of this host that have ended, as it removes their pending lock records; the files of
   another host's process are never removed, as stated. The writer connection holds a `BEGIN IMMEDIATE` transaction from a
   statement to the corpus commit, commits after the corpus commit is written and rolls back on every refusal; a SQLite
   commit that fails after a written corpus commit marks the database to be built again. A deferred transaction holds a
   reader connection from its first read and upgrades by comparing generations. A corpus the process may only read keeps
   its database in memory and its reads run one at a time, stated as such.
9. **The limits are stated (T44).** docs/spec/40-ops.md states the costs that T36 counts and the times the benchmarks
   measure at 1,000, 10,000 and 100,000 rows, as the limits of the path.

The unit tests of each row are dense: every rule a row adds has accepting and refusing cases, table-driven with its edge
cases, and each equality with the corpus or the database built from nothing is a fuzzed test whose seeds are the golden
fixtures. A row is done when its Red count of T36 is green, its tests pass, the lint and the gates of kit pass, and the spec
says what it changed.

**Ruling: a commit waits for the disk as few times as its atomicity needs.** A flush — the sync of a file or a folder, and
on Windows a write-through move — waits for the disk, from about a millisecond on Linux to tens of milliseconds with
`F_FULLFSYNC` on macOS and on a Windows runner, so the flushes of a commit, not its reads, bound its latency. The
protocol of T31 flushes each stage file and each name separately: a one-row commit makes 18 flushes, and a read that
takes the lock makes one, for its lock record. SQLite makes one or two for a commit with `synchronous=FULL`. Each row
below is counted by the cost counter of flushes, in TestCost, and its Red is that count:

1. **The flushes are counted (T55).** The journal adds each flush it asks of the disk to the counts of its corpus, which
   the write path shares; a corpus that does not wait (`--synchronous off`) makes none. TestCost pins the flushes of
   every operation at every size, so a flush added to a path is a failing count.
2. **A move waits only while the process waits (T56).** On Windows `--synchronous off` gives up `MOVEFILE_WRITE_THROUGH`
   as the other platforms give up their syncs.
3. **The lock record is not flushed (T57).** The lock appears whole through its link, so another process never reads a
   part of it; only a power loss leaves a lock whose record never reached the disk, and that power loss ended its
   holder. A lock record that is empty or holds only zero bytes is read as such a lock, a holder that has ended: an
   operation refuses at once, naming `flowmark unlock`, which removes it, as it removes the lock of a holder of an earlier
   boot. A read that takes the lock makes no flush.
4. **One record decides a commit (T58).** A commit writes and syncs a copy of each post-image, then one record that holds
   every rename with its pre- and post-hashes, the generation it takes, and the bytes of every pre- and post-image, synced
   with its folder: from the record on the commit is decided. The renames follow, a delete moving its target into the
   journal, and each folder of a target is synced once; then the generation is written as now, and the record is removed
   without a flush, since a record a power loss brings back names a generation that is written and is only collected. The
   commit record of T31 is gone: the generation says the renames are done. A one-row commit makes six flushes: the copy,
   the record and its folder, the target's folder, and the generation and its folder. A stage folder of an earlier engine
   is refused, naming the engine that completes it.

A row is done when its count in TestCost is the stated one and the crash tests at every step of the protocol pass.

**T9 conformance contract (stated now)**: markdown vectors are corpus-before + IR request + expected corpus-after / expected rows, run against the Go executor; SQL/binds comparison does not apply (there is no SQL). Cross-client comparison begins when other clients port the executor. (Revision 3: vectors run against the flowmark engine; orm's executor conformance pins a flowmark build.)

**Later phases (registered as `[ ]` checklist items only)**: T2 codec (Go), T3 validator, T4 executor, T5 orm markdown-executor integration (manifest settings, state-machine sub-grammar — orm-repo changes), T6 sync (the adapter contract and the database adapter; GitHub and Jira adapters register under it), T7 the three faces — CLI, MCP server and soksak sidecar (protocol v1: JSON lines over stdio, surface routing, host-owned lifecycle; the MCP tool list gets its mini-spec first, and the lock is held per operation or per server transaction (§5)), T8 checklist app (the local issue model), T9 conformance vectors, T10 the soksak plugin in its own repository — the local kanban board and issue tracker as one more thin consumer, with no view-specific write path.

## T61 — the checklist as a corpus

**Ruling.** A checklist is the corpus `docs/plans/<plan>/checklist/` of the catalog `dbspec/checklist.dbs` in that corpus, and the two files
that kit reads, `checklist.md` and `checklist.ko.md`, are views that the engine renders from it. An agent writes a row
and its state only through flowmark; `make checklist-check` fails when a view differs from what the corpus renders.

**Grounding.** The tables cannot be read as they stand, for three reasons:
- kit reads only the fixed path `docs/plans/<plan>/checklist.md` and its twin (K241);
- a corpus document is named `<table>.flowmark.md`, and its H1 is `# <table>`
  ([document model](../../spec/20-document-model.md), [canonical format](../../spec/10-canonical-format.md#document-order));
- a state cell of kit holds three values in one cell, `[!] cause: <cause>; retry: <condition>`, while a cell of a corpus
  holds one value.

Reading `checklist.md` as a corpus would need an exception to each of those rules, and the rules would then differ by
file. Changing kit's grammar would change the checklist of every polyspec repository for one consumer. A view keeps both
grammars whole, and the drift check makes the corpus the one source.

**The catalog.** `checklist.dbs` declares one table, `items`, with `store document table`:
- `id varchar(32)`, the key, matching the `idPattern` of `config/checklist.json`;
- `task`, `deliverables` and `verification`, as `text`, and the same three columns `_ko` for the twin. A twin holds the
  same rows, so one row holds both languages, and the view of each language takes its own columns;
- `position i32` with `order position`, so the view keeps the order of the rows;
- `state varchar(16)` with the check `state in ('waiting', 'active', 'verifying', 'done', 'bypassed', 'impossible')`.
  These are the state names of `config/checklist.json`, not its markers: the marker of the waiting state is `[ ]`, and a
  space at the end of a cell is not canonical;
- `cause text null`, `retry text null` and their `_ko` columns. A check ties them to the state: `bypassed` requires a
  cause and a retry, `impossible` requires a cause and no retry, and every other state has neither.

**The view.** It renders the header of kit's table from `config/checklist.json`, one row per item in order, and the state
cell as the marker that `config/checklist.json` declares for the state. It adds ` cause: <cause>; retry: <retry>` after
the bypassed marker, and ` cause: <cause>` after the impossible marker. The Korean view
uses the labels `원인` and `재시도`. Each view has the doc-id line and the H1 that the file has now. The twin's
`source-sha256` stamp is written by `make documents-stamp`, as for any twin.

**The states.** The engine and orm hold no transitions. polyspec fsm takes a state and writes it to the corpus with an
update of `state`, `cause` and `retry`. A state that the check refuses is a `CHECK` refusal of the write, never a
corrected value.

**The board.** A query of `items` by `state` answers it, and each row answers once.

**External edits.** An edit of a corpus file is read as every edit of a corpus is
([sync](../../spec/70-sync.md)). An edit of a view is drift, and the check fails on it, naming the view. The check never
rewrites the view.

**Order.** First this repository's checklist: the catalog, its cases (each check with an accepting and a refusing row),
the import of the current checklist into the corpus, the view and the check. Then each polyspec repository takes it
through a row of its own checklist.

## What the audit could not break

- Between two conforming flowmark writers, per-operation locking + stage/prepare/commit loses no update (the real holes were readers, humans, and crash windows — now ruled above).
- The dbspec-first derivation direction: the reverse direction genuinely creates the DDL-reconciliation conflict class this plan rejects. (Revision 3 keeps the direction and withdraws the standalone fragments: the catalog is the only schema.)
- IR expressiveness over a corpus: all kinds, relations (limit_per_parent, if_parent, flatten, key_by, composite keys), tuple_in with subqueries, group_count, restore and optimistic map to unambiguous in-memory definitions given type-directed ordering and PK-sorted determinism.

## Open questions

The open questions live in `110-open-questions.md` and are promoted into a specification when decided.

## Verification (T1a)

- kit branch `feat/fifth-state-K13`: `node --test tests/kit/checklist.test.mjs` 13/13, `make documents-check` green (verified in the worktree).
- flowmark `make kit-check` (lock intact), `make documents-check` (pairs, doc-id, anchors, fences, source-sha256), checklist reading with five states — green.
- `git log` (all repositories): conventional commit shape, feature-sized units, every `[o]` item accompanied by a changelog entry (flowmark) or its owning row (kit).
- Spec completion criteria (verifiable without implementation): cross-references resolve, one name per concept, every ruling a proposition with grounding, the two-clause idempotency stated as such.
- T1b can start with `[~]` on `10-canonical-format`.
