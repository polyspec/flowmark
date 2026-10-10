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
  of the database; every other file is outside it and draws no diagnostic.
- The walk reads every `*.flowmark.md` file. A file whose place matches no table — neither
  `<table>.flowmark.md` of a `document` table nor `<table>/<key>.flowmark.md` of a `files`
  table — is a `corpus.walk` diagnostic, and so is a `.flowmark.md` entry that cannot be read.
- Names are compared byte for byte. A `.flowmark.md` suffix, a table folder or a document file name
  in another case is `corpus.walk`.
- A file name whose key breaks the key shape is `format.block` on read.
- The walk never follows a symlink. A symlink whose name ends with `.flowmark.md`, or a symlinked
  folder named like a `files` table, is `corpus.walk`; any other symlink is outside the database.
- A hard link cannot be told from a file portably, so it is not a rule (the multi-platform
  requirement). The engine writes a document by a temp file and a rename, so a hard-linked document
  is unlinked by its first write.
- `dbspec/` holds only the `.dbs` set; another file there is a catalog diagnostic
  ([schema](30-schema.md)).

## Storage

Each table declares exactly one storage; a table without one is a schema error, and no default is
guessed.

- **`store files`** — one file per row: `<table>/<key>.flowmark.md` under the corpus root. The file
  name is the key; a key change renames the file in the same transaction.
- **`store document list`, `store document table`** — every row in the one file
  `<table>.flowmark.md` at the corpus root.
- **`store block <fk> list`, `store block <fk> table`** — the rows inside the scope of the parent
  row that the named foreign key references.

Aggregate or split is a storage declaration; the logical relations are the same in every storage,
so the same rows can be stored either way.

## Keys

- A markdown table has a single-column primary key; a composite primary key is a schema error.
- A `files` row's key is its file name without `.flowmark.md`.
- Every key is a file name, a URL path segment and part of a heading anchor at once. A key consists
  of `A`–`Z`, `a`–`z`, `0`–`9`, `_` and `-`, and begins with a letter or a digit: a leading `_`
  would render as emphasis and alter both the shown key and its anchor. A key holds no `--` and does
  not end with `-`: viewers that collapse hyphen runs (GitLab, MkDocs) would otherwise alter the
  anchor.
- A key is unique within its table without regard to case, because case-insensitive file systems
  and lowercased anchors would otherwise collide.
- A foreign key matches the referenced key byte for byte. A link text that differs from the key
  only in case is a dangling reference.
- A key or a table name equal, without regard to case, to `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`
  or `LPT0`–`LPT9` is refused: a table name as a schema error, a key as `CONSTRAINT`. These are
  reserved file names on Windows.
- A `files` key is at most 243 bytes, so its file name stays within 255 bytes.
- A file name that is not the canonical text of its key's type is `format.block`: for the integer
  key 7 the canonical file name is `7.flowmark.md`, and `007.flowmark.md` is not.
- A key change that differs only in case renames the file through an intermediate name, inside the
  journaled transaction.
- `identity` is allowed on every markdown table: an insert that omits the key gets `max+1`,
  allocated under the lock. Reuse after a git rewind is accepted — git history, not a sequence, is
  the allocator of record.
- A key with `key_prefix <p>` is allocated `<p><n + 1>`, where n is the largest decimal suffix
  without a leading zero among the keys of the form `<p><n>`. orm treats such a column as
  executor-assigned, as with `identity`, so an insert may omit it. A text key without `key_prefix`
  must be given by the insert.
- A hand-written key is accepted when it is unique and has the column's type; a key not of the form
  `<p><n>` does not count toward n.
- Keys are opaque: no ID grammar exists, and no relation is read from a key.

## Title and body

- `title <column>` and `body <column>` are optional per-table settings. A `list` storage and a
  `files` storage require `title` — the line text, the H1; a `table` storage may declare it as a
  cell. A declared title is non-null and refuses the empty text.
- `body` is a `markdown` column and never NULL. It is the text of a `files` row after its H1 and
  field list, or the text of a detail section before its first sub-heading; a row without a detail
  section has the empty text as its body.
- A view of kind kanban, list or gantt requires its entity table to declare both.

## Row scope

- A `files` row's scope is its file, with its sections at level 2.
- A block or document row's scope is its detail section `<h> <table> row <key>` (h = b + 1 under a block
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
  writing it is `format.block`. In `files` storage it is a field-list item whose sibling values are
  exactly 1..n; a gap or a duplicate is a validate diagnostic.
- An insert may omit it and the row appends; with k it places the row at k. An update to k moves
  the row and the siblings follow in the same transaction; k < 1 or k > n + 1 is `CONSTRAINT`,
  never clamped. n is the number of rows among the destination siblings excluding the row itself,
  so k lies in 1..n + 1 for an insert, a parent change and an in-place move alike. An update that
  assigns `order` matches exactly one row, otherwise `CONSTRAINT`.
  Changing `parent` without `order` appends to the new siblings, and the old siblings close the
  gap. Moving a row moves its subtree.
- Without an `order` column, siblings stand in key order (numeric for integer keys, codepoint for
  text) and each child follows its parent; any other line order is `format.noncanonical`.
- **Storage order** is the pre-order of the tree: each row, then its children in sibling order;
  rows of different placement parents follow their parents' storage order. A read without an IR
  `order` returns storage order; an IR `order` on the order column sorts by its value with ties
  in storage order.

## Relations

- Every foreign key is restrict-only ([constraints](50-constraints.md)): `cascade` and `set_null`
  are rejected at catalog load ([schema](30-schema.md)).
- A foreign key value is a link. Its text is the referenced key and its target is derived from the
  referenced row, as the [canonical format](10-canonical-format.md) states. Every relation is a
  link that a reader can follow in a markdown viewer with GitHub-style heading slugs (GitHub, GitLab,
  VS Code, Pandoc).
- A one-to-many across files stores the child table with `store files` and writes the foreign key
  in its field list. A one-to-many inside a file stores the child table with `store block`.
- Many-to-many is a link table with an `identity` key.
- A child row inside its parent's file has no file of its own, and the parent's transaction covers
  it.
