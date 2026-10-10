<!-- doc-id: spec-document-model -->
# Document model

[한국어](20-document-model.ko.md)

This specification pins how a corpus becomes tables and rows. The mapping is total: every byte of
a document belongs to exactly one modeled place, or the parser reports it. The byte forms are in
the [canonical format](10-canonical-format.md); the declarations are in the [schema](30-schema.md).

## Corpus root

- A corpus root is the directory given to a command or the server. It holds `dbspec/*.dbs` — the
  catalog — and `.flowmark/`. There is no upward search.
- The walk skips `.flowmark/`, `dbspec/` and hidden entries; a symlink or a hard link inside the
  corpus is a diagnostic; a nested corpus root is an error.
- This repository is not a corpus: its own checklists are kit-tracked documents.

## Storage

Each table declares exactly one storage; a table without one is a schema error, and no default is
guessed.

- **`store files`** — one file per row, in the folder named after the table under the corpus root.
  An insert writes `<table>/<key>.md`; the engine never renames a file, and a key change keeps the
  file.
- **`store document list`, `store document table`** — every row in the one file `<table>.md` at
  the corpus root.
- **`store block <fk> list`, `store block <fk> table`** — the rows inside the scope of the parent
  row that the named foreign key references.

Aggregate or split is a storage declaration; the logical relations are the same in every storage,
so the same rows can be stored either way.

## Keys

- A markdown table has a single-column primary key; a composite primary key is a schema error.
- A `files` row's key is its `<!-- doc-id: -->` value; the file name carries no meaning. A doc-id
  is unique across the rows and the `document` files of the corpus; twins share it.
- `identity` is allowed on every markdown table: an insert that omits the key gets `max+1`,
  allocated under the lock. Reuse after a git rewind is accepted — git history, not a sequence,
  is the allocator of record.
- A `varchar` key with `key_prefix <p>` is allocated `<p><n + 1>`, where n is the largest decimal
  suffix without a leading zero among the keys of the form `<p><n>`. A key without either must be
  given by the insert. A hand-written key is accepted when it is unique and has the column's type;
  a key not of the form `<p><n>` does not count toward n.
- Keys are opaque: no ID grammar exists, and no relation is read from a key.

## Title and body

- `title <column>` and `body <column>` are optional per-table settings. A `list` storage and a
  `files` storage require `title` — the line text, the H1; a `table` storage may declare it as a
  cell. A declared title is non-null and refuses the empty text.
- `body` is a `markdown` column and never NULL. It is the text after the H1 of a file, or the text
  of the row's detail section before its first sub-heading; a row without a detail section has
  the empty text as its body.
- A view of kind kanban, list or gantt requires its entity table to declare both.

## Row scope

- A `files` row's scope is its file, with its sections at level 2.
- A block or document row's scope is its detail section `<h> <key>` (h = b + 1 under a block
  section at level b, h = 2 in a document file), with its sections at level h + 1.
- A `markdown` column is always a section of the row scope — never a field or a cell — in every
  storage. Each nullable `markdown` column and each child block is a section of the row scope
  named by the column or the child table; a child table named like a markdown column of its
  parent is a schema error.

## Hierarchy

- Hierarchy is a self foreign key `parent`. In a list the indentation is the parent and no column
  is written; a table or a file writes the column.
- No parent is guessed; a cycle is refused; a `parent` outside the row's block is `CONSTRAINT` on
  write and `format.block` on read.

## Order

- `order <column>` names a non-null `i32` or `i64` column with no default, in no key, index or
  check. Its value is the row's 1-based position among the rows with the same placement parent
  and the same `parent`.
- In block and document storages it is never written: it is derived from line order on read, and
  writing it is `format.block`. In `files` storage it is a frontmatter field whose sibling values
  are exactly 1..n; a gap or a duplicate is a validate diagnostic.
- An insert may omit it and the row appends; with k it places the row at k. An update to k moves
  the row and the siblings follow in the same transaction; k < 1 or k > n + 1 is `CONSTRAINT`,
  never clamped. An update that assigns `order` matches exactly one row, otherwise `CONSTRAINT`.
  Changing `parent` without `order` appends to the new siblings, and the old siblings close the
  gap. Moving a row moves its subtree.
- Without an `order` column, siblings stand in key order (numeric for integer keys, codepoint for
  text) and each child follows its parent; any other line order is `format.noncanonical`.
- **Storage order** is the pre-order of the tree: each row, then its children in sibling order;
  rows of different placement parents follow their parents' storage order. A read without an IR
  `order` returns storage order; an IR `order` on the order column sorts by its value with ties
  in storage order.

## Relations

- Every foreign key is restrict-only ([constraints](50-constraints.md)). A one-to-many across files
  stores the child table with `store files` and writes the foreign key in its frontmatter; a
  one-to-many inside a file stores it with `store block`.
- Many-to-many is a link table with an `identity` key.
- A child row inside its parent's file has no file of its own, and the parent's transaction
  covers it.

## Translations

- A twin is a file `<name>.<lang>.md` whose doc-id equals a row's doc-id; it is not a row and the
  engine never writes it. The language list is the corpus setting `translations` in
  `dbspec/flowmark.json`, which kit's `config/documents.json` mirrors. A row whose source changed
  makes its twin stale under kit's `source-sha256` rule.
