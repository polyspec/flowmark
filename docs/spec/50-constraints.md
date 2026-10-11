<!-- doc-id: spec-constraints -->
# Constraints

[한국어](50-constraints.ko.md)

This specification pins what the engine enforces on a write and what the validator reports over a
corpus, including the rulings for a writer — a human editing a file — that bypasses the engine.
The failure codes are those of [operations](40-ops.md).

## Parse and type

- **Strict parse.** A line, block, section or key outside the [grammar](10-canonical-format.md) is
  a located diagnostic; there is no lenient mode and no invisible data.
- **Types.** Every value follows its dbspec type in the readable form of the [canonical format](10-canonical-format.md); a value that fails
  its type is the validator's `type`, and `CONSTRAINT` on a write that assigns it ([canonical format](10-canonical-format.md)).

## Keys and references

- **Primary and unique keys** are enforced over the whole table; a duplicate is `DUPLICATE_KEY` on a write that introduces it, and the validator's `key.duplicate` at the second occurrence with a pointer to the first. A unique key treats
  NULLs as distinct, as in SQL.
- **Foreign keys** resolve by exact byte match with the referenced key; a reference that names no
  row is a dangling reference: `FOREIGN_KEY` on a write that introduces it, and the validator's
  `reference.dangling` at the reference's place; it never makes a file unreadable, except a `parent` of a block or document row that names no row of its file, which is `format.block` ([canonical format](10-canonical-format.md#foreign-keys-as-links)).
- **Key update of a referenced row** is `FOREIGN_KEY` (restrict-only), so a link target never moves
  under a link.
- **Referential actions are restrict-only.** `cascade` and `set_null` are rejected at catalog load:
  they are unimplementable for a writer that bypasses the engine. Deleting a referenced row —
  including a row that has history rows — is `FOREIGN_KEY`.
- **Soft-deleted rows still occupy their unique keys and still satisfy foreign keys** (SQL
  parity), so a soft delete never produces a spurious dangling ruling and a re-insert of the same
  key still fails as a duplicate.

## Key rules

- **Shape.** A key of a markdown table consists of `A`–`Z`, `a`–`z`, `0`–`9`, `_` and `-` and begins
  with a letter or a digit; it holds no `--` and no `-_`. A key outside this
  shape is `CONSTRAINT` on write.
- **Reserved names.** A key equal, without regard to case, to `CON`, `PRN`, `AUX`, `NUL`,
  `COM0`–`COM9` or `LPT0`–`LPT9` is `CONSTRAINT` on write ([schema](30-schema.md) refuses the table
  name).
- **Length.** A `files` key longer than 243 bytes is `CONSTRAINT` on write.
- **Case-insensitive uniqueness.** A key equal to another key of its table without regard to case is
  `DUPLICATE_KEY` on a write that introduces it, naming the other row, and the validator's `key.duplicate`; the
  collision is a duplicate of the key's file name and anchor, and it never makes a file unreadable, except that two rows of one file whose keys are byte for byte equal are `format.block` when both have detail sections, since the two sections share one heading text ([canonical format](10-canonical-format.md#detail-headings-and-anchors)), or when a `parent` cell of the file names their key, since the parent cannot be assigned ([canonical format](10-canonical-format.md#table-rows)); two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)).
- **Canonical file name.** A `files` file whose name is not the canonical text of its key's type is
  `format.block` on read ([document model](20-document-model.md)).

## Readable-document refusals

On write, each of the following is `CONSTRAINT`; the rule that states it is named in brackets:

- a single-line value with a line feed or a CR, beginning or ending with an invisible character (as spec 10 defines it: white space other than U+1680, Cc, Cf other than a Prepended_Concatenation_Mark, or Default_Ignorable_Code_Point), or with `<` followed
  by `!`, `/` or a letter, in a list title, an H1, a cell, a list field or a field-list value
  ([canonical format](10-canonical-format.md));
- a `files` body whose first line is a `-` list item or, after a field list, whose first line is indented by two or more columns (spaces, or a tab) ([canonical format](10-canonical-format.md));
- an H1 title (of a `files` row) whose text ends in `\#` ([canonical format](10-canonical-format.md));
- a text or `markdown` value that cannot be written byte for byte, and a typed value that `CanonicalLiteral` refuses (a decimal with more fraction digits than its scale, zeros included, or more integer digits than precision − scale; a time or datetime with more fraction digits than its precision) ([canonical format](10-canonical-format.md));
- a logical write that rewrites a file with a `format.*` diagnostic, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`, apart from the exemptions of the commit ([canonical format](10-canonical-format.md)); a write never normalizes document bytes;
- a `markdown` value holding a top-level heading of level ≤ L, by the CommonMark block structure
  ([canonical format](10-canonical-format.md));
- a write that introduces a collision with a detail anchor (of a heading, an attribute-list id or an HTML `id` or `name`), located at the row and column whose written text holds the heading or id that takes the anchor — a `markdown` column's value, a `files` row's title for its H1, a title, field or cell of a list or table row, or an item of a `files` row's field list, at the column whose text holds the attribute list's `{` or the tag's `<`, or, for a section or block heading, the row whose scope it opens, at the `markdown` column or child table it names — naming the detail heading whose anchor it takes: a
  title update, a payload write, a row insert, a key update, a move or the creation of a detail section, other than a duplicate key, which is `DUPLICATE_KEY`
  ([canonical format](10-canonical-format.md#detail-headings-and-anchors));
- a date, time or datetime value written `now`, and a text longer than its column
  ([canonical format](10-canonical-format.md));
- a list title token `<declared column>\:` ([canonical format](10-canonical-format.md#list-rows));
- a `markdown` value holding a top-level setext heading, or leaving a fence or an HTML block open that the following
  element would be taken into, whichever write places that element ([canonical format](10-canonical-format.md#sections-and-their-values)).

On read, the walk reports `corpus.walk` and the format rules report `format.*`, as stated in the
[document model](20-document-model.md) and the [canonical format](10-canonical-format.md).

## Checks

A check is enforced at commit; a check whose predicate is unknown (NULL) passes, as in SQL.

## Hierarchy and order

- A `parent` cycle is `CONSTRAINT` on write, and `parent.cycle` on read for a cycle among a file's rows,
  which leaves that file unreadable ([document model](20-document-model.md#hierarchy)); a cycle through several `files` files is the validator's `parent.cycle` alone; a `parent` outside
  the row's block is `CONSTRAINT` on write and `format.block` on read.
- An order value outside 1..n + 1 (n as the document model defines it), or an update of the order
  column that matches more than one row, is `CONSTRAINT`; a write that leaves the order values of a block or document sibling group other than exactly 1..m, m being the group's size after the write, is `CONSTRAINT`; in `files` storage, sibling order values other than exactly 1..n
  are a diagnostic ([document model](20-document-model.md)).

## The state machine

- **Insert.** A row's state on insert is an initial state; a state outside the machine's state set
  is `CONSTRAINT`.
- **Transition.** An update of the state column follows a declared transition; any other change is
  `CONSTRAINT` naming the row, the current state and the refused target. A terminal state is never
  left (dbspec's terminal semantics) — the application decides what to do instead (the application schema inserts a new item
  with a `supersedes` link rather than reopening a terminal one).
- **Required columns.** A transition, or an entry into a terminal state, whose line has
  `require (c1, …)` assigns every listed column in the same update; an unassigned column, `null`
  or the empty text is `CONSTRAINT` naming the column. The state table keeps the values.
- **History.** Every transition the engine executes inserts one history row in the same
  transaction, with the row's foreign key, the from and to states, `at`, and a copy of the
  required columns. An insert writes no history.
- **Limit.** `limit <state> <n>` counts the table's rows in the state; a transition, insert or
  update that would exceed it is `CONSTRAINT` naming the limit. A corpus already over the limit
  after a human edit is a diagnostic that blocks no other write.

## A state changed outside the engine

- **Gap.** A row has a history gap when its state differs from the `to` of its last history row,
  or, without history rows, when its state is not an initial state. Validate reports
  `history.gap` at the row's line. An engine transition of a gapped row is `CONSTRAINT` until the
  gap is closed; updates of its other columns are allowed.
- **Chain.** Validate reports `history.chain` when a history row's `from` differs from the previous
  row's `to` — on the first row, when it is not an initial state — or when its pair is not a
  declared transition.
- Neither diagnostic blocks another write. A state change outside the engine is therefore
  reported, unless a human forges a consistent history.
- **Commit.** A commit first checks that its targets are unchanged since they were read, else
  `WRITE_CONFLICT`. The constraint checks, `history.gap` included, then run on the corpus re-read
  under the lock. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](10-canonical-format.md#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([canonical format](10-canonical-format.md#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose detail section appears or disappears, an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](40-ops.md#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child and history blocks, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable.
- **Repair.** `flowmark history repair <table> <key> [--from <initial state>]` inserts one history
  row from the last `to` — for a row without history, the one initial state, or `--from`, required
  when the machine has several — to the current state, with `at` from the engine clock and the
  required columns copied from the row. It refuses when the pair is not a declared transition,
  when a column the transition requires is null or empty on the row, or when the current state's
  limit is exceeded counting this row; the human then restores a legal state or fills the columns
  by editing. The repair is a normal transaction that the human commits.

## Human-write rulings

A database enforces a writer monopoly; a corpus does not have one. Per feature:

- **Foreign-key actions**: restrict-only, as above; a human deleting a parent file leaves dangling
  references, which validate reports.
- **`audit` and `immutable`** are schema errors on a corpus — markdown has no triggers. The history
  of a state machine is the recorded trail, and its gap and chain rules report what a human edit
  bypassed.
- **Defaults** are written explicitly by the engine on insert; a human-authored row missing a
  non-null column is a format diagnostic.

## Diagnostics

Every rule reports `{file, line, column, rule, message}`, with the column as the [canonical format](10-canonical-format.md#diagnostics) states; the rule names are `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `parent.cycle`, `order`, `state.transition`,
`state.required`, `state.limit`, `history.gap`, `history.chain`, `corpus.walk` (the walk of the
[document model](20-document-model.md)) and the `format.*` rules of the canonical format:
`format.noncanonical`, `format.block`, `format.section` and `format.encoding`. Validation is
corpus-global; a clean file in a broken corpus does not make the corpus valid. Opening a corpus, introspection, `flowmark validate` and `flowmark canon` are not requests over rows: they never fail on an unavailable table, apart from canon's link-derivation refusal ([operations](40-ops.md#the-canon-operation)). The validator reports every file's `format.*` and `parent.cycle` diagnostics, and the validator rules of every row it can read in a file without `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows; a rule that needs the complete rows of a table that holds such a file — `reference.dangling` into it, `key.duplicate`, `order` and `state.limit` within it — is not reported for that table, and the file's own diagnostic stands for it; a table unavailable only through byte-for-byte equal keys has every rule reported.
