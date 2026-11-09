<!-- doc-id: spec-document-model -->
# Document model

[한국어](20-document-model.ko.md)

This specification pins how a corpus becomes tables and rows. The mapping is total: every byte of
a corpus document belongs to exactly one modeled place, or the parser reports it. The byte forms
are in the [canonical format](10-canonical-format.md); the declarations are in the
[schema](30-schema.md).

## Corpus root

- A corpus root is the directory given to a command or the server. It holds `dbspec/*.dbs` — the
  catalog — and `.flowmark/`. There is no upward search.
- This repository is not a corpus: its own checklists are kit-tracked documents.

## Corpus documents and the walk

- A corpus document is a file whose name ends with `.flowmark.md`, under the corpus root and outside
  `dbspec/`, `.flowmark/` and hidden entries. The name is the only criterion that makes a file part
  of the database; every other file is outside it and draws no diagnostic, except a name whose suffix is `.flowmark.md` without regard to case, which is `corpus.walk`, and an entry at a `files` table's place that is not a folder (the symlink rule below).
- The walk reads every `*.flowmark.md` file. A file whose place matches no table — neither
  `<table>.flowmark.md` of a `document` table nor `<table>/<key>.flowmark.md` of a `files`
  table — is a `corpus.walk` diagnostic, and so is a `.flowmark.md` entry that cannot be read.
- Names are compared byte for byte. A `.flowmark.md` suffix, a table folder or a document file name
  in another case is `corpus.walk`.
- A file name whose key breaks the key shape is `format.block` on read.
- The walk never follows a symlink. A symlink whose name ends with `.flowmark.md`, and an entry named
  like a `files` table that is not a folder (a regular file, or a symlink to whatever it points to), is
  `corpus.walk`; any other symlink is outside the database. Grounding: a place of a table is its name,
  so a file that takes that name is in the table's place, not outside it, and the table is unavailable
  ([canonical format](10-canonical-format.md#diagnostics)).
- A hard link cannot be told from a file portably, so it is not a rule (the multi-platform
  requirement). The engine writes a document by a temp file and a rename, so a hard-linked document
  is unlinked by its first write.
- `dbspec/` holds only the `.dbs` set; another file there is a catalog diagnostic
  ([schema](30-schema.md)).

## Storage

Each table declares exactly one storage; a table without one is a schema error, and no default is
guessed.

- **`store files`** — one file per row: `<table>/<key>.flowmark.md` under the corpus root. The file
  name is the key. A key is never updated: an update that assigns the primary key is `IR_INVALID`,
  the orm rule on every database, so a file is never renamed by a write.
- **`store document list`, `store document table`** — every row in the one file
  `<table>.flowmark.md` at the corpus root.
- **`store block <fk> list`, `store block <fk> table`** — the rows inside the scope of the parent
  row that the named foreign key references. Its block section is a heading named by the child table with the list or table on the next line ([canonical format](10-canonical-format.md#document-order)).

- A missing table folder (`files`) or a missing document file (`document`) is an empty table: zero
  rows and no diagnostic. A document file without its H1 line is `format.noncanonical`. Grounding: git keeps no empty folders, so a checkout may hold no folder for a table without rows; an absent folder or file holds no rows, as an empty folder or a document file holding only `# <table>` does, so its absence is one form of zero rows, not a corpus defect.
- A `.flowmark.md` file at a place no table names is `corpus.walk`, as the walk states; it never stands in for a missing table.
- An insert creates the folder or the file. A delete of the last row of a `document` table leaves the
  file with its H1 line `# <table>` and one LF.

Aggregate or split is a storage declaration; the logical relations are the same in every storage,
so the same rows can be stored either way.

## Time shards

A `files` or `document` table whose `store` setting carries `shard <column> <period>` stores the rows
of each period apart, the column being a non-null `date` or `datetime` column and the period `year`,
`month` or `day` (the setting and its errors are orm's, `T119`). A table that no row owns and that
accumulates in time — an event log, time entries — would otherwise grow one file without end (`document`)
or one folder without end (`files`).

- **Period name.** The name of the period of a row is its column's value cut to the grain: `2026`,
  `2026-10` or `2026-10-11`. A `datetime` counts at its stored UTC value, a `date` as written. A value
  that is no date, such as a month past 12, has no period; the row is refused as a write and `type` as a read.
- **Paths.** A sharded `files` table stores a row in `<table>/<period>/<key>.flowmark.md`, a sharded
  `document` table stores the rows of a period in `<table>/<period>.flowmark.md`, whose H1 line is
  `# <table>`. A period without rows has no file or folder, as an empty table has none.
- **The walk.** The folder of a sharded table holds the folders (files) or the documents (document) of
  its periods, named at the grain of the shard, and nothing else that is a document: a document directly in
  the folder of a sharded `files` table, a folder or a document named as another grain or as no period, and
  a document below a period folder are `corpus.walk`.
- **Storage order.** A shard changes where the rows stand, never the order the table reads in, since the
  logical relations are the same in every storage. A `files` table keeps its order (spec 20 Order, by key
  without a hierarchy or an order column, as orm allows none on a sharded table), whatever the periods are. A
  `document` table reads its periods ascending, and each document in its own order.
- **Keys.** A key and an `identity` value are unique across all periods: the
  rows of every period are one table, read from the corpus the engine keeps in memory, and no index file
  stands beside the corpus. Grounding: the key check of a table reads the table's rows, and a shard changes
  where they stand, never which rows the table has.
- **A row in another period's file.** A row whose column value names a period other than the one of the
  file that holds it is the validator's `shard.period` at the row; the next write of the row moves it to the
  file of its period.
- **An update of the column** moves the row to the file of its new period and rewrites, in the same
  transaction, every file that links to it, since a link to a sharded row carries the path of its period.
  Grounding: a link is derived from where the row stands (spec 10), so a move changes the link.
- **Changing the clause** adds or removes the shard of a table: a storage change (spec 80).
- **The one method, and where it applies.** There is one shard method, the time shard, with the periods
  `year`, `month` and `day`. It is a statement about the storage of a markdown corpus, not a partition of a
  database: the SQL backends (MySQL, PostgreSQL, SQLite) ignore the clause as orm defines it (orm
  `docs/dbspec.md`, `T119`), so a database export (spec 70) creates one table for a sharded table and inserts
  its rows of every period in the order the table reads in. The clause applies to `files` and `document`
  storage; orm rejects it on a `block` table, on a table with a self foreign key and on a table with
  `order`. Shard by key range, by hash or by list, and a
  shard of more than one column, are not defined.

## Keys

- A table in `files` storage has a single-column primary key; a table in `document` or `block` storage may have a composite one ([schema](30-schema.md)). A
  negative integer key is refused: its `-` begins the key, against the shape below. On write it is
  `CONSTRAINT`; on read it is `format.block` in every storage, as any key that breaks the shape.
- A `files` row's key is its file name without `.flowmark.md`.
- A key of a table row is any single-line text of its type, quoted where its cell needs it ([canonical format](10-canonical-format.md#single-line-places)), and a
  negative integer is a key there: a cell holds it, and its row has an anchor of its own only through the rules below. The shape below binds the key of a
  `files` row and of a list row, which have no quoted form.
- Every key of that shape is a file name, a URL path segment and part of a heading anchor at once. A key consists
  of `A`–`Z`, `a`–`z`, `0`–`9`, `_` and `-`, and begins with a letter or a digit: a leading `_`
  would render as emphasis and alter both the shown key and its anchor. The shape is ASCII: these
  characters are the whole alphabet of a key, and no other character is a key character, because
  the case-insensitive comparison and the lowercased anchor are defined for these letters alone. A
  key holds no `-_`: a `_` after punctuation is left-flanking and can open emphasis (CommonMark 0.31.2 §6.2), so the key `a-_b_` would render `a-` and an emphasized `b`, altering both the shown key and its anchor.
- A key is unique within its table without regard to case, because case-insensitive file systems
  and lowercased anchors would otherwise collide. A key equal to another key of its table without regard to case is `DUPLICATE_KEY` on a write that
  introduces it and `key.duplicate` in the validator; it never makes a file unreadable, except that two rows of one file whose keys are byte for byte equal are `format.block` when both have detail sections, since the two sections share one heading text ([canonical format](10-canonical-format.md#detail-headings-and-anchors)), and, when both stand in one block, when a detail section names their key or a `parent` cell of that block names it, since the section or the parent cannot be assigned ([canonical format](10-canonical-format.md#document-order)); rows of different blocks are assigned by their block; two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)).
- A foreign key matches the referenced key byte for byte. A link text that differs from the key
  only in case is a dangling reference.
- A key or a table name equal, without regard to case, to `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`
  or `LPT0`–`LPT9` is refused: a table name as a schema error, a key as `CONSTRAINT`. These are
  reserved file names on Windows.
- A `files` key is at most 243 bytes, so its file name stays within 255 bytes.
- A file name that is not the canonical text of its key's type is `format.block`: for the integer
  key 7 the canonical file name is `7.flowmark.md`, and `007.flowmark.md` is not.
- `identity` is allowed only on an `i64` primary key column ([schema](30-schema.md)): an insert that omits the key gets one
  more than the larger of the largest key and the table's counter
  ([AUTOINCREMENT counters](40-ops.md#sql-statements)), allocated under the lock, so no key is given twice. A git rewind
  rewinds the counter with the rows, and reuse after it is accepted — git history is the allocator of record.
- A text key is given by the insert, and an insert that leaves it out is `IR_INVALID`: the engine
  allocates no text key, as no common database does (orm DBS-77 withdraws `key_prefix`). A
  hand-written key is accepted when it is unique and has the column's type.
- Keys are opaque: no ID grammar exists, and no relation is read from a key.

## Title and body

- `title <column>` and `body <column>` are optional per-table settings. A `list` storage and a
  `files` storage require `title` — the line text, the H1; a `table` storage may declare it as a
  cell. A declared title is non-null and refuses the empty text.
- `body` is a `markdown` column and never NULL. It is the text of a `files` row after its H1 and
  field list, or the text of a detail section before its first top-level heading of level ≤ h + 1, by the CommonMark block structure of [canonical format](10-canonical-format.md#sections-and-their-values); a row without a detail
  section has the empty text as its body.
- A view of kind kanban, list or gantt requires its entity table to declare both.

## Row scope

- A `files` row's scope is its file, with its sections at level 2.
- A block or document row's scope is its detail section `<h> <table> row <key>` (h = b + 1 under a block
  section at level b, h = 2 in a document file), with its sections at level h + 1.
- A `markdown` column is always a section of the row scope — never a field or a cell — in every
  storage. Each `markdown` column other than the body (a non-null one always present, a nullable one
  when it is not NULL) and each child block is a section of the row scope named by the column or
  the child table; a child block stands once in its row scope, and a second one is `format.section` ([canonical format](10-canonical-format.md#sections-and-their-values)); a child table named like a markdown column of its
  parent is a schema error.

## Hierarchy

- Hierarchy is a self foreign key `parent`. In a list the indentation is the parent and no column
  is written; a table or a file writes the column.
- No parent is guessed; a cycle is refused: `CONSTRAINT` on write. On read, a cycle among one file's rows is `parent.cycle` and leaves that file unreadable, since its rows have no storage order; a cycle through the rows of several `files` files is the validator's `parent.cycle` and never makes a file unreadable. A `parent`
  of a block or document row names a row of its own block — the one list or table that holds the row, which for a document row is its file's: any other is `CONSTRAINT` on write and `format.block` on read ([canonical format](10-canonical-format.md#document-order)). Grounding: a parent and its child share a placement parent, or the child has no place in the storage order, and a list's indentation cannot reach another block.

## Order

- `order <column>` names a non-null `i32` or `i64` column with no default, in no key, index or
  check (orm DBS-79). Its value is the row's place among the rows with the same placement parent and
  the same `parent`, its siblings: the siblings stand by ascending value, the rows that share a value
  in key order (numeric for integer keys, codepoint for text), and the rows whose value is not of the
  column's type after every other, in key order.
- The column is a stored scalar in every storage, written where any scalar of its storage is: a field
  of a list row, a cell of a table, a field-list item of a `files` row. It is required as every
  non-null stored scalar is, so a row without it is `format.block`, and a value not of its type is the
  validator's `type` ([constraints](50-constraints.md#hierarchy-and-order)). A read answers the value
  as written, and an IR `where` or `order` on the column compares it so.
- Nothing assigns it: an insert gives it as it gives any non-null column without a default, and an
  insert that leaves it out is `IR_INVALID`, as on the databases orm reaches. Any value of the type is
  accepted; siblings may share a value, and a gap is no fault. A write of the column, a parent change
  and a delete rewrite the written row's file or its document alone: no sibling is renumbered and no
  gap is closed. Grounding: no common database assigns a position, so the engine holds only the value
  the application writes; a stored value lets a move write one row, as an update of any column does.
- In block and document storages the rows of a sibling group stand in the line order of their values,
  so a document whose siblings stand in another order is `format.noncanonical`, and a write renders
  them in that order. Moving a row moves its subtree.
- Without an `order` column, siblings stand in key order (numeric for integer keys, codepoint for
  text) and each child follows its parent; any other line order is `format.noncanonical`.
- **Storage order** is the pre-order of the tree: each row, then its children in sibling order;
  rows of different placement parents follow their parents' storage order. A read without an IR
  `order` returns storage order; an IR `order` on the order column sorts by its value with ties
  in storage order. Storage order is total for every readable table. In `files` storage a row
  whose `parent` is NULL or names no row of its table is a root. The roots stand by sibling group
  — the group whose `parent` is NULL first, then each group whose `parent` names no row, by that
  `parent` value in key order — each group in its sibling order, and each root is followed by its
  subtree in pre-order. The rows no root reaches — a row whose `parent` is not of its column's
  type, a row on a cycle of parents across files, and every row under one of them — follow every
  reached row, in corpus order. Among siblings with an `order` column, rows sharing a value stand
  in key order, and rows whose value is not of its type stand after the others in key order.
  Grounding: a read never fails on a validator diagnostic of another row, so the storage order of a
  corpus that has one must be a function of the corpus alone, as corpus order is.

## Relations

- A foreign key takes the actions `restrict`, `cascade` and `set_null` on delete, which the engine
  applies in the write ([constraints](50-constraints.md)).
- A foreign key value is a link. Its text is the referenced key and its target is derived from the
  referenced row, as the [canonical format](10-canonical-format.md) states. A link from a block row to the `files` row whose file holds it is that file's name, derived from the file's own key ([canonical format](10-canonical-format.md#foreign-keys-as-links)). A dangling foreign key is the validator's `reference.dangling`; a write that introduces one is
  `FOREIGN_KEY`, and it never makes a file unreadable, except a `parent` of a block or document row that names no row of its block, which is `format.block` ([canonical format](10-canonical-format.md#foreign-keys-as-links)). Every relation is a link whose `#<anchor>` is the anchor that GitHub, GitLab and VS Code generate for the heading it names, as the [canonical format](10-canonical-format.md#detail-headings-and-anchors) states. Pandoc is not a viewer the claim is made for: its default does not rewrite relative link targets, so a link between corpus files is not claimed to be followed in its output.
- A one-to-many across files stores the child table with `store files` and writes the foreign key
  in its field list. A one-to-many inside a file stores the child table with `store block`.
- Many-to-many is a link table with an `identity` key.
- A child row inside its parent's file has no file of its own, and the parent's transaction covers
  it.
