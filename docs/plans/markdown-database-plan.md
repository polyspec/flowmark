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
- `flowmark install` adds the line `*.flowmark.md text eol=lf` to `<corpus>/.gitattributes`, creating the file when it is missing and keeping its other lines, and generates the storage skeleton from the catalog — the folder of each `files`
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

## The readable document

- **A corpus document is a file whose name ends with `.flowmark.md`**, under the corpus root and
  outside `dbspec/`, `.flowmark/` and hidden entries. The name is the only criterion that makes a
  file part of the database; every other file is outside it and draws no diagnostic, except a name whose suffix is `.flowmark.md` without regard to case, which is `corpus.walk`, and an entry at a `files` table's place that is not a folder.
- **Every corpus document is a complete markdown document** that a reader who does not know
  flowmark reads as it is: no comment, no marker, no hidden field. Its structure is ordinary
  markdown — headings, lists, tables, a field list — and every relation is a link the reader can
  follow in GitHub, GitLab, VS Code or MkDocs, whose anchors are the heading anchors of their rules. Pandoc is not a viewer the claim is made for: its default does not rewrite relative link targets. Grounding: the documents are the database's only storage, so
  they must stay a readable document; what only the machine needs belongs to the catalog
  (`dbspec/`) or to `.flowmark/`.
- Encoding: UTF-8 without a BOM, LF, exactly one final LF; a BOM, a CR, a missing final LF and a trailing blank line are `format.noncanonical`; invalid UTF-8 and U+0000 are `format.encoding`; a value never begins or ends with a line feed or a white-space-only line; a logical write that rewrites a file with a `format.*` diagnostic is `CONSTRAINT`, naming `flowmark canon` for `format.noncanonical`; a delete that removes a `files` row's whole file is exempt from the rewrite refusal, but its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child and history blocks, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable. A table that holds a file with `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows, or two rows with byte-for-byte equal keys, is unavailable: every request or check that needs its rows fails as `CONSTRAINT`, naming the file and the diagnostic; a block table holds its placement parent's files, recursively. Opening a corpus, introspection, `flowmark validate` and `flowmark canon` never fail on an unavailable table, and an export that needs one fails and writes nothing.

- Withdrawn with this ruling: the `<!-- doc-id -->` header, the provenance comment, the
  frontmatter of a `files` row, the twin rule and the `translations` setting with
  `dbspec/flowmark.json`. The sync provenance of an adapter-owned document moves out of the
  document; where it lives is decided with the adapter work (open question).

## The storage grammar

Every markdown-specific declaration is a dbspec **manifest setting**, after the precedent of the
`markdown <column>` setting: the SQL backends ignore it and introspection returns a plain column.
flowmark uses orm's Go `engine/dbspec` and `engine/ir` packages as libraries and never
reimplements them.

### Storage

- Exactly one per table, or a schema error:
  - `store files` — one file per row, `<table>/<key>.flowmark.md` under the corpus root. The file
    name is the key; a key change renames the file in the same transaction.
  - `store document list` / `store document table` — every row in the one file
    `<table>.flowmark.md` at the corpus root: `# <table>`, then the list or table directly under
    the H1 and the detail sections at H2.
  - `store block <fk> list` / `store block <fk> table` — the rows inside the scope of the parent
    row that the named foreign key references.
- A missing table folder (`files`) or a missing document file (`document`) is an empty table: zero
  rows and no diagnostic. Grounding: git keeps no empty folders, and an absent folder or file holds the rows
  that an empty one holds, none. So its absence is one form of zero rows, not a corpus defect. An insert creates the folder or the file; a delete of the last row of a
  `document` table leaves `# <table>` and one LF.

### Keys

- A markdown table has a single-column primary key; a composite key is a schema error. Its type is an
  integer, `bool`, `varchar`, `uuid` or `date`, and any other type is a schema error: `time`,
  `datetime`, `decimal` and `f64` hold characters the key shape refuses, and `text` and `bytes` are
  refused by dbspec's key rule. A negative integer key is refused: `CONSTRAINT` on write and
  `format.block` on read.
- A `files` row's key is its file name without `.flowmark.md`.
- **Every key is a file name, a URL path segment and part of a heading anchor at once**: it
  consists of `A`–`Z`, `a`–`z`, `0`–`9`, `_` and `-` only — ASCII, since the comparison and the
  lowercased anchor are defined for these letters alone — begins with a letter or a digit (a leading
  `_` would render as emphasis and alter both the shown key and its anchor), holds no `--` or `-_` (the MkDocs viewer collapses hyphen runs and would alter the
  anchor; a `_` after punctuation, as in `-_`, can open emphasis),
  and is unique within
  its table without regard to case (case-insensitive file systems and lowercased anchors would
  otherwise collide); a key equal to another key of its table without regard to case is `DUPLICATE_KEY` on a write that introduces it and `key.duplicate` in the validator. A foreign key matches the referenced key byte for byte; a link text that
  differs only in case is a dangling reference: the validator's `reference.dangling`, and `FOREIGN_KEY` on a write that introduces it; two rows with byte-for-byte equal keys make their table unavailable. A key or a table name equal, without regard to
  case, to `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9` or `LPT0`–`LPT9` is refused — a table name as a
  schema error, a key as `CONSTRAINT` — since those names are reserved file names on Windows; a
  `files` key is at most 243 bytes, so its file name stays within 255. A file name that is not the
  canonical text of its key's type (`007.flowmark.md` for an integer key) is `format.block`. A file
  name whose key breaks the key shape is `format.block` on read. A key change that differs only in case renames through an intermediate name inside the journaled
  transaction.
- `identity` is allowed only on an `i64` primary key column: an insert that omits the key gets `max+1`,
  allocated under the lock; reuse after a git rewind is accepted and stated. A text key with
  `key_prefix <p>` is allocated `<p><n + 1>`, where n is the largest decimal suffix without a
  leading zero among the keys of the form `<p><n>`; orm treats such a column as
  executor-assigned, as with `identity`, so an insert may omit it. A text key without
  `key_prefix` must be given by the insert. A `key_prefix` column is a `varchar` (dbspec keys no
  `text` column). A key written by hand is accepted when it is unique and has the column's type;
  a key not of the form `<p><n>` does not count toward n.
- Keys are opaque: no ID grammar exists, and no relation is read from a key. The suffix grammar
  (`T8.0.14-1`), `taskIDLess` and spec 10's "Task IDs and ordering" are removed.

### Title and body

- `title <column>` and `body <column>` are optional per-table settings. A `list` storage and a
  `files` storage require `title` (the line text, the H1); a `table` storage may declare it as a
  cell. The title is non-null and refuses the empty text. A title in an H1 whose final run of `#` follows a space or a tab, or that consists only of `#`, is written with its last `#` as `\#`; on read a final `\#` is the title's `#`, and an H1 closing sequence is `format.noncanonical`; a title ending in `\#` is `CONSTRAINT` on write, since it would read back as `#`.
- `body` is a `markdown` column and never NULL. It lives in the row scope in every storage: the
  text after the H1 of a file, or the text of the row's detail section before its first top-level heading (outside any container) of level ≤ h + 1; a row without a detail section has the empty text as its body.
- A `files` row is, in order: the H1 title, the field list of its stored scalar columns when it has
  one, the body, then the level-2 sections (spec 10's document order is amended). The body and every `markdown`
  column value of a row scope at level L end at the next top-level heading of level ≤ L, counted only
  outside fenced blocks and outside any container, a block quote or a list item (CommonMark block
  structure). A value holds only top-level headings deeper than L; a write of a value with a top-level
  heading line of level ≤ L outside a fence is `CONSTRAINT`. A value holds no top-level setext heading:
  a line of only `=` or only `-`, of at most three spaces of indent and any trailing spaces or tabs,
  outside a fence and outside any container, directly after a line of a top-level paragraph, is
  `CONSTRAINT` on write; on read, a setext heading is a heading where CommonMark reads one. A setext
  heading inside a container is not within this rule. A value that leaves a fence or an HTML block of
  CommonMark types 1–5 open is `CONSTRAINT` when an element follows it, whichever write places that
  element.
- A view of kind kanban, list or gantt requires its entity table to declare both; `flowmark.dbs`
  declares them on `items`.

### Row scope, sections and depth

- A `files` row's scope is its file (level L = 2 under the H1 title). A block storage's section is
  a heading at level b, and its rows' detail sections are at h = b + 1; a document storage's detail
  sections are at h = 2. A detail row's scope is at L = h + 1. Detail sections of nested list rows
  are flat siblings.
- A block or document row has a detail section exactly when its body is not empty, its table has a non-null `markdown` column other than the body, a nullable `markdown` column of it is not NULL, or it has a child block row; a detail section without any of them is
  `format.noncanonical`.
- **A detail heading is `<h> <table> row <key>`** — the table name, one space, the word `row`, one
  space, the key; never a title copy — and its anchor is `<table>-row-<lowercased key>`, so
  `## items row T2` has the anchor `items-row-t2`; the table is the anchor text before its first
  `-`, as table names hold no `-`. The collision rule: a heading of the file — every ATX heading, also after a block-quote or list-item
  marker, and every paragraph followed by a setext underline (its whole text), outside fences, skeleton
  and payload alike — other than a detail heading takes the anchor A of a detail heading when, under
  either of two folds of its rendered inline text, the fold of the heading and F(A) are equal once a
  final run of decimal digits is removed from each. F_G removes every character not of Unicode category
  L or Nd, lowercases, decomposes by NFKD, drops the combining marks and keeps the ASCII letters and
  digits; F_M lowercases, decomposes by NFKD, drops the combining marks and keeps the ASCII letters and
  digits. The folds take the text as GitHub slugs it (link text without its target, emphasis markers
  removed, code span content kept, entities decoded). F_G covers GitHub, GitLab and VS Code, and F_M
  covers MkDocs; Pandoc is not claimed. The rule covers every numbering: GitHub, GitLab and VS Code
  append `-<n>`, and MkDocs appends `_<n>` or raises a final `_<n>`, counting ids that attribute lists
  set as taken; comparing without the final digits covers all of them whichever heading or id came
  first, at the cost that a payload heading differing from a detail heading only in its final number is
  refused. Two detail headings collide only when their anchors are equal. A payload attribute list
  `{…}`, with or without a leading `:`, that sets the id A by `#A` or by `id=` followed by A bare or in single or double quotes, and a payload
  HTML `id` or `name` equal to A, without regard to case, collide too. A write that introduces a collision — a title update, a payload write, a row insert, a key update, a move or the creation of a detail section — is `CONSTRAINT`, naming the other heading, except a write giving a row a key equal (with or without regard to case) to another key of its table, which is `DUPLICATE_KEY`, the key rule; on read it is `format.block`, except between the detail headings of two rows whose keys differ only in case, which is `key.duplicate` alone. A detail heading in a
  non-canonical spelling (extra spaces, a wrong case of `row`) is `format.noncanonical`, which
  `flowmark canon` repairs. A detail section whose key names no row of its block is `format.block`, and so is a second detail section of the same row, whatever its spelling: its body and sections cannot be assigned.
- A `markdown` column is always a section of the row scope — never a field or a cell — in every
  storage.
- In a row scope at level L, each `markdown` column other than the body and each child block is a
  heading at level L named by the column or the child table. A nullable column's section is absent
  exactly when its value is NULL, and a non-null column's section is always present, so its absence
  is `format.block`; a present empty section is the empty text. A child table named like a markdown column of its parent is a schema
  error. A layout that needs a heading past H6 is a schema error.
- A row scope's sections stand in a fixed order: the `markdown` columns other than the body, nullable
  and non-null alike, in column order,
  then the child blocks in table name order (dbspec's canonical order). Detail sections stand in
  the storage order of their rows.
- Any content after a block's or document's list or table other than its rows' detail sections
  — up to the next top-level heading of level ≤ b in a block section, or to the end of a document file — is
  `format.block`.

### Scalar columns

- **Values are written in a readable form**, not as dbspec literals: text as it is; integers,
  decimals and floats in their canonical number form; `true`/`false`; a date `YYYY-MM-DD`; a
  datetime `YYYY-MM-DD HH:MM:SS[.f…]` and a time `HH:MM:SS[.f…]` with exactly the column's
  precision in fraction digits; a lowercase uuid; bytes as padded base64; a foreign key as a link
  (below). The type always comes from the catalog. The engine reads a number or `true`/`false`
  with orm's `dbspec.CanonicalLiteral(t, v)`, and a date, time, datetime or uuid with
  `CanonicalLiteral(t, "'" + v + "'")` with the quotes removed from the result; `now` is dbspec's keyword for the current time in a `datetime` default and is never a stored value (the engine writes the time it denotes on insert); a temporal value written `now` is not of its type (`type` on read, `CONSTRAINT` on write), and a text value `now` is text;
  text is checked against its column's length, and in a single-line place by the rules below. A
  value whose written form differs from its canonical form is `format.noncanonical` on read; a value not of its column type is the validator's `type`, and `CONSTRAINT` on write. On write, a number, `true`/`false`, a date, a time, a datetime or a uuid is written as `CanonicalLiteral` gives it (`1.5` into `decimal(4,2)` is `1.50`; a float in fixed notation with the fewest digits that round-trip, zero as `0`), and a value `CanonicalLiteral` refuses is `CONSTRAINT` (a decimal with more fraction digits than its scale, zeros included, or more integer digits than precision − scale; a time or datetime with more fraction digits than its precision); bytes are written as padded standard base64 (RFC 4648 §4), and on read any other text in a bytes column is `type`; a text or `markdown` value is written byte for byte, apart from its place's escapes, and one that cannot be is `CONSTRAINT`; a request that evaluates such a value fails as `CONSTRAINT`. A write never normalizes document bytes.
- **Single-line places** are a title (in a list row, an H1 or a cell), a list field, a field-list
  value and a table cell. A value there with a line feed, with leading or trailing white space, or
  containing `<` followed by `!`, `/` or a letter (raw HTML would hide or alter it) is `CONSTRAINT` on write. On read, a single-line value that renders differently from its text (a quoted list field or cell with leading or trailing white space, or `<` followed by `!`, `/` or a letter) is `format.block`, and white space that rendering drops around it is `format.noncanonical`. Each place has one canonical separator (`# ` after the H1 marker, `: ` after a field-list column or nothing for the empty text, one space between list tokens, `| v |` in a cell); any other white space there is `format.noncanonical`. A single-line value is inline markdown as written and is never escaped except as stated.
- `files`: a field list right after the H1 — one item `- <column>: <value>` per non-null stored
  scalar value, in column order, followed by a blank line when a non-empty body or a section follows it. NULL is an absent item; the empty text
  is an item with nothing after the colon. The body of a `files` row never begins with a `-` list item — a write
  whose body's first line is one is `CONSTRAINT` (a body may open with a `*` or `+` list or any
  other block) — so on read the `-` items directly after the H1 are the field list, a body that opens with a `-` item is `format.block`, as is a body whose first line is indented by two or more columns (spaces, or a tab) after the field list (a write of either is `CONSTRAINT`), and an item
  that is not `- <stored scalar column>: <value>` in column order is `format.block`. An absent item
  of a non-null column is `format.block`; the engine writes a default explicitly on insert.
  A `files` row without a non-null stored scalar has no field list; a non-empty body follows the H1 after one blank line, and an empty body writes nothing.
- `table` (block or document): a cell under the column-named header; every stored column has a
  header. The key is the first cell, under a header named by the key column; the stored scalar
  columns follow in column order. Text is written as it is; an empty cell is NULL; the empty text is `""`; a value
  beginning with `"` is written `"…"` with `\"` and `\\`; in a table cell each `|` of the value is `\|`, and no other backslash is added; on read a `|` directly after a `\` is never a cell boundary, whatever precedes the `\`, and that one `\` is removed; the quoted form is applied first and the pipe escape last. In a block table the placement foreign key is never a cell; `parent` is a cell, and a `parent` cell that names a key two rows of the file share is `format.block`, since the parent cannot be assigned.
- `list` (block or document): `- [<glyph>] <key> <title>[ <field>…]`, or
  `- <key> <title>[ <field>…]` without a state column. Fields are parsed from the right: a token
  `<declared column>:<value>`, the value a bare token without space, `"` or `\`, or `"…"` with
  `\"` and `\\` as escapes. An absent field is NULL; `col:""` is the empty text; every non-null
  column is written — the engine writes a default explicitly on insert, and a row missing a
  non-null field is `format.block`. A title token matching `<declared column>:` is written `<column>\:`, and a title token matching `<declared column>\:` reads as `<column>:`; no other `\:` is decoded; a title token matching `<declared column>\:` is `CONSTRAINT` on write, since it would read back as `<column>:`, and `<column>\:` and `<column>:` would be written alike. The `@` and date sigils are removed.
- The placement foreign key and `parent` are never written as fields of a block list row; writing one is
  `format.block`.

### State, hierarchy and order

- The state column is the `state_machine` column. A `list` storage of a state machine declares
  at least one `checkbox <column> <state> '<glyph>'`, each glyph distinct, which dbspec requires for every state; the state column of a list storage is non-null, as dbspec requires of every state machine's column.
- Hierarchy is a self foreign key `parent`: in a list the indentation is the parent; a table or a
  file writes the column. No parent is guessed; a cycle is refused (`CONSTRAINT` on write; `parent.cycle` on read for a cycle among a file's rows, which leaves that file unreadable, since its rows have no storage order; a cycle through the rows of several `files` files is the validator's `parent.cycle` and never makes a file unreadable); a `parent` outside the row's
  block is `CONSTRAINT` on write and `format.block` on read.
- `order <column>` names a non-null `i32` or `i64` column with no default, in no key, index or
  check. Its value is the row's 1-based position among the rows with the same placement parent
  and the same `parent`. In block and document storages it is a logical column, never written: a read fills it with the row's 1-based position among its siblings, a file holding an `order` value there is `format.block`, and a write takes each sibling group's values as exactly 1..m, m being the group's size after the write (anything else is `CONSTRAINT`); in `files` storage it is a field-list item whose
  sibling values must be exactly 1..n (a gap or a duplicate is a validate diagnostic). An insert
  may omit it — orm treats an order column as executor-assigned, so the client sends no value —
  and the row appends; with k it places the row at k. An update to k moves the row and the
  siblings follow in the same transaction. k < 1 or k > n + 1 is `CONSTRAINT`, never clamped.
  An update that assigns `order` matches exactly one row, otherwise `CONSTRAINT`. Changing
  `parent` without `order` appends to the new siblings, and the old siblings close the gap.
- Without an `order` column, the lines among siblings (the same placement parent and the same
  `parent`) stand in key order (numeric for integer keys, codepoint for text) and each child
  follows its parent; any other line order is `format.noncanonical`. With an `order` column the siblings stand in the order of their values, which a read takes from the line order, so no sibling order is noncanonical.
- Storage order is the pre-order of the tree: each row, then its children in sibling order (the
  order column, otherwise the key); rows of different placement parents follow their parents'
  storage order. A read without an IR `order` returns storage order; an IR `order` on the order
  column sorts by its integer value with ties in storage order. Moving a row moves its subtree. The codec computes a file's storage order from its row set and never refuses an unsorted row set.

### Relations, links and history

- Every foreign key is restrict-only; `cascade` and `set_null` are rejected. A one-to-many across
  files stores the child table with `store files` and writes the foreign key in its field list.
- **A foreign key value is a link** `[<key>](<target>)` in every place a scalar is written: the
  link text is the referenced key — the data — and the target is derived from it: the relative
  path, with `/` on every platform, to the file that holds the referenced row, plus `#<anchor>`
  exactly when that row has a detail section; a link inside the referenced row's own file is
  `#<anchor>` alone, or that file's name when the row has no anchor. A write that adds or removes a
  row's detail section rewrites every link to that row in the same transaction. A target that
  differs from the derived one is `format.noncanonical`; a key that names no row is a dangling reference (`reference.dangling` in the validator; `FOREIGN_KEY` on a write that introduces it; it never makes a file unreadable, and it keeps its written target (read without a target check and written as read), except a `parent` of a block or document row that names no row of its file, which is `format.block`); a key update of a referenced row is `FOREIGN_KEY` (restrict), so a target never
  moves under a link. A `parent` written in a `files` row is a link too; the placement foreign
  key of a block row is never written. Examples: a `files` row `comments/C5.flowmark.md` links its
  post `[P3](../posts/P3.flowmark.md)`; a block comment inside `posts/P3.flowmark.md` links its
  author `[max](../people.flowmark.md)`; the root `items.flowmark.md` links a row with a detail
  section in the same file `[T2](#items-row-t2)`.
- A list glyph other than ` ` and `x` renders as literal bracket text in common viewers; the state
  names are the catalog's, and the glyphs of `flowmark.dbs` are its convention.
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
  machine's states are the states its `initial`, transition and `terminal` lines name; when a table declares a `checkbox`, `checkbox` covers that set exactly, and `checkbox`, `limit` and `initial` naming another state is a
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
- `flowmark canon <corpus> [file…]` rewrites the named files, or every corpus document when none is named, to canonical form in one transaction, and refuses a named file with `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows; with no file named, it rewrites only the documents without a `parent.cycle` among their rows whose only `format.*` diagnostics are `format.noncanonical` and reports the others. It is the one operation that normalizes document bytes; a logical write never does. Canon changes no logical value, so the key, reference and constraint checks never refuse it; a target whose link derivation needs an unavailable table refuses a named-file canon as `CONSTRAINT`, naming both files, and with no file named it is reported and not rewritten. A value with a validator diagnostic keeps its written text.
- `flowmark history repair <table> <key> [--from <initial state>]` inserts one history row from
  the last `to` — for a row without history, the one initial state, or `--from`, required when
  the machine has several — to the current state, with `at` from the engine clock and the require
  columns copied from the row. It refuses when the pair is not a declared transition, when a
  column the transition requires is null or empty on the row, or when the current state's `limit`
  is exceeded counting this row; the human then restores a legal state or fills the columns by
  editing. The repair is a normal transaction that the human commits.
- A commit first checks that its targets are unchanged since they were read (`WRITE_CONFLICT`
  otherwise); the constraint checks, `history.gap` included, then run on the corpus re-read under
  the lock. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](../spec/10-canonical-format.md#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([canonical format](../spec/10-canonical-format.md#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([canonical format](../spec/10-canonical-format.md#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose detail section appears or disappears, an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](../spec/40-ops.md#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](../spec/80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child and history blocks, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable.
- A transition, or an entry into a terminal state, whose line has `require (c1, …)` must assign
  every listed column in the same update; an unassigned column, `null` or the empty text is
  `CONSTRAINT` naming the column. The history row copies the assigned values, and the state table
  keeps them.
- `state_machine <column> limit <state> <n>` counts the table's rows in that state. A transition,
  insert or update that would exceed it is `CONSTRAINT` naming the limit; a corpus already over
  it (a human edit) is a validate diagnostic that blocks no other write. The relative rule
  `doing < done + 2` is removed — its limit vanishes as done items accumulate.

### The walk

- The walk reads every `*.flowmark.md` file. One whose place matches no table — not
  `<table>.flowmark.md` of a document table, not `<table>/<key>.flowmark.md` of a files table — or a
  `.flowmark.md` entry that cannot be read is a `corpus.walk` diagnostic. Names are compared byte
  for byte: a `.flowmark.md` suffix, a table folder or a document file name in another case is
  `corpus.walk`. The walk never follows a symlink; a symlink whose name ends with `.flowmark.md`, or
  an entry named like a `files` table that is not a folder (a regular file, or a symlink to whatever it points to), is `corpus.walk`, and any other symlink is outside
  the database. A hard link cannot be told from a file portably, so it is not a rule (the
  multi-platform rule); the engine writes a document by a temp file and a rename, so a hard-linked
  document is unlinked by its first write. `dbspec/` holds only the `.dbs` set; another file there
  is a catalog diagnostic.

### Construct coverage

| dbspec construct | On a markdown corpus |
| --- | --- |
| types | every type, in the readable value form of the storage grammar (read through orm's `CanonicalLiteral`; text checked for length; `bytes` as padded base64) |
| primary key | single column; composite is a schema error |
| unique key, check | enforced at commit; a check whose predicate is unknown (NULL) passes and a unique key treats NULLs as distinct, as in SQL |
| index | accepted; derived in memory |
| default | written explicitly on insert |
| `null` | an absent field, cell or section, as the grammar states per storage |
| `identity` | allowed on an `i64` primary key only; `max+1` under the lock |
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
  impossible require cause; done and impossible terminal); `checkbox state waiting ' '`,
  `doing '~'`, `done 'o'`, `bypassed '!'`, `impossible '-'` (one character each, written between
  the brackets of a list row); `history item_history row
  item from from_state to to_state at at`. `item_history(id i64 identity, item varchar(32),
  from_state, to_state, at, evidence, cause, retry)` declares the same three `markdown` columns
  and has `foreign key fk_history_item (item) references items (id)` with its index. A
  multi-line evidence is a section (`items.flowmark.md` H1, `## items row T1`, `### item_history`,
  `#### item_history row <id>`, `##### evidence` — within H6).
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
  board redraws from the rows the operation returns, never from the drop position. `order` is set
  only by a same-parent drop in storage order; a cross-column drop is a state change only, so the
  card keeps its `order` and shows where the view's sort — or, without one, storage order — puts
  it.
- **A board example** proves a one-to-many relation in markdown: `posts` (title, body,
  author → people, created) with `comments` (body, author → people, created, a `parent` for
  replies). In one variant a post's comments are `store block fk_comments_post list` rows inside
  the post's file; in the other they are `store files` rows whose field list links to their post.
  Both variants run the same create, read, update and delete operations through the engine and
  yield the same rows, and every document reads as an ordinary board page — the post, its fields
  as links, its comments — in GitHub, GitLab, VS Code or MkDocs; Pandoc is not claimed.
- **The board app** is the same board built with the polyspec stack — hyper renders the list and
  the post-with-comments pages, crudui defines the post and comment forms, lists and validation,
  and orm reaches the corpus through `markdown://`. It runs on hyper's PHP, Node.js and Python
  servers, each through its orm client's markdown IR executor, and every server shows and changes
  the same rows.
- **The Wails example host stays** beside the board app: the plugin repository's example window
  loads the plugin page unchanged and bridges the sidecar to the flowmark binary, so it proves the
  views through the sidecar while the board app proves orm through `markdown://`. It moves to the
  table-scoped sidecar operations and `*.flowmark.md` demo corpora (the plugin's own checklist).

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
  - T12.11 the demos as one dataset in three storages, and the hardcoding inventory at zero;
  - T12.11.1 the board example: posts with comments and replies, in the block and the files
    variants, driven through the engine;
  - T12.11.2 the board app with hyper, crudui and orm over `markdown://` on hyper's three servers (requires T12.9 and orm `T64`, `T65`, `T67`);
  - T12.12 the orm release tag required instead of the pseudo-version.
  The new engine is built beside the old one, whose codec, executor and application paths the
  faces still use; T12.10 switches the faces to the new engine and removes the old paths with no
  compatibility layer, and T12.12 requires the released orm.
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
- Test density: every grammar and constraint rule has accepting and rejecting cases; P1/P2 golden
  fixtures per storage cover every column type and both NULL and empty forms; parse and emit are
  fuzzed with the golden seeds.
- The board example: creating a post, adding comments and a reply, editing and deleting through the
  engine read back the same rows in both variants.
- The board app: on each hyper server a post is created, commented, replied to, edited and deleted through the forms, and the pages are captured in screenshots.
