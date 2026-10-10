<!-- doc-id: plans-markdown-database -->
# The markdown database plan

[한국어](markdown-database-plan.ko.md)

The plan of the T12 items in `markdown-database-checklist.md`: the storage grammar, the relations,
the layering and the views of the markdown database. Each ruling is promoted into its
specification by T12.1; when a specification and this plan disagree, the specification wins and
this plan gets the correction.

## Context

The kanban desync fixes (T10-1 to T10-4) exposed that the markdown database had cases, not rules:

- **The three shapes were demo cases.** The list, table and detailed demos each grew a shape that
  no rule defines. The `## T2: title` sections of the detailed demo are read by no code, and the
  copied title has already drifted from the item (`작성1` against `작성`).
- **The model lacks its basic parts.** An item has no body column, no parent field, no relation
  and no history record; a child is guessed from the ID string, so `MoveItem` leaves the children
  behind and `EditTitle` drops the `@assignee` and due tokens.
- **The layering ruling is wrong.** The design plan (§1) placed the markdown executor in the orm
  repository, but orm is the query layer above databases, and the code never followed that
  ruling: orm holds two dbspec settings and flowmark imports nothing of orm.
- **Behaviours are hardcoded** (see the hardcoding inventory).

**Goal.** Markdown works as a real database: a finite and complete storage grammar maps every
database construct to a markdown construct or rejects it by name; each table chooses whether its
rows live in one file or one file per row, as database normalization chooses; orm reaches the
markdown database by DSN as it reaches any database; kanban, list and gantt are views over the
same rows.

**Settled.** The `.dbs` set is the only origin of the schema. A declared title is never empty; a
declared body is never NULL and may be empty; a view's entity table declares both. No backward
compatibility, no fallback, no hidden error, no trick; rules first.

## Rulings

### flowmark is the engine; orm reaches it as an IR executor

- Grounding (orm `docs/protocol.md`, `docs/config.md`): an orm client validates and plans in its
  own process and reaches an external database through the native driver its DSN scheme
  selects. orm contains no database engine. The markdown engine — parse, constraints,
  transactions, lock, journal — is storage-engine work: inside orm, each of the five clients
  would carry its own engine. As a separate engine there is one implementation.
- A SQL driver receives rendered SQL, which a markdown engine cannot take, so **orm's protocol
  gains IR executors** (orm protocol §5): a DSN scheme may select an IR executor instead of a SQL
  dialect. The client validates the request (`ir.Validate`), does not plan, and sends
  `{ir_version, manifest_hash, request, params, now}` per request; `params` use the runtime value
  model JSON (decimal as a string, bytes as base64, datetime as `YYYY-MM-DD HH:MM:SS.ffffff`
  UTC). Codec stages stay on the client, as for SQL. The executor answers a read with the rows
  already assembled to the request's shape (joins, relations, `key_by`, `flatten`) and a write
  with `{affected, keys}` — `keys` lists the generated or given primary key of each inserted row
  in insert order — or with one error of `errors.yaml`. The orm protocol item adds
  `WRITE_CONFLICT` (a target changed since it was read, naming the file and line, never retried
  by the client); a lock wait that ends is `CANCELED`, as a SQLite `busy_timeout` is. A hash
  mismatch is `SCHEMA_HASH_MISMATCH`, an `optimistic` update with no match is `OPTIMISTIC_LOCK`.
- A connection is a session. Besides requests it carries the control messages
  `{op: begin | commit | rollback | savepoint | release | rollback_to, name?}`, each answered
  `{ok}` or with an error. `readOnly` is honoured and takes the lock like any transaction; a
  transaction with audit values, `isolation` and `timeoutMs` are `CAPABILITY_UNSUPPORTED`.
- Every surface the executor lacks — `getQuery`, statement events, `force_index`, the lock,
  local-setting, privilege and AES-rotation utilities, schema install — is
  `CAPABILITY_UNSUPPORTED` in one capability table of orm `docs/dialects.md`. The Go client
  implements the IR executor first; every other client is an orm checklist item.
- The earlier recommendation to move the engine into orm was made without checking this and is
  withdrawn; the design plan's seam ruling is corrected.

### The server and transactions

- `flowmark serve <corpus>` is a resident face of the engine — spec 00's "no resident daemon"
  and spec 40's "no long-lived lock" are amended. It binds loopback TCP and writes a random token
  to `.flowmark/server.json` once — it is kept across restarts and replaced only by
  `flowmark serve --new-token`; the port is the required flag `--port`. The DSN is
  `markdown://127.0.0.1:<port>/<corpus>?token=<token>`, where `<corpus>` is the corpus root's
  directory name. A corpus or token mismatch is `CONFIG`.
- The portable lock (spec 40) is held for one operation, or for one transaction from `begin` to
  `commit` or `rollback`. The holder identity is `<pid>@<host>/<session>`. A closed connection, or
  a transaction idle longer than `tx_idle_ms` (a server flag), rolls the transaction back and
  releases the lock. Requests inside a transaction read the staged post-image; a savepoint is a
  nested stage. The CLI, MCP and sidecar wait on the same lock and fail with `LOCK_TIMEOUT`
  naming the holder. Spec 40 maps each face name to its orm code: `LOCK_TIMEOUT` to `CANCELED`,
  `CORPUS_CONFLICT` to `WRITE_CONFLICT`.
- A face (CLI, MCP, sidecar) retries a single-operation write once after a re-read; a server
  transaction is never retried by the server, and the orm client receives `WRITE_CONFLICT`.

### The catalog

- A corpus root is the directory given to the command or the server; it holds `dbspec/*.dbs` (the
  document set, the catalog) and `.flowmark/`. There is no upward search.
- Documents carry no schema stamp: the `dbspec/` set is the catalog, as a database keeps its
  schema once and never per row. Spec 30's stamp, standalone documents and embedded fragments
  are withdrawn.
- `flowmark install` generates the storage skeleton from the catalog — the folder of each `files`
  table and the document of each `document` table, and no block section, since an empty block is
  not written — as `CREATE TABLE` creates storage. Schema install
  through orm over `markdown://` is `CAPABILITY_UNSUPPORTED`.
- Introspection validates every document against the catalog and returns the catalog; a document
  that fails is a located diagnostic. A tool that drafts a catalog from a corpus without one is
  not part of this plan.
- **T8.5 is withdrawn**: a Mermaid block inside a document is no authority. `ParseMermaid`,
  `ApplySchema`, the `schema.mermaid` operation and the board's diagram strip are removed.
  Mermaid views (orm's `erDiagram` included) are not a present requirement.
- This repository is not a corpus: `make validate` and the prose ID checker are removed with the
  old engine; `make refs-check` keeps checking the kit-tracked checklists.

## The storage grammar

Every markdown-specific declaration is a dbspec **manifest setting**, after the precedent of the
`markdown <column>` setting: the SQL backends ignore it and introspection returns a plain column.
flowmark uses orm's Go `engine/dbspec` and `engine/ir` packages as libraries and never
reimplements them.

### Storage

- Exactly one per table, or a schema error:
  - `store files` — one file per row in the folder named after the table, under the corpus root.
    An insert writes `<table>/<key>.md`; the engine never renames a file, and a key change keeps
    the file.
  - `store document list` / `store document table` — every row in the one file `<table>.md` at
    the corpus root: `<!-- doc-id: <table> -->`, `# <table>`, then the list or table directly
    under the H1 and the detail sections at H2. A doc-id is unique across the rows and
    `document` files of the corpus; its twins share it.
  - `store block <fk> list` / `store block <fk> table` — the rows inside the scope of the parent
    row that the named foreign key references.

### Keys

- A markdown table has a single-column primary key; a composite key is a schema error.
- A `files` row's key is its `<!-- doc-id: -->` value; the file name carries no meaning.
- `identity` is allowed on every markdown table: an insert that omits the key gets `max+1`,
  allocated under the lock; reuse after a git rewind is accepted and stated. A text key with
  `key_prefix <p>` is allocated `<p><n + 1>`, where n is the largest decimal suffix without a
  leading zero among the keys of the form `<p><n>`; orm treats such a column as
  executor-assigned, as with `identity`, so an insert may omit it. A text key without
  `key_prefix` must be given by the insert. A `key_prefix` column is a `varchar` (dbspec keys no
  `text` column). A key written by hand is accepted when it is unique and has the column's type;
  a key not of the form `<p><n>` does not count toward n. A key in a list or table storage
  contains no space, `#`, `|` or line feed.
- Keys are opaque: no ID grammar exists, and no relation is read from a key. The suffix grammar
  (`T8.0.14-1`), `taskIDLess` and spec 10's "Task IDs and ordering" are removed.

### Title and body

- `title <column>` and `body <column>` are optional per-table settings. A `list` storage and a
  `files` storage require `title` (the line text, the H1); a `table` storage may declare it as a
  cell. The title is non-null and refuses the empty text.
- `body` is a `markdown` column and never NULL. It lives in the row scope in every storage: the
  text after the H1 of a file, or the text of the row's detail section before its first
  sub-heading; a row without a detail section has the empty text as its body.
- A `files` row is, in order: the header comments, the frontmatter, the H1 title, the body, then
  the level-2 sections (spec 10's document order is amended). The body and every `markdown`
  section value end at the next heading of level ≤ L, counted only outside fenced blocks
  (CommonMark block structure); such a value contains only headings deeper than L, and a write of
  a value with a heading line of level ≤ L outside a fence is `CONSTRAINT`.
- A view of kind kanban, list or gantt requires its entity table to declare both; `flowmark.dbs`
  declares them on `items`.

### Row scope, sections and depth

- A `files` row's scope is its file (level L = 2 under the H1 title). A block storage's section is
  a heading at level b, and its rows' detail sections `<h> <key>` are at h = b + 1; a document
  storage's detail sections are at h = 2. A detail row's scope is at L = h + 1. The heading holds
  the key only, never a title copy; a detail section whose key names no row of its block is
  `format.block`. Detail sections of nested list rows are flat siblings.
- A `markdown` column is always a section of the row scope — never a field or a cell — in every
  storage.
- In a row scope at level L, each nullable `markdown` column and each child block is a heading at
  level L named by the column or the child table. An absent section is NULL; a present empty
  section is the empty text. A child table named like a markdown column of its parent is a schema
  error. A key containing `#` or a line feed is refused in a block or document table. A layout
  that needs a heading past H6 is a schema error.

### Scalar columns

- `files`: a frontmatter key per column.
- `table` (block or document): a cell under the column-named header; every stored column has a
  header.
- `list` (block or document): `- [<glyph>] <key> <title>[ <field>…]`, or
  `- <key> <title>[ <field>…]` without a state column. Fields are parsed from the right: a token `<declared column>:<value>`,
  the value a bare token without space, `"` or `\`, or `"…"` with `\"` and `\\` as escapes. A
  table cell writes `|` as `\|`. An absent field is NULL; `col:""` is the empty text; a
  non-null column is written — the engine writes a default explicitly on insert, and a row
  missing a non-null field is `format.block`. A title token matching `<declared column>:` is
  written `<column>\:`. The `@` and date sigils are removed.
- A value with a line feed in a single-line place (a list title or field, a table cell) is
  `CONSTRAINT` on write. The placement foreign key and `parent` are never written in a block row;
  writing one is `format.block`.

### State, hierarchy and order

- The state column is the `state_machine` column. A `list` storage with a state column declares
  `checkbox <column> <state> '<glyph>'` for every state, each glyph distinct.
- Hierarchy is a self foreign key `parent`: in a list the indentation is the parent; a table or a
  file writes the column. No parent is guessed; a cycle is refused; a `parent` outside the row's
  block is `CONSTRAINT` on write and `format.block` on read.
- `order <column>` names a non-null `i32` or `i64` column with no default, in no key, index or
  check. Its value is the row's 1-based position among the rows with the same placement parent
  and the same `parent`. In block and document storages it is never written — derived from line
  order on read, `format.block` when written; in `files` storage it is a frontmatter field whose
  sibling values must be exactly 1..n (a gap or a duplicate is a validate diagnostic). An insert
  may omit it — orm treats an order column as executor-assigned, so the client sends no value —
  and the row appends; with k it places the row at k. An update to k moves the row and the
  siblings follow in the same transaction. k < 1 or k > n + 1 is `CONSTRAINT`, never clamped.
  An update that assigns `order` matches exactly one row, otherwise `CONSTRAINT`. Changing
  `parent` without `order` appends to the new siblings, and the old siblings close the gap.
- Without an `order` column, the lines among siblings (the same placement parent and the same
  `parent`) stand in key order (numeric for integer keys, codepoint for text) and each child
  follows its parent; any other line order is `format.noncanonical`.
- Storage order is the pre-order of the tree: each row, then its children in sibling order (the
  order column, otherwise the key); rows of different placement parents follow their parents'
  storage order. A read without an IR `order` returns storage order; an IR `order` on the order
  column sorts by its integer value with ties in storage order. Moving a row moves its subtree.

### Relations, links and history

- Every foreign key is restrict-only; `cascade` and `set_null` are rejected. A one-to-many across
  files stores the child table with `store files` and writes the foreign key in its frontmatter.
- Many-to-many is a link table with an `identity` key, such as
  `item_links(id, from_item, to_item, kind)` with
  `check ck_item_links_kind (kind in ('blocks', 'relates', 'duplicates', 'supersedes'))`.
  A gantt dependency edge is `blocks`; superseding a done item inserts a new item and a
  `supersedes` link.
- `audit` and `immutable` are schema errors on a corpus — markdown has no triggers. Instead,
  `state_machine <column> history <table> row <fk column> from <column> to <column> at <column>`
  makes every transition the engine executes insert one history row in the same transaction. The
  setting names the history table's columns (a column's name never selects a behaviour): the
  `<fk column>` has a foreign key to the state table with its leading index, `from` and `to` have
  the state column's type, `at` is `datetime(6)`, and one nullable column per required column has
  that column's name and type; there is no other column besides the `identity` key, and any
  mismatch is a setting error at the history table's name. The history table is stored
  `block <fk> table` and declares no `title` or `body`.
  Only transitions write history; an insert writes none. A row that has history rows is refused
  delete with `FOREIGN_KEY` (restrict-only).
- On a corpus a table with `state_machine` declares its `history`; without it the schema is an
  error.
- `state_machine <column> initial <state>` may repeat; on a corpus a machine has at least one. A
  machine's states are the states its `initial`, transition and `terminal` lines name; `checkbox`
  covers that set exactly, and `checkbox`, `limit` and `initial` naming another state is a
  setting error. An insert's state is an initial state, and a written state outside the set is
  `CONSTRAINT`. A `default` of the state column is an initial state, otherwise a setting error; an
  initial state that is also terminal is a setting error.
- A row has a history gap when its state differs from the `to` of its last history row, or,
  without history rows, when its state is not an initial state — a human edit bypassed the
  engine. Validate reports `history.gap` at the row's line, and `history.chain` when a history
  row's `from` differs from the previous row's `to` (on the first row, is not an initial state) or
  its pair is not a declared transition. Neither blocks another write; an engine transition of a
  gapped row is `CONSTRAINT` until the gap is closed, while updates of its other columns are
  allowed. A state change outside the engine is therefore reported, unless a human forges a
  consistent history.
- `flowmark history repair <table> <key> [--from <initial state>]` inserts one history row from
  the last `to` — for a row without history, the one initial state, or `--from`, required when
  the machine has several — to the current state, with `at` from the engine clock and the require
  columns copied from the row. It refuses when the pair is not a declared transition, when a
  column the transition requires is null or empty on the row, or when the current state's `limit`
  is exceeded counting this row; the human then restores a legal state or fills the columns by
  editing. The repair is a normal transaction that the human commits.
- A commit first checks that its targets are unchanged since they were read (`WRITE_CONFLICT`
  otherwise); the constraint checks, `history.gap` included, then run on the corpus re-read under
  the lock. A gap on another row never fails the commit.
- A transition, or an entry into a terminal state, whose line has `require (c1, …)` must assign
  every listed column in the same update; an unassigned column, `null` or the empty text is
  `CONSTRAINT` naming the column. The history row copies the assigned values, and the state table
  keeps them.
- `state_machine <column> limit <state> <n>` counts the table's rows in that state. A transition,
  insert or update that would exceed it is `CONSTRAINT` naming the limit; a corpus already over
  it (a human edit) is a validate diagnostic that blocks no other write. The relative rule
  `doing < done + 2` is removed — its limit vanishes as done items accumulate.

### Translations

- A twin is a file `<name>.<lang>.md` whose doc-id equals a row's; it is not a row and the engine
  never writes it. The language list is the corpus setting `translations` in `dbspec/flowmark.json`,
  which kit's `config/documents.json` mirrors; a row whose source changed makes its twin stale
  under kit's `source-sha256` rule.

### Construct coverage

| dbspec construct | On a markdown corpus |
| --- | --- |
| types | every type, in the dbspec canonical literal form in every storage; `bytes` as base64 |
| primary key | single column; composite is a schema error |
| unique key, check | enforced at commit; a check whose predicate is unknown (NULL) passes and a unique key treats NULLs as distinct, as in SQL |
| index | accepted; derived in memory |
| default | written explicitly on insert |
| `null` | an absent field, cell or section, as the grammar states per storage |
| `identity` | allowed; `max+1` under the lock |
| foreign key | restrict-only; `cascade`, `set_null` rejected |
| `codec` | applied by the client on an orm request; a write the engine originates (CLI, MCP, sidecar, history) applies it through orm's Go codec package, and validation decodes with the same package; the stored value is the encoded one (a byte-producing stage as base64) |
| `aes`, `aes_version`, `blind_index` | schema error — the key never leaves the client |
| `soft_delete`, `updated`, `default now` | assigned by the engine: the request's `now` for an orm request, otherwise the engine's UTC clock truncated to microseconds |
| `select explicit`, `entity`, `navigation` | client-side; no storage effect |
| `audit`, `immutable` | schema error; `state_machine … history` instead |
| `state_machine`, `markdown` | enforced by the engine |
| `use` documents | within the `dbspec/` set; an external document is a schema error |

### The application schema and the views

- `flowmark.dbs` is the standard application schema, extended by adding columns: `items` (`id
  varchar(32)` with `key_prefix 'T'`, `parent varchar(32) null`, `state`, `title varchar(255)`
  with `title title`, `body text` with `markdown body` and `body body`,
  `assignee varchar(32) null` → people, `start_on date null`, `start_at datetime(0) null`,
  `due_on date null`, `due_at datetime(0) null`, `priority i16 null`, `position i32` with
  `order position`, `evidence text null`, `cause text null`, `retry text null`, with
  `markdown evidence`, `markdown cause`, `markdown retry`) and the checks
  `start_on is null or start_at is null` and `due_on is null or due_at is null`, plus `people(id
  varchar(32), name varchar(255))` with `key_prefix 'P'` and `title name`, `item_links(id i64
  identity, from_item varchar(32), to_item varchar(32), kind varchar(16))` with the check
  `ck_item_links_kind`, `item_history` and `views`. Every foreign key (`parent`, `assignee`,
  `from_item`, `to_item`, `item`) has its leading index. A NULL assignee is self — no name is ever
  guessed for it (the T11 ruling).
- The two columns per point in time keep the T11 ruling that a due may carry a time: no dbspec
  type holds "date or datetime", and a datetime-only column would turn a date-only due into an
  ambiguous midnight. A gantt draws a date-only start from the start of its day and a date-only
  due through the end of its day.
- The `items` machine is declared whole: `state varchar(16)`; `state_machine state initial
  waiting`; the transitions, terminal and `require` lines of spec 100 (waiting → doing; doing →
  done require evidence; doing → bypassed require cause, retry; bypassed → doing; waiting →
  impossible require cause; done and impossible terminal); `checkbox state waiting '[ ]'`,
  `doing '[~]'`, `done '[o]'`, `bypassed '[!]'`, `impossible '[-]'`; `history item_history row
  item from from_state to to_state at at`. `item_history(id i64 identity, item varchar(32),
  from_state, to_state, at, evidence, cause, retry)` declares the same three `markdown` columns
  and has `foreign key fk_history_item (item) references items (id)` with its index. A
  multi-line evidence is a section (`items.md` H1, `## T1`, `### item_history`, `#### <id>`,
  `##### evidence` — within H6).
- `flowmark.dbs` declares no `limit`: the owner's WIP rule — completion before starting, with
  independent items allowed in parallel — is a working rule that no count expresses, and a
  number would be invented. The `limit` setting stays for a schema that names one. The engine
  enforces no WIP rule: the relative gate (`app/apply.go:40,178`) is removed, the owner's rule
  stays a working rule in AGENTS, and spec 100's WIP-gate ruling and design-plan §9's "column
  limit" are corrected in T12.1.
- Storages of `flowmark.dbs`: `items` `store document list`; `people`, `item_links` and `views`
  `store document table`; `item_history` `store block fk_history_item table`.
- A view is data: `views(id i64 identity, title varchar(255), body text, kind varchar(16),
  entity varchar(63), filter text, sort text, group_by varchar(63) null)` with
  `check ck_views_kind (kind in ('kanban', 'list', 'gantt'))`, `codec filter ordered_json`,
  `codec sort ordered_json`, `title title`, `markdown body` and `body body`. `filter` holds an
  IR `Group` and `sort` an IR `Order` list, validated with `ir.Validate` against the runtime model
  on write and on validate; a view made invalid by a schema change is a located diagnostic. A
  filter saved in the GUI never edits the schema.
- A kanban drop sets `order` only when the view has no `sort` (storage order) and both neighbours
  share the card's parent; a drop across columns changes the state. Any other drop is refused
  and the board says why — it never moves a card it does not store. After an accepted drop the
  board redraws from the rows the operation returns, never from the drop position; a drop across
  columns places the card where the view's sort puts it.

## Layering

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── IR executor ── DSN markdown:// ──┐
                                                                                          ▼
flowmark = the markdown database engine (one Go binary)
   engine: codec · validator · IR executor · write path (lock, journal, transactions) · history
   app:    application schema (flowmark.dbs) · install (catalog → skeleton) · sync
   faces:  server · CLI · MCP · soksak sidecar — thin faces of one engine
   uses:   orm engine/dbspec and engine/ir as libraries
                                                                                          ▲
soksak plugin ── sidecar protocol v1 ────────────────────────────────────────────────────┘
   views (kanban, list, gantt) · detail modal · no write path of its own
```

## Steps

Each repository runs its own checklist; the work in orm and in the plugin is tracked there and
named here.

- **flowmark** (`markdown-database-checklist.md`):
  - T12.1 the rulings and specifications 00, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100 and 110,
    the design plan's seam correction and the T8.5 withdrawal;
  - T12.2 the storage codec with P1/P2 for every storage (requires the orm settings);
  - T12.3 the validator (keys, foreign keys, cycles, title, block shape, order, limits, history,
    views);
  - T12.4 the IR executor under the IR-executor protocol (params, assembled relations, write
    answers, errors; requires the orm protocol item);
  - T12.5 the write path: lock, journal, transactions, savepoints, history and its repair, order
    moves;
  - T12.6 introspection;
  - T12.7 sync on the runtime model — the export renders through orm's SQL dialect renderer;
  - T12.8 the conformance vectors rewritten for the grammar;
  - T12.9 the server (`serve`, token, session control messages; requires the orm protocol item);
  - T12.10 the CLI, MCP and sidecar operations over any table, and `flowmark install`;
  - T12.11 the demos as one dataset in three storages, and the hardcoding inventory at zero.
  Each item removes the old path it replaces.
- **orm** (its own checklist): the protocol's IR executors and capability table; the dbspec
  settings `store`, `key_prefix`, `title`, `body`, `checkbox`, `order`,
  `state_machine … limit`, `state_machine … history` and `state_machine … initial`, with the
  corpus rejections and the corpus requirement of `history` and `initial`; the Go
  `markdown` IR executor with the conformance against a flowmark build pinned in
  `tests/markdown/flowmark.version`; one item per other client.
- **plugin** (its own checklist): view switching — kanban, list with group, sort and filter,
  gantt with the date pairs and `blocks` edges; the detail modal (every column, body editing,
  children, links, history); the hierarchy; separated UI strings; answers judged by structure.

Each item runs red, green, `[o]` with the changelog pair, and the commit; uncommitted changes
cover one item.

## Findings removed by the rewrite (never patched)

- `EditTitle` drops `@assignee` and the due; `MoveItem` leaves the children; the continuation
  check of `ApplySchema` is always false (`app/schema.go:100`); a table document bypasses the WIP
  gate; the validator registers no table-form ID, so duplicates pass and prose references read as
  dangling; transition evidence is never stored.
- An unknown required field passes `requiredMet` unchecked; the frontmatter is parsed and read by
  no code.
- The lock and the journal of spec 40 are not implemented.

## Hardcoding inventory (every item read from the catalog or removed)

| Place | Hardcoded |
| --- | --- |
| `app/apply.go:15,40,178` | the WIP glyphs `[~]` and `[o]`, and `WIPHeadroom = 2` |
| `app/apply.go:159-167` | the names `evidence`, `cause`, `retry` and substring checks |
| `app/apply.go:315`, `corpus/validate.go:24` | the ID prefix `T` |
| `executor/executor.go:217` | `taskIDLess`, the ID-grammar ordering |
| `codec/blocks.go:71`, `executor/executor.go:112` | table kinds guessed from column names |
| `codec/blocks.go:24-45` | the `@assignee` and date sigils |
| `app/schema.go` | the in-document Mermaid (removed) |
| `sidecar/loop.go:351` | `firstFlowmarkFile` |
| `sync/export.go` | the `documents` and `items` tables and hand-rendered SQL |
| `corpus/validate.go:51`, `**/*.md` | the twin suffix and the selection |
| plugin `ui/board.js` | UI strings in code, answers matched by sentence |

## Verification

- orm: the dbspec rule tests and the IR-executor protocol tests pass; the Go executor passes the
  conformance over every IR kind against the pinned flowmark build.
- flowmark: P1/P2 for every storage; one dataset in three storages yields the same rows; the
  conformance vectors (corpus before, request, corpus after) pass.
- Introspection: a valid corpus returns its catalog; an invalid document is a located
  diagnostic.
- Write path: `go test -race`; multi-process writes; a CLI write attempted mid-transaction waits
  and fails with `LOCK_TIMEOUT` naming the server session; a human edit landed mid-transaction
  makes the commit fail with `CORPUS_CONFLICT` naming the file and line; crash simulation with
  journal replay; idle and closed-connection rollback.
- GUI: kanban, list, gantt and the modal captured through the observation server and headless
  Chrome; a refused transition returns to its place.
- Documents and hardcoding: `make documents-check`, `make refs-check` and `make kit-check` pass,
  and a grep of the inventory finds nothing.
