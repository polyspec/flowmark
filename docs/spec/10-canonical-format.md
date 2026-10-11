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
- Its structure is ordinary markdown: headings, lists, tables and a field list. Structure — the blocks,
  the inlines, what is code, what is a heading, what is a table — is judged by CommonMark 0.31.2, and by
  GFM 0.29 for tables and task-list items: the specifications that GitHub (cmark-gfm), GitLab and VS
  Code (markdown-it) implement. Grounding: a specification is one fixed measure that these viewers
  follow; an implementation that follows neither specification has its own list, setext, fence,
  reference-definition and extension behaviour, which changes with its releases and extensions, so
  following it would chase one implementation's deviations rather than a standard, and it is no
  criterion.
- The specifications do not define the targets of a fragment. Anchors — heading slugs and the ids and
  names of raw HTML that CommonMark reads outside code — are judged by GitHub, GitLab and VS Code (the
  F_G fold for headings), because a link `[k](file#anchor)` must resolve in them. Every relation is a
  link whose `#<anchor>` is the anchor that GitHub, GitLab and VS Code generate for the heading it
  names, by the rules of [detail headings and anchors](#detail-headings-and-anchors). Pandoc is not a
  viewer the claim is made for: its default does not rewrite relative link targets, so a link between
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
  rows directly under the H1; the detail sections at level 2. An H1 whose text equals the table name
  after lowercasing, compared as a detail heading's table name is
  ([Detail headings and anchors](#detail-headings-and-anchors)) — `# Items` in `items.flowmark.md` — is
  the file's H1 line in a non-canonical spelling and is `format.noncanonical`. Grounding: the file name
  fixes the table, so the H1 names it and carries no data; a viewer shows the rows under it alike, and
  every named viewer lowercases a heading's anchor, so its anchor and its fold under the collision rule
  are those of `# <table>`; `flowmark canon` writes `# <table>`. A missing document file is an empty
  table ([document model](20-document-model.md#storage)); when its last row is deleted, the file keeps
  the H1 line and one LF. The list or table of its rows stands directly under the H1 line, with no
  blank line between them: one or more blank lines there (a blank line being a line of only spaces and
  tabs) are `format.noncanonical`. Grounding: an ATX heading is a single line, so CommonMark 0.31.2 and
  GFM read the list or table that follows alike with or without blank lines, and every viewer renders
  the two alike; the rows are what the H1 names, as a block's rows are what its heading names, so both
  stand directly under their heading, and a blank line separates two elements, the rows and the first
  detail section.
- **A detail section** is the heading `<h> <table> row <key>` — the table name, one space, the word
  `row`, one space and the key, never a title copy — followed by the row's body and then the row
  scope's sections at level h + 1. A detail row's scope is at level h + 1. Detail sections of nested
  list rows are flat siblings.
- **A block section** is a heading at level b named by the child table, inside the parent row's
  scope; its list or table stands on the line directly after the heading, and its rows' detail sections follow at level b + 1. Blank lines (lines of only spaces and tabs) between the heading and its list or table, in any number, are `format.noncanonical`. Grounding: an ATX heading ends at its line, so CommonMark and GFM read a list or table after it alike with or without blank lines, and the meaning is certain; the list or table is the heading's own element, as a document file's rows stand under its H1 and a `files` field list under its H1, so it stands directly under it, and one form is canonical (P1).
- The rows of one list or table — a document file's, or one block section's — are one **block**; a document file holds one block. A `parent` of a block or document row names a row of its own block: a write that sets another is `CONSTRAINT`, and on read a `parent` that names no row of its block, a row of another block of the file included, is `format.block`. Grounding: `parent` is a self foreign key, and a row's siblings share its placement parent and its `parent` ([document model](20-document-model.md#order)), so a parent in another block would leave its child no place in the storage order; a list writes `parent` as indentation, which cannot reach another block, so a wider scope would make a table hold hierarchies its list form cannot, against the rule that the same rows can be stored either way ([document model](20-document-model.md#storage)).
- A row scope's sections stand in a fixed order: the `markdown` columns other than the body, in column
  order, then the child blocks in table name order (dbspec's canonical order). Detail sections stand in
  the storage order of their rows. Grounding: the order is a function of the catalog, so a row has one
  form.
- Any content after a block's or document's list or table other than its rows' detail sections — up to
  the next top-level heading of level ≤ b in a block section, or to the end of a document file — is
  `format.block`.
- One blank line stands between a block's or document's list or table and the element that follows
  it — the first detail section, or the next heading of the enclosing row scope — and none when the
  list or table ends the file. On read, a blank line is a line of only spaces and tabs, as CommonMark
  0.31.2 defines it (§2.1), so such a line there is never content after the rows; any other number of
  blank lines there, none included, and a blank line that holds a space or a tab, are
  `format.noncanonical`. Grounding: CommonMark ends a list at an ATX heading that is not indented to an
  item's content (§5.2), and GFM ends a table at a blank line or at the start of another block (GFM
  0.29, Tables), so the rows and the heading after them read alike with any number of blank lines
  between them; the blank line separates two elements, as the framing of values does, so one form is
  canonical (P1).
- A `files` row without a non-null stored scalar has no field list: its first body or section follows the H1 after exactly one blank line, and an empty body writes nothing. Grounding: the field list is the H1's own item block and stands right after the H1, whereas a body or section is a separate element; any other count of blank lines there is `format.noncanonical`.
- A block or document row has a detail section exactly when its body is not empty, its table has a
  non-null `markdown` column other than the body, a nullable `markdown` column of it is not NULL, or it
  has a child block row. A detail section without any of these is `format.noncanonical`. A row without
  a detail section has the empty text as its body. Grounding: a non-null section is always written, so
  its row always has a detail section; a row with nothing to write has none.
- A detail section whose key names no row of its block is `format.block`, and so is a second detail
  section of the same row, whatever its spelling: its body and sections cannot be assigned.
- A detail section is assigned by its place to its block and by its key to a row of that block. A detail section whose key two rows of its block share is `format.block`, even when only one of them has a detail section, since the section cannot be assigned. A key that rows of different blocks of the file share byte for byte does not hinder the assignment: when only one of them has a detail section the file reads, and the two rows are `key.duplicate`, which makes their table unavailable ([Unavailable table](#diagnostics)); when both have one, their headings collide and the file is `format.block` ([Detail headings and anchors](#detail-headings-and-anchors)). Grounding: a block's detail sections stand inside its block section, so their place decides the block, and only the key within the block can fail to decide the row. Keys equal only without regard to case match one row each, since a detail section takes the row whose key it names byte for byte; such a pair is assigned, and it is `key.duplicate`.
- **Storage order.** The codec computes a file's storage order from its row set, as the
  [document model](20-document-model.md#order) defines it, and never refuses an unsorted row set. A
  cycle among a file's rows is `parent.cycle` on read, and the file is unreadable, since its rows have
  no storage order. Grounding: the storage order is a function of the rows, so a row set has one form.
- A layout that needs a heading past H6 is a schema error, decided when the catalog loads.

## Detail headings and anchors

- The anchor of a detail heading is `<table>-row-<lowercased key>`: the heading `## items row T2`
  has the anchor `items-row-t2`. The table is the anchor text before its first `-`, since a table
  name contains no `-`.
- **The fold.** One fold of a heading's rendered inline text, F_G, decides a collision: the text as
  GitHub slugs it (the text of a link without its target, emphasis markers removed, code span content
  kept, entities decoded, raw HTML tags not text). The rendered inline text is the one CommonMark 0.31.2
  parses, with the link reference definitions of the whole file: a definition is read where CommonMark
  reads one, a block at the start of a paragraph, inside a container or not, wherever it stands. A
  full, collapsed or shortcut reference link whose label matches such a definition counts by its link
  text, and a bracketed text whose label matches none counts as its literal text, brackets and label
  included. Grounding: GitHub, GitLab and VS Code parse CommonMark, where a link reference definition
  is a block at the start of a paragraph, so a defined `[items row T1][r]` renders the text
  `items row T1`. An image counts under F_G both by its alt text and by no text: a heading takes the
  anchor when either reading collides. Grounding: VS Code's markdown language service builds a
  heading's slug from the text of its inline tokens, an image's alt text included; GitHub and GitLab
  take the heading's text, and reading F_G both ways covers them whichever they take. F_G removes every
  character whose Unicode general category is not a letter (L) or a decimal digit (Nd), lowercases the
  rest, decomposes it by Unicode compatibility (NFKD), drops the combining marks, and keeps only the
  ASCII letters and digits that remain. Grounding: the anchors of GitHub, GitLab and VS Code keep
  letters, digits, hyphens, underscores and spaces and remove other characters, superscript and
  circled digits (category No) among them, which F_G removes. `café` folds to `cafe` under F_G.
- **The collision rule.** A heading is every ATX heading, also after a block-quote or list-item marker,
  and every paragraph followed by a setext underline, the whole paragraph after the link reference
  definitions at its start being the heading, outside fenced blocks; a detail heading is one too. Those
  definitions are no part of the heading: a setext heading begins on the first line they leave, they are
  content before it, and definitions alone followed by an underline make no heading. Grounding: CommonMark 0.31.2
  reads the definitions at the start of a paragraph before it closes the paragraph as a setext heading
  (§4.3, §4.7), so `[r]: /u`, `people row P1`, `---` is a definition and the heading `people row P1`, as
  commonmark.js, cmark-gfm and markdown-it read it. For this rule, a detail heading is a top-level heading at the detail level of a table of the file's layout — the level of its rows' detail headings, one less than their row scope — whose text, its spaces, tabs and the line endings of a setext heading aside, is that table's name and the word `row`, each equal to its canonical text once lowercased as the viewers lowercase it, and a key: the heading of a detail section, in its canonical or a non-canonical spelling. Every other heading is a payload heading, a `<table> row <key>` heading at another level or inside a block quote or list item included, and its fold is compared with every detail anchor. Grounding: a detail section is assigned by its place, so a heading elsewhere names no row and renders an anchor of its own, which a viewer numbers when it repeats a detail anchor. A heading H other than a detail heading takes the anchor
  A of a detail heading when F_G(H) and F_G(A) are equal once a final run of decimal digits is removed
  from each. Two detail headings collide only when their anchors are equal. Grounding: every named
  viewer numbers a repeated id by appending `-<n>` (GitHub, GitLab and VS Code), so comparing the folds
  without their final digits covers every numbering, whichever heading came first. The cost is that a
  payload heading that differs from a detail heading only in its final number is refused too.
  Accepted: keys `T1` and `T10` in one file never collide, since both are detail headings and only equal
  anchors collide between them. Refused: a payload heading `items row T` beside the detail heading
  `items row T-1`, since a second `items row T` is numbered `items-row-t-1`; a payload heading `Notes`
  beside it is not refused. The check applies to every heading of a file, skeleton and payload alike.
  It also refuses, anywhere in the file, the skeleton included, an HTML `id` or `name` whose value is A,
  in raw HTML as CommonMark 0.31.2 reads it (a code span or code block holds none); all compare without
  regard to case. Grounding: an HTML viewer reads `id` and `name` as anchors, whatever the text of the
  heading; GitHub prefixes both a heading's id and a raw `id` or `name` with `user-content-`, so they
  still collide, and VS Code keeps them as written. A `{#…}` is literal text in CommonMark and GFM, and
  no criterion viewer reads an id from it.
- A write that introduces a collision is `CONSTRAINT`, located at the row and column whose written text holds the heading or id that takes the anchor — a `markdown` column's value, a `files` row's title for its H1, or, for a section or block heading, the row whose scope it opens, at the `markdown` column or child table it names — naming the detail heading whose anchor it takes: a title update, a payload write, a row insert that creates a detail section, a key update, a move, or the creation of a detail section; except that a write giving a row a key equal, with or without regard to case, to another key of its table is `DUPLICATE_KEY`, the key rule of the [document model](20-document-model.md#keys). A row without a detail section has no anchor, so it collides with nothing. On read a collision is `format.block`, except between two detail headings whose keys differ only in case, which is `key.duplicate` alone. Two detail sections of one file under the same
  heading text collide, so they are `format.block`, since neither can be assigned to a row.
- On read, a collision is located at the element that collides ([Diagnostics](#diagnostics)): a heading at
  the first line of its text and the first character of its text;
  an HTML `id` or `name` at the `<` of its tag. This is the read location only: a
  write that introduces a collision is located as [Write faults](#diagnostics) and the clause above state, at
  the row and column whose written text holds the heading or id that takes the anchor, and its message names
  the detail heading. Grounding: a column locates the text a rule names, and a block's first line can stand
  far from an `id` deep inside an HTML block.
- A detail heading, section heading or block heading in a non-canonical spelling (a run of spaces or tabs other than the
  canonical single space — before its text, between its words or after it —, a tab after its hashes, a
  setext underline in place of the hashes, a case of the table name or of `row` other than the canonical
  one) is `format.noncanonical`, so `flowmark canon` repairs it. Its diagnostic is located at the first
  character of its text ([Diagnostics](#diagnostics)). The
  canonical separator after the hashes is one space; a tab is never written, though CommonMark reads
  `##` followed by a tab as the same heading. Nor is a setext underline written, though CommonMark reads
  a paragraph over a `-` underline as the same level-2 heading, and over a `=` underline as the same
  level-1 heading (§4.3), so a setext section or block heading names the same section or block. Grounding: the table name is fixed by the place of the
  detail level and the word `row` by the grammar, and every named viewer lowercases both in the
  anchor, so a heading that differs from the canonical one only in their case names the same row
  under the same anchor. The two match when each equals its canonical text after lowercasing, as the viewers lowercase it: U+017F is not `s`, and U+212A lowercases to `k`.
- A heading at the detail level of a list or table that is not `<table> row <key>` for its own table — one naming another table once lowercased, such as `## people row P1` in `items.flowmark.md`, or any other text — is `format.block`. Grounding: a heading at the detail level ends the row scope before it and opens no scope of this table, so its content belongs to no row the file holds, and `flowmark canon` cannot know where it belongs. The key is case-sensitive: `## items row t1` is `format.block`.

## Sections and their values

- The body and every `markdown` column value of a row scope at level L end at the next top-level heading
  of level ≤ L. The block structure is the one CommonMark 0.31.2 parses: a top-level heading is an ATX
  heading at top level, or a setext heading closing a top-level paragraph, and fences, HTML blocks of every type (1–7),
  containers (block quotes and list items), lazy continuation lines and the rules on what may interrupt a
  paragraph count exactly as CommonMark states them. A value holds only top-level headings deeper than L;
  a write of a value with a top-level heading of level ≤ L is `CONSTRAINT`. Grounding: a file is read the
  way a viewer reads it, so a value or a section can only be what CommonMark parses; a heading inside a
  container is content of that container, not a section of the row; and a narrower list of constructs
  either misses one (HTML types 6 and 7, lazy lines) or refuses a harmless one (a block open inside a
  closed container).
- Every `markdown` column other than the body is a section of the row scope at level L named by the
  column: a non-null column's section is always present, and a nullable column's section is absent
  exactly when its value is NULL. A column's section stands at most once in a row scope: a second heading at level L named by the same column, whatever the spelling of its heading and even when its value equals the first, is `format.section`. Grounding: a column holds one value, and two sections give two values that no rule merges or chooses between — both sides of a git merge kept, or a stale copy — so their meaning is not certain; `flowmark canon` could repair it only by dropping the bytes of one value, which the renderer never does to payload ([The skeleton and the preservation boundary](#the-skeleton-and-the-preservation-boundary)), and comparing the two values would make the file's readability depend on its payload; a write never produces two. An absent non-null section is `format.block` on read and `CONSTRAINT`
  on write. A present empty section is the empty text. The body is never NULL and is not a section.
  Each child block is a heading at level L named by the child table. A child block stands at most once in a row scope: a second heading at level L named by the same child table is `format.section`. Grounding: a row holds one set of rows per child table, and two lists or tables give two sets that no rule merges or chooses between — both sides of a git merge kept, or a stale copy — so their meaning is not certain; a write never produces two. An undeclared heading at level L is
  `format.section`. Grounding: a non-null value is always written, so its section cannot be absent; a
  NULL value has no text, so its section is absent.
- A value is framed in the file: a blank line follows its heading only when the value is non-empty, the
  value's lines follow, and a blank line precedes the next element when one follows. The value neither
  begins nor ends with a line feed and holds no blank line at either end (a line of only spaces and tabs, which is CommonMark's blank line); a write of a
  value that does is `CONSTRAINT`. Grounding: the framing blank lines separate the elements, so a value
  that began or ended with a line feed would read back with its framing changed, and a blank line
  at either end would be read as a framing line.
- The framing blank lines are skeleton: on read, a further blank line (a line of only spaces and tabs) at either end of a value is not part of the value and is `format.noncanonical`, and so is a missing framing blank line where the element still reads back, except a body line directly after the last field-list item, which is `format.block`; `flowmark canon` repairs both.
- A value holds no top-level setext heading: a line of only `=` or only `-`, of at most three spaces of
  indent and any trailing spaces or tabs, directly after a line of a top-level paragraph of the value, is
  `CONSTRAINT` on write, a link reference definition at the start of a paragraph being no line of it
  ([The collision rule](#detail-headings-and-anchors)); on read, a setext heading is a heading where CommonMark reads one. Grounding: a
  top-level setext underline turns the value's last line into a top-level heading, and a setext heading is of
  level 1 or 2 while every row scope's level L is at least 2 (2 for a `files` row, h + 1 ≥ 3 inside a detail
  section at level h), so no setext heading lies deeper than L: every top-level setext heading in a value is a
  section boundary, and the rule refuses every one of them.
- A value that leaves a fence or an HTML block open is refused only when the element that follows would be
  taken into it under the CommonMark block structure: `CONSTRAINT` on write, whichever write places that element (the check
  runs on the file the write produces). On read, the lines that such a block takes in are the value's text: under
  the CommonMark block structure they are no boundary, and a viewer shows them inside the block, so no read fault
  is reported for them. A block left open inside a container that a following top-level element closes is no fault.
  A top-level fence that the value leaves open takes in every line up to its closing fence or to the end of the
  file, so on read the value runs on past those lines to the next top-level heading of level ≤ L or to the end of
  the file, and a heading inside the fence is no boundary, as CommonMark reads it. Grounding:
  an open fence or HTML block that takes in the element that follows makes that element the value's text, so a
  write that places it would not store the element it places.
- A child block section is written only when the block holds at least one row — an empty block and an
  absent block both mean zero rows, so only one form can be canonical (P1); a parent row with no child
  rows has no block heading, and an empty block heading is `format.noncanonical`.
- A `markdown` column is always a section of the row scope, never a field or a cell, in every storage.

## Stored scalar columns

The stored scalar columns of a table are its columns minus the key, the `title` of a `list` or `files` storage, the `body`, the
`markdown` columns, the state column of a `list`, the placement foreign key, the `parent` of a
`list`, and the `order` column of a `block` or `document` storage, which is a logical column and is never
written ([document model](20-document-model.md#order)). The key, the title, the state
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
- `now` is dbspec's keyword for the current time in a `datetime` default and is never a stored value:
  the engine writes the time it denotes on insert. A temporal value written `now` is not of its type
  (`type` on read, `CONSTRAINT` on write); a text value `now` is text. Grounding: `now` names the time of
  the write, so a stored `now` would name another time on every read. Text is checked against its
  column's length, and in a single-line place against the rules of
  [Single-line places](#single-line-places).
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
  token without any invisible character, `"` or `\`, or `"…"` with `\"` and `\\` as its escapes. A value holding an invisible character is written quoted, since an invisible character inside a bare token reads as a separator or not at all. On read, a token `<declared column>:<value>` at the place of a field — a field, or the last token of the title — whose bare value holds an invisible character is `format.block`, since the write rule never produces it there; elsewhere in the title the token is an unescaped title token (see below). A field carries no
  sigil. Only stored scalar columns are fields.
- An absent field is NULL; `col:""` is the empty text. Every non-null column is written; a row
  missing a non-null field is `format.block`.
- A title token is a run of characters between spaces and tabs. A title token matches `<declared column>:` when it begins with the name of a declared column followed by `:`, so `owner:x` and `owner:` match and `xowner:y` does not; such a token is written `<column>\:`, and a title token that begins with `<declared column>\:` reads as `<column>:`; no other `\:` is decoded. The escape changes only the token's colon; the white space between title tokens is kept byte for byte. On read, an unescaped matching title token that another title token follows is `format.noncanonical`. The last token of the title — directly before the first field, or the last token of a row without fields — stands where a field stands, so an unescaped matching token there that is not a well-formed field (`owner:`, `owner:a"b`) is `format.block`: it may be a field miswritten (the empty text is `owner:""`) or title text. A title token beginning with `<declared column>\:` is `CONSTRAINT` on write, since it would read back as `<column>:`. The title is the text between the key and the first field. Grounding: fields are parsed from the right, so a title whose last token matched would read back as a field, and escaping every matching token wherever it stands gives a title one written form. A matching token that remains in the title after the fields are parsed, with another title token after it, has a certain place, and a viewer renders `\:` as `:`, so its unescaped spelling only differs from the canonical one. Title tokens are split on the same space and tab that separate the fields.
- A child row is indented two spaces deeper than its parent, which is the CommonMark list-item content
  indent of a parent written with one space after its marker. The indentation is the `parent` column;
  nothing else writes it.
- The run of spaces and tabs after the list marker, measured in columns, a tab advancing to the next
  multiple of four, sets where the item's content begins in CommonMark 0.31.2: after one to four
  columns the content begins after the run, and after five or more it begins after the first space, the
  rest opening an indented code block (§5.2, rules 1 and 2). One space is canonical. On read, a run of
  two to four columns is `format.noncanonical`, since the item holds the same paragraph, so a viewer
  shows the same task item and `flowmark canon` writes one space; a row indented two spaces under such
  a row is `format.block`, since CommonMark reads it as the next item of the outer list, a sibling,
  while the grammar reads it as the child; a row whose marker stands where such a row's content
  begins, or up to three columns past it, at an indentation to which the two-space grammar gives no
  place (an odd indent, or a level skipped), is that row's child, as CommonMark reads it, and is
  `format.noncanonical`; `flowmark canon` writes it two spaces deeper than its parent; a run of five or
  more columns is `format.block`, since CommonMark reads the glyph, key, title and fields as code, so a
  viewer shows no task item and no inline markdown. Grounding: a separator that only moves where the
  item's paragraph begins changes nothing a viewer shows, whereas one that changes the nesting or the
  block shown leaves two readings of the row. Nesting is read as CommonMark reads it, and an
  indentation is canonical only two spaces under the parent: where the two-space grammar and
  CommonMark both place a row and disagree, the file has two readings and is `format.block`, so a
  separator never re-parents a row silently; where only CommonMark places it, the meaning is certain
  and only its indentation is non-canonical. CommonMark 0.31.2 is the only measure of nesting, as of all
  structure ([The readable document](#the-readable-document)): an implementation that nests otherwise
  does not decide it.
- The placement foreign key and `parent` are never written as fields of a block list row; writing one is
  `format.block`.

## Table rows

- The header row names the stored scalar columns and is written `| <key column> | <column> | … |`. Every stored scalar column has a header; a missing header, or a header that names no column, is `format.block`. The key is the first cell, under a header named by the key column; the stored scalar columns follow in column order. A line that GFM reads as the header row of these columns in another shape is `format.noncanonical`: indented by at most three spaces with no tab before its first character, the `|` at either end optional (a one-column header may hold none), spaces and tabs around a cell, and any of space, tab, vertical tab (U+000B) and form feed (U+000C) at the end of the line. A line indented by four columns or more, a tab in its indent advancing to the next multiple of four, is `format.block`. Grounding: GFM reads the header as a row whose pipes at either end are optional, ending in the white space its delimiter row allows (GFM 0.29, Tables), on a paragraph line, from which CommonMark 0.31.2 removes up to three spaces of indent (§4.8), while four columns open an indented code block (§4.4), a tab advancing to the next tab stop (§2.2); every such header gives the same table, so `flowmark canon` rewrites it. GFM 0.29 is the measure of a table, as for the delimiter row, since structure is judged by the specification ([The readable document](#the-readable-document)): VS Code's markdown-it reads no table from a header without `|`, and that deviation does not decide it.
- The delimiter row is `---` for each column, written `| --- | --- |`. A line that GFM reads as the header's delimiter row is `format.noncanonical`: indented by at most three spaces with no tab before its first character, its cells separated by `|`, each cell one or more hyphens with at most one colon at either end and around them any of space, tab, vertical tab (U+000B) and form feed (U+000C), the `|` at either end optional and any of those four characters after it, with as many cells as the header — except a line without a leading `|` that CommonMark reads first as another block: a line of only hyphens and trailing spaces or tabs (a setext underline, which makes the header a heading), or a line whose first character after its indent is a `-` followed by a space or a tab (a list item). An alignment colon is therefore noncanonical. Any other line in its place is `format.block`. Grounding: the delimiter row is skeleton, and no column states an alignment, so a write cannot keep one and one form is canonical; GFM's delimiter row allows these four white space characters around its cells, so every such row reads as the same table and `flowmark canon` rewrites it; the white space of the delimiter row is skeleton, not a single-line place; a tab in the indent reaches the fourth column, so CommonMark reads the line as indented, which continues the header's paragraph and opens no table (§2.2, §4.4). A line that GFM does not read as a delimiter row leaves no table, and a viewer shows the header and the rows as paragraph text.
- A declared title is a cell under its column's header, in column order.
- In a block table the placement foreign key is never a cell; `parent` is a cell. A `parent` cell that
  names a key two rows of its block share is `format.block`: the parent cannot be assigned. A key that two rows of different blocks share is assigned by the block.
- A cell holds text as it is. An empty cell is NULL, and the empty text is written `""`. Every
  non-null stored scalar column has a value.
- A row is written with a `|` at either end. On read, a row without its last `|` is read as GFM reads
  it — its last cell runs to the end of the line, and a final `\|` is a `|` of that cell — and is
  `format.noncanonical`. Grounding: the pipes on either end of a GFM row are optional, so the row holds
  the same cells, and `flowmark canon` writes the `|`.
- A cell value that begins with `"` is written `"…"` with `\"` and `\\` as its escapes. In a table cell
  each `|` of the value is written `\|`, and no other backslash is added. On read, a `|` directly after a
  `\` is never a cell boundary, whatever precedes the `\`, and that one `\` is removed. The quoted form is
  applied first and the pipe escape last, so a read removes the pipe escape first. Grounding: GFM reads
  any `|` after a `\` as cell text and removes that `\` before inline parsing.

## Single-line places

A title (in a list row, in an H1 or in a cell), a list field, a field-list value and a table cell are
single-line places. A single-line value is inline markdown as written and is never escaped, except as
this section states. In this section, white space is a character with the Unicode White_Space property,
and an invisible character is a white space character other than U+1680, a control character (general category Cc), a format
character (general category Cf, such as U+FEFF) other than a Prepended_Concatenation_Mark (U+0600–U+0605, U+06DD, U+070F, U+0890–U+0891, U+08E2, U+110BD and U+110CD in Unicode 17.0.0), or a Default_Ignorable_Code_Point (such as U+3164, U+115F,
U+034F and the variation selectors); the Unicode properties are those of Unicode 17.0.0, the version of
the `unicode` package of Go that the engine is built with. The edge rules of this section refuse an invisible character at
either end of a value; a character inside a value is not an edge. Inside a separator the canonical
white space is the space (U+0020). Grounding: such a character renders with no glyph, so a reader cannot see it; U+1680 and the prepended concatenation marks render with a glyph and are text. Refusing every invisible character at an end keeps the value's text and what a reader sees the same. CommonMark 0.31.2 strips the space and the tab from the ends of a paragraph
and of an ATX heading's content, so these two are part of the set.

- On write, a value there that contains a line feed or a CR, that begins or ends with an invisible character,
  or that contains `<` followed by `!`, `/` or a letter is `CONSTRAINT`: a line feed cannot be written
  on one line, and raw HTML would hide or alter the value.
- On read, a single-line value that renders differently from its text beyond its inline markdown is `format.block`; this is exactly a quoted list field or quoted cell (`"…"`) with an invisible character at either end inside its quotes, or a value with `<` followed by `!`, `/` or a letter (raw HTML
  would hide or alter it). On read, a space or a tab at an end of a single-line place where CommonMark strips it at the ends of a line's content (or GFM at the ends of a cell) is `format.noncanonical`; a run of spaces and tabs before the key (other than the run after the list marker), the title or a field of a list row is a separator, and any such run other than one space, a tab that ends a list title before a field included, is `format.noncanonical`, since the field parser splits on the space and the tab alike and the write rule refuses a title that ends with either, so the run is never title text; a run of two to four spaces after the list marker is `format.noncanonical` and one of five or more is `format.block` ([List rows](#list-rows)); the run after the list marker is measured in columns, a tab advancing to the next multiple of four (CommonMark 0.31.2 §2.2), and is ruled as a run of spaces of that width ([List rows](#list-rows)), a tab in it being `format.noncanonical`; the white space between the tokens of a title is title text, not a separator; any other invisible character at an end is `format.block`, since the write rule cannot produce it again. Each place has one canonical separator (`# ` after the H1 marker, `: ` after a field-list column or nothing for the empty text, one space after the list marker and between the glyph, the key, the title and each field, `| v |` in a cell); any other space or tab there is `format.noncanonical`.
- A title in an H1 whose final run of `#` follows a space or a tab, or that consists only of `#`, is
  written with its last `#` as `\#`, since CommonMark would read the run as a closing sequence (a line
  of `#` alone is an empty heading). On read, the title is the raw heading content with CommonMark's leading and trailing spaces, tabs and closing sequence removed, and a final `\#` read as `#`; no other escape is decoded. A final `\#` that the write rule would not produce (as in `# C\#`, whose title `C#` is written
  `# C#`) is `format.noncanonical`. A title whose decoded text the write refuses — the empty text (as in `#` or `# #`), or a text ending in
  `\#` (as `# a\\#`, decoded `a\#`), which would read back as `#`, since `\#` and `#`, and `#\#` and `##`,
  would be written alike — is `format.block` on read and `CONSTRAINT` on write. On read, an H1 closing sequence is
  `format.noncanonical`. Grounding: a title is written so that it reads back as the same text; a title
  the write refuses has no H1 that the writer produces, so its file cannot round-trip (P1, P2).

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
- An item that is not `- <stored scalar column>: <value>` in column order is `format.block`; a space or a tab other than the canonical separator is `format.noncanonical`, as [Single-line places](#single-line-places) states. An absent
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
- A target that differs from the derived one is `format.noncanonical`. A key that names no row is a dangling reference: the validator reports it as `reference.dangling`, a write that introduces it is `FOREIGN_KEY`, and it never makes the file unreadable. A dangling reference
  keeps its written target: it has no derived target, so it is read without a target check and written as
  read; the one exception is a `parent` of a block or document row that names no row of its block, which is
  `format.block`. Grounding: a missing row has no target to derive, and a target a write did not change
  must keep its bytes; a hierarchy within one block must be complete to have a storage order. A key update of
  a referenced row is `FOREIGN_KEY` (restrict-only), so a target never moves under a link.
- A link's derived target is that of the one row its key names. A `parent` of a block or document row names a row of its own block ([Document order](#document-order)), so its target is derived from that row, whatever rows of other blocks share its key (two rows of its own block sharing the key make the cell `format.block`, [Table rows](#table-rows)). Any other foreign key whose key two or more rows of the referenced table share byte for byte names no one row: it has no derived target, and, like a dangling reference, it is read without a target check and written as read. Grounding: a target is a function of the referenced row, and a key that two rows share gives two targets that no rule chooses between; choosing by the order of the rows or files would make a file's diagnostics depend on that order. The table is unavailable through `key.duplicate` ([Unavailable table](#diagnostics)), so no write rewrites such a link and `flowmark canon` refuses to derive it ([operations](40-ops.md#the-canon-operation)); once the keys are repaired, the target is derived and checked. Keys equal only without regard to case name one row each, byte for byte, so a link to either has its derived target.
- A `parent` written in a `files` row is a link too. The placement foreign key of a block row is
  never written.
- A link from a block row to the `files` row whose file holds it is that file's name, as for any row without an anchor in its own file, and the codec derives it from the file's own key, without any other file or table. Grounding: a `files` row has no detail heading — its H1 is its title, which a title update changes — so its file is the only target derived from the key alone; the file name is the relative path the general rule gives, and a viewer opens the file at the row.
- Examples: a `files` row `comments/C5.flowmark.md` links its post as
  `[P3](../posts/P3.flowmark.md)`; a block comment inside `posts/P3.flowmark.md` links its author as
  `[max](../people.flowmark.md)`; the root `items.flowmark.md` links a row that has a detail section
  in the same file as `[T2](#items-row-t2)`; a block row inside `tasks/T4.flowmark.md` links T4 as `[T4](T4.flowmark.md)`.

## Encoding and line structure

- A document is UTF-8 without a byte order mark, and LF is its canonical line ending; payload bytes are
  otherwise preserved as they are.
- A line ending is an LF, a CRLF, or a CR not followed by an LF, as CommonMark reads it. A byte order
  mark, and a CR in any line ending, is valid input under P2 and is `format.noncanonical`: P1 fails,
  since the canonical form has neither. On read, a value reads every line ending as LF, so a bare CR
  splits its line as it does in CommonMark. Grounding: multi-platform — a Windows checkout with `autocrlf` must read the same
  values as a Unix checkout. Only `flowmark canon` removes these bytes.
- Invalid UTF-8, and a U+0000, is `format.encoding`: no text can be read from them.
- A document ends with exactly one LF. A document without a final LF, or with a trailing blank line, is
  `format.noncanonical`. Grounding: the grammar has no element after the last line, so a further line
  feed is not part of any element and would be lost on emit.
- A logical write that rewrites a file with a `format.*` diagnostic is `CONSTRAINT` apart from the exemptions below. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([Unavailable table](#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([Unavailable table](#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose detail section appears or disappears, an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](40-ops.md#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only, since removing a file rewrites none of its bytes, which is what the refusal protects — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child blocks and its history rows, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable. Without the `.gitattributes` that install writes, a Windows `autocrlf` checkout reads every value but refuses every write that rewrites an existing document, apart from the exemptions above, until `flowmark canon`. Grounding: a write must not change bytes the user did not write; a migration is a separate, explicit transaction.
- `flowmark install` adds the line `*.flowmark.md text eol=lf` to `<corpus>/.gitattributes`, creating the file when it is missing and keeping its other lines, so a git checkout keeps LF on every platform ([schema](30-schema.md#install)).

## Diagnostics

Every read rule above, and the walk, reports `{file, line, column, rule, message}`, where `column` is the 1-based column, in code
points, of the first character of the element the rule names (the glyph, the key, the title, the field
token, the cell content or the heading text); a fault of a whole line is column 1. A `format.encoding`
fault at an invalid UTF-8 byte is at that byte: its column is 1 plus the code points of its line before
it, the byte occupying one position. A fault of a whole file, or of a corpus entry with no text of its
own (a `corpus.walk` entry, a folder or a symlink included), is line 1, column 1. A `catalog` diagnostic
counts code points too. Grounding: a column locates the text a rule names, so a byte that is no character
takes the position a decoder reads it at; orm's dbspec counts columns per code point in its lexer
(`splitSource` and `lexLine`, `dbspec.go` `Diagnostic`), and its whole-document faults (the size limit,
the byte order mark) are at line 1, column 1. The read rules are
`format.noncanonical`, `format.section`, `format.block`, `format.encoding` and `parent.cycle`, and the
walk's rule is `corpus.walk` of the [document model](20-document-model.md).

- **Read and write faults.** On read, these are `format.block`: a missing non-null field or section; a
  single-line value that renders differently from its text beyond its inline markdown (see [Single-line places](#single-line-places));
  a `files` body whose first line is a `-` list item; a `files` body whose first line is indented by two or more columns (spaces, or a tab) after a field list. The same faults are `CONSTRAINT` on write. On read, a space or a tab at an end of a single-line place where CommonMark strips it at the ends of a line's content (or GFM at the ends of a cell) is `format.noncanonical`; a run of spaces and tabs before the key (other than the run after the list marker), the title or a field of a list row is a separator, and any such run other than one space, a tab that ends a list title before a field included, is `format.noncanonical`, since the field parser splits on the space and the tab alike and the write rule refuses a title that ends with either, so the run is never title text; a run of two to four spaces after the list marker is `format.noncanonical` and one of five or more is `format.block` ([List rows](#list-rows)); the run after the list marker is measured in columns, a tab advancing to the next multiple of four (CommonMark 0.31.2 §2.2), and is ruled as a run of spaces of that width ([List rows](#list-rows)), a tab in it being `format.noncanonical`; the white space between the tokens of a title is title text, not a separator; any other invisible character at an end is `format.block`. Grounding: a
  read reports what the document holds, and a write refuses to produce what could not be read back.
- **Write faults.** A refused write writes no bytes, so its faults are located in the request, not in a file: each reports `{table, key, column, rule, message}`, where `table` and `key` name the row the rule refuses and `column` names the column whose value the rule names — the key column for a key fault, the title, a field, cell or `markdown` column, a child table, for its block heading, `parent`, `order` — and is empty for a fault of the whole row. A value given under a name that is no column or child table of its table is refused at that name, a child block's rows at the name of their block. When rows given to one write share a key byte for byte, each fault of such a row also carries its 0-based index among the rows given, and the duplicate fault is at every such row after the first, naming the first. Of two rows whose keys are equal only without regard to case, the duplicate fault is at the row whose key the write introduces — an inserted row or a changed key — naming the other; when the write introduces both, it is at the one whose key is greater in byte order. A row has at most one duplicate fault, at its key column. When a row given before it, or a row of the table that the request does not give, shares its key byte for byte, the fault names the first such row given before it, or else that row of the table; otherwise it names, of the rows, given or of the table, whose keys equal its key only without regard to case and whose pair places the fault at it, the one whose key is least in byte order, the first given when several given rows hold that key. Grounding: a fault adds a location, and a second fault at one `{table, key, index, column}` adds none; the byte-for-byte fault is named first, since it holds whichever rows the write introduces and names a position in the request, while the place of a case-only fault depends on which key the write introduces. The key checks of a write — byte for byte and without regard to case, within the request and against the table's other rows — run before the write renders any file, and a write they refuse renders none; so these locations are those of the request, its indices being the rows' positions in the request. A sibling-order fault is at each row of the group whose value is missing, outside 1..m or shared with a sibling, at column `order`; a value not of the column's type is the type fault alone, counts toward m, and is neither outside 1..m nor shared. A parent cycle is a fault at each row on the cycle, at column `parent`. A row under a parent that names no row, or under a cycle, has only its own faults. A write refused because a file it must rewrite has a read diagnostic names that file and carries that diagnostic's `{file, line, column}`. Grounding: the bytes a refused write would produce never exist, so none of their lines can be opened, and their positions would depend on how refused text and tied rows were placed. The request names rows by table and key and values by column, as a SQL engine names the table, constraint and column of a violation, so this location is a function of the request alone.
- **Validator rules.** A value not of its column type is `type`, a dangling foreign key is `reference.dangling`, and a key equal to another key of its table without regard to case is `key.duplicate`. The validator reports them over the corpus. They never make a file unreadable, except a `parent` of a block or document row that names no row of its block, which is `format.block` ([canonical format](#foreign-keys-as-links)), and a `parent` cell that names a key two rows of its block share, which is `format.block` too ([Table rows](#table-rows)), and a detail section whose key two rows of its block share, or two detail sections of one file under one heading text, which are `format.block` ([Document order](#document-order), [Detail headings and anchors](#detail-headings-and-anchors)). Grounding: a file's readability must not depend on other files, or deleting a parent file would make its children unreadable and unrepairable. A value not of its column type is `CONSTRAINT` on write; a write that introduces a dangling foreign key or a duplicate key is `FOREIGN_KEY` or `DUPLICATE_KEY`. The commit refusal is stated under [Encoding and line structure](#encoding-and-line-structure). Opening a corpus, introspection, `flowmark validate` and `flowmark canon` are not requests over rows: they never fail on an unavailable table, apart from canon's link-derivation refusal ([operations](40-ops.md#the-canon-operation)). The validator reports every file's `format.*` and `parent.cycle` diagnostics, and the validator rules of every row it can read in a file without `format.block`, `format.section`, `format.encoding` or `parent.cycle`; a rule that needs the complete rows of a table that holds such a file — `reference.dangling` into it, `key.duplicate`, `order` and `state.limit` within it — is not reported for that table, and the file's own diagnostic stands for it; the validator's `parent.cycle` across several `files` files is reported among the rows it can read, since a cycle is made by its own rows' `parent` values and no unread row can undo it; a table unavailable only through byte-for-byte equal keys has every rule reported. A foreign key whose key is not a value of its column's type is `type` alone, never `reference.dangling`. Grounding: one fault, one report — such a key is no value the column can hold, so whether it names a row is not asked. A write that assigns such a key is `CONSTRAINT`, never `FOREIGN_KEY`.
- **Corpus order** is the order of the rows' locations: the corpus-relative path of the file, with `/`, in byte order, then the line, then the column. It is total for every readable corpus, whereas a storage order is undefined across a `parent` cycle of `files` rows or a sibling group with shared `order` values.
- **Unavailable table.** A table is unavailable when it holds a file with `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows, or when two of its rows have byte-for-byte equal keys; the request then names the second row's file and `key.duplicate`. A table holds every corpus document at a place its storage names: a `files` table every `<table>/<name>.flowmark.md`, a `document` table `<table>.flowmark.md`, and a block table every file its placement parent's table holds, recursively, so a history table stored as a block is unavailable with its state table. A table is also unavailable while a `corpus.walk` entry stands at such a place, or at a name equal to one without regard to case — an entry that cannot be read, a symlink or a non-folder entry at a `files` table's place, or a suffix, folder or document name in another case. Grounding: a missing folder is an empty table, but an entry in its place is neither a folder nor absent, and the engine neither renames nor removes an entry it does not own, so the table's rows cannot be read or written. A file whose name does not end with `.flowmark.md`, compared without regard to case, is outside the database and never makes a table unavailable. Every request that needs any of its rows — a read of it, a point lookup by key included, an insert, update or delete in it, a join, relation or subquery over it, the rewrite of the links to a row whose detail section appears or disappears when a link may lie in it, and every constraint check over it (a primary or unique key, a reference, a link target, a `limit` count, an `order` sibling set, a `check`) — fails as `CONSTRAINT`, naming the file and the diagnostic. A request never answers from the other files' rows alone. A `files` file whose name is not the canonical text of a key is `format.block`, so its table is unavailable too. Grounding: an answer or a check over rows the engine cannot read would be wrong without saying so, and a request cannot name one of two rows with byte-for-byte equal keys. The engine writes nothing in an unavailable table apart from `flowmark canon` on its files ([operations](40-ops.md#the-canon-operation)): a human repairs, renames or removes the row or file that makes it unavailable by editing the corpus, and the table is available again on the next read. Grounding: a request cannot name one of two byte-equal rows, and a row the engine cannot read cannot be deleted through it.
