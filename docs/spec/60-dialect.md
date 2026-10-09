<!-- doc-id: spec-dialect -->
# Capability table

[한국어](60-dialect.ko.md)

The markdown backend is a fourth **executor** selected by the DSN scheme `markdown://<path>`. The
shared planner validates the IR request exactly as for SQL backends; the executor consumes the
request and renders no SQL. This table is the integration contract: what each surface supports,
and what it rejects with `CAPABILITY_UNSUPPORTED` or `OPERATOR_NOT_ALLOWED`, in the orm
tradition. Support is **Go-only** until another client ports the executor.

| Surface | Ruling |
| --- | --- |
| `one`, `all`, `count`, `group_count`, `sum`, `avg` | Supported over the in-memory corpus; ordering is type-directed, `group_count` groups NULLs together |
| `paginate` | Supported; the session carries the corpus revision and fails `CORPUS_CHANGED` on mismatch, or degrades to keyset pagination |
| `insert` (single and multi-row) | Supported inside one transaction; an omitted primary key is an error (no `identity` on markdown corpora) |
| `update` | Supported and **strengthened**: every commit re-reads under the lock, so a concurrent human edit fails `CORPUS_CONFLICT` instead of being overwritten |
| `delete`, `restore` | Supported; `restore` keeps its SQL shape (soft-delete column cleared, unique keys still held while deleted) |
| `optimistic` | Supported — a plain update is already optimistic by the strengthening above, and the explicit form stays distinguishable (`OPTIMISTIC_LOCK` vs `NO_ROWS`) |
| `on_duplicate` | Supported with the conflict target **any unique key** (the MySQL rule); on a table with several unique keys this diverges from the PostgreSQL and SQLite target rule, and the divergence is documented here |
| `lock` (`update`, `share`, `update_nowait`, `share_nowait`) | `update` and `share` are accepted as documented no-ops (one per-operation lock already serializes); `*_nowait` is rejected `CAPABILITY_UNSUPPORTED` |
| relations (`limit_per_parent`, `if_parent`, `flatten`, `key_by`, composite keys, `no_cascade_delete`) | Supported — separate statements over the corpus with derived indexes |
| subqueries (`tuple_in`, `tuple_not_in`, scalar `Sub` aggregates) | Supported |
| `now`, `default now` | Supported; the client clock in UTC, exactly the orm clock rule |
| `soft_delete` setting | Supported; soft-deleted rows occupy keys and satisfy foreign keys |
| `audit` setting | Advisory — validate-time rejection only (no trigger-time prevention for human writes) |
| codec columns (AES, styled values, `blind_index`) | Supported; validation and codec operations require the key |
| savepoints | Nested staging inside one transaction |
| `orderByRandom` | Supported, seeded per corpus revision |
| `getQuery()`, statement events, `subscribe()` | `CAPABILITY_UNSUPPORTED` — there is no SQL text to report |
| `force_index` | `CAPABILITY_UNSUPPORTED` — indexes are derived, not named |
| `fulltext`, the `Lb` operator | `OPERATOR_NOT_ALLOWED` |
| `isolation`, `timeoutMs` connection options | `CAPABILITY_UNSUPPORTED` |
| dbplan chains (`plans.md` lifecycle) | Rejected in v1 — see [migration](80-migration.md); corpus creation and additive column additions only |

The conformance contract for this table is [conformance](90-conformance.md): corpus before,
request, expected corpus after — no SQL, no binds.
