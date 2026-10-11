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

## The canon operation

- `flowmark canon <corpus> [file…]` rewrites the named files, or every corpus document when none is named,
  to canonical form in one transaction. Its targets are exempt from the commit refusal of a file with a `format.*` diagnostic. A named file with `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows refuses the command as `CONSTRAINT`, naming the file, and nothing is written; with no file named, its targets are the corpus documents without a `parent.cycle` among their rows whose only `format.*` diagnostics are `format.noncanonical`, and it reports every other file and does not rewrite it. A `corpus.walk` file is never a target. Canon changes no logical value, so the key, reference and constraint checks never refuse it; a target whose link derivation needs an unavailable table; a link to a `files` row derives from its key alone and needs no table, and a link to a block or document row needs that row's table; such a target refuses a named-file canon as `CONSTRAINT`, naming both files, and with no file named it is reported and not rewritten. A value with a validator diagnostic keeps its written text. It is the one operation that normalizes document bytes; a logical write never does.
  Grounding: a migration is a separate, explicit transaction, so a logical write never changes bytes
  the user did not write.

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
   set. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](10-canonical-format.md#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([canonical format](10-canonical-format.md#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose detail section appears or disappears, an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child and history blocks, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable.
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
| `DUPLICATE_KEY` | `DUPLICATE_KEY` | a primary or unique key value already exists; names both rows (table and key) |
| `FOREIGN_KEY` | `FOREIGN_KEY` | a reference names no row, or a referenced row is deleted while referenced |
| `CONSTRAINT` | `CONSTRAINT` | any other rule of the catalog refused the write — a check, a title, a transition, a required column, a limit, an order, a single-line place; names the rule, and the row and column ([canonical format](10-canonical-format.md#diagnostics)); or a write that must rewrite a non-canonical or malformed document, or a request, read or write, that needs the rows of an unavailable table ([canonical format](10-canonical-format.md#diagnostics)), naming the file and the diagnostic |
| recovery refusal | — | names the target and both recorded hashes; opening refuses until resolved |

Every failure message names the cause and the fix.
