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
- **Referential actions.** `on delete cascade` and `on delete set_null` are applied by the engine in
  the write that deletes the row: a cascade takes each referencing row with it, level by level, and
  set_null clears the referencing column, all in the one commit; the affected count is the rows the
  request matched, as in SQL. A human deleting a parent file applies no action, and validate reports
  the dangling references.
  A foreign key with `restrict` refuses: Deleting a referenced row is
  `FOREIGN_KEY`. Restrict holds against the rows before
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
fault it leaves fails it only when a check reads it. A value not of its column type is `type` alone. A column with the stage `aes` is decoded by orm's `HostDecode` with the key of the
version its row records ([schema](30-schema.md#keys)); a version the keyring does not declare is `codec`, and in a process
without a keyring the column is not judged. Any other column whose stages include
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
values, so a field without equality — a codec field whose codec is not exactly `ip` — has no groups.

## Hierarchy and order

- A `parent` cycle is `CONSTRAINT` on write, and `parent.cycle` on read for a cycle among a file's rows,
  which leaves that file unreadable ([document model](20-document-model.md#hierarchy)); a cycle through several `files` files is the validator's `parent.cycle` alone; a `parent` outside
  the row's block is `CONSTRAINT` on write and `format.block` on read.
- An order value is any value of the column's type: a value shared with a sibling and a gap between
  values are no fault, and an insert that leaves the column out is `IR_INVALID`, as for any non-null
  column without a default; a value not of the column's type is `type` alone, and its row stands after
  its readable siblings ([document model](20-document-model.md#order)). Grounding: a database holds the
  value the application writes, and the order of a sibling group is a function of the values and keys
  alone.

## State columns

- **No state machine.** A database holds no state machine: the values a column may hold are its type and the
  constraints of the schema, and no database refuses an update for the value it replaces. orm's grammar holds
  no `state_machine` line (orm DBS-63 refuses it as an unknown setting), and the engine applies no machine, as MySQL,
  PostgreSQL and SQLite apply none: no insert is refused for its state, no update for the state it replaces, no history
  row is written, and the validator reports no rule of a machine. A table that keeps the history of a state is an
  ordinary table. A state column holds the values of its type that the schema's
  constraints allow, as on every database: a check `in (…)` refuses another value as any check does
  ([Checks](#checks)), and a foreign key to a table of states refuses a value that names no row as any foreign key does
  ([Keys and references](#keys-and-references)). Grounding: orm's one rule is the same result on every database, so the
  engine holds only what those databases hold; the transitions of a state and their trail are polyspec fsm's, and a state
  that fsm takes is written to the corpus as any value is.
- **The task marker.** A list shows a state of one character, or a boolean, as its task marker: the table's
  `task_marker` names the column, whose value is written between the brackets of the list row
  ([canonical format](10-canonical-format.md#list-rows), [schema](30-schema.md#markdown-settings)); its values are those
  of its type and constraints, as any column's. A list table without a `task_marker` writes the column as a field, and
  in `table` and `files` storage the column is a cell or a field item as any scalar is.

## The commit

- A commit first checks that its targets are unchanged since they were read, else `WRITE_CONFLICT`. The constraint
  checks then run on the corpus re-read under the lock.
- A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](10-canonical-format.md#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([canonical format](10-canonical-format.md#diagnostics)).
- A commit that must rewrite a file — the row's own file, a file holding a link to a row whose derived target changes (its detail section appears or disappears, or an insert gives a dangling link its row) — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](40-ops.md#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child blocks, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable.

## Audit

A table with an `audit` setting is audited as a database's audit triggers audit it (orm dbspec DBS-93). Every insert,
update and soft delete that the engine runs on a row of the table writes one row of the setting's history table, with
the action (`insert`, or `update` for an update and a soft delete), the previous audit record and each recorded column
of the row, and, when the setting names its `tables` table, one row of that table naming the table for the audit
record, unless that row already stands. The written row's audit column holds the audit record of the request, which the
request message carries in its `audit` member; a write of an audited table without one is `CONFIG`. A row of an audited
table is deleted through its soft delete column, and any other delete of one is `CONSTRAINT`. `rows.load` writes rows in
their stored form and records nothing, and the difference of a SQL statement already holds the rows that its
database's triggers wrote ([operations](40-ops.md#sql-statements)), so the engine adds none. Grounding: an audit is rows
that a database writes with each write it runs, so the engine writes them for the writes it runs, as a database does.

## Human-write rulings

A database enforces a writer monopoly; a corpus does not have one. Per feature:

- **Foreign-key actions**: the engine applies them to the writes it runs, as above; a human deleting a
  parent file leaves dangling references, which validate reports.
- **`immutable`**: an update or a delete through the engine of a row of the table is `CONSTRAINT`
  (`table <name> is immutable`), an insert is accepted, and a human edit of the file is what a file
  system lets through.
- **`audit`**: the engine writes the audit of the writes it runs ([Audit](#audit)); a human edit of the file
  writes no audit row and is what a file system lets through.
- **Defaults** are written explicitly by the engine on insert; a human-authored row missing a
  non-null column is a format diagnostic.

## Diagnostics

Every rule reports `{file, line, column, rule, message}`, with the column as the [canonical format](10-canonical-format.md#diagnostics) states; the rule names are `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `parent.cycle`, `corpus.walk` (the walk of the
[document model](20-document-model.md)) and the `format.*` rules of the canonical format:
`format.noncanonical`, `format.block`, `format.section` and `format.encoding`. Validation is
corpus-global; a clean file in a broken corpus does not make the corpus valid. Opening a corpus, introspection, `flowmark validate` and `flowmark canon` are not requests over rows: they never fail on an unavailable table, apart from canon's link-derivation refusal ([operations](40-ops.md#the-canon-operation)). The validator reports every file's `format.*` and `parent.cycle` diagnostics, and the validator rules of every row it can read in a file without `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows; a rule that needs the complete rows of a table that holds such a file — `reference.dangling` into it and `key.duplicate` within it — is not reported for that table, and the file's own diagnostic stands for it; the validator's `parent.cycle` across several `files` files is reported among the rows it can read, since a cycle is made by its own rows' `parent` values and no unread row can undo it; a table unavailable only through byte-for-byte equal keys has every rule reported.
