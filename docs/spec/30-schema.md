<!-- doc-id: spec-schema -->
# Schema

[한국어](30-schema.ko.md)

This specification pins the schema language relationship: dbspec is the root grammar and the
authority, and a document carries only projections and provisional fragments of it.

## Derivation ruling

- **dbspec ⇒ frontmatter.** The `.dbs` dbspec of the corpus is the single authority. Frontmatter
  DDL is not an independent language: it is a dbspec fragment embedded in the frontmatter, and
  derivation always flows from dbspec into documents.
- Grounding: schema is law and rows are data — a per-document copy of the law diverging is itself
  the drift bug class; the orm planner, IR and conformance bind to the dbspec manifest hash; and
  the reverse direction would compose the corpus schema by merging per-document declarations,
  creating DDL reconciliation across branches.

## The embedding

- The fragment is always a **YAML literal block scalar** (`|`), which settles YAML-hostile tokens
  — names like `yes` or `on`, datetime colons, `#` inside the fragment — and keeps one parser
  and one type system.
- dbspec diagnostics are **offset-mapped** into file coordinates, so an embedded-fragment error
  names its place in the document.
- The fragment is canonicalized (LF, final newline) at embed time; the stamp carries the
  **canonical** fragment — there is no second, minified emission form.

## Standalone documents

- A document outside any repository is its own **provisional authority**: its embedded fragment
  compiles to the same IR over a one-document set, and `use` declarations are not valid in a
  standalone fragment.
- On import into a corpus, the repository's `.dbs` becomes the authority and the embedded
  fragment becomes a consistency check target: a mismatch is `schema.drift`.

## The schema stamp

Every document of a corpus carries a stamp rendered from the `.dbs`: the manifest hash, the
`schema_version` and the canonical fragment of its table. The stamp is a **verified cache, not
data** — a mismatch with the repository's manifest is a located diagnostic, never a silent
reparse, and a human may not edit it.

## Markdown-specific schema surface

- **Manifest-level settings only, never rendered types.** Markdown-flavored column traits and
  checklist child-table sugar are settings of the manifest, as `updated` and `soft_delete` are:
  a real database introspects them back as plain `text`, so render→introspect→same-document
  holds. The state machine is likewise a manifest-level setting with its own sub-grammar, which
  the SQL backends ignore as they ignore `navigation` and the markdown validator enforces.
- **`identity` columns are rejected** on markdown corpora: keys are human-assigned through the
  [ID grammar](10-canonical-format.md), an insert that omits the primary key is an error, and
  `flowmark next-id` is the allocation helper. Key non-reuse under git rewind is abandoned by
  design — git history is the allocator of record.
- **Foreign-key actions are restrict-only**; `cascade` and `set_null` are rejected at schema
  parse, because they are unimplementable for a writer that bypasses the executor.
- **`audit` and `immutable` are advisory**: violations are validate-time rejections; there is no
  trigger-time prevention for a human's file edit.
- **Defaults**: a human-authored file missing a non-null defaulted column is rejected by
  validate; `flowmark ingest` stamps defaults explicitly.
