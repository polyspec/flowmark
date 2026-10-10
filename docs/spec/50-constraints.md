<!-- doc-id: spec-constraints -->
# Constraints

[한국어](50-constraints.ko.md)

This specification pins what the engine enforces on a write and what the validator reports over a
corpus, including the rulings for a writer — a human editing a file — that bypasses the engine.
The failure codes are those of [operations](40-ops.md).

## Parse and type

- **Strict parse.** A line, block, section or key outside the [grammar](10-canonical-format.md) is
  a located diagnostic; there is no lenient mode and no invisible data.
- **Types.** Every value follows its dbspec type in the canonical literal form; a value that fails
  its type is a diagnostic at the value's place and `CONSTRAINT` on write.

## Keys and references

- **Primary and unique keys** are enforced over the whole table; a duplicate is `DUPLICATE_KEY` on
  write and a diagnostic at the second occurrence with a pointer to the first. A unique key treats
  NULLs as distinct, as in SQL.
- **Foreign keys** resolve by exact match after NFC normalization; a reference that names no row
  is `FOREIGN_KEY` on write and a diagnostic at the reference's place.
- **Referential actions are restrict-only.** `cascade` and `set_null` are rejected at catalog load:
  they are unimplementable for a writer that bypasses the engine. Deleting a referenced row —
  including a row that has history rows — is `FOREIGN_KEY`.
- **Soft-deleted rows still occupy their unique keys and still satisfy foreign keys** (SQL
  parity), so a soft delete never produces a spurious dangling ruling and a re-insert of the same
  key still fails as a duplicate.

## Checks

A check is enforced at commit; a check whose predicate is unknown (NULL) passes, as in SQL.

## Hierarchy and order

- A `parent` cycle is `CONSTRAINT`; a `parent` outside the row's block is `CONSTRAINT` on write and
  `format.block` on read.
- An order value out of 1..n + 1, or an update of the order column that matches more than one row,
  is `CONSTRAINT`; in `files` storage, sibling order values other than exactly 1..n are a
  diagnostic ([document model](20-document-model.md)).

## The state machine

- **Insert.** A row's state on insert is an initial state; a state outside the machine's state set
  is `CONSTRAINT`.
- **Transition.** An update of the state column follows a declared transition; any other change is
  `CONSTRAINT` naming the row, the current state and the refused target. A terminal state is never
  left — the application decides what to do instead (the application schema inserts a new item
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

Every rule reports `{file, line, column, rule, message}`; the rule names are `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `parent.cycle`, `order`, `state.transition`,
`state.required`, `state.limit`, `history.gap`, `history.chain` and the `format.*` rules of the
canonical format. Validation is corpus-global; a clean file in a broken corpus does not make the
corpus valid.
