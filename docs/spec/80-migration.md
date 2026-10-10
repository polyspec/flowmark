<!-- doc-id: spec-migration -->
# Migration (scoped skeleton)

[한국어](80-migration.ko.md)

This document states the scope and the settled rulings of schema evolution; the detailed
specification is written when evolution beyond the v1 scope starts.

## Scope

- **v1 supports corpus creation and additive column additions.** `flowmark install` creates the
  storage skeleton from the catalog ([schema](30-schema.md)). dbplan chains (the orm `plans.md`
  lifecycle) are `CAPABILITY_UNSUPPORTED` over `markdown://`: their server-session, hold-table and
  introspection machinery has no markdown equivalent in v1.
- An added nullable column needs no rewrite of a `files` field list, a list row or a section, where an absent item, field or section is NULL; a `table` storage gains the column's header and an empty cell in each row, written by the same one-shot migration transaction, whose targets are exempt for the `format.block` of the missing header. An added non-null column is added with a default, and a one-shot migration transaction writes the default into every existing row, because every non-null column is written ([canonical format](10-canonical-format.md)); its targets are exempt from the commit refusal for the diagnostics the added column itself causes — the `format.block` of its absent values and the `format.noncanonical` of a link to a row whose detail section the column adds — and any other `format.*` diagnostic in a target refuses it, naming the file. The diagnostics the added column causes do not make its table unavailable to that migration.

## Settled rulings

- **Additive-first**: a rename is an explicit step, never "add new and drop old".
- **A storage change is a transaction**: moving a table from one storage to another rewrites every
  row in one transaction, and the rows read the same before and after; a file with a `format.*` diagnostic refuses it, naming the file.
- **Keys are opaque**, so no key codemod rewrites references in prose; a key change is refused while any row
  references the key; the referencing rows are updated first, in the same transaction, by the
  caller.

## Open for later

The evolution of a declared storage beyond a whole-table move, the effect vocabulary for
file-level steps, and the rejection or design of a markdown plan dialect.
