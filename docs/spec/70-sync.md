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
  catalog's runtime model — it hand-renders no SQL and names no table of its own. An export that needs an unavailable table fails as `CONSTRAINT`, naming the file and the diagnostic, and writes nothing; it never exports the other tables alone ([canonical format](10-canonical-format.md#diagnostics)). An export that reads a value with a `type` diagnostic fails the same way and writes nothing.

- db→md (and every adapter→md) is a materialization: the adapter's rows are written into their
  declared storages through the engine, marked by their sync provenance, and a commit records them.
  The provenance is not part of the document; where it lives is open question 5
  ([open questions](110-open-questions.md)).

## The export

- The export is computed whole, from one corpus as a read request reads it, before anything is
  written: a failed export gives no statement, so it writes nothing.
- Its statements are, first, the schema statements orm's renderer (`dbspec.RenderStatements`)
  writes for the catalog's document set in the target dialect — MySQL, PostgreSQL or SQLite — in
  the renderer's order; then one insert a row, which orm's planner compiles from an `insert`
  request of the catalog's runtime model that assigns every column of the table, by its value or
  by NULL. Each insert carries its bind slots as the planner writes them and its request
  parameters in the runtime value model JSON a read answers with
  ([capability table](60-dialect.md#the-wire)). Grounding: orm owns the SQL text, the quoting and
  the placeholders of each dialect, so the export renders none; a column an insert left out would
  take the database's default instead of the stored value.
- Every row of every table is exported, soft-deleted rows included. Grounding: the database is
  rebuilt whole, and a read of it leaves those rows out as a read of the corpus does.
- The tables fill in the order of their foreign keys: a table after every other table its foreign
  keys reference, ties in the order the schema statements create the tables; the tables of a cycle
  of references across tables follow in that creation order once no other table can come first.
  A table's rows stand in storage order ([document model](20-document-model.md#order)), so a
  parent row precedes its children. Grounding: a database that checks a foreign key at each
  statement refuses a row whose referenced row is not yet inserted. The rows of such a cycle that
  reference each other load only into a database that defers its foreign key checks; the export
  never breaks a cycle by writing a value other than the stored one.
- One corpus gives the same statements on every run.
- A table with an identity column loads its rows with their stored keys, through orm's `load` insert, which
  takes a value for the identity column, and its rows are followed by orm's `restart_identity` statement, which
  moves the identity past the table's largest key, so the keys that other rows reference are kept and the next
  key a database generates follows them. Grounding: a key is a row's identity that other rows reference; an
  export that generated new keys would break those references.
- A table with a codec column fails the export as `CAPABILITY_UNSUPPORTED`, naming the table and the column.
  Grounding: a codec column stores the encoded value, while orm's insert takes the decoded value and encodes
  it, and the engine decodes no stored value yet. It waits on orm (open question 6).
- Binding the parameters to a database driver and running the statements is a database
  executor's work, which orm keeps inside its clients; orm's `DB.Execute` runs one statement with
  its slots and binds a SQLite `datetime(p)` or `time(p)` with exactly p digits, taking client
  values (integers, numbers, bytes decoded), not the wire form, so the export still ends at its
  statements (open question 6).

## Settled rulings

- **Truth ownership is per table.** Each table declares which side holds the authority; the other
  side is a projection.
- **The provenance header is withdrawn.** A corpus document carries no comment
  ([canonical format](10-canonical-format.md)), so no `<!-- owner: … -->` marker or hash of a
  document's own bytes exists. The sync provenance of an adapter-owned document lives outside the
  document; where it lives is open question 5.
- **Readonly is the default**: the validator rejects an edit of an adapter-owned file. `writable`
  accepts an edit after validation and re-ingests it.
- **Drift handling is git-aware**: a working tree that differs from HEAD is restored to HEAD and
  reported; a clean tree whose HEAD differs from the last ingested version is accepted and
  re-baselined. The materialized-from revision, recorded where open question 5 decides, gives
  re-ingest a merge base.
- **Merge conflicts on adapter-owned paths** are resolved by discarding both sides and
  re-materializing; a hand merge there is a diagnostic.
- **Sync runs as one-shot commands**; the server is a face of the engine, not a sync daemon.
- **Projections rebuild from on-file data**: a changelog derives from the history rows, never from
  the ignored `.flowmark/` journal.

## Open for the adapters

Field mappings per adapter, the scheduling of one-shot commands, conflict report formats, the
adapter registration schema, and the location of the sync provenance of an adapter-owned document
(open question 5).
