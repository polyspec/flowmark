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
  link whose `#<anchor>` is the anchor that GitHub, GitLab, VS Code and MkDocs generate for the heading
  it names, by the rules of [detail headings and anchors](#detail-headings-and-anchors). Pandoc is not
  a viewer the claim is made for: its default does not rewrite relative link targets, so a link between
  corpus files is not claimed to be followed in its output.
- Grounding: the documents are the database's only storage, so they must stay readable documents.
  What only the machine needs belongs to the catalog (`dbspec/`) or to `.flowmark/`.

## The skeleton and the preservation boundary

The renderer owns exactly the **skeleton**: the H1 line, the field list of a `files` row, the
heading lines of the block, detail and markdown-column sections, the list lines and table lines of
the rows, and the blank lines between skeleton elements. The values of `markdown` columns — the body
and the column sections — are payload: the renderer moves them with their row and never rewrites their bytes, apart from the line endings and the byte order mark that `flowmark canon` removes ([Encoding and line structure](#encoding-and-line-structure)).

- Non-canonical input (a BOM, CRLF, a skeleton line in a non-canonical shape, rows out of their
  storage order) is a `format.noncanonical` diagnostic. `flowmark canon` is a one-shot migration
  transaction that rewrites the document to canonical form; a logical write never normalizes as
  a side effect.

## Document order

- **A `files` row** is the file `<table>/<key>.flowmark.md`. In order it holds: the H1 line
  `# <title>`; the field list of its stored scalar values, when it has one; the body; the level-2
  sections of the row scope. The blank lines between these elements are stated under the field list
  and under the framing of values.
- **A `document` file** `<table>.flowmark.md` holds: the H1 line `# <table>`; the list or table of its
  rows directly under the H1; the detail sections at level 2. A missing document file is an empty
  table ([document model](20-document-model.md#storage)); when its last row is deleted, the file keeps
  the H1 line and one LF.
- **A detail section** is the heading `<h> <table> row <key>` — the table name, one space, the word
  `row`, one space and the key, never a title copy — followed by the row's body and then the row
  scope's sections at level h + 1. A detail row's scope is at level h + 1. Detail sections of nested
  list rows are flat siblings.
- **A block section** is a heading at level b named by the child table, inside the parent row's
  scope; its list or table follows the heading, and its rows' detail sections follow at level b + 1.
- A row scope's sections stand in a fixed order: the `markdown` columns other than the body, in column
  order, then the child blocks in table name order (dbspec's canonical order). Detail sections stand in
  the storage order of their rows. Grounding: the order is a function of the catalog, so a row has one
  form.
- Any content after a block's or document's list or table other than its rows' detail sections — up to
  the next heading of level ≤ b in a block section, or to the end of a document file — is
  `format.block`.
- A `files` row without a non-null stored scalar has no field list; a non-empty body follows the H1 after one blank line, and an empty body writes nothing.
- A block or document row has a detail section exactly when its body is not empty, its table has a
  non-null `markdown` column other than the body, a nullable `markdown` column of it is not NULL, or it
  has a child block row. A detail section without any of these is `format.noncanonical`. A row without
  a detail section has the empty text as its body. Grounding: a non-null section is always written, so
  its row always has a detail section; a row with nothing to write has none.
- A detail section whose key names no row of its block is `format.block`.
- A layout that needs a heading past H6 is a schema error, decided when the catalog loads.

## Detail headings and anchors

- The anchor of a detail heading is `<table>-row-<lowercased key>`: the heading `## items row T2`
  has the anchor `items-row-t2`. The table is the anchor text before its first `-`, since a table
  name contains no `-`.
- **The folds.** Two folds of a heading's rendered inline text decide a collision: the text as GitHub
  slugs it (the text of a link without its target, emphasis markers removed, code span content kept,
  entities decoded, raw HTML tags not text). F_G removes every character whose Unicode general
  category is not a letter (L) or a decimal digit (Nd), lowercases the rest, decomposes it by Unicode
  compatibility (NFKD), drops the combining marks, and keeps only the ASCII letters and digits that
  remain. F_M lowercases the text, decomposes it by NFKD, drops the combining marks, and keeps only the
  ASCII letters and digits that remain. Grounding: the anchors of GitHub, GitLab and VS Code keep
  letters, digits, hyphens, underscores and spaces and remove other characters, superscript and
  circled digits (category No) among them, which F_G removes. MkDocs keeps the ASCII letters and digits
  of the NFKD form of the text, which F_M keeps; F_G does not, so both folds are used. `café` folds to
  `cafe` under both.
- **The collision rule.** A heading is every ATX heading, also after a block-quote or list-item marker,
  and every paragraph followed by a setext underline, the whole paragraph being the heading, outside
  fenced blocks; a detail heading is one too. A heading H other than a detail heading takes the anchor
  A of a detail heading when, under F = F_G or under F = F_M, F(H) and F(A) are equal once a final run
  of decimal digits is removed from each. The fold of an anchor is the same under both folds, since its
  letters and digits are ASCII. Two detail headings collide only when their anchors are equal.
  Grounding: every named viewer numbers a repeated or taken id only by appending a number or by
  replacing a final number (GitHub, GitLab and VS Code append `-<n>`; MkDocs appends `_<n>` or raises a
  final `_<n>`, and it counts ids that attribute lists set as taken), so comparing the folds without
  their final digits covers every numbering, whichever heading or id came first. The cost is that a
  payload heading that differs from a detail heading only in its final number is refused too.
  Accepted: keys `T1` and `T10` in one file never collide, since both are detail headings and only equal
  anchors collide between them. Refused: a payload heading `items row T` beside the detail heading
  `items row T_1`; a payload heading `Notes` beside it is not refused. The check applies to every
  heading of a file, skeleton and payload alike. It also refuses, in the payload of a file, an attribute
  list `{…}`, with or without a leading `:`, that sets the id A by `#A` or by `id=` followed by A bare or in single or double quotes, and an
  HTML `id` or `name` whose value is A, all compared without regard to case. Grounding:
  Python-Markdown's attr_list extension, which MkDocs can enable, reads such a list at the end of a
  heading or block as its id, and an HTML viewer reads `id` and `name` as anchors, whatever the text of
  the heading.
- A write that introduces a collision is `CONSTRAINT`, naming the other heading: a title update, a payload write, a row insert that creates a detail section, a key update, a move, or the creation of a detail section; except that a write giving a row a key equal, with or without regard to case, to another key of its table is `DUPLICATE_KEY`, the key rule of the [document model](20-document-model.md#keys). A row without a detail section has no anchor, so it collides with nothing. On read a collision is `format.block`, except between two detail headings whose keys differ only in case, which is `key.duplicate` alone. Two detail sections of one file under the same
  heading text collide, so they are `format.block`, since neither can be assigned to a row.
- A detail heading in a non-canonical spelling (extra spaces, a wrong case of `row`) is
  `format.noncanonical`, so `flowmark canon` repairs it.

## Sections and their values

- The body and every `markdown` column value of a row scope at level L end at the next heading of level
  ≤ L, counted only outside fenced blocks (CommonMark block structure). A value holds only headings
  deeper than L; a write of a value with a heading line of level ≤ L outside a fence is `CONSTRAINT`.
- Every `markdown` column other than the body is a section of the row scope at level L named by the
  column: a non-null column's section is always present, and a nullable column's section is absent
  exactly when its value is NULL. An absent non-null section is `format.block` on read and `CONSTRAINT`
  on write. A present empty section is the empty text. The body is never NULL and is not a section.
  Each child block is a heading at level L named by the child table. An undeclared heading at level L is
  `format.section`. Grounding: a non-null value is always written, so its section cannot be absent; a
  NULL value has no text, so its section is absent.
- A value is framed in the file: a blank line follows its heading only when the value is non-empty, the
  value's lines follow, and a blank line precedes the next element when one follows. The value neither
  begins nor ends with a line feed and holds no line of only white space at either end; a write of a
  value that does is `CONSTRAINT`. Grounding: the framing blank lines separate the elements, so a value
  that began or ended with a line feed would read back with its framing changed, and a line of only
  white space at either end would be read as a framing line.
- The framing blank lines are skeleton: on read, a further blank line or line of only white space at either end of a value is not part of the value and is `format.noncanonical`, and so is a missing framing blank line where the element still reads back, except a body line directly after the last field-list item, which is `format.block`; `flowmark canon` repairs both.
- A child block section is written only when the block holds at least one row — an empty block and an
  absent block both mean zero rows, so only one form can be canonical (P1); a parent row with no child
  rows has no block heading, and an empty block heading is `format.noncanonical`.
- A `markdown` column is always a section of the row scope, never a field or a cell, in every storage.

## Stored scalar columns

The stored scalar columns of a table are its columns minus the key, the `title` of a `list` or `files` storage, the `body`, the
`markdown` columns, the state column of a `list`, the placement foreign key, the `parent` of a
`list`, and the `order` column of a `block` or `document` storage. The key, the title, the state
and the hierarchy have their own places. A stored scalar column is a field-list item of a `files`
row, a field of a list row or a cell of a table. Every non-null stored scalar column is written; the
engine writes a default explicitly on insert.

## Readable values

Scalar values are written in a readable form, not as dbspec literals. The type always comes from the
catalog.

- Text is written as it is. An integer, a decimal or a float is written in its canonical number form;
  `true` and `false` are written as such.
- A date is `YYYY-MM-DD`; a datetime is `YYYY-MM-DD HH:MM:SS[.f…]`; a time is `HH:MM:SS[.f…]`. A datetime
  and a time have exactly the column's precision in fraction digits.
- A uuid is lowercase. Bytes are written as padded base64.
- A foreign key is a link (see [Foreign keys as links](#foreign-keys-as-links)).
- The engine reads a number, `true` or `false` with orm's `dbspec.CanonicalLiteral(t, v)`, and a date,
  time, datetime or uuid with `CanonicalLiteral(t, "'" + v + "'")`, with the quotes removed from the
  result.
- `now` is refused as a value. Text is checked against its column's length, and in a single-line place
  against the rules of [Single-line places](#single-line-places).
- On read, a value whose written form differs from its canonical form is `format.noncanonical`. A value not of its column type is the validator's `type`; it never makes the file unreadable, and a request that evaluates that value — returns it, or uses it in a predicate, join, sort, group or aggregate — fails as `CONSTRAINT`, naming the diagnostic. A write that assigns a value not of its column type is `CONSTRAINT`; a `type` diagnostic on a value the write does not assign, in its own row or another, fails the commit only when a constraint check of the commit must evaluate that value (a unique key, a `check`, a `limit`), as `CONSTRAINT` naming the diagnostic, and every rewrite of the file, `flowmark canon` included, keeps that value's written text byte for byte. A key is not a value under this rule: a key not of its type, or not in its canonical text, is `format.block` ([document model](20-document-model.md#keys)). Grounding: a value the engine cannot read as its type has no answer and no comparison, so a request or a check that needs it cannot be answered, while one that does not need it is unaffected.

- On write, a number, `true`/`false`, a date, a time, a datetime or a uuid is written as `CanonicalLiteral` gives it (`1.5` into `decimal(4,2)` is `1.50`; a float in fixed notation with the fewest digits that round-trip, zero as `0`), and a value `CanonicalLiteral` refuses is `CONSTRAINT` — for example a decimal with more fraction digits than its scale (zeros included) or more integer digits than precision − scale, or a time or datetime with more fraction digits than its precision. Bytes are written as padded standard base64 (RFC 4648 §4); on read, any other text in a bytes column is `type`. Grounding: orm parity.
- A text or `markdown` value is written byte for byte, apart from the escapes its place states (a title's `\:` and `\#`, a quoted list field or cell, a cell's `\|`); one that cannot be written so is `CONSTRAINT`: a CR, a U+0000, invalid UTF-8, a single-line violation or a framing violation.
- `flowmark canon` ([operations](40-ops.md#the-canon-operation)) is the one operation that normalizes document bytes; a logical write never does.

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

- The header row names the stored scalar columns. Every stored scalar column has a header; a missing header, or a header that names no column, is `format.block`. The key is the first cell, under a header named by the key
  column; the stored scalar columns follow in column order.
- A declared title is a cell under its column's header, in column order.
- In a block table the placement foreign key is never a cell; `parent` is a cell.
- A cell holds text as it is. An empty cell is NULL, and the empty text is written `""`. Every
  non-null stored scalar column has a value.
- A cell value that begins with `"` is written `"…"` with `\"` and `\\` as its escapes. A `|` is
  written `\|`, and a `\` before `|` is written `\\`.

## Single-line places

A title (in a list row, in an H1 or in a cell), a list field, a field-list value and a table cell are
single-line places. A single-line value is inline markdown as written and is never escaped, except as
this section states.

- On write, a value there that contains a line feed or a CR, that has leading or trailing white space,
  or that contains `<` followed by `!`, `/` or a letter is `CONSTRAINT`: a line feed cannot be written
  on one line, and raw HTML would hide or alter the value.
- On read, a single-line value that renders differently from its text beyond its inline markdown is `format.block`; this is exactly a quoted list field or cell with leading or trailing white space, or a value with `<` followed by `!`, `/` or a letter (raw HTML
  would hide or alter it). White space that rendering drops around a single-line place is `format.noncanonical`. Each place has one canonical separator (`# ` after the H1 marker, `: ` after a field-list column or nothing for the empty text, one space between list tokens, `| v |` in a cell); any other white space there is `format.noncanonical`.
- A title in an H1 whose final run of `#` follows a space or a tab, or that consists only of `#`, is
  written with its last `#` as `\#`, since CommonMark would read the run as a closing sequence (a line
  of `#` alone is an empty heading). On read, the title is the raw heading content with CommonMark's leading and trailing spaces, tabs and closing sequence removed, and a final `\#` read as `#`; no other escape is decoded. A final `\#` that the write rule would not produce (as in `# C\#`) is `format.noncanonical`. A title ending in `\#` is `CONSTRAINT` on write, since it would
  read back as `#`: `\#` and `#`, and `#\#` and `##`, would be written alike. On read, an H1 closing
  sequence is `format.noncanonical`. Grounding: a title is written so that it reads back as the same
  text.

## The field list of a files row

- The field list is written right after the H1 line: one item `- <column>: <value>` for each non-null
  stored scalar value, in column order. A blank line follows it exactly when a non-empty body or a section follows it, so a row with neither ends at its last item. Grounding: the blank line separates two elements; with
  none after it, there is none to separate.
- A NULL value is an absent item. The empty text is an item with nothing after the colon: `- <column>:`.
- The body of a `files` row never begins with a `-` list item, and never with a line indented by two or more columns (spaces, or a tab) after the field list: a write of such a body is `CONSTRAINT`. On read, the `-` items
  directly after the H1 are the field list, so a body that opens with a `-` item is `format.block`, and
  so is a body whose first line is indented by two or more columns (spaces, or a tab) when a field list precedes it. On read, a body line directly after the last field-list item without the blank line is `format.block`, since CommonMark reads it as a continuation of that item. The body may open with a
  `*` or `+` list or any other block. Grounding: a `-` item after the H1 is a field in the grammar, so
  such a body could not be read back as the body it was written from.
- An item that is not `- <stored scalar column>: <value>` in column order is `format.block`; white space other than the canonical separator is `format.noncanonical`, as [Single-line places](#single-line-places) states. An absent
  item of a non-null column is `format.block`; the engine writes a default explicitly on insert.

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
- A target that differs from the derived one is `format.noncanonical`. A key that names no row is a dangling reference: the validator reports it as `reference.dangling`, a write that introduces it is `FOREIGN_KEY`, and it never makes the file unreadable. A key update of a referenced
  row is `FOREIGN_KEY` (restrict-only), so a target never moves under a link.
- A `parent` written in a `files` row is a link too. The placement foreign key of a block row is
  never written.
- Examples: a `files` row `comments/C5.flowmark.md` links its post as
  `[P3](../posts/P3.flowmark.md)`; a block comment inside `posts/P3.flowmark.md` links its author as
  `[max](../people.flowmark.md)`; the root `items.flowmark.md` links a row that has a detail section
  in the same file as `[T2](#items-row-t2)`.

## Encoding and line structure

- A document is UTF-8 without a byte order mark, and LF is its canonical line ending; payload bytes are
  otherwise preserved as they are.
- A byte order mark, and a CR (bare or in a CRLF line ending), is valid input under P2 and is
  `format.noncanonical`: P1 fails, since the canonical form has neither. On read, a value reads every
  line ending as LF. Grounding: multi-platform — a Windows checkout with `autocrlf` must read the same
  values as a Unix checkout. Only `flowmark canon` removes these bytes.
- Invalid UTF-8, and a U+0000, is `format.encoding`: no text can be read from them.
- A document ends with exactly one LF. A document without a final LF, or with a trailing blank line, is
  `format.noncanonical`. Grounding: the grammar has no element after the last line, so a further line
  feed is not part of any element and would be lost on emit.
- A logical write that rewrites a file with a `format.*` diagnostic is `CONSTRAINT` apart from the exemptions below. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([Unavailable table](#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([Unavailable table](#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose detail section appears or disappears, an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](40-ops.md#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child and history blocks, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable. Without the `.gitattributes` that install writes, a Windows `autocrlf` checkout reads every value but refuses every write that rewrites an existing document, apart from the exemptions above, until `flowmark canon`. Grounding: a write must not change bytes the user did not write; a migration is a separate, explicit transaction.
- `flowmark install` adds the line `*.flowmark.md text eol=lf` to `<corpus>/.gitattributes`, creating the file when it is missing and keeping its other lines, so a git checkout keeps LF on every platform ([schema](30-schema.md#install)).

## Diagnostics

Every rule above reports `{file, line, column, rule, message}` with the rule names `format.noncanonical`, `format.section`, `format.block` and `format.encoding`, and the walk's rule `corpus.walk` of the
[document model](20-document-model.md).

- **Read and write faults.** On read, these are `format.block`: a missing non-null field or section; a
  single-line value that renders differently from its text beyond its inline markdown (see [Single-line places](#single-line-places));
  a `files` body whose first line is a `-` list item; a `files` body whose first line is indented by two or more columns (spaces, or a tab) after a field list. The same faults are `CONSTRAINT` on write. White space that rendering drops around a single-line place is `format.noncanonical` on read. Grounding: a
  read reports what the document holds, and a write refuses to produce what could not be read back.
- **Validator rules.** A value not of its column type is `type`, a dangling foreign key is `reference.dangling`, and a key equal to another key of its table without regard to case is `key.duplicate`. The validator reports them over the corpus. They never make a file unreadable. Grounding: a file's readability must not depend on other files, or deleting a parent file would make its children unreadable and unrepairable. A value not of its column type is `CONSTRAINT` on write; a write that introduces a dangling foreign key or a duplicate key is `FOREIGN_KEY` or `DUPLICATE_KEY`. The commit refusal is stated under [Encoding and line structure](#encoding-and-line-structure). Opening a corpus, introspection, `flowmark validate` and `flowmark canon` are not requests over rows: they never fail on an unavailable table, apart from canon's link-derivation refusal ([operations](40-ops.md#the-canon-operation)). The validator reports every file's `format.*` diagnostics, and the validator rules of every row it can read in a file without `format.block`, `format.section` or `format.encoding`; a rule that needs the complete rows of a table that holds a file with `format.block`, `format.section` or `format.encoding` — `reference.dangling` into it, `key.duplicate`, `order` and `state.limit` within it — is not reported for that table, and the file's own diagnostic stands for it; a table unavailable only through byte-for-byte equal keys has every rule reported.
- **Unavailable table.** A table is unavailable when it holds a file with `format.block`, `format.section` or `format.encoding`, or when two of its rows have byte-for-byte equal keys; the request then names the second row's file and `key.duplicate`. A table holds every corpus document at a place its storage names: a `files` table every `<table>/<name>.flowmark.md`, a `document` table `<table>.flowmark.md`, and a block table every file its placement parent's table holds, recursively, so a history block is unavailable with its state table. A table is also unavailable while a `corpus.walk` entry stands at such a place, or at a name equal to one without regard to case — an entry that cannot be read, a symlink, a symlinked table folder, or a suffix, folder or document name in another case. A file whose name does not end with `.flowmark.md`, compared without regard to case, is outside the database and never makes a table unavailable. Every request that needs any of its rows — a read of it, a point lookup by key included, an insert, update or delete in it, a join, relation or subquery over it, the rewrite of the links to a row whose detail section appears or disappears when a link may lie in it, and every constraint check over it (a primary or unique key, a reference, a link target, a `limit` count, an `order` sibling set, a `check`) — fails as `CONSTRAINT`, naming the file and the diagnostic. A request never answers from the other files' rows alone. A `files` file whose name is not the canonical text of a key is `format.block`, so its table is unavailable too. Grounding: an answer or a check over rows the engine cannot read would be wrong without saying so, and a request cannot name one of two rows with byte-for-byte equal keys. The engine writes nothing in an unavailable table apart from `flowmark canon` on its files ([operations](40-ops.md#the-canon-operation)): a human repairs, renames or removes the row or file that makes it unavailable by editing the corpus, and the table is available again on the next read. Grounding: a request cannot name one of two byte-equal rows, and a row the engine cannot read cannot be deleted through it.
