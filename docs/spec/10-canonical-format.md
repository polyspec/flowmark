<!-- doc-id: spec-canonical-format -->
# Canonical format

[한국어](10-canonical-format.ko.md)

This specification pins the byte form of the corpus documents that the
[document model](20-document-model.md) names. It states the idempotency propositions, the readable
document, the skeleton, the document order, the detail headings and their anchors, the sections,
the readable values, the single-line places, the field list, the list and table grammars, the
links, and the encoding. Everything the renderer owns is stated here; everything else is preserved
payload.

## Idempotency

- **P1 (canonical identity).** For a canonical input `s`: `emit(parse(s)) == s`, byte for byte.
- **P2 (emission stability).** For any valid input `d`: `emit(parse(emit(d))) == emit(d)`.
- P1 and P2 replace any single byte-identity claim: a document that a Windows editor saves with
  CRLF is valid input, and P2 still holds; restoring P1 is a migration, not a side effect of a
  logical write.

## The readable document

- A corpus document is a file whose name ends with `.flowmark.md`; the
  [document model](20-document-model.md) states the criterion.
- A corpus document is a complete markdown document. A reader who does not know flowmark reads it
  as it is: it carries no comment, no marker, no frontmatter and no hidden field.
- Its structure is ordinary markdown: headings, lists, tables and a field list. Every relation is a
  link that a reader can follow in any markdown viewer.
- Grounding: the documents are the database's only storage, so they must stay readable documents.
  What only the machine needs belongs to the catalog (`dbspec/`) or to `.flowmark/`.

## The skeleton and the preservation boundary

The renderer owns exactly the **skeleton**: the H1 line, the field list of a `files` row, the
heading lines of the block, detail and markdown-column sections, the list lines and table lines of
the rows, and the blank lines between skeleton elements. The values of `markdown` columns — the body
and the column sections — are payload: the renderer moves them with their row and never rewrites
their bytes.

- Non-canonical input (a BOM, CRLF, a skeleton line in a non-canonical shape, rows out of their
  storage order) is a `format.noncanonical` diagnostic. `flowmark canon` is a one-shot migration
  transaction that rewrites the document to canonical form; a logical write never normalizes as
  a side effect.

## Document order

- **A `files` row** is the file `<table>/<key>.flowmark.md`. In order it holds: the H1 line
  `# <title>`; the field list of its stored scalar values, followed by a blank line; the body; the
  level-2 sections of the row scope.
- **A `document` file** `<table>.flowmark.md` holds: the H1 line `# <table>`; the list or table of
  its rows directly under the H1; the detail sections at level 2.
- **A detail section** is the heading `<h> <table> <key>` — the table name, one space and the key,
  never a title copy — followed by the row's body and then the row scope's sections at level h + 1.
  A detail row's scope is at level h + 1. Detail sections of nested list rows are flat siblings.
- **A block section** is a heading at level b named by the child table, inside the parent row's
  scope; its list or table follows the heading, and its rows' detail sections follow at level
  b + 1.
- A row scope's sections stand in a fixed order: the nullable `markdown` columns in column order, then the child blocks in table name order (dbspec's canonical order). Detail sections stand in the storage order of their rows.
- Any content after a block's or document's list or table other than its rows' detail sections — up to the next heading of level ≤ b in a block section, or to the end of a document file — is `format.block`.
- A `files` row without a non-null stored scalar has no field list; the body follows the H1 after one blank line.
- A row has a detail section exactly when its body is not empty, a nullable `markdown` column of it
  is not NULL, or it has a child block row. A detail section without any of these is
  `format.noncanonical`. A row without a detail section has the empty text as its body.
- A detail section whose key names no row of its block is `format.block`.
- A layout that needs a heading past H6 is a schema error, decided when the catalog loads.

## Detail headings and anchors

- The anchor of a detail heading is `<table>-<lowercased key>`: the heading `## items T2` has the
  anchor `items-t2`.
- The engine computes the slug of every heading in a file, skeleton and payload alike, in document
  order. The slug is GitHub's: the heading's rendered inline text, lowercased, with every character
  other than a letter, a digit, a space, `-` or `_` removed and each space written as `-`; a
  repeated slug takes the suffix `-1`, `-2` in document order.
- A detail heading whose slug differs from its anchor, because an earlier heading took it, is
  `CONSTRAINT` on write, naming the earlier heading, and `format.block` on read.

## Sections and their values

- The body and every `markdown` column value of a row scope at level L end at the next heading
  of level ≤ L, counted only outside fenced blocks (CommonMark block structure). A value holds
  only headings deeper than L; a write of a value with a heading line of level ≤ L outside a
  fence is `CONSTRAINT`.
- In a row scope at level L, each nullable `markdown` column and each child block is a heading at
  level L named by the column or the child table. An absent section is NULL; a present empty
  section is the empty text. The body is never NULL. An undeclared heading at level L is
  `format.section`.
- A child block section is written only when the block holds at least one row — an empty block and an absent block both mean zero rows, so only one form can be canonical (P1); a parent row with
  no child rows has no block heading, and an empty block heading is `format.noncanonical`.
- A `markdown` column is always a section of the row scope, never a field or a cell, in every
  storage.

## Stored scalar columns

The stored scalar columns of a table are its columns minus the key, the `title` of a `list` or `files` storage, the `body`, the
`markdown` columns, the state column of a `list`, the placement foreign key, the `parent` of a
`list`, and the `order` column of a `block` or `document` storage. The key, the title, the state
and the hierarchy have their own places. A stored scalar column is a field-list item of a `files`
row, a field of a list row or a cell of a table. Every non-null stored scalar column is written; the
engine writes a default explicitly on insert.

## Readable values

Scalar values are written in a readable form, not as dbspec literals. The type always comes from
the catalog.

- Text is written as it is. An integer, a decimal or a float is written in its canonical number
  form; `true` and `false` are written as such.
- A date is `YYYY-MM-DD`; a datetime is `YYYY-MM-DD HH:MM:SS[.f…]`; a time is `HH:MM:SS[.f…]`. A
  datetime and a time have exactly the column's precision in fraction digits.
- A uuid is lowercase. Bytes are written as padded base64.
- A foreign key is a link (see [Foreign keys as links](#foreign-keys-as-links)).
- The engine reads a number, `true` or `false` with orm's `dbspec.CanonicalLiteral(t, v)`, and a
  date, time, datetime or uuid with `CanonicalLiteral(t, "'" + v + "'")`, with the quotes removed
  from the result.
- `now` is refused as a value. Text is checked against its column's length, and in a single-line
  place against the rules of the next section.
- A value whose canonical form differs from its written form is `format.noncanonical`.

## List rows

- A list row is `- [<glyph>] <key> <title>[ <field>…]`, or `- <key> <title>[ <field>…]` when the
  table has no state column. The glyph is the `checkbox` glyph of the row's state; it is exactly
  one character.
- A list glyph other than ` ` and `x` renders as literal bracket text in common viewers. The state
  names are the catalog's; the glyphs of `flowmark.dbs` are its convention.
- Fields are parsed from the right: a token `<declared column>:<value>`, where the value is a bare
  token without space, `"` or `\`, or `"…"` with `\"` and `\\` as its escapes. A field carries no
  sigil. Only stored scalar columns are fields.
- An absent field is NULL; `col:""` is the empty text. Every non-null column is written; a row
  missing a non-null field is `format.block`.
- A title token matching `<declared column>:` is written `<column>\:`. The title is the text between
  the key and the first field.
- A child row is indented two spaces deeper (the CommonMark list-item content indent) than its parent. The indentation is the `parent` column;
  nothing else writes it.
- The placement foreign key and `parent` are never written as fields of a block list row; writing one is
  `format.block`.

## Table rows

- The header row names the stored scalar columns. Every stored scalar column has a header; a header
  that names no column is `format.block`. The key is the first cell, under a header named by the key
  column; the stored scalar columns follow in column order.
- A declared title is a cell under its column's header, in column order.
- In a block table the placement foreign key is never a cell; `parent` is a cell.
- A cell holds text as it is. An empty cell is NULL, and the empty text is written `""`. Every
  non-null stored scalar column has a value.
- A cell value that begins with `"` is written `"…"` with `\"` and `\\` as its escapes. A `|` is
  written `\|`, and a `\` before `|` is written `\\`.

## Single-line places

A title (in a list row, in an H1 or in a cell), a list field, a field-list value and a table cell
are single-line places. A value there that contains a line feed, that has leading or trailing white
space, or that contains `<` followed by `!`, `/` or a letter is `CONSTRAINT` on write: raw HTML
would hide or alter it. A single-line value is inline markdown as written and is never escaped,
except as this section states.

- A title whose text ends with a space and one or more `#` is written in its H1 with its last `#`
  as `\#`, since CommonMark would read it as a closing sequence.

## The field list of a files row

- The field list is written right after the H1 line: one item `- <column>: <value>` for each
  non-null stored scalar value, in column order, followed by a blank line.
- A NULL value is an absent item. The empty text is an item with nothing after the colon:
  `- <column>:`.
- The body never begins with a `-` list item: a write whose body's first line is one is
  `CONSTRAINT`. A body may open with a `*` or `+` list or any other block. So on read, the `-`
  items directly after the H1 are the field list.
- An item that is not `- <stored scalar column>: <value>` in column order is `format.block`. An
  absent item of a non-null column is `format.block`; the engine writes a default explicitly on
  insert.

## Foreign keys as links

- A foreign key value is a link `[<key>](<target>)` in every place a scalar is written: a field of
  a list row, a cell of a table, an item of a `files` field list.
- The link text is the referenced key, which is the data. The target is derived from the
  referenced row:
  - the relative path, with `/` on every platform, to the file that holds the referenced row,
    followed by `#<anchor>` exactly when that row has a detail section;
  - a link inside the referenced row's own file is `#<anchor>` alone, or that file's name when the
    row has no anchor.
- A write that adds or removes a row's detail section rewrites every link to that row in the same
  transaction.
- A target that differs from the derived one is `format.noncanonical`. A key that names no row is a
  dangling reference. A key update of a referenced row is `FOREIGN_KEY` (restrict-only), so a target
  never moves under a link.
- A `parent` written in a `files` row is a link too. The placement foreign key of a block row is
  never written.
- Examples: a `files` row `comments/C5.flowmark.md` links its post as
  `[P3](../posts/P3.flowmark.md)`; a block comment inside `posts/P3.flowmark.md` links its author as
  `[max](../people.flowmark.md)`; the root `items.flowmark.md` links a row that has a detail section
  in the same file as `[T2](#items-t2)`.

## Encoding

UTF-8 without a BOM; LF is the canonical line ending; input accepts CRLF as valid (P2) and
reports it as non-canonical (P1); payload bytes are otherwise preserved as they
are.

## Diagnostics

Every rule above reports `{file, line, column, rule, message}` with the rule names
`format.noncanonical`, `format.section`, `format.block` and `format.encoding`. The walk's rule,
`corpus.walk`, is stated in the [document model](20-document-model.md).
