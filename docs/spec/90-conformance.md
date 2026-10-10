<!-- doc-id: spec-conformance -->
# Conformance (scoped skeleton)

[한국어](90-conformance.ko.md)

This document states the conformance contract of the engine; the vector format details are fixed
with the rewritten vectors.

## The contract

- A vector is **a corpus before — its catalog and documents — plus a request, plus the expected
  corpus after, rows or error**. There is no SQL text and no bind list to compare; the SQL
  conformance harness does not apply, and this contract replaces it.
- The request is an IR request under the [capability table](60-dialect.md), or a face operation
  where the vector tests a face.
- Vectors run against the flowmark engine. orm's conformance of its `markdown` IR executors runs
  against a flowmark build pinned in orm, and is orm's own contract.
- Every vector states its name, and a run that cannot execute is a failure, in the orm conformance
  tradition.

## Required properties

- **Round trip**: every vector's corpus before is canonical (`emit(parse(s)) == s`) in every
  storage; the corpus after is canonical by construction.
- **Storage equivalence**: one dataset stored as `files`, as `document` and as `block` yields the
  same rows for the same read request.
- **Idempotency of operations**: repeating a read vector yields the same rows; repeating a write
  vector is refused (a duplicate key, a transition from the new state) or produces the same state
  where the operation is an upsert, and the vector states which.
- **Determinism**: reads follow storage order or the IR `order`; an `orderByRandom` vector compares
  the row set and count, never the order.

## The vector shape

A vector is one JSON file under `tests/conformance/vectors/`: `name`, `corpus` (a directory with
`dbspec/` and its documents), `request`, and `expect` — `rows` as key projections, `corpus` as the
expected files, or `error` as an orm code. The runner copies the corpus to a temporary root, opens
it, runs the request and compares the expectation; expected rows name their columns, so a row
comparison is a projection, not a whole-row equality.
