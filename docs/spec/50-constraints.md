<!-- doc-id: spec-constraints -->
# Constraints

[한국어](50-constraints.ko.md)

This specification pins what the validator enforces over a corpus, and the rulings for the
features whose database guarantees assumed a writer monopoly that a human's file edit voids.

## Parse and type

- **Strict parse.** A key, line or block outside the [grammar](10-canonical-format.md) is a
  located diagnostic; there is no lenient mode and no invisible data.
- **Types.** Frontmatter values follow the dbspec type forms; a value that fails its type is a
  diagnostic at the value's place.

## Keys and references

- **Primary key / unique.** Enforced over the whole corpus by the canonical string form; a
  duplicate is reported at the second occurrence with a pointer to the first.
- **Foreign keys.** Resolved after NFC normalization by exact match; a dangling reference is a
  diagnostic at the reference's place. Referential actions are restrict-only — see the rulings
  below.
- **Soft-deleted rows still occupy their unique keys and still satisfy foreign keys** (SQL
  parity), so a soft delete never produces a spurious dangling ruling and a re-insert of the
  same key still fails as a duplicate.

## The state machine

- The state machine is a **dbspec-level manifest setting** — a transition table with terminals
  and required fields per transition, expressed by its own sub-grammar as the `audit` setting
  is. The SQL backends ignore it as they ignore `navigation`; the markdown validator enforces it.
- A transition outside the table is a diagnostic naming the row, the current state and the
  refused target. A terminal state is refused for update — the application encodes what to do
  instead (the checklist application spawns a superseded item rather than reopening a terminal
  one).
- Required fields per transition are validated on the transition itself; the checklist
  application requires a cause on a bypass and an impossible state, with a retry condition on a
  bypass.

## Human-write rulings

The database enforced a writer monopoly; a corpus does not have one. Per feature:

- **Foreign-key actions**: restrict-only, `cascade` and `set_null` rejected at schema parse —
  they are unimplementable for a writer that bypasses the executor (a human deleting a parent
  file leaves orphans; the validator reports them and the corpus refuses until they are
  resolved).
- **`audit` and `immutable` are advisory**: violations are validate-time rejections; there is no
  trigger-time prevention for a human's edit.
- **Defaults**: a human-authored file missing a non-null defaulted column is rejected by
  validate; `flowmark ingest` stamps defaults explicitly.

## Secrets and keyless runs

Validation of a corpus whose schema declares codec columns **requires the key**: a keyless run
fails on those columns rather than approximating. CI over such corpora provides the key.

## Diagnostics

Every rule reports `{file, line, column, rule, message}`; rule names are
`schema.drift`, `type`, `key.duplicate`, `reference.dangling`, `state.transition`,
`state.required` and `format.*` of the canonical format. Validation of the whole corpus is
corpus-global; a clean file in a broken corpus does not make the corpus valid.
