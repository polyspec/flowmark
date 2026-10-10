<!-- doc-id: spec-sync -->
# Sync (scoped skeleton)

[한국어](70-sync.ko.md)

This document states the scope and the settled rulings of the sync engine; the detailed
specification is written when an adapter beyond the database export starts.

## Scope

- The sync engine is an **adapter architecture with one contract**. The database is the first
  adapter; GitHub (issues, projects) and Jira follow under the same contract.
- md→db is a derived export: the database is rebuildable from the corpus. The export reads the
  rows through the engine and renders its statements through orm's SQL dialect renderer from the
  catalog's runtime model — it hand-renders no SQL and names no table of its own.
- db→md (and every adapter→md) is a materialization: the adapter's rows are written into their
  declared storages through the engine, marked by a provenance header, and a commit records them.

## Settled rulings

- **Truth ownership is per table.** Each table declares which side holds the authority; the other
  side is a projection.
- **The provenance header** is `<!-- owner: <adapter>:<table>@<sha256-of-own-bytes> -->` on each
  file an adapter owns; the hash covers the file's own bytes, so the check is self-verifying and
  clone-safe. Its scope is accident prevention, not adversary resistance.
- **Readonly is the default**: the validator rejects an edit of an adapter-owned file. `writable`
  accepts an edit after validation and re-ingests it.
- **Drift handling is git-aware**: a working tree that differs from HEAD is restored to HEAD and
  reported; a clean tree whose HEAD differs from the last ingested version is accepted and
  re-baselined. The header records the materialized-from revision, which gives re-ingest a merge
  base.
- **Merge conflicts on adapter-owned paths** are resolved by discarding both sides and
  re-materializing; a hand merge there is a diagnostic.
- **Sync runs as one-shot commands**; the server is a face of the engine, not a sync daemon.
- **Projections rebuild from on-file data**: a changelog derives from the history rows, never from
  the ignored `.flowmark/` journal.

## Open for the adapters

Field mappings per adapter, the scheduling of one-shot commands, conflict report formats, and the
adapter registration schema.
