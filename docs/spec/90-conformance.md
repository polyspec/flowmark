<!-- doc-id: spec-conformance -->
# Conformance

[한국어](90-conformance.ko.md)

This document states the conformance contract of the engine and the format of its vectors.

## The contract

- A vector is **a corpus before — its catalog and documents — plus a request, plus the expected
  corpus after, rows or error**. There is no SQL text and no bind list to compare; the SQL
  conformance harness does not apply, and this contract replaces it.
- The request is an IR request under the [capability table](60-dialect.md), or a face operation
  where the vector tests a face. The vectors of the format below carry IR requests; a face
  operation vector arrives with the faces over the new engine (`T12.10`).
- Vectors run against the flowmark engine: the request runs through the write path of
  [operations](40-ops.md) — the lock, the journal and the commit with its checks — as a request
  outside a transaction, so a vector's answer is the answer a client receives. orm's conformance of
  its `markdown` IR executors runs against a flowmark build pinned in orm, and is orm's own
  contract.
- Every vector states its name, and a run that cannot execute is a failure, in the orm conformance
  tradition.

## Required properties

- **Round trip**: every vector's corpus before is canonical (`emit(parse(s)) == s`) in every
  storage and valid, except the files a vector declares with their diagnostics; the corpus after is
  canonical by construction and holds no diagnostic but the declared ones of the files the request
  left unchanged, and the runner checks both by validating the corpus. The exception
  exists because the write path's own answers — the refusal of a commit that must rewrite a file
  with a `format.*` diagnostic, the delete of a whole `files` file exempt from it, the unavailable
  table — are answers to a corpus that holds such a file ([operations](40-ops.md#the-commit-protocol)).
- **Storage equivalence**: one dataset stored as `files`, as `document` and as `block` yields the
  same rows for the same read request. Vectors that show it name one equivalence group, and the
  runner requires the group to hold a `files`, a `document` and a `block` vector with the same
  request and the same expected answer.
- **Idempotency of operations**: repeating a read vector yields the same answer; repeating a refused
  write is refused the same way, since it changed nothing. Repeating a write that succeeded has the
  outcome its vector states: refused (a duplicate key, a transition from the new state), the same
  state (an update that sets the values the row already holds), or another change where the request
  is not idempotent by nature and no rule refuses it (an insert whose key the engine assigns). The
  earlier wording — refused, or the same state for an upsert — missed that third case, which no rule
  of [constraints](50-constraints.md) refuses, as no SQL database refuses a second insert into an
  identity key.
- **Determinism**: reads follow storage order or the IR `order`; a vector whose request orders by
  `random` compares the row set and count, never the order.

## The vector shape

A vector is one JSON file under `tests/conformance/vectors/`, named after its `name`. A member the
format does not name is refused, so a misspelt expectation is a failure rather than a check that
never runs.

- `name`: the vector's name, its file name without `.json`.
- `storage`: the storage of the request's entity — `files`, `document list`, `document table`,
  `block list` or `block table` — which the runner checks against the catalog, so the coverage of
  each storage is stated and checked.
- `catalog`: the name of a catalog under `tests/conformance/catalogs/`, written as
  `dbspec/<catalog>.dbs` of the corpus before. Vectors share a catalog, so one dataset in one
  storage has one law, kept once.
- `corpus`: the documents of the corpus before, a corpus path to its text. A text is a JSON string of
  its exact bytes, or an array of its lines, each ending with a line feed; the array keeps a vector
  readable, the string states bytes the array cannot (no final line feed, a carriage return).
- `diagnostics`: optional, a corpus path to the validator rules the corpus before holds in that file
  ([canonical format](10-canonical-format.md#diagnostics)). The runner requires the corpus before
  to hold exactly these diagnostics; every other file holds none.
- `equivalent`: optional, the name of the vector's storage equivalence group.
- `request`: the IR request without `ir_version`, `manifest_hash` and `n_params`, which the runner
  fills from the IR version, the catalog's manifest hash and `params`.
- `params`: the parameter values of the request message.
- `now`: the request clock of the request message, in the datetime wire form.
- `expect`: the answer, one of two forms.
  - `error`: the orm code of a refused request, with `diagnostic`, optionally, the validator rule the
    refusal carries (the unavailable table, the rewrite refusal). The corpus after is the corpus
    before, byte for byte.
  - The members of a successful answer — `rows`, `count`, `value`, `affected`, `keys` — exactly
    those the answer holds. `rows` are projections: each expected row names the columns it
    compares, in the answer's order unless the request orders by `random`; the others compare
    whole. `corpus` names the files the request changed, a corpus path to its text after or `null`
    for a removed file; every file it does not name keeps its bytes, and no file appears that it
    does not name.
- `repeat`: for a write that succeeds, required: the outcome of the same request message on the
  corpus after, in the form of `expect`, its `corpus` naming the files the repeat changed. A read
  and a refused write carry none: the runner requires their repeat to answer as the first run.

The runner writes the catalog and the corpus before to a temporary root, loads the catalog, checks
the declared diagnostics, opens the corpus through the write path, runs the request in a session,
and compares the answer, the corpus after — by its bytes and by validating it — and the repeat. The
journal's own folder, `.flowmark/`, is not a corpus file; the runner requires it to hold no stage
after each run.
