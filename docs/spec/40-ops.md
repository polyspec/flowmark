<!-- doc-id: spec-ops -->
# Operations

[한국어](40-ops.ko.md)

This specification pins the operational semantics: opening a corpus, the transaction protocol,
the lock, the revision token and the cost model. The order of the commit protocol is normative.

## Opening a corpus

- Opening runs **recovery first**: a stage with a prepare record is completed, a stage without
  one is discarded, and a target whose current hash matches neither recorded hash — a human edit
  landed mid-crash — is refused and reported, never silently overwritten.
- The walk builds the corpus view: folders bound by the schema, documents parsed, indexes
  derived, the parsed-row cache keyed by file content hash reused for unchanged files.

## The lock

- **One portable lock, per operation.** Every operation — read or write, CLI, MCP or sidecar —
  takes the same lock file with holder identity and stale detection for its own duration, with a
  timeout that names the holder. There is no long-lived lock, no reader/writer split, and no
  daemon: a face runs inside its host's lifetime.
- A commit conflict — a target changed since it was read — fails with the file and line and a
  defined **re-read-and-retry-once** contract. No silent merge, ever.
- Multi-platform grounding: no flock, no OS-specific durability; the lock is a lock file with
  holder identity (the kit holder-lock precedent), and crash safety comes from the journal
  replay below.

## The commit protocol

1. Write every post-image file into `.flowmark/tx/<id>/` content-addressed and fsync it.
2. Copy the pre-image of every target into the stage and fsync.
3. Write and fsync the **prepare record**: every rename with its pre- and post-hashes.
4. Perform the renames through the runtime's atomic rename.
5. Write and fsync the commit record.
6. Collect the stage.

Validation runs at commit **against the corpus freshly re-read under the lock** — not against a
BEGIN snapshot and not against a predicted touched-file set (referential discovery makes that
set unknowable in advance). Durability across power loss relies on this journal and the recovery
on open, not on OS-specific directory fsync semantics.

## Corpus revision

- A monotonically increasing counter in `.flowmark/`, bumped by every commit. A multi-call read
  session — paginate — carries the token and fails with `CORPUS_CHANGED` on mismatch, or degrades
  to keyset pagination. `orderByRandom` is seeded per revision.

## Cost model

- A per-command p95 latency target is declared here (hundreds of milliseconds at the corpus
  envelope of [open questions](110-open-questions.md)).
- The parsed-row cache avoids re-parsing unchanged files; the default per-transaction validation
  is the FK/UK closure of the post-image; full `flowmark validate` runs on demand and on schema
  change.
- A long-lived face (the sidecar) may watch the corpus for reads and push change events to its
  host; writes still go through the transaction protocol only.

## Failure vocabulary

`LOCK_TIMEOUT` names the holder; `CORPUS_CONFLICT` names the file and line; `CORPUS_CHANGED`
names the revision; recovery refusals name the target and both recorded hashes. Every failure
message names the cause and the fix.
