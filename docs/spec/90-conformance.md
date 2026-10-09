<!-- doc-id: spec-conformance -->
# Conformance (scoped skeleton)

[한국어](90-conformance.ko.md)

This document states the conformance contract; the vector format details are written with the
first vectors at T9.

## The contract

- A markdown vector is **corpus before + IR request + expected corpus after / expected rows**.
  There is no SQL text and no bind list to compare — the SQL conformance harness does not apply
  to this backend, and this contract replaces it.
- Vectors run against the Go executor. Cross-client comparison starts when another client ports
  the executor; until then the capability table marks markdown support Go-only.
- Every vector states its name, and a run that cannot execute is a failure, in the orm
  conformance tradition.

## Required properties

- **Round-trip**: every vector's corpus-before is canonical (`emit(parse(s)) == s`); corpus-after
  is canonical by construction.
- **Idempotency of operations**: repeating a read vector yields the same rows; repeating a
  write vector is rejected as a conflict (or produces the same state where the operation is an
  upsert), and the vector states which.
- **Determinism**: ordering follows the type-directed rules; `orderByRandom` vectors pin the
  revision seed.

## Open for T9

The vector JSON shape, the runner's report format, and the shared vector set with the orm
`tests/dbspec` style.
