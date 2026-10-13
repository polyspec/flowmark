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
  its type is the validator's `type`, `CONSTRAINT` on a write the engine originates that assigns it, and `CODEC_ENCODE` for an orm parameter outside its type ([canonical format](10-canonical-format.md), [capability table](60-dialect.md#the-wire)).

## Keys and references

- **Primary and unique keys** are enforced over the whole table — for a block table, over every block of every file that holds it; a duplicate is `DUPLICATE_KEY` on a write that introduces it, and the validator's `key.duplicate` is at each occurrence after the first in [corpus order](10-canonical-format.md#diagnostics), at its key, naming the first. A unique key treats
  NULLs as distinct, as in SQL. The validator's `key.duplicate` of a unique key compares its values byte for byte, each in its canonical readable form, and is at each occurrence after the first in corpus order, at the key's first column, naming the first; a row whose value in any of its columns is NULL or not of its column type takes no part. Grounding: SQL parity — orm gives every character column one binary collation that compares code points (`utf8mb4_0900_bin`, `COLLATE "C"`, `BINARY`) and lets NULLs repeat — and a value the engine cannot read has no comparison, its `type` standing for it. The primary key compares without regard to case only because it is a file name and an anchor ([document model](20-document-model.md#keys)); a unique key's values are neither, so that reason does not reach them.
- **Foreign keys** resolve by exact byte match with the referenced key; a reference that names no
  row is a dangling reference: `FOREIGN_KEY` on a write that introduces it, and the validator's
  `reference.dangling` at the reference's place; it never makes a file unreadable, except a `parent` of a block or document row that names no row of its file, which is `format.block` ([canonical format](10-canonical-format.md#foreign-keys-as-links)).
- **Keys are never updated.** An update that assigns the primary key, and a restore that writes a new value of it (any value of a key column outside the key that the restore picked), is `IR_INVALID`
  (orm's rule on every database), so a link target never moves under a link; a key changes by
  inserting the row under the new key, pointing the referencing rows at it and deleting the old row,
  in one transaction.
- **Referential actions are restrict-only.** `cascade` and `set_null` are rejected at catalog load:
  they are unimplementable for a writer that bypasses the engine. Deleting a referenced row —
  including a row that has history rows — is `FOREIGN_KEY`. Restrict holds against the rows before
  the write: a delete is `FOREIGN_KEY` when a row it removes is referenced, also by a row the same
  request removes; the referencing rows are deleted by an earlier request. Grounding: orm renders
  `RESTRICT`, checked immediately; MySQL and SQLite then refuse or accept such a delete by their row
  order and PostgreSQL accepts it, and a set delete has no row order, so the check before the write
  is the one rule independent of order.
- **Soft-deleted rows still occupy their unique keys and still satisfy foreign keys** (SQL
  parity), so a soft delete never produces a spurious dangling ruling and a re-insert of the same
  key still fails as a duplicate.

## Key rules

- **Shape.** A key of a markdown table consists of `A`–`Z`, `a`–`z`, `0`–`9`, `_` and `-` and begins
  with a letter or a digit; it holds no `-_`. A key outside this
  shape is `CONSTRAINT` on write.
- **Reserved names.** A key equal, without regard to case, to `CON`, `PRN`, `AUX`, `NUL`,
  `COM0`–`COM9` or `LPT0`–`LPT9` is `CONSTRAINT` on write ([schema](30-schema.md) refuses the table
  name).
- **Length.** A `files` key longer than 243 bytes is `CONSTRAINT` on write.
- **Case-insensitive uniqueness.** A key equal to another key of its table without regard to case is
  `DUPLICATE_KEY` on a write that introduces it, naming the other row, and the validator's `key.duplicate`; the
  collision is a duplicate of the key's file name and anchor, and it never makes a file unreadable, except that two rows of one file whose keys are byte for byte equal are `format.block` when both have detail sections, since the two sections share one heading text ([canonical format](10-canonical-format.md#detail-headings-and-anchors)), or when a `parent` cell of the file names their key, since the parent cannot be assigned ([canonical format](10-canonical-format.md#table-rows)); two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)). The validator's fault at a row whose key equals an earlier row's byte for byte names the first such row and says byte for byte; otherwise it names the first row whose key equals its key without regard to case; a row has one `key.duplicate`, as a write fault has ([canonical format](10-canonical-format.md#diagnostics)).
- **Canonical file name.** A `files` file whose name is not the canonical text of its key's type is
  `format.block` on read ([document model](20-document-model.md)).

## Readable-document refusals

On write, each of the following is `CONSTRAINT`; the rule that states it is named in brackets:

- a single-line value with a line feed or a CR, beginning or ending with an invisible character (as spec 10 defines it: white space other than U+1680, Cc, Cf other than a Prepended_Concatenation_Mark, or Default_Ignorable_Code_Point), or with `<` followed
  by `!`, `/` or a letter, in a list title, an H1, a cell, a list field or a field-list value
  ([canonical format](10-canonical-format.md));
- a `files` body whose first line is a `-` list item or, after a field list, whose first line is indented by two or more columns (spaces, or a tab) ([canonical format](10-canonical-format.md));
- an H1 title (of a `files` row) whose text ends in `\#` ([canonical format](10-canonical-format.md));
- a text or `markdown` value that cannot be written byte for byte, and a typed value that `CanonicalLiteral` refuses (a decimal with more fraction digits than its scale, zeros included, or more integer digits than precision − scale; a time or datetime with more fraction digits than its precision); for an orm parameter, a typed value outside its type, U+0000 included, is `CODEC_ENCODE` instead ([canonical format](10-canonical-format.md));
- a logical write that rewrites a file with a `format.*` diagnostic, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`, apart from the exemptions of the commit ([canonical format](10-canonical-format.md)); a write never normalizes document bytes;
- a `markdown` value holding a top-level heading of level ≤ L, by the CommonMark block structure
  ([canonical format](10-canonical-format.md));
- a write that introduces a collision with a detail anchor (of a heading or an HTML `id` or `name`), located at the row and column whose written text holds the heading or id that takes the anchor — a `markdown` column's value, a `files` row's title for its H1, or, for a section or block heading, the row whose scope it opens, at the `markdown` column or child table it names — naming the detail heading whose anchor it takes: a
  title update, a payload write, a row insert, a move or the creation of a detail section, other than a duplicate key, which is `DUPLICATE_KEY`
  ([canonical format](10-canonical-format.md#detail-headings-and-anchors));
- a date, time or datetime value written `now`, and a text longer than its column (for an orm
  parameter, `CODEC_ENCODE`) ([canonical format](10-canonical-format.md));
- a list title token `<declared column>\:` ([canonical format](10-canonical-format.md#list-rows));
- a `markdown` value holding a top-level setext heading, or leaving a fence or an HTML block open that the following
  element would be taken into, whichever write places that element ([canonical format](10-canonical-format.md#sections-and-their-values)).

On read, the walk reports `corpus.walk` and the format rules report `format.*`, as stated in the
[document model](20-document-model.md) and the [canonical format](10-canonical-format.md).

## Checks

A check is enforced at commit; a check whose predicate is unknown (NULL) passes, as in SQL. The validator evaluates each check of a table over each row it can read, with the row's logical values — every column as a read returns it, the placement foreign key as the key of the parent row — and reports `check` when the predicate is false, at the row's first line, naming the check and the row. Text and `uuid` compare by code point (orm's one binary collation), numbers by value, and dates, times and datetimes chronologically. A value not of its column type is not NULL for `is [not] null`, and a comparison or an `in` that reads it, with no NULL operand, is evaluated twice, once true and once false: a dbspec predicate has no `not`, so it is monotone in these atoms, and the first evaluation is the greatest result any value could give and the second the least. The validator reports `check` when the first is false — the check is false whatever that value is — and the value's `type` stands for the rest; a commit is `CONSTRAINT` naming the check when the first is false, `CONSTRAINT` naming the `type` diagnostic when only the second is, since the check then needs that value, and passes otherwise. Grounding: a check is a predicate of its own row, so it is reported wherever the row is readable, in an unavailable table too; SQL names a violated check by its name, not by a column.

## Codec values

A column with a `codec` setting holds the encoded value in its type's readable form
([canonical format](10-canonical-format.md#readable-values)). The validator decodes each non-NULL
value of its type with orm's Go codec package — `orm.Decode`, which holds the stages `gz`, `base64`,
`serialize`, `yaml` and `ordered_json` — a `bytes` value from its base64 text, and reports `codec` at
the value's element when the stages refuse it, carrying orm's message; a write that assigns such a
value — an insert assigns every value — is `CONSTRAINT` at its column, its message naming `codec`, and
the commit judges it with the validator's decode. A value the write leaves is not judged, as a `type`
fault it leaves fails it only when a check reads it. A value not of its column type is `type` alone. A column whose stages include
the host stage `hex` or `ip` is not judged until orm exports its host decode (orm `T74`):
`orm.Decode` refuses every list that holds a host stage, so judging it would report every value;
the whole list is then decoded in reverse, the host stages by orm's `HostDecode` and the others by
`orm.Decode`. Grounding: a value its own stages cannot decode has no answer for any client that
reads it, so it is invisible data the validator names; the decode is orm's, since the client that
reads the value decodes with it, and the engine reimplements no stage.

## Views

The view table is the table `views` of the application schema
([application schema](100-checklist.md#views)); each of its rows is a view with its `kind`,
`entity`, `filter`, `sort` and `group_by`. The validator judges every row of it that it can read:

- **Entity.** `entity` names an entity of orm's runtime model of the catalog; when `kind` is
  `kanban`, `list` or `gantt`, that entity's table declares `title` and `body`
  ([document model](20-document-model.md#title-and-body)). When `kind` is not a value of its type,
  `entity` is judged only as naming an entity.
- **Filter.** `filter` holds one JSON object with exactly the members `where`, an IR `Group`, and
  `params`, an array whose items are null, a boolean, a number or a string; no filter is
  `{"where":{"items":[]},"params":[]}`. The validator runs orm's `ir.Validate` on the request
  `{ir_version, manifest_hash of the model, kind all, entity, where, n_params}`, n_params being the
  number of params, and reports the code it returns; a param that no `p` or `ps` index uses is a
  fault.
- **Sort.** `sort` holds a JSON array of IR `Order`, `[]` for no sort, validated with `ir.Validate`
  on `{…, kind all, entity, order, n_params 0}`.
- **Group.** `group_by` is NULL for no grouping, or names a field of the entity that
  `ir.OpAllowed(field, "eq")` admits.
- **Shape.** A repeated object member at any depth, an unknown or missing member, a non-integer
  index and any other shape of `filter` or `sort` is malformed.

Each fault is `view`, at the element of its column, one report per column; when `entity` is faulty,
`filter`, `sort` and `group_by` are not judged. A `type`, `codec` or `check` fault on a view column
stands alone, and `kind` carries no `view` of its own. The rule needs only its own row, so it is
reported in an unavailable table too. A view made invalid by a catalog change is reported and never
refused on a read; a write that leaves a view invalid is `CONSTRAINT`: the commit judges each view row
it writes, whichever columns the write assigns, with the validator's judgement, and each fault is at
its column, its message naming `view`, so a view a catalog change made invalid is written only
valid. The validator does not judge a
param against the type of its use: that type is the slot type that orm's planner computes and has not
published (orm `T89`), the source the executor's parameter check waits on too; a param of another
type is refused by the read that runs the view, and the validator judges each param against every
use once orm publishes the slots. What `ir.Validate` checks is orm's contract; its gaps — a
column-to-column `ref` it does not resolve and a `conn` on a group's first item (orm `T73`) — close
in orm. Grounding: a view is a stored request, so the rules that judge a request judge it, against
the current catalog — no manifest hash is stored, since a view is data and a catalog change must not
orphan it unseen; a param no index uses is invisible data; and grouping gathers rows with equal
values, so a field without equality — a codec field whose codec is not exactly `ip`, `aes` being
refused on a corpus — has no groups.

## Hierarchy and order

- A `parent` cycle is `CONSTRAINT` on write, and `parent.cycle` on read for a cycle among a file's rows,
  which leaves that file unreadable ([document model](20-document-model.md#hierarchy)); a cycle through several `files` files is the validator's `parent.cycle` alone; a `parent` outside
  the row's block is `CONSTRAINT` on write and `format.block` on read.
- An order value outside 1..n + 1 (n as the document model defines it), or an update of the order
  column that matches more than one row, is `CONSTRAINT`; a write that leaves the order values of a block or document sibling group other than exactly 1..m, m being the group's size after the write, is `CONSTRAINT`; in `files` storage the order item holds a rank, and the validator's `order` is at each row whose rank is shared with a sibling, at its `order` item — a gap between ranks is no fault, since it changes no position — and a rank not of the column's type is `type` alone and is shared with none ([document model](20-document-model.md#order)). Grounding: a value that cannot be read has no comparison, so the type fault stands for it, and a fault the other siblings have whatever that value is remains reported.
- A row whose `parent` is not of its column's type belongs to no known sibling group: its own `order` is not judged, and its `type` stands for it; a rank shared within a group is reported whichever group such a row joins.

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
  update that would exceed it is `CONSTRAINT` naming the limit. The count includes soft-deleted
  rows: a soft-deleted row is a stored row in its state, as it still holds its unique keys, so a
  soft delete frees no place and a restore never exceeds the limit. A corpus already over the limit
  after a human edit is the validator's `state.limit`, which blocks no other write: when m readable rows of the table, in every block and file that holds it, are in the state and m > n, each row after the n-th in [corpus order](10-canonical-format.md#diagnostics) is reported, at its state (the glyph of a list row, the state cell, the state item of a `files` field list), naming the state, the limit and m. Grounding: the limit admits n rows in the state, so m − n rows stand beyond it, each one fault, as `key.duplicate` is reported at each occurrence after the first ([Keys](#keys-and-references)); which rows they are is not decided by their content — a human edit leaves no record of which row entered the state last, which is what `history.gap` reports — so they are chosen by an order that is a function of the corpus alone.

## A state changed outside the engine

- **Gap.** A row has a history gap when its state differs from the `to` of its last history row,
  or, without history rows, when its state is not an initial state. Validate reports
  `history.gap` at the row's line. An engine transition of a gapped row is `CONSTRAINT` until the
  gap is closed; updates of its other columns are allowed.
- **Chain.** Validate reports `history.chain` at a history row whose `from` differs from the
  previous row's `to` — on the first row, when it is not an initial state. Grounding: the history
  rows are a chain of the row's states, so a break in it is a state change that no history row
  records.
- **Transition.** Validate reports `state.transition` at a history row whose pair is not a declared
  transition; a row may carry both `history.chain` and `state.transition`, one report for each
  fault. Grounding: the engine executes only declared transitions, so an undeclared pair is a step
  it never took, and whether a row follows the one before it and whether its pair is declared are
  two facts about the row.
- **Required.** Validate reports `state.required` at a history row whose pair is declared and which
  holds NULL or the empty text in a column that the transition's `require` list, or the `require`
  list of its `to` when that is a terminal state, names — at that column's element, or at the row's
  first line when the row has no element of it (an absent section, an empty cell); a row whose pair
  is not declared is `state.transition` alone. Grounding: every transition the engine executes
  copies the required columns into its history row, and refuses NULL and the empty text there
  ([Required columns](#the-state-machine)), so such a row was not written by the engine; which
  columns are required is known only for a declared pair.
- **Type alone.** A state, `from` or `to` not of its column type is `type` alone: no
  `history.gap`, `history.chain`, `state.transition` or `state.required` is judged on it — not
  the gap of the row whose state or last `to` it is, nor the chain of the history row after it.
  Grounding: one fault, one report, as for `order` and foreign keys
  ([Hierarchy and order](#hierarchy-and-order)); a value that cannot be read has no comparison.
- None of these diagnostics blocks another write. A state change outside the engine is therefore
  reported, unless a human forges a consistent history.
- **Commit.** A commit first checks that its targets are unchanged since they were read, else
  `WRITE_CONFLICT`. The constraint checks, `history.gap` included, then run on the corpus re-read
  under the lock. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `order`, `state.transition`, `state.required`, `state.limit`, `history.gap`, `history.chain`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](10-canonical-format.md#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([canonical format](10-canonical-format.md#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose derived target changes (its detail section appears or disappears, or an insert gives a dangling link its row), an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](40-ops.md#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child blocks and its history rows, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable.
- **Repair.** `flowmark history repair <corpus> <table> <key> [--from <initial state>]` inserts one history
  row from the last `to` — for a row without history, the one initial state, or `--from`, required
  when the machine has several — to the current state, with `at` from the engine clock and the
  required columns copied from the row. It refuses when the pair is not a declared transition,
  when a column the transition requires is null or empty on the row, or when the current state's
  limit is exceeded counting this row; the human then restores a legal state or fills the columns
  by editing. The repair is a normal transaction that the human commits.
  The repair also refuses, as `CONSTRAINT` at the state column, a row without a gap — its state is the
  `to` of its last history row, or, without history rows, an initial state, or its state is NULL —
  since inserting a history row for a transition that never happened would forge the trail; as
  `CONFIG`, a `--from` given for a row that has history rows (the repair goes from the last `to`), a
  `--from` that is not an initial state, a missing `--from` where the machine has several initial
  states, and a table without a state machine with a history; as `ENTITY_UNKNOWN` a table the catalog
  does not declare, orm's code of a name the schema does not hold; and as `NO_ROWS` a key that names no
  row byte for byte, orm's code of a one-row read that matched none. It runs outside a transaction as one
  transaction under the lock, or inside an open one as any write; its `at` is the engine clock read
  once for the repair, so the commit's re-run of it writes the same row
  ([operations](40-ops.md#the-commit-protocol)).

## Human-write rulings

A database enforces a writer monopoly; a corpus does not have one. Per feature:

- **Foreign-key actions**: restrict-only, as above; a human deleting a parent file leaves dangling
  references, which validate reports.
- **`audit` and `immutable`** are schema errors on a corpus — markdown has no triggers. The history
  of a state machine is the recorded trail, and its gap, chain, transition and required rules report
  what a human edit bypassed.
- **Defaults** are written explicitly by the engine on insert; a human-authored row missing a
  non-null column is a format diagnostic.

## Diagnostics

Every rule reports `{file, line, column, rule, message}`, with the column as the [canonical format](10-canonical-format.md#diagnostics) states; the rule names are `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `parent.cycle`, `order`, `state.transition`,
`state.required`, `state.limit`, `history.gap`, `history.chain`, `corpus.walk` (the walk of the
[document model](20-document-model.md)) and the `format.*` rules of the canonical format:
`format.noncanonical`, `format.block`, `format.section` and `format.encoding`. Validation is
corpus-global; a clean file in a broken corpus does not make the corpus valid. Opening a corpus, introspection, `flowmark validate` and `flowmark canon` are not requests over rows: they never fail on an unavailable table, apart from canon's link-derivation refusal ([operations](40-ops.md#the-canon-operation)). The validator reports every file's `format.*` and `parent.cycle` diagnostics, and the validator rules of every row it can read in a file without `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows; a rule that needs the complete rows of a table that holds such a file — `reference.dangling` into it, `key.duplicate`, `order` and `state.limit` within it — is not reported for that table, and the file's own diagnostic stands for it; the validator's `parent.cycle` across several `files` files is reported among the rows it can read, since a cycle is made by its own rows' `parent` values and no unread row can undo it; a table unavailable only through byte-for-byte equal keys has every rule reported.
