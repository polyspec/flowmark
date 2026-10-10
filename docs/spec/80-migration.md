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
- An added nullable column needs no rewrite: an absent field, cell, key or section is NULL. An
  added non-null column is added with a default, and a one-shot transaction writes the default
  into every existing row, because every non-null column is written
  ([canonical format](10-canonical-format.md)).

## Settled rulings

- **Additive-first**: a rename is an explicit step, never "add new and drop old".
- **A storage change is a transaction**: moving a table from one storage to another rewrites every
  row in one transaction, and the rows read the same before and after.
- **Keys are opaque**, so no key codemod rewrites references in prose; a key change is an update of
  the key and of every foreign key that references it, refused while a restrict-only reference
  would dangle.

## Open for later

The evolution of a declared storage beyond a whole-table move, the effect vocabulary for
file-level steps, and the rejection or design of a markdown plan dialect.
