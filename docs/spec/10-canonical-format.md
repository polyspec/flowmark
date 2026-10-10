<!-- doc-id: spec-canonical-format -->
# Canonical format

[한국어](10-canonical-format.ko.md)

This specification pins the byte form of the storages the [document model](20-document-model.md)
names. It states the idempotency propositions, the document order, the list and table grammars,
the sections, and the encoding. Everything the renderer owns is stated here; everything else is
preserved payload.

## Idempotency

- **P1 (canonical identity).** For a canonical input `s`: `emit(parse(s)) == s`, byte for byte.
- **P2 (emission stability).** For any valid input `d`: `emit(parse(emit(d))) == emit(d)`.
- P1 and P2 replace any single byte-identity claim: a document that a Windows editor saves with
  CRLF is valid input, and P2 still holds; restoring P1 is a migration, not a side effect of a
  logical write.

## The skeleton and the preservation boundary

The renderer owns exactly the **skeleton**: the header comments, the frontmatter block, the H1,
the heading lines of block and detail sections, the list lines and table lines of the rows, and
the blank lines between skeleton elements. The values of `markdown` columns — the body and the
column sections — are payload: the renderer moves them with their row and never rewrites their
bytes.

- Non-canonical input (a BOM, CRLF, a skeleton line in a non-canonical shape, rows out of their
  storage order) is a `format.noncanonical` diagnostic. `flowmark canon` is a one-shot migration
  transaction that rewrites the document to canonical form; a logical write never normalizes as
  a side effect.

## Document order

- **A `files` row** is, in order: the header comments (`<!-- doc-id: <key> -->` first, then any
  provenance comment); the frontmatter block; the H1 title; the body; the level-2 sections of the
  row scope.
- **A `document` file** `<table>.md` is: `<!-- doc-id: <table> -->`; `# <table>`; the list or table
  of the rows directly under the H1; the detail sections at level 2.
- **A block section** is a heading at level b named by the child table, inside the parent row's
  scope; its list or table follows the heading, and its rows' detail sections follow at level
  b + 1.
- **A detail section** is the heading `<h> <key>` — the key alone, never a title copy — followed
  by the row's body and then the row scope's sections at level h + 1. A detail section whose key
  names no row of its block is `format.block`. Detail sections of nested list rows are flat
  siblings. Rows without a detail section have the empty body.
- A heading that a layout would need past H6 is a schema error, decided when the catalog loads.

## Sections and their values

- The body and every `markdown` column value of a row scope at level L end at the next heading
  of level ≤ L, counted only outside fenced blocks (CommonMark block structure). A value holds
  only headings deeper than L; a write of a value with a heading line of level ≤ L outside a
  fence is `CONSTRAINT`.
- In a row scope at level L, each nullable `markdown` column and each child block is a heading at
  level L named by the column or the child table. An absent section is NULL; a present empty
  section is the empty text. The body is never NULL. An undeclared heading at level L is
  `format.section`.

## List rows

- A list row is `- [<glyph>] <key> <title>[ <field>…]`, or `- <key> <title>[ <field>…]` when the
  table has no state column. The glyph is the `checkbox` glyph of the row's state.
- Fields are parsed from the right: a token `<declared column>:<value>`, where the value is a bare
  token without space, `"` or `\`, or `"…"` with `\"` and `\\` as its escapes. Only declared
  scalar columns are fields. An absent field is NULL; `col:""` is the empty text; every non-null
  column is written — the engine writes a default explicitly on insert, and a row missing a
  non-null field is `format.block`.
- A title token matching `<declared column>:` is written `<column>\:`. The title is the text
  between the key and the first field.
- A child row is indented two spaces deeper than its parent; the indentation is the `parent`
  column and nothing else writes it. The placement foreign key and `parent` are never written as
  fields; writing one is `format.block`.

## Table rows

- A table's header row names the stored scalar columns; every stored scalar column has a header,
  and a header that names no column is `format.block`. A cell writes `|` as `\|`.
- An empty cell is NULL; the empty text is written `""`; every non-null column has a value.

## Single-line places

A list title, a list field and a table cell are single-line: a value with a line feed there is
`CONSTRAINT` on write. A key in a list or table storage contains no space, `#`, `|` or line feed.

## Frontmatter profile

- YAML **1.2 core schema**, block style in the value region; anchors, aliases and tags are
  rejected; comments are rejected in v1 (a comment-preserving round-trip emitter is a stated
  non-goal until a requirement names it).
- Each key is a declared scalar column of the `files` table; an unknown key is `format.frontmatter`.
  An absent key is NULL; every non-null column has its key, and a missing one is
  `format.frontmatter`.
- Value syntax per type: every value uses the dbspec canonical literal form; `bytes` is base64.

## Encoding

UTF-8 without a BOM; LF is the canonical line ending; input accepts CRLF as valid (P2) and
reports it as non-canonical (P1); keys are NFC; payload bytes are otherwise preserved as they
are.

## Diagnostics

Every rule above reports `{file, line, column, rule, message}` with the rule names
`format.noncanonical`, `format.header`, `format.frontmatter`, `format.section`, `format.block`
and `format.encoding`.
