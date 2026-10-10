<!-- doc-id: spec-ops -->
# Operations

[한국어](40-ops.ko.md)

This specification pins the operational semantics: opening a corpus, the server and its
sessions, the lock, the transaction and commit protocol, the clock and the failure vocabulary.
The order of the commit protocol is normative.

## Opening a corpus

- Opening runs **recovery first**: a stage with a prepare record is completed, a stage without one
  is discarded, and a target whose current hash matches neither recorded hash — a human edit
  landed mid-crash — is refused and reported, never silently overwritten.
- Opening loads the catalog, walks the corpus, parses the documents, derives the indexes, and
  reuses the parsed-row cache keyed by file content hash for unchanged files.

## The server

- `flowmark serve <corpus> --port <port>` is the resident face that orm clients reach. `--port` is
  required. It binds loopback TCP only. On its first start it writes a random token to `.flowmark/server.json`; the
  token is kept across restarts and replaced only by `flowmark serve --new-token`.
- The DSN is `markdown://127.0.0.1:<port>/<corpus>?token=<token>`, where `<corpus>` is the corpus
  root's directory name. A corpus or token mismatch is `CONFIG`. Remote access and other
  authentication are outside v1.
- A connection is a session: JSON lines of requests and control messages, each answered in order
  ([capability table](60-dialect.md) holds the request and answer shapes).

## The lock

- **One portable lock.** Every operation — read or write, from any face — takes the same lock
  file with holder identity and stale detection: a lock file with `<pid>@<host>/<session>`, after
  the kit holder-lock precedent; no flock, no OS-specific primitive.
- The lock is held for one operation, or for one server transaction from `begin` to `commit` or
  `rollback`. A closed connection, or a transaction idle longer than the server flag
  `tx_idle_ms`, rolls the transaction back and releases the lock.
- The CLI, MCP and sidecar wait on the same lock and fail with `LOCK_TIMEOUT` naming the holder.

## Transactions

- Requests inside a transaction read the staged post-image; a savepoint is a nested stage, and
  `rollback_to` discards the stages above it.
- A face retries a single-operation write once after a re-read when the commit meets
  `CORPUS_CONFLICT`. A server transaction is never retried by the server; the orm client receives
  `WRITE_CONFLICT` and never retries it.

## The commit protocol

1. Check that every target is unchanged since it was read; otherwise fail `CORPUS_CONFLICT`
   naming the file and line.
2. Run the constraint checks against the corpus re-read under the lock with the post-image
   applied — not against a snapshot taken at `begin`, and not against a predicted touched-file
   set. A history gap on another row never fails the commit.
3. Write every post-image file into `.flowmark/tx/<id>/` content-addressed and fsync it.
4. Copy the pre-image of every target into the stage and fsync.
5. Write and fsync the **prepare record**: every rename with its pre- and post-hashes.
6. Perform the renames through the runtime's atomic rename.
7. Write and fsync the commit record.
8. Collect the stage.

Durability across power loss relies on this journal and the recovery on open, not on
OS-specific directory fsync semantics.

## The clock

Every engine-assigned time — `updated`, `soft_delete`, `default now`, a history row's `at` — is
the request's `now` for an orm request, and otherwise the engine's UTC clock truncated to
microseconds.

## Cost model

- A per-command p95 latency target is declared here (hundreds of milliseconds at the corpus
  envelope of [open questions](110-open-questions.md)).
- The parsed-row cache avoids re-parsing unchanged files. The commit checks every constraint whose
  outcome the transaction can change: the keys, references, checks, order, limits, transitions and
  `history.gap` of the target rows, and the rows they reference or that reference them, evaluated
  on the corpus re-read under the lock. Full `flowmark validate` runs on demand and on catalog
  change.
- A long-lived face (the sidecar, the server) may watch the corpus and push change events to its
  host; writes still go through the transaction protocol only.

## Failure vocabulary

| Face name | orm code | Meaning |
| --- | --- | --- |
| `LOCK_TIMEOUT` | `CANCELED` | the lock wait ended; names the holder — `CANCELED`, as a SQLite `busy_timeout` is |
| `CORPUS_CONFLICT` | `WRITE_CONFLICT` | a target changed since it was read; names the file and line |
| `DUPLICATE_KEY` | `DUPLICATE_KEY` | a primary or unique key value already exists; names both places |
| `FOREIGN_KEY` | `FOREIGN_KEY` | a reference names no row, or a referenced row is deleted while referenced |
| `CONSTRAINT` | `CONSTRAINT` | any other rule of the catalog refused the write — a check, a title, a transition, a required column, a limit, an order, a single-line place; names the rule and the place |
| recovery refusal | — | names the target and both recorded hashes; opening refuses until resolved |

Every failure message names the cause and the fix.
