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

## Keys

- A markdown table has a single-column primary key; a composite primary key is a schema error. A
  negative integer key is refused: its `-` begins the key, against the shape below. On write it is
  `CONSTRAINT`; on read it is `format.block` in every storage, as any key that breaks the shape.
- A `files` row's key is its file name without `.flowmark.md`.
- Every key is a file name, a URL path segment and part of a heading anchor at once. A key consists
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
- `identity` is allowed only on an `i64` primary key column ([schema](30-schema.md)): an insert that omits the key gets `max+1`,
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
  check. Its value is the row's 1-based position among the rows with the same placement parent
  and the same `parent`.
- In block and document storages it is a logical column and is never written: a read fills it with the
  row's 1-based position among its siblings, a file holding an `order` value there is `format.block`, and
  a write takes each sibling group's values as exactly 1..m, m being the group's size after the write, anything else being `CONSTRAINT`. Grounding: the
  line order of the siblings is their order, so the position is the value, and a value written in the file
  would be a second order.
- In `files` storage the field-list item holds the row's **rank**, not its position: any value of
  the column's type. The siblings stand in ascending rank, and a read fills the column with the
  row's 1-based position in that sibling order, as in the other storages, so a read, an IR `where`
  or `order` on the column, an answer and the export see positions, never ranks. The item is
  required as every non-null stored scalar is, so a file without it is `format.block`. The
  validator's `order` is at each row whose rank is shared with a sibling, since the two ranks do
  not order the rows; a rank not of the column's type is `type` alone and is shared with none
  ([constraints](50-constraints.md#hierarchy-and-order)). A row whose `parent` is not of its
  column's type belongs to no known sibling group: its own `order` is not judged, and its `type`
  stands for it. A read that evaluates the position of a row whose rank is not of its type fails
  as `CONSTRAINT` naming that `type` diagnostic, as every read of an unreadable value does
  ([canonical format](10-canonical-format.md#readable-values)): the position derives from a rank
  that has no comparison; one whose `parent` is not of its type fails so too, since its sibling
  group is not known.
- A `files` write that places a row — an insert, an update of `order`, a parent change — gives
  it a rank strictly between its new neighbours' and writes no other file while such a rank
  exists. With a the rank before the row (0 when it stands first) and b the rank after it, a row
  whose own rank already lies between them keeps it; otherwise it takes the midpoint
  a + ⌊(b − a) / 2⌋ when b − a ≥ 2, or a + 1000 when it stands last and that does not exceed the
  type's maximum. When neither holds, the write **renumbers** the group: the i-th row of the
  group, in its order after the write, takes the rank i·s, s = min(1000, ⌊max / m⌋), max the
  type's maximum and m the group's size, and only a file whose rank changes is rewritten; a
  renumber of a group of more than max rows is `CONSTRAINT`, since no spacing parts them. A delete and a parent change leave the ranks of
  the old siblings as they stand, since a gap changes no position. A write that must read a
  neighbour's rank, or renumber a group, holding a rank not of its type fails as `CONSTRAINT`
  naming that `type` diagnostic; a placement between two readable ranks does not read it, and
  the rows whose rank is not of its type stand after every readable one, so a row placed just
  before the first of them stands last among the readable ranks and takes its rank as a row
  that stands last does. A renumber rewrites
  sibling files, so a sibling with a `format.*` diagnostic refuses it as the
  [commit refusal](10-canonical-format.md#encoding-and-line-structure) states. Grounding: under
  a stored position, an insert at k, a move, a delete and a parent change rewrite every following
  sibling — an insert at position 1 among 1,000 siblings rewrote 1,000 files —, while a rank
  rewrites the placed row's file alone until its place has no free rank. With the spacing 1000
  a place takes nine inserts before its gap is exhausted (1000 halves to 500, 250, 125, 62,
  31, 15, 7, 3, 1), so a move writes one file in the common case and the group's m files once
  per exhausted gap; a group too large for the spacing 1000 in its type has a smaller s and
  fewer inserts per gap. A decimal spacing keeps a rank readable and editable by hand: a person
  moves a row between 2000 and 3000 by writing 2500. A dense group 1..n is a valid set of ranks,
  so a corpus written as positions reads unchanged and is renumbered by its first placement that
  finds no free rank.
- An insert may omit it and the row appends; with k it places the row at k. An update to k moves
  the row; in block and document storages the siblings follow in the same transaction. k < 1 or
  k > n + 1 is `CONSTRAINT`, never clamped. n is the number of rows among the destination
  siblings excluding the row itself, so k lies in 1..n + 1 for an insert, a parent change and an
  in-place move alike. An update that assigns `order` matches exactly one row, otherwise
  `CONSTRAINT`. Changing `parent` without `order` appends to the new siblings; in block and
  document storages the old siblings close the gap. Moving a row moves its subtree.
- Without an `order` column, siblings stand in key order (numeric for integer keys, codepoint for
  text) and each child follows its parent; any other line order is `format.noncanonical`. With an
  `order` column, the siblings stand in the order of their values, which a read takes from the line
  order, so no sibling order is noncanonical.
- **Storage order** is the pre-order of the tree: each row, then its children in sibling order;
  rows of different placement parents follow their parents' storage order. A read without an IR
  `order` returns storage order; an IR `order` on the order column sorts by its value with ties
  in storage order. Storage order is total for every readable table. In `files` storage a row
  whose `parent` is NULL or names no row of its table is a root. The roots stand by sibling group
  — the group whose `parent` is NULL first, then each group whose `parent` names no row, by that
  `parent` value in key order — each group in its sibling order, and each root is followed by its
  subtree in pre-order. The rows no root reaches — a row whose `parent` is not of its column's
  type, a row on a cycle of parents across files, and every row under one of them — follow every
  reached row, in corpus order. Among siblings with an `order` column, rows sharing a rank stand
  in corpus order, and rows whose rank is not of its type stand after the others in corpus order.
  Grounding: a read never fails on a validator diagnostic of another row, so the storage order of a
  corpus that has one must be a function of the corpus alone, as corpus order is.

## Relations

- Every foreign key is restrict-only ([constraints](50-constraints.md)): `cascade` and `set_null`
  are rejected at catalog load ([schema](30-schema.md)).
- A foreign key value is a link. Its text is the referenced key and its target is derived from the
  referenced row, as the [canonical format](10-canonical-format.md) states. A link from a block row to the `files` row whose file holds it is that file's name, derived from the file's own key ([canonical format](10-canonical-format.md#foreign-keys-as-links)). A dangling foreign key is the validator's `reference.dangling`; a write that introduces one is
  `FOREIGN_KEY`, and it never makes a file unreadable, except a `parent` of a block or document row that names no row of its block, which is `format.block` ([canonical format](10-canonical-format.md#foreign-keys-as-links)). Every relation is a link whose `#<anchor>` is the anchor that GitHub, GitLab and VS Code generate for the heading it names, as the [canonical format](10-canonical-format.md#detail-headings-and-anchors) states. Pandoc is not a viewer the claim is made for: its default does not rewrite relative link targets, so a link between corpus files is not claimed to be followed in its output.
- A one-to-many across files stores the child table with `store files` and writes the foreign key
  in its field list. A one-to-many inside a file stores the child table with `store block`.
- Many-to-many is a link table with an `identity` key.
- A child row inside its parent's file has no file of its own, and the parent's transaction covers
  it.
