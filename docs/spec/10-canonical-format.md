<!-- doc-id: spec-canonical-format -->
# Canonical format

[한국어](10-canonical-format.ko.md)

This specification pins the byte form of one document. It states the grammar subset, the
idempotency propositions, the ordering rules and the encoding. Everything the renderer owns is
stated here; everything else is preserved payload.

## Idempotency

- **P1 (canonical identity).** For a canonical input `s`: `emit(parse(s)) == s`, byte for byte.
- **P2 (emission stability).** For any valid input `d`: `emit(parse(emit(d))) == emit(d)`.
- P1 and P2 replace any single byte-identity claim: a document that a Windows editor saves with
  CRLF is valid input, and P2 still holds; restoring P1 is a migration, not a side effect of a
  logical write.

## The skeleton and the preservation boundary

The renderer owns exactly the **skeleton**: the header comments it stamps (`doc-id`,
`source-sha256`, provenance), the frontmatter block, heading lines, the item and table lines it
re-sorts, and the blank lines between skeleton elements. Every other byte — prose runs, fenced
blocks, inline formatting, HTML comments other than the owned markers — is **preserved payload**:
the renderer moves, wraps and re-sorts skeleton lines and never rewrites payload bytes.

- Non-canonical input (a BOM, CRLF, a skeleton line in a non-canonical shape) is a
  `format.noncanonical` diagnostic. `flowmark canon` is a one-shot migration transaction that
  rewrites the document to canonical form; a logical write never normalizes as a side effect.

## Document grammar

A document is, in order: header comments; an H1 title; the preamble; a YAML frontmatter block;
then H2 sections and structured blocks. The strict parser accepts exactly:

- **Header comments**: `<!-- doc-id: <id> -->` exactly once, first; `<!-- source-sha256: … -->` on
  a twin; provenance comments on adapter-owned documents.
- **Preamble**: the text between the H1 and the first H2 maps to the reserved long-text column
  `preamble`.
- **H2 sections**: one H2 heading per declared long-text column; an absent section is NULL, a
  present but empty section is the empty text. A heading with no declared column is a diagnostic.
- **Structured blocks** are the child-table layouts of the [document model](20-document-model.md).

## Frontmatter profile

- YAML **1.2 core schema**, block style in the value region; anchors, aliases and tags are
  rejected; comments are rejected in v1 (a comment-preserving round-trip emitter is a stated
  non-goal until a requirement names it).
- Key syntax follows the dbspec name rule, NFC-normalized; unknown keys are diagnostics.
- Value syntax per type: decimal, datetime and uuid literals use the dbspec canonical forms;
  `bytes` is base64; a text value that may hold a newline cannot live in a markdown-table cell.

## Task IDs and ordering

- **Grammar**: `T<n>(.<n>)*(-<n>)?` with no leading zeros and no suffix `-0`; equality is string
  equality on the canonical form. The suffix is part of the identity and travels with its base
  under renumber.
- **Ordering is type-directed**: ID-typed keys compare segment-wise numeric — `T8.0.9` sorts
  before `T8.0.10` — the suffix compares numerically after its base path, and children of a
  suffixed item bind the suffix to the last path segment (`T8.0.14-1.3` is a child of
  `T8.0.14-1`). Text compares by codepoint; composite keys compare lexicographically by
  component type. `group_count` groups NULLs together.
- **References are structural**: a reference is a task-ID token in prose, and prose excludes
  fenced blocks and inline code spans — a grammar example is not a reference. Codemods that
  rewrite references are the stated exception to payload preservation.

## Encoding

UTF-8 without a BOM; LF is the canonical line ending; input accepts CRLF as valid (P2) and
reports it as non-canonical (P1); IDs and keys are NFC; payload bytes are otherwise preserved
as they are.

## Diagnostics

Every rule above reports `{file, line, column, rule, message}` with the rule names
`format.noncanonical`, `format.header`, `format.frontmatter`, `format.section`,
`format.block` and `format.encoding`.
