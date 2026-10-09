<!-- doc-id: spec-migration -->
# Migration (scoped skeleton)

[한국어](80-migration.ko.md)

This document states the scope and the settled rulings of schema evolution; the detailed
specification is written when schema evolution beyond the v1 scope starts.

## Scope

- **v1 supports corpus creation, additive column additions, and explicit transactional
  codemods.** dbplan chains (the orm `plans.md` lifecycle) are rejected on markdown corpora:
  the server-session, hold-table and introspection machinery has no markdown equivalent in v1,
  and additive-only evolution covers the design phase.
- A codemod is a transaction: `flowmark renumber` rewrites an item and every structural
  reference in one transaction — the suffix changes with its base — and the transaction is
  rejected when a dangling reference or a key collision would result.

## Settled rulings

- **Additive-first**: a new column is optional until a migration stamps it; a rename is an
  explicit step, never "add new and drop old".
- **Reference rewriting is the stated exception to payload preservation** (canonical format):
  the codemod rewrites reference tokens in prose and never rewrites inside fenced blocks or
  inline code spans.
- **The checklist cutover** — when the checklist application reads its own corpus — is one
  transaction: the tracker leaves `config/checklist.json`, the document converts to corpus
  format, and `flowmark validate` passes in the same commit.

## Open for later

The markdown plan dialect (or its permanent rejection), effect vocabulary for file-level steps,
and collision handling for range renumbers.
